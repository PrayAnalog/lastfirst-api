package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ytreverse/internal/config"
	"ytreverse/internal/httpapi"
	"ytreverse/internal/youtube"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	responseHeadroom  = 5 * time.Second
	writeTimeout      = readTimeout + httpapi.RequestTimeout + responseHeadroom
	shutdownTimeout   = writeTimeout
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if cfg.YTAPIKey == "" {
		log.Fatal("YT_API_KEY is required")
	}

	yt, err := youtube.New(ctx, cfg.YTAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	api := httpapi.New(yt, cfg.StaticDir, cfg.TrustProxyHeaders)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s", listener.Addr())
	if err := serve(ctx, srv, listener); err != nil {
		log.Fatal(err)
	}
}

func serve(ctx context.Context, srv *http.Server, listener net.Listener) error {
	started := make(chan struct{})
	listener = &startupListener{Listener: listener, started: started}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(listener)
	}()

	// Serve registers the listener before its first Accept call. Waiting for
	// that call prevents an immediately cancelled context from racing ahead of
	// listener registration and leaving Serve blocked forever.
	select {
	case <-started:
	case err := <-serveErr:
		return normalizeServeError(err)
	}

	select {
	case err := <-serveErr:
		drainErr := drainServer(srv)
		return errors.Join(normalizeServeError(err), drainErr)
	case <-ctx.Done():
		log.Print("shutdown signal received; draining connections")
		drainErr := drainServer(srv)
		listenErr := normalizeServeError(<-serveErr)
		if listenErr != nil {
			return errors.Join(listenErr, drainErr)
		}
		if drainErr != nil {
			// A bounded drain that reaches its deadline is an expected rollout
			// outcome after active connections have been force-closed.
			log.Printf("graceful shutdown did not complete: %v", drainErr)
			if !errors.Is(drainErr, context.DeadlineExceeded) {
				return drainErr
			}
		}
		return nil
	}
}

type startupListener struct {
	net.Listener
	started chan struct{}
	once    sync.Once
}

func (l *startupListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.started) })
	return l.Listener.Accept()
}

func drainServer(srv *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		closeErr := srv.Close()
		return errors.Join(err, closeErr)
	}
	return nil
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve HTTP: %w", err)
}
