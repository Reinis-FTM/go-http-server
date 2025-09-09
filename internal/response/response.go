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

func (w *Writer) WriteHeaders(headers headers.Headers) error {
	if headers == nil {
		_, err := io.WriteString(w.writer, "\r\n")
		return err
	}

	for l := range w.Headers {
		headers.Override(l, w.Headers.Get(l))
	}

	// Collect & sort keys (case-insensitive)
	keys := make([]string, 0, len(headers)) // len=0, cap=len(hdrs)
	for k := range headers {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)

	// Emit "Key: Value\r\n" for each
	for _, k := range keys {
		display := textproto.CanonicalMIMEHeaderKey(k)
		if _, err := fmt.Fprintf(w.writer, "%s: %s\r\n", display, headers.Get(k)); err != nil {
			return err
		}
	}

	// Final CRLF to end the header block
	_, err := io.WriteString(w.writer, "\r\n")
	return err
}

func (w *Writer) WriteBody(p []byte) (int, error) {

	return w.writer.Write(p)
}

func (w *Writer) WriteChunkedBody(p []byte) (int, error) {

	return 0, nil
}

func (w *Writer) WriteChunkedBodyDone() (int, error) {

	return 0, nil
}
