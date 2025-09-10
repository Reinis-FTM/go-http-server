package main

import (
	"httpfromtcp/internal/request"
	"httpfromtcp/internal/response"
	"httpfromtcp/internal/server"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const PORT = 42069

var client = &http.Client{
	Transport: &http.Transport{
		ForceAttemptHTTP2: false, // disable HTTP/2
	},
}

func main() {
	server, err := server.Serve(PORT, func(w *response.Writer, req *request.Request) {
		w.Headers.Set("content-type", "text/html")

		if req.RequestLine.RequestTarget == "/yourproblem" {
			w.Status = response.BAD_REQUEST

			body := `<html>
  <head>
    <title>400 Bad Request</title>
  </head>
  <body>
    <h1>Bad Request</h1>
    <p>Your request honestly kinda sucked.</p>
  </body>
</html>`

			w.SetBody([]byte(body))
			return
		}

		if req.RequestLine.RequestTarget == "/myproblem" {
			w.Status = response.INTERNAL_SERVER_ERROR

			body := `<html>
  <head>
    <title>500 Internal Server Error</title>
  </head>
  <body>
    <h1>Internal Server Error</h1>
    <p>Okay, you know what? This one is on me.</p>
  </body>
</html>`

			w.SetBody([]byte(body))
			return
		}

		if strings.HasPrefix(req.RequestLine.RequestTarget, "/httpbin/stream") {
			w.Status = response.OK
			w.Headers.Set("Transfer-Encoding", "chunked")
			w.Headers.Override("content-type", "text/plain")

			target := req.RequestLine.RequestTarget
			req, err := http.NewRequest("GET", "https://httpbin.org/"+target[len("/httpbin/"):], nil)
			if err != nil {
				panic(err)
			}

			// Just to be explicit:
			req.Proto = "HTTP/1.1"
			req.ProtoMajor = 1
			req.ProtoMinor = 1

			resp, err := client.Do(req)
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			w.Body = body
			return
		}

		if req.RequestLine.RequestTarget == "/video" {
			w.Status = response.OK
			w.Headers.Set("Transfer-Encoding", "chunked")
			w.Headers.Override("content-type", "video/mp4")

			f, err := os.Open("/assets/vim.mp4")
			if err != nil { /* 404/500 */
			}
			defer f.Close()

			buf := make([]byte, 32*1024)
			for {
				n, rerr := f.Read(buf)
				if n > 0 {
					w.Body = append(w.Body, buf[:n]...)
				}
				if rerr == io.EOF {
					break
				}
				if rerr != nil {
					/* handle read error */
					break
				}
			}

			return
		}

		w.Status = response.OK

		body := `<html>
  <head>
    <title>200 OK</title>
  </head>
  <body>
    <h1>Success!</h1>
    <p>Your request was an absolute banger.</p>
  </body>
</html>`

		w.SetBody([]byte(body))
	})

	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}

	defer server.Close()
	log.Println("Server started on port:", PORT)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Server gracefully stopped")
}
