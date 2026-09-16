package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	yt "google.golang.org/api/youtube/v3"
)

// stubClient answers playlistItems.list with bodies, one per page request, and
// reports how many requests it received.
func stubClient(t *testing.T, bodies ...func(w http.ResponseWriter)) (*Client, *int) {
	t.Helper()

	got := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got >= len(bodies) {
			t.Errorf("page request %d has no stubbed response", got+1)
			http.Error(w, "{}", http.StatusInternalServerError)
			return
		}
		body := bodies[got]
		got++
		body(w)
	}))
	t.Cleanup(srv.Close)

	svc, err := yt.NewService(context.Background(), option.WithAPIKey("test"), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return &Client{svc: svc}, &got
}

func page(itemCount int, nextPageToken string) func(http.ResponseWriter) {
	res := yt.PlaylistItemListResponse{NextPageToken: nextPageToken}
	for range itemCount {
		res.Items = append(res.Items, &yt.PlaylistItem{
			Snippet: &yt.PlaylistItemSnippet{Title: "title"},
			ContentDetails: &yt.PlaylistItemContentDetails{
				VideoId:          "videoID",
				VideoPublishedAt: "2024-01-01T00:00:00Z",
			},
		})
	}
	body, err := json.Marshal(res)
	if err != nil {
		panic(err)
	}
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}

func badRequest(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	io.WriteString(w, `{"error":{"code":400,"message":"invalid"}}`)
}

// A page that comes back short still cost a page request, so the count of
// requests made cannot be recovered from the number of items returned.
func TestFetchPlaylistItemsCountsShortPages(t *testing.T) {
	c, got := stubClient(t, page(2, "next"), page(1, ""))

	items, attempted, err := c.FetchPlaylistItems(context.Background(), "PL", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Errorf("collected %d items, want 3", len(items))
	}
	if attempted != 2 {
		t.Errorf("reported %d page requests, want 2 (the server saw %d)", attempted, *got)
	}
}

// The request that failed had already gone out, so it counts too.
func TestFetchPlaylistItemsCountsTheFailedPage(t *testing.T) {
	c, got := stubClient(t, page(PageSize, "next"), badRequest)

	items, attempted, err := c.FetchPlaylistItems(context.Background(), "PL", 10)
	if err == nil {
		t.Fatal("a failing page request returned no error")
	}
	if len(items) != PageSize {
		t.Errorf("collected %d items, want the %d gathered before the failure", len(items), PageSize)
	}
	if attempted != 2 {
		t.Errorf("reported %d page requests, want 2 (the server saw %d)", attempted, *got)
	}
}

// An empty playlist still costs the one request that reports it empty.
func TestFetchPlaylistItemsCountsTheRequestForAnEmptyPlaylist(t *testing.T) {
	c, _ := stubClient(t, page(0, ""))

	items, attempted, err := c.FetchPlaylistItems(context.Background(), "PL", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("collected %d items, want 0", len(items))
	}
	if attempted != 1 {
		t.Errorf("reported %d page requests, want 1", attempted)
	}
}

// A playlist that keeps handing back a next-page token is stopped at maxPages
// rather than allowed to spend the whole daily budget on one request.
func TestFetchPlaylistItemsStopsAtMaxPages(t *testing.T) {
	c, _ := stubClient(t, page(PageSize, "next"), page(PageSize, "next"), page(PageSize, "next"))

	items, attempted, err := c.FetchPlaylistItems(context.Background(), "PL", 2)
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("error %v, want ErrTooManyPages", err)
	}
	if attempted != 2 {
		t.Errorf("made %d page requests, want to stop at the 2 allowed", attempted)
	}
	if len(items) != 2*PageSize {
		t.Errorf("collected %d items, want the %d from the 2 pages read", len(items), 2*PageSize)
	}
}
