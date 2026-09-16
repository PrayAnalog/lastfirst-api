package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	yt "google.golang.org/api/youtube/v3"
)

var (
	ErrNotFound         = errors.New("playlist not found or private")
	ErrQuotaExceeded    = errors.New("YouTube API quota exceeded, try again later")
	ErrPageLimitReached = errors.New("playlist grew beyond the allowed size while it was being fetched")
	ErrPageNotReserved  = errors.New("playlist page was not reserved")
)

const ReverseTitlePrefix = "[Reversed] "

// PageSize is how many items playlistItems.list returns per page; each page
// costs one YouTube API quota unit.
const PageSize = 50

type Item struct {
	VideoID     string     `json:"videoId"`
	Title       string     `json:"title"`
	PublishedAt *time.Time `json:"publishedAt"`
}

type Playlist struct {
	ID           string
	Title        string
	ChannelTitle string
	ItemCount    int64
}

type Client struct {
	svc *yt.Service
}

func New(ctx context.Context, apiKey string) (*Client, error) {
	httpClient := &http.Client{Transport: &apiKeyTransport{
		apiKey: apiKey,
		base:   http.DefaultTransport,
	}}
	svc, err := yt.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc}, nil
}

// apiKeyTransport keeps the credential out of request URLs, which are much
// more likely than headers to be captured by proxies and error logs.
type apiKeyTransport struct {
	apiKey string
	base   http.RoundTripper
}

func (t *apiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("X-Goog-Api-Key", t.apiKey)
	return t.base.RoundTrip(cloned)
}

// FetchPlaylistMeta costs a single quota unit and reports the playlist's
// item count, so the caller can work out the cost of FetchPlaylistItems
// before spending it.
func (c *Client) FetchPlaylistMeta(ctx context.Context, id string) (*Playlist, error) {
	res, err := c.svc.Playlists.List([]string{"snippet", "contentDetails"}).Id(id).Context(ctx).Do()
	if err != nil {
		return nil, apiErr(err)
	}
	if len(res.Items) == 0 {
		return nil, ErrNotFound
	}
	it := res.Items[0]
	return &Playlist{ID: id, Title: it.Snippet.Title, ChannelTitle: it.Snippet.ChannelTitle, ItemCount: it.ContentDetails.ItemCount}, nil
}

// FetchPlaylistItems costs one quota unit per page. beforePage must reserve the
// next page before its request begins. The returned call count includes a
// failed attempted page so the caller can account conservatively.
func (c *Client) FetchPlaylistItems(ctx context.Context, id string, maxPages int, beforePage func() (func(), error)) ([]Item, int, error) {
	var items []Item
	pageToken := ""
	calls := 0
	for {
		if calls >= maxPages {
			return nil, calls, ErrPageLimitReached
		}
		complete, err := beforePage()
		if err != nil {
			return nil, calls, err
		}
		call := c.svc.PlaylistItems.List([]string{"snippet", "contentDetails"}).PlaylistId(id).MaxResults(PageSize)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		calls++
		page, err := func() (*yt.PlaylistItemListResponse, error) {
			defer complete()
			return call.Context(ctx).Do()
		}()
		if err != nil {
			return nil, calls, apiErr(err)
		}
		for _, it := range page.Items {
			item := Item{VideoID: it.ContentDetails.VideoId, Title: it.Snippet.Title}
			if t, err := time.Parse(time.RFC3339, it.ContentDetails.VideoPublishedAt); err == nil {
				item.PublishedAt = &t
			}
			items = append(items, item)
		}
		if page.NextPageToken == "" {
			return items, calls, nil
		}
		pageToken = page.NextPageToken
	}
}

func ReverseTitle(sourceTitle string) string {
	return ReverseTitlePrefix + sourceTitle
}

func Reversed(items []Item) []string {
	var ids []string
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].PublishedAt != nil {
			ids = append(ids, items[i].VideoID)
		}
	}
	return ids
}

func apiErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == 403 {
			for _, e := range gerr.Errors {
				if e.Reason == "quotaExceeded" {
					return ErrQuotaExceeded
				}
			}
		}
		// googleapi.Error may include the upstream response body. Preserve the
		// useful status without allowing a provider response into application logs.
		return fmt.Errorf("YouTube API request failed with HTTP status %d", gerr.Code)
	}
	// Transport errors can include the full request URL (and API key). Return a
	// stable error instead of propagating credentials into logs.
	return errors.New("YouTube API transport request failed")
}
