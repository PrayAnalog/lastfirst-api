package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"google.golang.org/api/option"
	yt "google.golang.org/api/youtube/v3"
)

// stubClient answers playlistItems.list with bodies, one per page request, and
// appends "request" to log as each arrives.
func stubClient(t *testing.T, log *[]string, bodies ...func(w http.ResponseWriter) error) *Client {
	t.Helper()

	got := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*log = append(*log, "request")
		if got >= len(bodies) {
			t.Errorf("page request %d has no stubbed response", got+1)
			http.Error(w, "{}", http.StatusInternalServerError)
			return
		}
		body := bodies[got]
		got++
		if err := body(w); err != nil {
			t.Errorf("writing the stubbed response for page request %d: %v", got, err)
		}
	}))
	t.Cleanup(srv.Close)

	svc, err := yt.NewService(context.Background(), option.WithAPIKey("test"), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return &Client{svc: svc}
}

// granting records a reservation for every page and never refuses.
func granting(log *[]string) func() error {
	return func() error {
		*log = append(*log, "reserve")
		return nil
	}
}

// grantingOnce records reservations but refuses after the first n.
func grantingOnce(log *[]string, n int, refusal error) func() error {
	granted := 0
	return func() error {
		if granted == n {
			return refusal
		}
		granted++
		*log = append(*log, "reserve")
		return nil
	}
}

func page(itemCount int, nextPageToken string) func(http.ResponseWriter) error {
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
	return func(w http.ResponseWriter) error {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(body)
		return err
	}
}

func badRequest(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, err := io.WriteString(w, `{"error":{"code":400,"message":"invalid"}}`)
	return err
}

// The unit has to be claimed before the request that spends it, every time —
// including for the short second page, which the item count cannot predict.
func TestFetchPlaylistItemsReservesBeforeEveryPageRequest(t *testing.T) {
	var log []string
	c := stubClient(t, &log, page(2, "next"), page(1, ""))

	items, err := c.FetchPlaylistItems(context.Background(), "PL", 10, granting(&log))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Errorf("collected %d items, want 3", len(items))
	}
	want := []string{"reserve", "request", "reserve", "request"}
	if !slices.Equal(log, want) {
		t.Errorf("sequence %v, want %v", log, want)
	}
}

// An empty playlist still costs the one request that reports it empty.
func TestFetchPlaylistItemsReservesForAnEmptyPlaylist(t *testing.T) {
	var log []string
	c := stubClient(t, &log, page(0, ""))

	items, err := c.FetchPlaylistItems(context.Background(), "PL", 10, granting(&log))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("collected %d items, want 0", len(items))
	}
	want := []string{"reserve", "request"}
	if !slices.Equal(log, want) {
		t.Errorf("sequence %v, want %v", log, want)
	}
}

// A refused reservation stops the walk before the request it would have paid
// for, and comes back as the refusal the caller gave.
func TestFetchPlaylistItemsStopsWhenAPageCannotBeReserved(t *testing.T) {
	refusal := errors.New("budget exhausted")
	var log []string
	c := stubClient(t, &log, page(PageSize, "next"))

	items, err := c.FetchPlaylistItems(context.Background(), "PL", 10, grantingOnce(&log, 1, refusal))
	if !errors.Is(err, refusal) {
		t.Fatalf("error %v, want the caller's refusal", err)
	}
	if len(items) != PageSize {
		t.Errorf("collected %d items, want the %d from the page that was paid for", len(items), PageSize)
	}
	want := []string{"reserve", "request"}
	if !slices.Equal(log, want) {
		t.Errorf("sequence %v, want %v — the unreserved page must not be requested", log, want)
	}
}

// The cap stops the walk without reserving a unit for a request it will not
// make.
func TestFetchPlaylistItemsStopsAtMaxPages(t *testing.T) {
	var log []string
	c := stubClient(t, &log, page(PageSize, "next"), page(PageSize, "next"))

	items, err := c.FetchPlaylistItems(context.Background(), "PL", 2, granting(&log))
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("error %v, want ErrTooManyPages", err)
	}
	if len(items) != 2*PageSize {
		t.Errorf("collected %d items, want the %d from the 2 pages read", len(items), 2*PageSize)
	}
	want := []string{"reserve", "request", "reserve", "request"}
	if !slices.Equal(log, want) {
		t.Errorf("sequence %v, want %v", log, want)
	}
}

// A page request that failed had already gone out, so the unit it reserved
// stays spent rather than coming back.
func TestFetchPlaylistItemsKeepsTheUnitForAFailedPage(t *testing.T) {
	var log []string
	c := stubClient(t, &log, page(PageSize, "next"), badRequest)

	items, err := c.FetchPlaylistItems(context.Background(), "PL", 10, granting(&log))
	if err == nil {
		t.Fatal("a failing page request returned no error")
	}
	if len(items) != PageSize {
		t.Errorf("collected %d items, want the %d gathered before the failure", len(items), PageSize)
	}
	want := []string{"reserve", "request", "reserve", "request"}
	if !slices.Equal(log, want) {
		t.Errorf("sequence %v, want %v", log, want)
	}
}

// A context that is already done cannot reach YouTube, so nothing is reserved
// for it and no request goes out.
func TestFetchPlaylistItemsReservesNothingForADoneContext(t *testing.T) {
	var log []string
	c := stubClient(t, &log, page(PageSize, ""))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	items, err := c.FetchPlaylistItems(ctx, "PL", 10, granting(&log))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
	if len(items) != 0 {
		t.Errorf("collected %d items, want 0", len(items))
	}
	if len(log) != 0 {
		t.Errorf("sequence %v, want nothing reserved and nothing requested", log)
	}
}
