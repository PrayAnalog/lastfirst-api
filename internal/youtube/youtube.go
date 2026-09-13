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

const ReverseTitlePrefix = "[역순] "

type Item struct {
	VideoID     string     `json:"videoId"`
	Title       string     `json:"title"`
	PublishedAt *time.Time `json:"publishedAt"`
}

type Playlist struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ChannelTitle string `json:"channelTitle"`
	Items        []Item `json:"items"`
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

func (c *Client) FetchPlaylist(ctx context.Context, id string) (*Playlist, error) {
	res, err := c.svc.Playlists.List([]string{"snippet"}).Id(id).Context(ctx).Do()
	if err != nil {
		return nil, apiErr(err)
	}
	if len(res.Items) == 0 {
		return nil, ErrNotFound
	}
	p := &Playlist{ID: id, Title: res.Items[0].Snippet.Title, ChannelTitle: res.Items[0].Snippet.ChannelTitle}

	call := c.svc.PlaylistItems.List([]string{"snippet", "contentDetails"}).PlaylistId(id).MaxResults(50)
	err = call.Pages(ctx, func(page *yt.PlaylistItemListResponse) error {
		for _, it := range page.Items {
			item := Item{VideoID: it.ContentDetails.VideoId, Title: it.Snippet.Title}
			if t, err := time.Parse(time.RFC3339, it.ContentDetails.VideoPublishedAt); err == nil {
				item.PublishedAt = &t
			}
			p.Items = append(p.Items, item)
		}
		return nil
	})
	if err != nil {
		return nil, apiErr(err)
	}
	return p, nil
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
