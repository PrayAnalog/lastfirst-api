package httpapi

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
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

func TestWatchLinksLabelIncludesReverseTitle(t *testing.T) {
	ids := make([]string, watchChunk+3)
	for i := range ids {
		ids[i] = "v" + strconv.Itoa(i)
	}
	links := watchLinks("[Reversed] My list", ids)
	want := []string{"[Reversed] My list 1–50", "[Reversed] My list 51–53"}
	if len(links) != len(want) {
		t.Fatalf("len(watchLinks) = %d, want %d", len(links), len(want))
	}
	for i, l := range links {
		if l.Label != want[i] {
			t.Errorf("links[%d].Label = %q, want %q", i, l.Label, want[i])
		}
	}
}

func TestCaddyBodyLimitMatchesApp(t *testing.T) {
	caddyfile, err := os.ReadFile("../../deploy/caddy/Caddyfile")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("max_size %dKiB", maxRequestBody>>10)
	if !strings.Contains(string(caddyfile), want) {
		t.Errorf("Caddyfile does not contain %q", want)
	}
}
