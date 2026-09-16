package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestServeWaitsForInFlightRequestOnCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close listener: %v", err)
		}
	})

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseHandler) }) })
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
		w.WriteHeader(http.StatusNoContent)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- serve(ctx, srv, listener) }()

	requestDone := request(listener.Addr().String())
	waitFor(t, handlerStarted, "request handler did not start")
	cancel()
	select {
	case err := <-serverDone:
		t.Fatalf("serve returned before the handler completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releaseHandler) })
	if err := waitError(t, requestDone, "request did not complete"); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if err := waitError(t, serverDone, "server did not finish shutdown"); err != nil {
		t.Fatalf("serve returned an error: %v", err)
	}
}

func TestServeDrainsInFlightRequestAfterListenerFailure(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := base.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close listener: %v", err)
		}
	})
	listenerFailure := errors.New("listener failed")
	fail := make(chan struct{})
	var failOnce sync.Once
	t.Cleanup(func() { failOnce.Do(func() { close(fail) }) })
	failListener := &failAfterFirstAcceptListener{
		Listener: base,
		fail:     fail,
		err:      listenerFailure,
	}

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseHandler) }) })
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
		w.WriteHeader(http.StatusNoContent)
	})}
	serverDone := make(chan error, 1)
	go func() { serverDone <- serve(context.Background(), srv, failListener) }()

	requestDone := request(base.Addr().String())
	waitFor(t, handlerStarted, "request handler did not start")
	failOnce.Do(func() { close(failListener.fail) })
	select {
	case err := <-serverDone:
		t.Fatalf("serve returned before draining the handler: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releaseHandler) })
	if err := waitError(t, requestDone, "request did not complete"); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if err := waitError(t, serverDone, "server did not return after listener failure"); !errors.Is(err, listenerFailure) {
		t.Fatalf("serve error = %v, want listener failure", err)
	}
}

type failAfterFirstAcceptListener struct {
	net.Listener
	mu       sync.Mutex
	accepted bool
	fail     chan struct{}
	err      error
}

func (l *failAfterFirstAcceptListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if !l.accepted {
		l.accepted = true
		l.mu.Unlock()
		return l.Listener.Accept()
	}
	l.mu.Unlock()
	<-l.fail
	return nil, l.err
}

func request(address string) <-chan error {
	done := make(chan error, 1)
	go func() {
		client := &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{},
		}
		defer client.CloseIdleConnections()
		response, err := client.Get("http://" + address)
		if err != nil {
			done <- err
			return
		}
		_, copyErr := io.Copy(io.Discard, response.Body)
		done <- errors.Join(copyErr, response.Body.Close())
	}()
	return done
}

func waitFor(t *testing.T, done <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal(failure)
	}
}

func waitError(t *testing.T, done <-chan error, failure string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal(failure)
		return nil
	}
}
