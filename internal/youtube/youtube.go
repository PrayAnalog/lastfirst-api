package youtube

import (
	"context"
	"errors"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	yt "google.golang.org/api/youtube/v3"
)

var (
	ErrNotFound      = errors.New("playlist not found or private")
	ErrQuotaExceeded = errors.New("YouTube API quota exceeded, try again later")
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
	svc, err := yt.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc}, nil
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

// FetchPlaylistItems costs one quota unit per page request, which is one
// request per PageSize items plus however many short pages the API decides to
// return. It reports how many page requests it made, counting a failed one:
// that request may still have reached YouTube and been billed. Items gathered
// before a failure come back alongside the error, so a caller that gave up
// early still knows what its reservation bought.
func (c *Client) FetchPlaylistItems(ctx context.Context, id string) ([]Item, int, error) {
	call := c.svc.PlaylistItems.List([]string{"snippet", "contentDetails"}).PlaylistId(id).MaxResults(PageSize).Context(ctx)
	var items []Item
	pages := 0
	for {
		pages++
		page, err := call.Do()
		if err != nil {
			return items, pages, apiErr(err)
		}
		for _, it := range page.Items {
			item := Item{VideoID: it.ContentDetails.VideoId, Title: it.Snippet.Title}
			if t, err := time.Parse(time.RFC3339, it.ContentDetails.VideoPublishedAt); err == nil {
				item.PublishedAt = &t
			}
			items = append(items, item)
		}
		if page.NextPageToken == "" {
			return items, pages, nil
		}
		call = call.PageToken(page.NextPageToken)
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
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code == 403 {
		for _, e := range gerr.Errors {
			if e.Reason == "quotaExceeded" {
				return ErrQuotaExceeded
			}
		}
	}
	return err
}
