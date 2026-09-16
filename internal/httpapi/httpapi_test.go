package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name: "ingress-set X-Real-IP wins over a caller-supplied X-Forwarded-For",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.7, 198.51.100.20",
				"X-Real-IP":       "198.51.100.20",
			},
			want: "198.51.100.20",
		},
		{
			name:    "X-Forwarded-For alone is not trusted",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.7"},
			want:    "192.0.2.1",
		},
		{
			name: "connection address without proxy headers",
			want: "192.0.2.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/playlists", nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := clientIP(r); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
