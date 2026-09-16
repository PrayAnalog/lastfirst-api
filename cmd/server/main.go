package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
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
//
// shutdownTimeout counts from SIGTERM and has to outlast the longest response
// still in flight then, which writeTimeout bounds. terminationGracePeriodSeconds
// in deploy/deployment.yaml counts from the start of preStop, so it covers the
// preStop delay plus shutdownTimeout.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = httpapi.RequestTimeout + 5*time.Second
	idleTimeout       = 90 * time.Second
	shutdownTimeout   = writeTimeout + 5*time.Second
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

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()

	select {
	case err := <-serveErr:
		log.Fatal(err)
	case <-sigCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	// Released only after draining, so a repeated SIGTERM during the drain is
	// absorbed instead of killing the handlers being drained.
	stop()

	// Shutdown made ListenAndServe return; its error is still unread when the
	// signal won the select, and anything but ErrServerClosed is a listener
	// failure that happened on its own.
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	if shutdownErr != nil {
		log.Print(shutdownErr)
	}
}
