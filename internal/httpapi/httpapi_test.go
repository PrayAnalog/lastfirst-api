package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ytreverse/internal/ytinput"
)

func TestCreatePlaylistRequestBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "body over the limit",
			body:       `{"input":"` + strings.Repeat("a", maxRequestBodyBytes) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantError:  "request body too large",
		},
		{
			name:       "valid object followed by data past the limit",
			body:       `{"input":"x"}` + strings.Repeat(" ", maxRequestBodyBytes),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantError:  "request body too large",
		},
		{
			name:       "second JSON value after the object",
			body:       `{"input":"x"}{"input":"y"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "garbage after the object",
			body:       `{"input":"x"} trailing`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "single object with trailing whitespace is read as the request",
			body:       "{\"input\":\"not a playlist!\"}\n",
			wantStatus: http.StatusBadRequest,
			wantError:  ytinput.ErrInvalid.Error(),
		},
	}
	s := New(nil, t.TempDir())
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/playlists", strings.NewReader(tt.body))
			r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", i+1)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			var got map[string]string
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatalf("decoding error response: %v", err)
			}
			if got["error"] != tt.wantError {
				t.Errorf("error = %q, want %q", got["error"], tt.wantError)
			}
		})
	}
}
