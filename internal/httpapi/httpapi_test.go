package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	want := fmt.Sprintf("max_size %d", maxRequestBody)
	for _, line := range strings.Split(string(caddyfile), "\n") {
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Errorf("Caddyfile has no line %q", want)
}

func TestSPAUnrootedPathStaysInStaticDir(t *testing.T) {
	root := t.TempDir()
	staticDir := filepath.Join(root, "static")
	if err := os.Mkdir(staticDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{staticDir: staticDir}
	get := func(path string) (int, string) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.URL.Path = path
		w := httptest.NewRecorder()
		s.spa().ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}

	existCode, existBody := get("../outside.txt")
	missCode, missBody := get("../missing.txt")
	if existCode != missCode || existBody != missBody {
		t.Fatalf("response reveals a file outside staticDir: existing %d %q, missing %d %q", existCode, existBody, missCode, missBody)
	}
}
