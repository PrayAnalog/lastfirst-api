package httpapi_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	_ "time/tzdata"

	"ytreverse/internal/httpapi"
	"ytreverse/internal/youtube"
)

type fakePlaylist struct {
	title     string
	channel   string
	itemCount int
	items     []fakeItem
}

type fakeItem struct {
	videoID     string
	publishedAt string
}

type fakeYouTube struct {
	playlists  map[string]fakePlaylist
	metaStatus int
	itemStatus int
	itemCalls  atomic.Int64
}

func (f *fakeYouTube) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch r.URL.Path {
	case "/youtube/v3/playlists":
		if f.metaStatus != 0 {
			writeUpstreamError(w, f.metaStatus)
			return
		}
		items := []any{}
		if p, ok := f.playlists[q.Get("id")]; ok {
			items = append(items, map[string]any{
				"id":             q.Get("id"),
				"snippet":        map[string]any{"title": p.title, "channelTitle": p.channel},
				"contentDetails": map[string]any{"itemCount": p.itemCount},
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	case "/youtube/v3/playlistItems":
		f.itemCalls.Add(1)
		if f.itemStatus != 0 {
			writeUpstreamError(w, f.itemStatus)
			return
		}
		p := f.playlists[q.Get("playlistId")]
		start := 0
		if tok := q.Get("pageToken"); tok != "" {
			fmt.Sscanf(tok, "p%d", &start)
		}
		end := min(start+youtube.PageSize, len(p.items))
		page := []any{}
		for _, it := range p.items[start:end] {
			page = append(page, map[string]any{
				"snippet":        map[string]any{"title": "title " + it.videoID},
				"contentDetails": map[string]any{"videoId": it.videoID, "videoPublishedAt": it.publishedAt},
			})
		}
		res := map[string]any{"items": page}
		if end < len(p.items) {
			res["nextPageToken"] = fmt.Sprintf("p%d", end)
		}
		json.NewEncoder(w).Encode(res)
	default:
		http.NotFound(w, r)
	}
}

func writeUpstreamError(w http.ResponseWriter, status int) {
	reason := "backendError"
	if status == http.StatusForbidden {
		reason = "quotaExceeded"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code":    status,
		"message": "upstream secret detail",
		"errors":  []any{map[string]any{"reason": reason, "message": "upstream secret detail"}},
	}})
}

func newIntegrationHandler(t *testing.T, fake *fakeYouTube) http.Handler {
	t.Helper()
	srv := httptest.NewTLSServer(fake)
	t.Cleanup(srv.Close)

	orig := http.DefaultTransport
	http.DefaultTransport = &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	t.Cleanup(func() { http.DefaultTransport = orig })

	yt, err := youtube.New(context.Background(), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.New(yt, t.TempDir()).Handler()
}

func postPlaylist(t *testing.T, h http.Handler, ip, body string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/playlists", strings.NewReader(body))
	r.Header.Set("X-Forwarded-For", ip)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("response %q is not JSON: %v", w.Body.String(), err)
	}
	return w.Code, got
}

func TestCreatePlaylistReturnsReversedWatchLinks(t *testing.T) {
	items := make([]fakeItem, 53)
	for i := range items {
		items[i] = fakeItem{videoID: fmt.Sprintf("v%d", i), publishedAt: "2024-01-02T03:04:05Z"}
	}
	items[1].publishedAt = ""
	fake := &fakeYouTube{playlists: map[string]fakePlaylist{
		"PLbig": {title: "My list", channel: "My channel", itemCount: len(items), items: items},
	}}
	h := newIntegrationHandler(t, fake)

	code, got := postPlaylist(t, h, "203.0.113.7", `{"input":"https://www.youtube.com/playlist?list=PLbig"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %v", code, http.StatusOK, got)
	}

	var first []string
	for i := 52; i >= 3; i-- {
		first = append(first, fmt.Sprintf("v%d", i))
	}
	want := map[string]any{
		"sourceId":      "PLbig",
		"sourceTitle":   "My list",
		"sourceChannel": "My channel",
		"reverseTitle":  "[Reversed] My list",
		"totalCount":    float64(53),
		"includedCount": float64(52),
		"excludedCount": float64(1),
		"watchLinks": []any{
			map[string]any{
				"label":        "[Reversed] My list 1–50",
				"url":          "https://www.youtube.com/watch_videos?video_ids=" + strings.Join(first, ","),
				"thumbnailUrl": "https://i.ytimg.com/vi/v52/hqdefault.jpg",
				"index":        float64(1),
				"total":        float64(2),
			},
			map[string]any{
				"label":        "[Reversed] My list 51–52",
				"url":          "https://www.youtube.com/watch_videos?video_ids=v2,v0",
				"thumbnailUrl": "https://i.ytimg.com/vi/v2/hqdefault.jpg",
				"index":        float64(2),
				"total":        float64(2),
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("response =\n%v\nwant\n%v", got, want)
	}
}

func TestCreatePlaylistErrors(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		fake          *fakeYouTube
		wantStatus    int
		wantError     string
		wantItemCalls int64
	}{
		{
			name:       "malformed body",
			body:       `{"input":`,
			fake:       &fakeYouTube{},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name: "second JSON value after the first",
			body: `{"input":"PLx"}{"input":"PLx"}`,
			fake: &fakeYouTube{
				playlists: map[string]fakePlaylist{"PLx": {title: "x"}},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name: "body over 8 KiB after a valid value",
			body: `{"input":"PLx"}` + strings.Repeat(" ", 8<<10),
			fake: &fakeYouTube{
				playlists: map[string]fakePlaylist{"PLx": {title: "x"}},
			},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantError:  "request body too large",
		},
		{
			name:       "not a playlist",
			body:       `{"input":"https://example.com/watch?v=abc"}`,
			fake:       &fakeYouTube{},
			wantStatus: http.StatusBadRequest,
			wantError:  "not a YouTube playlist URL or ID",
		},
		{
			name:       "watch later",
			body:       `{"input":"WL"}`,
			fake:       &fakeYouTube{},
			wantStatus: http.StatusBadRequest,
			wantError:  "mixes, Watch Later, Liked videos and temporary lists are not supported",
		},
		{
			name:       "missing playlist",
			body:       `{"input":"PLmissing"}`,
			fake:       &fakeYouTube{},
			wantStatus: http.StatusNotFound,
			wantError:  "playlist not found or private",
		},
		{
			name:       "quota exceeded on metadata",
			body:       `{"input":"PLx"}`,
			fake:       &fakeYouTube{metaStatus: http.StatusForbidden},
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "YouTube API quota exceeded, try again later",
		},
		{
			name:       "upstream failure on metadata",
			body:       `{"input":"PLx"}`,
			fake:       &fakeYouTube{metaStatus: http.StatusInternalServerError},
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal error",
		},
		{
			name: "quota exceeded on items",
			body: `{"input":"PLx"}`,
			fake: &fakeYouTube{
				playlists:  map[string]fakePlaylist{"PLx": {title: "x", itemCount: 3}},
				itemStatus: http.StatusForbidden,
			},
			wantStatus:    http.StatusServiceUnavailable,
			wantError:     "YouTube API quota exceeded, try again later",
			wantItemCalls: 1,
		},
		{
			name: "upstream failure on items",
			body: `{"input":"PLx"}`,
			fake: &fakeYouTube{
				playlists:  map[string]fakePlaylist{"PLx": {title: "x", itemCount: 3}},
				itemStatus: http.StatusInternalServerError,
			},
			wantStatus:    http.StatusInternalServerError,
			wantError:     "internal error",
			wantItemCalls: 1,
		},
		{
			name: "playlist over 2000 items",
			body: `{"input":"PLhuge"}`,
			fake: &fakeYouTube{
				playlists: map[string]fakePlaylist{"PLhuge": {title: "huge", itemCount: 2001}},
			},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantError:  "playlist too large to process right now",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newIntegrationHandler(t, c.fake)
			code, got := postPlaylist(t, h, "203.0.113.7", c.body)
			if code != c.wantStatus {
				t.Errorf("status = %d, want %d", code, c.wantStatus)
			}
			if want := map[string]any{"error": c.wantError}; !reflect.DeepEqual(got, want) {
				t.Errorf("body = %v, want %v", got, want)
			}
			if n := c.fake.itemCalls.Load(); n != c.wantItemCalls {
				t.Errorf("playlistItems calls = %d, want %d", n, c.wantItemCalls)
			}
		})
	}
}

func TestCreatePlaylistLimitsBurstPerIP(t *testing.T) {
	h := newIntegrationHandler(t, &fakeYouTube{})
	for i := range 5 {
		if code, got := postPlaylist(t, h, "203.0.113.7", `{"input":"WL"}`); code != http.StatusBadRequest {
			t.Fatalf("request %d: status = %d, want %d; body %v", i+1, code, http.StatusBadRequest, got)
		}
	}
	code, got := postPlaylist(t, h, "203.0.113.7", `{"input":"WL"}`)
	if code != http.StatusTooManyRequests || got["error"] != "too many requests, please slow down and try again shortly" {
		t.Errorf("6th request: status = %d, body %v; want %d rate-limit error", code, got, http.StatusTooManyRequests)
	}
	if code, _ := postPlaylist(t, h, "203.0.113.8", `{"input":"WL"}`); code != http.StatusBadRequest {
		t.Errorf("other IP: status = %d, want %d", code, http.StatusBadRequest)
	}
}

func TestCreatePlaylistStopsAtDailyBudget(t *testing.T) {
	items := make([]fakeItem, 2000)
	for i := range items {
		items[i] = fakeItem{videoID: fmt.Sprintf("v%d", i), publishedAt: "2024-01-02T03:04:05Z"}
	}
	fake := &fakeYouTube{playlists: map[string]fakePlaylist{
		"PLmax": {title: "max", itemCount: len(items), items: items},
	}}
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().In(pacific).Format(time.DateOnly)
	h := newIntegrationHandler(t, fake)

	for i := range 195 {
		ip := fmt.Sprintf("198.51.100.%d", i)
		if code, got := postPlaylist(t, h, ip, `{"input":"PLmax"}`); code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d; error %v", i+1, code, http.StatusOK, got["error"])
		}
	}
	code, got := postPlaylist(t, h, "192.0.2.200", `{"input":"PLmax"}`)
	if time.Now().In(pacific).Format(time.DateOnly) != day {
		t.Skip("crossed midnight Pacific Time, when the daily budget resets")
	}
	if code != http.StatusTooManyRequests || got["error"] != "daily API quota exhausted, try again after quota resets at midnight Pacific Time" {
		t.Errorf("196th request: status = %d, error %v; want %d quota error", code, got["error"], http.StatusTooManyRequests)
	}
	if n := fake.itemCalls.Load(); n != 7800 {
		t.Errorf("playlistItems calls = %d, want 7800", n)
	}
}
