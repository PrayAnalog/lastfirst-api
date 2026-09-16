package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The YouTube client is nil throughout: every case below has to be settled
// before the handler would reach it, and a nil dereference says it was not.
func post(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/api/playlists", strings.NewReader(body))
	w := httptest.NewRecorder()
	New(nil, t.TempDir()).Handler().ServeHTTP(w, r)
	return w
}

func errorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var res struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("response body %q is not the documented error shape: %v", w.Body.String(), err)
	}
	return res.Error
}

// Decode returns at the end of the first JSON value, so without a second
// Decode the rest of the body is never read and never measured.
func TestCreatePlaylistRejectsTrailingDataAfterTheJSONValue(t *testing.T) {
	w := post(t, `{"input":"PLtest"}`+strings.Repeat("x", maxRequestBodyBytes*4))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if msg := errorMessage(t, w); msg != "invalid request body" {
		t.Errorf("error %q, want the body to be rejected as malformed", msg)
	}
}

func TestCreatePlaylistRejectsABodyOverTheLimit(t *testing.T) {
	w := post(t, `{"input":"`+strings.Repeat("x", maxRequestBodyBytes)+`"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// The trailing-data check must not reject an ordinary body: this one gets
// past it and is turned away by input parsing instead.
func TestCreatePlaylistAcceptsOneJSONValue(t *testing.T) {
	w := post(t, `{"input":"not a playlist"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if msg := errorMessage(t, w); msg == "invalid request body" {
		t.Error("a well-formed body was rejected as malformed JSON")
	}
}

// The readiness and liveness probes in deploy/deployment.yaml call this, so
// the SPA fallback must not take the route.
func TestHealthz(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	New(nil, t.TempDir()).Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "" {
		t.Errorf("body %q, want an empty 200", body)
	}
}

func TestClientIPPrefersXRealIPOverXForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/playlists", nil)
	r.RemoteAddr = "10.0.0.1:4321"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	r.Header.Set("X-Real-IP", "198.51.100.7")

	if got := clientIP(r); got != "198.51.100.7" {
		t.Errorf("clientIP = %q, want the ingress-set X-Real-IP", got)
	}
}

func TestClientIPFallsBackToTheConnectionAddress(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/playlists", nil)
	r.RemoteAddr = "10.0.0.1:4321"

	if got := clientIP(r); got != "10.0.0.1" {
		t.Errorf("clientIP = %q, want the connection address", got)
	}
}
