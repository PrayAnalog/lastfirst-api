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

// Connection bounds, each one written over the bound it has to outlast,
// because they do not start counting from the same event. ReadHeaderTimeout
// and ReadTimeout run from the moment the connection is accepted. WriteTimeout
// runs from the moment the headers have been read, while httpapi.RequestTimeout
// only starts once the handler runs, which is after the body is decoded — so
// writeTimeout carries that body-read window plus room to flush the response.
// shutdownTimeout in turn has to outlast the longest connection the server
// will still be holding when SIGTERM arrives, and
// terminationGracePeriodSeconds in deploy/deployment.yaml has to outlast
// shutdownTimeout plus the preStop delay that runs before it.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	idleTimeout       = 90 * time.Second
	writeTimeout      = httpapi.RequestTimeout + readTimeout + 5*time.Second
	shutdownTimeout   = writeTimeout + 10*time.Second
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
		stop()
		log.Fatal(err)
	case <-sigCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)

	// Only once draining is over: until then a second SIGTERM has to keep
	// being held by the signal handler rather than killing the very handlers
	// being drained.
	stop()

	// ListenAndServe has returned by now — Shutdown closed the listener —
	// and the signal branch above left its error unread. Anything other than
	// ErrServerClosed means the listener had already failed on its own.
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	if shutdownErr != nil {
		// Draining outran its budget. The listener is shut either way, and
		// exiting non-zero would report an ordinary rollout as a crashed
		// container.
		log.Print(shutdownErr)
	}
}
