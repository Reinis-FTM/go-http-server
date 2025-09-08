package headers

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestHeadersParsing(t *testing.T) {
	// Test: Valid single header
	headers := NewHeaders()
	data := []byte("host: localhost:42069\r\n\r\n")
	n, done, err := headers.Parse(data)
	require.NoError(t, err)
	require.NotNil(t, headers)
	assert.Equal(t, "localhost:42069", headers.Get("host"))
	assert.Equal(t, len(data), n)
	assert.True(t, done)

	// Test: Invalid spacing header
	headers = NewHeaders()
	data = []byte("       Host : localhost:42069       \r\n\r\n")
	n, done, err = headers.Parse(data)
	require.Error(t, err)
	assert.Equal(t, 0, n)
	assert.False(t, done)

	// Test: Valid headers with repeating headers
	headers = NewHeaders()
	data = []byte("host: localhost:42069\r\nX-Person: some1   \r\nX-Person: some2   \r\nX-Person: some3   \r\n\r\n")
	n, done, err = headers.Parse(data)
	require.NoError(t, err)
	require.NotNil(t, headers)
	assert.Equal(t, "localhost:42069", headers.Get("host"))
	assert.Equal(t, "some1,some2,some3", headers.Get("x-person"))
	assert.Equal(t, len(data), n)
	assert.True(t, done)

	// Valid, two lines + terminator
	data = []byte("Host: localhost:42069\r\nXforward: somethingdddd   \r\n\r\n")
	h := NewHeaders()
	n, done, err = h.Parse(data)
	require.NoError(t, err)
	require.True(t, done)
	assert.Equal(t, len(data), n)
	assert.Equal(t, "localhost:42069", h.Get("Host"))
	assert.Equal(t, "somethingdddd", h.Get("XForward"))

	// Space before colon => invalid
	_, _, err = NewHeaders().Parse([]byte("Host : localhost\r\n\r\n"))
	require.Error(t, err)

	// Long line without CRLF => ErrHeaderLineTooLong
	big := bytes.Repeat([]byte("A"), maxHeaderLine+1)
	_, _, err = NewHeaders().Parse(append(big, 'B'))
	require.ErrorIs(t, err, ErrHeaderLineTooLong)

	// Duplicate header => concatenated
	h = NewHeaders()
	n, done, err = h.Parse([]byte("Vary: accept\r\nVary: encoding\r\n\r\n"))
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, "accept,encoding", h.Get("Vary"))
}
