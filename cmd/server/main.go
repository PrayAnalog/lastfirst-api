package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"ytreverse/internal/config"
	"ytreverse/internal/httpapi"
	"ytreverse/internal/youtube"
)

// ReadHeaderTimeout and ReadTimeout count from the start of reading a request.
// WriteTimeout counts from the moment its headers have been read, which is also
// when the handler starts and httpapi.RequestTimeout begins, so it is written
// over RequestTimeout with room to write the response. IdleTimeout has to
// outlast the ingress's upstream keep-alive (60s by default), or nginx sends a
// request down a connection this server has just closed.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = httpapi.RequestTimeout + 5*time.Second
	idleTimeout       = 90 * time.Second
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	yt, err := youtube.New(ctx, cfg.YTAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	srv := httpapi.New(yt, cfg.StaticDir)
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	log.Fatal(server.ListenAndServe())
}
