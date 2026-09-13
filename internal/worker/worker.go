package worker

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	"ytreverse/internal/ordercheck"
	"ytreverse/internal/quota"
	"ytreverse/internal/store"
	"ytreverse/internal/youtube"
)

const (
	pollInterval  = 5 * time.Second
	maxTitleRunes = 150
)

type Worker struct {
	store *store.Store
	yt    *youtube.Client
}

func New(st *store.Store, yt *youtube.Client) *Worker {
	return &Worker{store: st, yt: yt}
}

func (w *Worker) Run(ctx context.Context) {
	for {
		job, err := w.store.ClaimJob(ctx)
		if err != nil {
			log.Printf("claim job: %v", err)
		}
		if job != nil {
			w.handle(ctx, job)
			continue
		}
		time.Sleep(pollInterval)
	}
}

func (w *Worker) handle(ctx context.Context, job *store.Job) {
	err := w.process(ctx, job)
	status, runAfter := "done", job.RunAfter
	var errMsg *string
	switch {
	case errors.Is(err, quota.ErrExhausted):
		status, runAfter = "waiting_quota", quota.NextReset(time.Now())
	case err != nil:
		log.Printf("job %d: %v", job.ID, err)
		msg := err.Error()
		status, errMsg = "failed", &msg
	}
	if err := w.store.FinishJob(ctx, job, status, runAfter, errMsg); err != nil {
		log.Printf("finish job %d: %v", job.ID, err)
	}
}

func (w *Worker) process(ctx context.Context, job *store.Job) error {
	rp, err := w.store.GetReverse(ctx, job.ReversePlaylistID)
	if err != nil {
		return err
	}
	if err := w.store.SetReverseStatus(ctx, rp.ID, "building"); err != nil {
		return err
	}
	if job.Type == "sync" {
		p, err := w.yt.FetchPlaylist(ctx, rp.SourceID)
		if err != nil {
			return err
		}
		if err := w.store.SaveSource(ctx, p, ordercheck.Check(p.Items)); err != nil {
			return err
		}
		rp.SourceTitle = p.Title
	}
	items, err := w.store.SourceItems(ctx, rp.SourceID)
	if err != nil {
		return err
	}

	token, err := w.store.RefreshToken(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return errors.New("service channel is not connected")
	}
	if err != nil {
		return err
	}
	wr, err := w.yt.Writer(ctx, token)
	if err != nil {
		return err
	}

	if rp.YTPlaylistID == nil {
		id, err := wr.CreatePlaylist(ctx, playlistTitle(rp.SourceTitle),
			"원본: https://www.youtube.com/playlist?list="+rp.SourceID)
		if err != nil {
			return err
		}
		if err := w.store.SetYTPlaylistID(ctx, rp.ID, id); err != nil {
			return err
		}
		rp.YTPlaylistID = &id
	}
	return w.apply(ctx, wr, rp.ID, *rp.YTPlaylistID, youtube.Reversed(items))
}

func (w *Worker) apply(ctx context.Context, wr *youtube.Writer, rpID int64, playlistID string, target []string) error {
	current, err := w.store.ReverseItems(ctx, rpID)
	if err != nil {
		return err
	}

	want := make(map[string]bool, len(target))
	for _, id := range target {
		want[id] = true
	}
	for i := len(current) - 1; i >= 0; i-- {
		if want[current[i].VideoID] {
			continue
		}
		if err := wr.DeleteItem(ctx, current[i].YTItemID); err != nil {
			return err
		}
		if err := w.store.DeleteItemAt(ctx, rpID, i); err != nil {
			return err
		}
		current = slices.Delete(current, i, i+1)
	}

	for i, videoID := range target {
		if i < len(current) && current[i].VideoID == videoID {
			continue
		}
		j := slices.IndexFunc(current[i:], func(it store.Item) bool { return it.VideoID == videoID })
		if j < 0 {
			itemID, err := wr.InsertItem(ctx, playlistID, videoID, i)
			if err != nil {
				return err
			}
			if err := w.store.InsertItemAt(ctx, rpID, i, videoID, itemID); err != nil {
				return err
			}
			current = slices.Insert(current, i, store.Item{VideoID: videoID, YTItemID: itemID})
			continue
		}
		j += i
		item := current[j]
		if err := wr.MoveItem(ctx, playlistID, item.YTItemID, videoID, i); err != nil {
			return err
		}
		if err := w.store.MoveItem(ctx, rpID, j, i); err != nil {
			return err
		}
		current = slices.Insert(slices.Delete(current, j, j+1), i, item)
	}
	return nil
}

func playlistTitle(source string) string {
	r := []rune("[역순] " + source)
	if len(r) > maxTitleRunes {
		r = r[:maxTitleRunes]
	}
	return string(r)
}
