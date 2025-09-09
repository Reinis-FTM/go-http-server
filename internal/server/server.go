package server

import (
	"errors"
	"fmt"
	"httpfromtcp/internal/headers"
	"httpfromtcp/internal/request"
	"httpfromtcp/internal/response"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

type Server struct {
	Port     int
	listener net.Listener
	closed   atomic.Bool
	handler  Handler
}

type HandlerError struct {
	StatusCode response.StatusCode
	Message    string
}

type Handler func(w *response.Writer, req *request.Request)

func Serve(port int, handler Handler) (*Server, error) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	s := &Server{
		Port:     port,
		listener: l,
		handler:  handler,
	}
	go s.listen()
	return s, nil
}

func (s *Server) Close() error {
	// Make Close idempotent.
	if s.closed.Swap(true) {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) listen() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() || errors.Is(err, net.ErrClosed) {
				return
			}
			// transient accept error; keep going
			continue
		}
		go s.handle(conn)
	}
}

// helper: format duration compactly
func fmtDur(d time.Duration) string {
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	start := time.Now()

	remoteHost, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

	req, err := request.RequestFromReader(conn)
	if err != nil {
		// Log the bad request with a 400 status
		log.Printf("%s\t%s\t%s\t%d\t%s\terr=%q",
			remoteHost, "-", "-", 400, fmtDur(time.Since(start)), err.Error(),
		)
		// Return a proper HTTP error so clients don’t see a reset.
		_, _ = io.WriteString(conn, "HTTP/1.1 400 Bad Request\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")

		return
	}

	method := req.RequestLine.Method
	target := req.RequestLine.RequestTarget

	// Build your response
	writer := response.NewWriter(conn)
	writer.Headers = headers.NewHeaders()

	s.handler(writer, req)

	// 1) status line
	if err := writer.WriteStatusLine(writer.Status); err != nil {
		log.Printf("%s\t%s\t%s\t%d\t%s\terr=%q",
			remoteHost, method, target, 500, fmtDur(time.Since(start)), err.Error(),
		)
		return
	}

	// 2) headers (with correct Content-Length)
	h := response.GetDefaultHeaders(len(writer.Body))
	if err := writer.WriteHeaders(h); err != nil {
		log.Printf("%s\t%s\t%s\t%d\t%s\terr=%q",
			remoteHost, method, target, 500, fmtDur(time.Since(start)), err.Error(),
		)
		return
	}

	// 3) body
	_, err = writer.WriteBody(writer.Body)
	if err != nil {
		log.Printf("%s\t%s\t%s\t%d\t%s\terr=%q",
			remoteHost, method, target, 500, fmtDur(time.Since(start)), err.Error(),
		)
		return
	}

	// Access log (success)
	log.Printf("%s\t%s\t%s\t%d\t%s",
		remoteHost, method, target, int(writer.Status), fmtDur(time.Since(start)),
	)
}
