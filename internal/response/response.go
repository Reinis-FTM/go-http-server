package response

import (
	"fmt"
	"httpfromtcp/internal/headers"
	"io"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
)

type StatusCode int

const (
	OK                    StatusCode = 200
	BAD_REQUEST           StatusCode = 400
	INTERNAL_SERVER_ERROR StatusCode = 500
)

var StatusCodeName = map[StatusCode]string{
	OK:                    "OK",
	BAD_REQUEST:           "Bad Request",
	INTERNAL_SERVER_ERROR: "Internal Server Error",
}

const httpVersion = "HTTP/1.1"

// GetDefaultHeaders returns a fresh headers map containing sensible defaults.
// Keys are stored lowercase to match your headers.Headers behavior.
func GetDefaultHeaders(contentLen int) headers.Headers {
	h := headers.NewHeaders()
	h.Set("content-length", strconv.Itoa(contentLen))
	h.Set("connection", "close")
	h.Set("content-type", "text/plain")
	return h
}

type Writer struct {
	writer       io.Writer
	WriterStatus WriterStatus
	Status       StatusCode
	Headers      headers.Headers
	Body         []byte
}

type WriterStatus int

const (
	WritingStatusLine WriterStatus = iota + 1
	WritingHeaders
	WritingBody
)

var WriterStatusName = map[WriterStatus]string{
	WritingStatusLine: "WRITING_STATUS_LINE",
	WritingHeaders:    "WRITING_HEADERS",
	WritingBody:       "WRITING_BODY",
}

func NewWriter(conn io.Writer) *Writer {
	return &Writer{writer: conn}
}

func (w *Writer) SetBody(body []byte) {
	w.Body = body
}

func (w *Writer) WriteStatusLine(statusCode StatusCode) error {
	reason, ok := StatusCodeName[statusCode]
	if !ok {
		reason = "Unknown"
	}
	_, err := fmt.Fprintf(w.writer, "%s %d %s\r\n", httpVersion, int(statusCode), reason)
	return err
}

func (w *Writer) WriteHeaders(h headers.Headers) error {
	if h == nil {
		_, err := io.WriteString(w.writer, "\r\n")
		return err
	}

	// Overlay writer-level defaults/overrides if you have them
	if w.Headers != nil {
		for k := range w.Headers {
			h.Override(k, w.Headers.Get(k))
		}
	}

	// If Transfer-Encoding contains "chunked", do not send Content-Length
	te := strings.ToLower(h.Get("transfer-encoding"))
	if tokenListContains(te, "chunked") {
		h.Delete("content-length")
	}

	// Collect keys (your Headers store uses lowercase keys already)
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		display := textproto.CanonicalMIMEHeaderKey(k)
		if _, err := fmt.Fprintf(w.writer, "%s: %s\r\n", display, h.Get(k)); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w.writer, "\r\n") // end of header block
	return err
}

func tokenListContains(list, token string) bool {
	for t := range strings.SplitSeq(list, ",") {
		if strings.TrimSpace(t) == token {
			return true
		}
	}
	return false
}

func (w *Writer) WriteBody(p []byte) (int, error) {

	return w.writer.Write(p)
}

func (w *Writer) WriteChunkedBody(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		// take up to 1024 bytes
		chunkSize := min(len(p), 1024)
		chunk := p[:chunkSize]
		p = p[chunkSize:]

		// write chunk size in hex followed by \r\n
		if _, err := fmt.Fprintf(w.writer, "%x\r\n", len(chunk)); err != nil {
			return total, err
		}

		// write chunk data
		n, err := w.writer.Write(chunk)
		total += n
		if err != nil {
			return total, err
		}

		// write \r\n after chunk
		if _, err := w.writer.Write([]byte("\r\n")); err != nil {
			return total, err
		}
	}
	return total, nil
}

// To finish the body, you need to send the terminating "0\r\n\r\n".
func (w *Writer) Close() error {
	_, err := w.writer.Write([]byte("0\r\n\r\n"))
	return err
}

func (w *Writer) WriteChunkedBodyDone() (int, error) {
	n, err := w.writer.Write([]byte("0\r\n\r\n"))
	return n, err
}
