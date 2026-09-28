package httpapi_test

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreatePlaylistResponsesAreNotStored(t *testing.T) {
	fake := &fakeYouTube{playlists: map[string]fakePlaylist{
		"PLone": {title: "One", channel: "Ch", itemCount: 1, items: []fakeItem{{videoID: "v0", publishedAt: "2024-01-02T03:04:05Z"}}},
	}}
	h := newIntegrationHandler(t, fake)

	for name, body := range map[string]string{
		"success": `{"input":"PLone"}`,
		"error":   `{"input":`,
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/playlists", strings.NewReader(body))
			r.Header.Set("X-Forwarded-For", "203.0.113.9")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if got := w.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("status %d: Cache-Control = %q, want %q", w.Code, got, "no-store")
			}
		})
	}
}
