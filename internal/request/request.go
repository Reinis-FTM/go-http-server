package request

import (
	"bytes"
	"errors"
	"fmt"
	"httpfromtcp/internal/headers"
	"io"
	"strconv"
	"strings"
)

// Request holds the parsed state of an HTTP request.
// Currently we parse only the start-line and then transition to ParsingHeaders.
type Request struct {
	RequestLine *RequestLine
	Headers     headers.Headers
	Body        []byte
	state       RequestState // 1 = initialized, 2 = parsing_headers, 3 = parsing_body, 4 = done, 5 = error
	parseErr    error
}

type RequestState int

const (
	// Keep numeric order aligned with your comment: 1 -> 2 -> 3 -> 4
	RequestInitialized    RequestState = iota + 1 // 1
	RequestParsingHeaders                         // 2 (start-line parsed; headers not parsed here yet)
	RequestParsingBody                            // 3 (start-line parsed; headers not parsed here yet)
	RequestDone                                   // 4
	RequestError                                  // 5
)

var RequestStateName = map[RequestState]string{
	RequestInitialized:    "initialized",
	RequestParsingHeaders: "parsing_headers",
	RequestParsingBody:    "parsing_body",
	RequestDone:           "done",
	RequestError:          "error",
}

// RequestLine represents the three components of an HTTP/1.1 request line:
//
//	<method> <request-target> <HTTP-version>
type RequestLine struct {
	HTTPVersion   string
	RequestTarget string
	Method        string
}

// Predefined errors for different validation failures.
var (
	ErrMalformedRequestLine   = errors.New("malformed request-line")
	ErrUnsupportedHTTPVersion = errors.New("unsupported http version")
	ErrUnsupportedHTTPMethod  = errors.New("unsupported http method")
	ErrMissingRequestTarget   = errors.New("missing request target")
	ErrMessageTooLarge        = errors.New("http message exceeds drain limit")
	ErrRequestBodyExceedsCL   = errors.New("http body exceeds content length")

	// Precompiled regexes for supported methods and version.
	// methodRE  = regexp.MustCompile(`^(GET|HEAD|POST|PUT|DELETE|CONNECT|OPTIONS|TRACE|PATCH)$`)
	// versionRE = regexp.MustCompile(`^HTTP/1\.1$`)

	// Allowed HTTP methods for validation (map lookup is faster than regex).
	allowedMethods = map[string]struct{}{
		"GET": {}, "HEAD": {}, "POST": {}, "PUT": {}, "DELETE": {},
		"CONNECT": {}, "OPTIONS": {}, "TRACE": {}, "PATCH": {},
	}

	// HTTP spec requires CRLF as line terminator.
	separator = []byte("\r\n")
)

// Maximum allowed size of the start-line, per RFC 9112 recommendations.
// Prevents DoS attacks from sending extremely long lines.
const maxStartLine = 8 * 1024         // 8 KiB cap
const maxBodyBytes = 10 * 1024 * 1024 // 10 MiB

// newRequest initializes a Request in state=Initialized (ready to parse).
func newRequest() *Request {
	return &Request{
		state:   RequestInitialized,
		Headers: headers.NewHeaders(), // <-- initialize to avoid panic
	}
}

// done reports whether we have completed the start-line phase.
// We stop the outer read loop once we reach ParsingHeaders (or Done).
func (r *Request) done() bool {
	return r.state == RequestDone
}

func (r *Request) error() bool {
	return r.state == RequestError
}

func (r *Request) setErr(err error) error {
	r.parseErr = err
	r.state = RequestError
	return err
}

// hasBody inspects headers and tells whether the request has a body,
// and if so, how many bytes are expected (via Content-Length).
// It currently does NOT support chunked TE; returns an error in that case.
//
// Returns:
//
//	has  = true iff there is a body with positive Content-Length
//	want = exact number of body bytes to read when has==true
//	err  = framing/size errors (e.g., bad CL, chunked TE, too large)
func (r *Request) hasBody() (has bool, want int, err error) {
	te := strings.ToLower(strings.TrimSpace(r.Headers.Get("transfer-encoding")))
	if te != "" {
		// You can broaden this if you later implement chunked.
		if strings.Contains(te, "chunked") {
			return false, 0, fmt.Errorf("transfer-encoding: chunked not supported")
		}
		// Any TE without chunked is unsupported in this simple parser
		return false, 0, fmt.Errorf("unsupported transfer-encoding: %q", te)
	}

	clStr := strings.TrimSpace(r.Headers.Get("content-length"))
	if clStr == "" {
		// No TE, no CL => no body for requests (HTTP/1.1)
		return false, 0, nil
	}

	cl, perr := strconv.ParseInt(clStr, 10, 64)
	if perr != nil || cl < 0 {
		return false, 0, fmt.Errorf("bad Content-Length: %q", clStr)
	}

	if cl == 0 {
		return false, 0, nil
	}

	if cl > int64(maxBodyBytes) {
		return false, 0, ErrMessageTooLarge
	}
	return true, int(cl), nil
}

// parse consumes data and attempts to parse the request line.
// Returns bytes consumed and any error.
// Contract:
//   - If not enough data, returns (0, nil).
//   - On success, sets state to ParsingHeaders and returns bytes consumed.
func (r *Request) parse(data []byte) (int, error) {
	read := 0

outer:
	for {
		currentData := data[read:]
		switch r.state {
		case RequestError:
			break outer

		case RequestInitialized:
			rl, n, err := ParseRequestLine(currentData)
			if err != nil {
				return 0, r.setErr(err)
			}

			if n == 0 {
				break outer // need more bytes for start-line
			}

			r.RequestLine = rl
			read += n
			r.state = RequestParsingHeaders // transition

		case RequestParsingHeaders:
			n, endOfHeaders, err := r.Headers.Parse(currentData)
			if err != nil {
				return 0, r.setErr(err)
			}

			if n == 0 && !endOfHeaders {
				break outer // need more bytes for headers
			}

			read += n

			if endOfHeaders {
				has, _, err := r.hasBody()
				if err != nil {
					return 0, r.setErr(err)
				}

				if !has {
					r.state = RequestDone
					break outer
				}

				// There is a positive-length body; start consuming now.
				r.state = RequestParsingBody
				// Optionally stash want somewhere if you don't want to call hasBody() again.
				continue
			}

		case RequestParsingBody:
			has, want, err := r.hasBody()
			if err != nil {
				return 0, r.setErr(err)
			}

			if !has {
				// Defensive: headers said no body (or CL=0)
				r.state = RequestDone
				break outer
			}

			have := len(r.Body)
			if have > want {
				return 0, r.setErr(ErrRequestBodyExceedsCL)
			}

			if have == want {
				r.state = RequestDone
				break outer
			}

			// Consume up to remaining bytes from currentData
			remaining := want - have
			toRead := min(remaining, len(currentData))
			if toRead > 0 {
				r.Body = append(r.Body, currentData[:toRead]...)
				read += toRead
			}

			if len(r.Body) == want {
				r.state = RequestDone
			}
			break outer

		case RequestDone:
			break outer

		default:
			return 0, r.setErr(fmt.Errorf("unknown state: %d", r.state))
		}
	}

	return read, nil
}

// RequestFromReader reads from r until the start-line is parsed,
// or an error occurs. It enforces maxStartLine size.
// Any extra bytes read (e.g., beginning of headers) remain in the
// caller's buffer in this implementation; we stop at ParsingHeaders.
func RequestFromReader(r io.Reader) (*Request, error) {
	req := newRequest()

	// buf accumulates bytes we haven't yet parsed.
	buf := make([]byte, 0, 256)
	// tmp is a scratch buffer for each read from r.
	tmp := make([]byte, 1024)

	for !req.done() {
		n, err := r.Read(tmp)

		if n > 0 {
			// Append new data into our buffer
			buf = append(buf, tmp[:n]...)

			// Enforce start-line cap ONLY before the start-line is parsed.
			if req.state == RequestInitialized && len(buf) > maxStartLine {
				return nil, ErrMalformedRequestLine
			}

			// Try to parse what we have so far
			readN, perr := req.parse(buf)
			if perr != nil {
				return nil, perr
			}

			if readN > 0 {
				// Shift leftover (unparsed) data down to front of buffer
				copy(buf, buf[readN:])
				buf = buf[:len(buf)-readN]
			}
		}

		if err != nil {
			if err == io.EOF {
				// give parser a last chance if you want; then:
				if req.done() {
					break
				}

				// if we errored earlier, surface that; else short body
				if req.error() {
					return nil, req.parseErr
				}

				return nil, io.ErrUnexpectedEOF
			}

			return nil, err
		}
	}

	if req.error() {
		return nil, req.parseErr
	}

	return req, nil
}

// ParseRequestLine attempts to parse a single HTTP request line from s.
// Returns (*RequestLine, bytesConsumedIncludingCRLF, error).
// If no CRLF yet, returns (nil, 0, nil).
func ParseRequestLine(s []byte) (*RequestLine, int, error) {
	// Find CRLF terminator
	idx := bytes.Index(s, separator)
	if idx == -1 {
		// Not enough data yet
		return nil, 0, nil
	}

	startLine := s[:idx]

	// Split on spaces/tabs into tokens
	tokens := bytes.Fields(startLine)
	if len(tokens) != 3 {
		return nil, 0, ErrMalformedRequestLine
	}

	m, target, ver := tokens[0], tokens[1], tokens[2]

	// Validate method
	if _, ok := allowedMethods[string(m)]; !ok {
		return nil, 0, ErrUnsupportedHTTPMethod
	}
	// Validate version (only HTTP/1.1 supported here)
	if !bytes.Equal(ver, []byte("HTTP/1.1")) {
		return nil, 0, ErrUnsupportedHTTPVersion
	}

	// Number of bytes consumed includes CRLF
	parsedBytes := idx + len(separator)

	// Build RequestLine struct
	return &RequestLine{
		Method:        string(m),
		RequestTarget: string(target),
		HTTPVersion:   string(bytes.Split(ver, []byte("/"))[1]), // "1.1"
	}, parsedBytes, nil
}
