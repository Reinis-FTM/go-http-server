package main

import (
	"httpfromtcp/internal/headers"
	"httpfromtcp/internal/request"
	"httpfromtcp/internal/response"
	"httpfromtcp/internal/server"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const PORT = 42069

func main() {
	server, err := server.Serve(PORT, func(w *response.Writer, req *request.Request) {
		w.Headers = headers.NewHeaders()
		w.Headers.Set("content-type", "text/html")

		if req.RequestLine.RequestTarget == "/yourproblem" {
			w.Status = response.BAD_REQUEST

			body := `
<html>
  <head>
    <title>400 Bad Request</title>
  </head>
  <body>
    <h1>Bad Request</h1>
    <p>Your request honestly kinda sucked.</p>
  </body>
</html>
			`

			w.SetBody([]byte(body))
			return
		}

		if req.RequestLine.RequestTarget == "/myproblem" {
			w.Status = response.INTERNAL_SERVER_ERROR

			body := `
<html>
  <head>
    <title>500 Internal Server Error</title>
  </head>
  <body>
    <h1>Internal Server Error</h1>
    <p>Okay, you know what? This one is on me.</p>
  </body>
</html>
			`

			w.SetBody([]byte(body))
			return
		}

		w.Status = response.OK

		body := `
<html>
  <head>
    <title>200 OK</title>
  </head>
  <body>
    <h1>Success!</h1>
    <p>Your request was an absolute banger.</p>
  </body>
</html>			`

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
