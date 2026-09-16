package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ytreverse/internal/ratelimit"
	"ytreverse/internal/youtube"
)

type fakeYouTube struct {
	mu           sync.Mutex
	metaCalls    int
	itemCalls    int
	meta         *youtube.Playlist
	items        []youtube.Item
	pages        int
	itemMaxPages int
	metaErr      error
	itemsErr     error
	metaStarted  chan struct{}
	releaseMeta  chan struct{}
}

func (f *fakeYouTube) FetchPlaylistMeta(ctx context.Context, _ string) (*youtube.Playlist, error) {
	f.mu.Lock()
	f.metaCalls++
	f.mu.Unlock()
	if f.metaStarted != nil {
		select {
		case f.metaStarted <- struct{}{}:
		default:
		}
	}
	if f.releaseMeta != nil {
		select {
		case <-f.releaseMeta:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.meta, f.metaErr
}

func (f *fakeYouTube) FetchPlaylistItems(_ context.Context, _ string, maxPages int) ([]youtube.Item, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.itemCalls++
	f.itemMaxPages = maxPages
	return f.items, f.pages, f.itemsErr
}

func (f *fakeYouTube) calls() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.metaCalls, f.itemCalls
}

func TestCreatePlaylistRejectsOversizedBodyBeforeYouTubeCall(t *testing.T) {
	yt := &fakeYouTube{meta: &youtube.Playlist{ID: "PL123"}}
	server := New(yt, t.TempDir(), false)
	body, err := json.Marshal(map[string]string{"input": strings.Repeat("a", maxRequestBody)})
	if err != nil {
		t.Fatal(err)
	}

	recorder := performPlaylistRequest(server, string(body))
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
	if metaCalls, itemCalls := yt.calls(); metaCalls != 0 || itemCalls != 0 {
		t.Fatalf("YouTube calls = (%d, %d), want (0, 0)", metaCalls, itemCalls)
	}
}

func TestCreatePlaylistRejectsTrailingJSON(t *testing.T) {
	yt := &fakeYouTube{meta: &youtube.Playlist{ID: "PL123"}}
	server := New(yt, t.TempDir(), false)

	recorder := performPlaylistRequest(server, `{"input":"PL123"}{}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestCreatePlaylistReservesQuotaBeforeMetadataCall(t *testing.T) {
	yt := &fakeYouTube{meta: &youtube.Playlist{ID: "PL123"}}
	server := New(yt, t.TempDir(), false)
	server.budget = ratelimit.NewDailyBudget(0)

	recorder := performPlaylistRequest(server, `{"input":"PL123"}`)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if metaCalls, _ := yt.calls(); metaCalls != 0 {
		t.Fatalf("metadata calls = %d, want 0", metaCalls)
	}
}

func TestCreatePlaylistRejectsWorkWhenConcurrencyIsFull(t *testing.T) {
	yt := &fakeYouTube{
		meta:        &youtube.Playlist{ID: "PL123"},
		pages:       1,
		metaStarted: make(chan struct{}, 1),
		releaseMeta: make(chan struct{}),
	}
	server := New(yt, t.TempDir(), false)
	server.slots = make(chan struct{}, 1)

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- performPlaylistRequest(server, `{"input":"PL123"}`)
	}()

	select {
	case <-yt.metaStarted:
	case <-time.After(time.Second):
		t.Fatal("first request did not reach metadata call")
	}

	second := performPlaylistRequest(server, `{"input":"PL123"}`)
	if second.Code != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusServiceUnavailable)
	}
	close(yt.releaseMeta)
	select {
	case first := <-firstDone:
		if first.Code != http.StatusOK {
			t.Fatalf("first status = %d, want %d", first.Code, http.StatusOK)
		}
	case <-time.After(time.Second):
		t.Fatal("first request did not complete")
	}
}

func TestClientIPOnlyTrustsConfiguredProxyHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Real-IP", "198.51.100.20")

	if got := clientIP(request, false); got != "192.0.2.10" {
		t.Fatalf("untrusted clientIP = %q, want remote address", got)
	}
	if got := clientIP(request, true); got != "198.51.100.20" {
		t.Fatalf("trusted clientIP = %q, want proxy address", got)
	}
	request.Header.Set("X-Real-IP", "not-an-ip")
	if got := clientIP(request, true); got != "192.0.2.10" {
		t.Fatalf("invalid forwarded clientIP = %q, want remote address", got)
	}
}

func TestHealthIncludesSecurityHeaders(t *testing.T) {
	server := New(&fakeYouTube{}, t.TempDir(), false)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("Content-Security-Policy is missing")
	}
}

func TestCreatePlaylistMapsDeadlineToGatewayTimeout(t *testing.T) {
	yt := &fakeYouTube{metaErr: context.DeadlineExceeded}
	server := New(yt, t.TempDir(), false)

	recorder := performPlaylistRequest(server, `{"input":"PL123"}`)
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
	}
}

func TestCreatePlaylistMapsPageLimitToRequestTooLarge(t *testing.T) {
	yt := &fakeYouTube{
		meta:     &youtube.Playlist{ID: "PL123", ItemCount: 1},
		pages:    1,
		itemsErr: youtube.ErrPageLimitReached,
	}
	server := New(yt, t.TempDir(), false)

	recorder := performPlaylistRequest(server, `{"input":"PL123"}`)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCreatePlaylistAllowsGrowthWithinPlaylistLimit(t *testing.T) {
	publishedAt := time.Now()
	yt := &fakeYouTube{
		meta:  &youtube.Playlist{ID: "PL123", ItemCount: youtube.PageSize},
		items: []youtube.Item{{VideoID: "video", PublishedAt: &publishedAt}},
		pages: 2,
	}
	server := New(yt, t.TempDir(), false)
	server.budget = ratelimit.NewDailyBudget(maxPlaylistCost)

	recorder := performPlaylistRequest(server, `{"input":"PL123"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	yt.mu.Lock()
	if yt.itemMaxPages != maxPlaylistPages {
		t.Fatalf("max pages = %d, want %d", yt.itemMaxPages, maxPlaylistPages)
	}
	yt.mu.Unlock()

	if _, ok := server.budget.Reserve(maxPlaylistCost - 3); !ok { // one metadata call and two item calls
		t.Fatal("unused item-page reservation capacity was not released")
	}
}

func performPlaylistRequest(server *Server, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/playlists", strings.NewReader(body))
	request.RemoteAddr = "192.0.2.1:1234"
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}
