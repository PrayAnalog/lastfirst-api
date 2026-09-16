package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The static directory has no index.html, so a request that falls through to
// the SPA handler gets 404 instead of the probe's 200.
func TestHealthz(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	New(nil, t.TempDir()).Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
