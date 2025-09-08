package main

import (
	"fmt"
	"httpfromtcp/internal/request"
	"io"
	"net"
	"net/textproto"
	"os"
	"sort"
	"time"
)

const PORT = ":42069"

func main() {
	tcp, err := net.Listen("tcp", PORT)
	if err != nil {
		fmt.Println("ERROR: failed to open.\n", err.Error())
		os.Exit(1)
	}
	defer tcp.Close()

	fmt.Println("Listening for TCP traffic on", PORT)
	for {
		conn, err := tcp.Accept()
		if err != nil {
			fmt.Println("ERROR: failed to accept.\n", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second)) // optional safety

	req, err := request.RequestFromReader(conn)
	if err != nil {
		fmt.Println("ERROR: failed to parse request:", err)
		return
	}

	fmt.Printf("Request line:\n- Method: %s\n- Target: %s\n- Version: %s\n",
		req.RequestLine.Method, req.RequestLine.RequestTarget, req.RequestLine.HTTPVersion)

	// Print headers
	fmt.Println("Headers:")
	if len(req.Headers) == 0 {
		fmt.Println("- (none)")
	} else {
		keys := make([]string, 0, len(req.Headers))
		for k := range req.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := req.Headers.Get(k)
			// Canonicalize for display (e.g., "content-type" -> "Content-Type")
			fmt.Printf("- %s: %s\n", textproto.CanonicalMIMEHeaderKey(k), v)
		}
	}

	fmt.Println("Body:")
	if req.Body == nil {
		fmt.Println("- (none)")
	} else {
		fmt.Println(string(req.Body))
	}

	// Minimal HTTP/1.1 response; tell client we're closing the connection.
	resp := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 2\r\n" +
		"Connection: close\r\n" +
		"\r\n" +
		"OK"
	_, _ = io.WriteString(conn, resp)
}
