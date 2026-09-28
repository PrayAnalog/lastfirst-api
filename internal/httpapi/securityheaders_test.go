package httpapi

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ytreverse/internal/youtube"
)

func TestHandlerSetsSecurityHeaders(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	yt, err := youtube.New(context.Background(), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	h := New(yt, dir).Handler()

	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'; img-src 'self' https://i.ytimg.com; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/guide/watch-youtube-playlist-in-order"},
		{"POST", "/api/playlists"},
	} {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
			for k, v := range want {
				if got := w.Header().Get(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
		})
	}
}
