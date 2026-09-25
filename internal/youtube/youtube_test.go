package youtube

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIKeyStaysOutOfTransportErrors(t *testing.T) {
	const key = "test-secret-api-key"
	got := make(chan *http.Request, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Clone(context.Background())
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Close()
	}))
	defer srv.Close()

	orig := http.DefaultTransport
	http.DefaultTransport = &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	t.Cleanup(func() { http.DefaultTransport = orig })

	c, err := New(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.FetchPlaylistMeta(context.Background(), "PL123")
	if err == nil {
		t.Fatal("FetchPlaylistMeta succeeded, want transport error")
	}
	if strings.Contains(err.Error(), key) {
		t.Errorf("error exposes API key: %v", err)
	}
	r := <-got
	if q := r.URL.Query().Get("key"); q != "" {
		t.Errorf("key query parameter = %q, want none", q)
	}
	if h := r.Header.Get("X-Goog-Api-Key"); h != key {
		t.Errorf("X-Goog-Api-Key = %q, want %q", h, key)
	}
}
