package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	yt "google.golang.org/api/youtube/v3"

	"ytreverse/internal/quota"
)

const (
	writeCost = 50
	cacheTTL  = 10 * time.Minute
)

var ErrNotFound = errors.New("playlist not found or private")

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
	svc   *yt.Service
	oauth *oauth2.Config
	quota *quota.Counter
	rdb   *redis.Client
}

func New(ctx context.Context, apiKey string, oauth *oauth2.Config, q *quota.Counter, rdb *redis.Client) (*Client, error) {
	svc, err := yt.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc, oauth: oauth, quota: q, rdb: rdb}, nil
}

func (c *Client) FetchPlaylist(ctx context.Context, id string) (*Playlist, error) {
	if err := c.quota.Add(ctx, 1); err != nil {
		return nil, err
	}
	res, err := c.svc.Playlists.List([]string{"snippet"}).Id(id).Context(ctx).Do()
	if err != nil {
		return nil, apiErr(ctx, c.quota, err)
	}
	if len(res.Items) == 0 {
		return nil, ErrNotFound
	}
	p := &Playlist{ID: id, Title: res.Items[0].Snippet.Title, ChannelTitle: res.Items[0].Snippet.ChannelTitle}

	call := c.svc.PlaylistItems.List([]string{"snippet", "contentDetails"}).PlaylistId(id).MaxResults(50)
	err = call.Pages(ctx, func(page *yt.PlaylistItemListResponse) error {
		if err := c.quota.Add(ctx, 1); err != nil {
			return err
		}
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
		return nil, apiErr(ctx, c.quota, err)
	}
	return p, nil
}

func (c *Client) FetchPlaylistCached(ctx context.Context, id string) (*Playlist, error) {
	key := "src:" + id
	if b, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var p Playlist
		if json.Unmarshal(b, &p) == nil {
			return &p, nil
		}
	}
	p, err := c.FetchPlaylist(ctx, id)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if err := c.rdb.Set(ctx, key, b, cacheTTL).Err(); err != nil {
		return nil, err
	}
	return p, nil
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

type Writer struct {
	svc   *yt.Service
	quota *quota.Counter
}

func (c *Client) Writer(ctx context.Context, refreshToken string) (*Writer, error) {
	ts := c.oauth.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	svc, err := yt.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	return &Writer{svc: svc, quota: c.quota}, nil
}

func (w *Writer) CreatePlaylist(ctx context.Context, title, description string) (string, error) {
	if err := w.quota.Reserve(ctx, writeCost); err != nil {
		return "", err
	}
	pl, err := w.svc.Playlists.Insert([]string{"snippet", "status"}, &yt.Playlist{
		Snippet: &yt.PlaylistSnippet{Title: title, Description: description},
		Status:  &yt.PlaylistStatus{PrivacyStatus: "public"},
	}).Context(ctx).Do()
	if err != nil {
		return "", apiErr(ctx, w.quota, err)
	}
	return pl.Id, nil
}

func (w *Writer) InsertItem(ctx context.Context, playlistID, videoID string, position int) (string, error) {
	if err := w.quota.Reserve(ctx, writeCost); err != nil {
		return "", err
	}
	item, err := w.svc.PlaylistItems.Insert([]string{"snippet"}, &yt.PlaylistItem{
		Snippet: itemSnippet(playlistID, videoID, position),
	}).Context(ctx).Do()
	if err != nil {
		return "", apiErr(ctx, w.quota, err)
	}
	return item.Id, nil
}

func (w *Writer) MoveItem(ctx context.Context, playlistID, itemID, videoID string, position int) error {
	if err := w.quota.Reserve(ctx, writeCost); err != nil {
		return err
	}
	_, err := w.svc.PlaylistItems.Update([]string{"snippet"}, &yt.PlaylistItem{
		Id:      itemID,
		Snippet: itemSnippet(playlistID, videoID, position),
	}).Context(ctx).Do()
	if err != nil {
		return apiErr(ctx, w.quota, err)
	}
	return nil
}

func (w *Writer) DeleteItem(ctx context.Context, itemID string) error {
	if err := w.quota.Reserve(ctx, writeCost); err != nil {
		return err
	}
	if err := w.svc.PlaylistItems.Delete(itemID).Context(ctx).Do(); err != nil {
		return apiErr(ctx, w.quota, err)
	}
	return nil
}

func itemSnippet(playlistID, videoID string, position int) *yt.PlaylistItemSnippet {
	return &yt.PlaylistItemSnippet{
		PlaylistId:      playlistID,
		Position:        int64(position),
		ResourceId:      &yt.ResourceId{Kind: "youtube#video", VideoId: videoID},
		ForceSendFields: []string{"Position"},
	}
}

func apiErr(ctx context.Context, q *quota.Counter, err error) error {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code == 403 {
		for _, e := range gerr.Errors {
			if e.Reason == "quotaExceeded" {
				if err := q.Exhaust(ctx); err != nil {
					return err
				}
				return quota.ErrExhausted
			}
		}
	}
	return err
}
