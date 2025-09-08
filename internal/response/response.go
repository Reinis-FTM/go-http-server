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

// WriteStatusLine writes: "HTTP/1.1 200 OK\r\n"
func WriteStatusLine(w io.Writer, statusCode StatusCode) error {
	reason, ok := StatusCodeName[statusCode]
	if !ok {
		reason = "Unknown"
	}
	_, err := fmt.Fprintf(w, "%s %d %s\r\n", httpVersion, int(statusCode), reason)
	return err
}

// GetDefaultHeaders returns a fresh headers map containing sensible defaults.
// Keys are stored lowercase to match your headers.Headers behavior.
func GetDefaultHeaders(contentLen int) headers.Headers {
	h := headers.NewHeaders()
	h.Set("content-length", strconv.Itoa(contentLen))
	h.Set("connection", "close")
	h.Set("content-type", "text/plain")
	return h
}

// WriteHeaders writes the provided headers as "Key: Value\r\n" lines,
// sorted by key for deterministic output, and then writes the final
// blank line that terminates the header section.
//
// NOTE: This function DOES NOT compute Content-Length for you.
// Pass in headers from GetDefaultHeaders(len(body)) and then override/add.
func WriteHeaders(w io.Writer, hdrs headers.Headers) error {
	if hdrs == nil {
		_, err := io.WriteString(w, "\r\n")
		return err
	}

	// Collect & sort keys (case-insensitive)
	keys := make([]string, 0, len(hdrs)) // len=0, cap=len(hdrs)
	for k := range hdrs {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)

	// Emit "Key: Value\r\n" for each
	for _, k := range keys {
		display := textproto.CanonicalMIMEHeaderKey(k)
		if _, err := fmt.Fprintf(w, "%s: %s\r\n", display, hdrs.Get(k)); err != nil {
			return err
		}
	}

	// Final CRLF to end the header block
	_, err := io.WriteString(w, "\r\n")
	return err
}
