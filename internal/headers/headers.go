package headers

import (
	"bytes"
	"errors"
	"strings"
)

type Headers map[string]string

var (
	ErrMalformedHeaderLine = errors.New("malformed header-line")
	ErrHeaderLineTooLong   = errors.New("header line too long")

	separator = []byte("\r\n")
)

// Per-line cap; enforce a total cap at a higher layer.
const maxHeaderLine = 8 * 1024 // 8 KiB

func NewHeaders() Headers { return Headers{} }

// Get should be case-insensitive.
func (h Headers) Get(name string) string {
	return h[strings.ToLower(name)]
}

func (h Headers) Delete(name string) {
	delete(h, strings.ToLower(name))
}

func (h Headers) Set(name, value string) {
	name = strings.ToLower(name)

	if old, ok := h[name]; ok {
		h[name] = old + "," + value
	} else {
		h[name] = value
	}
}

func (h Headers) Override(name, value string) {
	name = strings.ToLower(name)
	h[name] = value
}

func (h Headers) Parse(data []byte) (n int, done bool, err error) {
	off := 0
	for {
		idx := bytes.Index(data[off:], separator)
		if idx == -1 {
			// If current unterminated line exceeds cap, fail (prevents DoS).
			if len(data)-off > maxHeaderLine {
				return 0, false, ErrHeaderLineTooLong
			}
			return off, false, nil // need more bytes
		}
		if idx > maxHeaderLine {
			return 0, false, ErrHeaderLineTooLong
		}

		line := data[off : off+idx]
		off += idx + len(separator) // consume line + CRLF

		// Blank line => end of headers
		if len(line) == 0 {
			return off, true, nil
		}

		// Reject obsolete folding (starts with SP/HTAB)
		if line[0] == ' ' || line[0] == '\t' {
			return 0, false, ErrMalformedHeaderLine
		}

		// Find first colon (values may contain additional colons)
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 { // no colon or empty field-name
			return 0, false, ErrMalformedHeaderLine
		}

		nameRaw := line[:colon]

		// Field-name MUST NOT contain SP/HTAB anywhere.
		if bytes.ContainsAny(nameRaw, " \t") {
			return 0, false, ErrMalformedHeaderLine
		}

		// Validate token and normalize to lowercase (HTTP/2-friendly)
		if !isTokenTable(nameRaw) {
			return 0, false, ErrMalformedHeaderLine
		}
		name := strings.ToLower(string(nameRaw))

		// Trim optional whitespace around the value
		val := strings.Trim(string(line[colon+1:]), " \t")

		h.Set(name, val)
	}
}

var allowed [256]bool

func init() {
	for c := byte('0'); c <= '9'; c++ {
		allowed[c] = true
	}
	for c := byte('A'); c <= 'Z'; c++ {
		allowed[c] = true
	}
	for c := byte('a'); c <= 'z'; c++ {
		allowed[c] = true
	}
	for _, c := range []byte("!#$%&'*+-.^_`|~") {
		allowed[c] = true
	}
}

func isTokenTable(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c > 127 || !allowed[c] {
			return false
		}
	}
	return true
}
