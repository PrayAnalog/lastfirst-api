package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name string
		xff  []string
		want string
	}{
		{"proxy-written only", []string{"203.0.113.7"}, "203.0.113.7"},
		{"client prefix on same line", []string{"1.2.3.4, 203.0.113.7"}, "203.0.113.7"},
		{"proxy line after client line", []string{"1.2.3.4", "203.0.113.7"}, "203.0.113.7"},
		{"empty last entry", []string{"1.2.3.4, "}, "192.0.2.1"},
		{"no header", nil, "192.0.2.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/playlists", nil)
			for _, v := range c.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIP(r); got != c.want {
				t.Errorf("clientIP() = %q, want %q", got, c.want)
			}
		})
	}
}
