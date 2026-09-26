package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ytreverse/internal/youtube"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	yt, err := youtube.New(context.Background(), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	return New(yt, t.TempDir()).Handler()
}

func requestRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var got []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		if rec["msg"] == "request" {
			got = append(got, rec)
		}
	}
	return got
}

func TestHandlerLogsEachRequest(t *testing.T) {
	buf := captureLogs(t)
	h := newTestHandler(t)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	parsed := httptest.NewRequestWithContext(canceled, "POST", "/api/playlists", strings.NewReader(`{"input":"https://www.youtube.com/playlist?list=PLabc123"}`))
	parsed.Header.Set("X-Forwarded-For", "203.0.113.7")
	h.ServeHTTP(httptest.NewRecorder(), parsed)

	invalid := httptest.NewRequest("POST", "/api/playlists", strings.NewReader(`{"input":"not a playlist"}`))
	invalid.Header.Set("X-Forwarded-For", "203.0.113.8")
	h.ServeHTTP(httptest.NewRecorder(), invalid)

	got := requestRecords(t, buf)
	if len(got) != 2 {
		t.Fatalf("got %d request log records, want 2; output:\n%s", len(got), buf.String())
	}

	want := []map[string]any{
		{"method": "POST", "path": "/api/playlists", "status": float64(http.StatusInternalServerError), "ip": "203.0.113.7", "playlist_id": "PLabc123"},
		{"method": "POST", "path": "/api/playlists", "status": float64(http.StatusBadRequest), "ip": "203.0.113.8"},
	}
	for i, w := range want {
		for k, v := range w {
			if got[i][k] != v {
				t.Errorf("record %d: %s = %v, want %v", i, k, got[i][k], v)
			}
		}
		if _, ok := got[i]["duration_ms"]; !ok {
			t.Errorf("record %d: missing duration_ms", i)
		}
	}
	if _, ok := got[1]["playlist_id"]; ok {
		t.Errorf("record 1: playlist_id = %v, want absent for unparsed input", got[1]["playlist_id"])
	}
}

func TestHandlerLogTruncatesLongPath(t *testing.T) {
	buf := captureLogs(t)
	h := newTestHandler(t)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/"+strings.Repeat("a", 5000), nil))

	got := requestRecords(t, buf)
	if len(got) != 1 {
		t.Fatalf("got %d request log records, want 1", len(got))
	}
	if want := "/" + strings.Repeat("a", 255); got[0]["path"] != want {
		t.Errorf("path has %d bytes, want the first 256", len(got[0]["path"].(string)))
	}
}
