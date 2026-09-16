package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The YouTube client is nil, so reaching the metadata call panics: an
// exhausted budget has to turn the request away before any billable call.
func TestCreatePlaylistReservesBeforeTheMetadataCall(t *testing.T) {
	s := New(nil, t.TempDir())
	if !s.budget.Reserve(dailyQuotaBudget) {
		t.Fatal("could not exhaust the budget")
	}

	r := httptest.NewRequest(http.MethodPost, "/api/playlists", strings.NewReader(`{"input":"PLtest"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}
	if got["error"] != quotaExhaustedMessage {
		t.Errorf("error = %q, want %q", got["error"], quotaExhaustedMessage)
	}
}
