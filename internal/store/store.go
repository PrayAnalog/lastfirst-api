package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ytreverse/internal/ordercheck"
	"ytreverse/internal/youtube"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

type Check struct {
	ID        int64     `json:"id"`
	CheckedAt time.Time `json:"checkedAt"`
	ordercheck.Result
}

type ReversePlaylist struct {
	ID            int64      `json:"id"`
	SourceID      string     `json:"sourceId"`
	SourceTitle   string     `json:"sourceTitle"`
	SourceChannel string     `json:"sourceChannel"`
	YTPlaylistID  *string    `json:"ytPlaylistId"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastSyncedAt  *time.Time `json:"lastSyncedAt"`
	Done          int        `json:"done"`
	Total         int        `json:"total"`
	Verdict       string     `json:"verdict"`
}

type Item struct {
	VideoID  string
	YTItemID string
}

type Job struct {
	ID                int64     `json:"id"`
	ReversePlaylistID int64     `json:"reversePlaylistId"`
	Type              string    `json:"type"`
	Trigger           string    `json:"trigger"`
	Status            string    `json:"status"`
	RunAfter          time.Time `json:"runAfter"`
	Error             *string   `json:"error"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Store) SaveSource(ctx context.Context, p *youtube.Playlist, check ordercheck.Result) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO source_playlists (id, title, channel_title, item_count, last_fetched_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (id) DO UPDATE SET
				title = EXCLUDED.title,
				channel_title = EXCLUDED.channel_title,
				item_count = EXCLUDED.item_count,
				last_fetched_at = EXCLUDED.last_fetched_at`,
			p.ID, p.Title, p.ChannelTitle, len(p.Items))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM source_items WHERE source_playlist_id = $1`, p.ID); err != nil {
			return err
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"source_items"},
			[]string{"source_playlist_id", "position", "video_id", "title", "published_at"},
			pgx.CopyFromSlice(len(p.Items), func(i int) ([]any, error) {
				it := p.Items[i]
				return []any{p.ID, i, it.VideoID, it.Title, it.PublishedAt}, nil
			}))
		if err != nil {
			return err
		}
		anomalies := check.Anomalies
		if anomalies == nil {
			anomalies = []int{}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO order_checks (source_playlist_id, verdict, basis, title_signal, date_signal, anomalies)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			p.ID, check.Verdict, check.Basis, check.Title, check.Date, anomalies)
		return err
	})
}

func (s *Store) SourceItems(ctx context.Context, sourceID string) ([]youtube.Item, error) {
	rows, err := s.db.Query(ctx, `
		SELECT video_id, title, published_at FROM source_items
		WHERE source_playlist_id = $1 ORDER BY position`, sourceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (youtube.Item, error) {
		var it youtube.Item
		err := row.Scan(&it.VideoID, &it.Title, &it.PublishedAt)
		return it, err
	})
}

const checkQuery = `
	SELECT id, checked_at, verdict, basis, title_signal, date_signal, anomalies
	FROM order_checks WHERE source_playlist_id = $1 ORDER BY id DESC`

func scanCheck(row pgx.Row) (Check, error) {
	var c Check
	err := row.Scan(&c.ID, &c.CheckedAt, &c.Verdict, &c.Basis, &c.Title, &c.Date, &c.Anomalies)
	return c, err
}

func (s *Store) LatestCheck(ctx context.Context, sourceID string) (Check, error) {
	c, err := scanCheck(s.db.QueryRow(ctx, checkQuery+` LIMIT 1`, sourceID))
	return c, notFound(err)
}

func (s *Store) Checks(ctx context.Context, sourceID string) ([]Check, error) {
	rows, err := s.db.Query(ctx, checkQuery, sourceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Check, error) { return scanCheck(row) })
}

const reverseQuery = `
	SELECT rp.id, rp.source_playlist_id, sp.title, sp.channel_title, rp.yt_playlist_id, rp.status,
		rp.created_at, rp.last_synced_at,
		(SELECT count(*) FROM reverse_playlist_items i WHERE i.reverse_playlist_id = rp.id),
		(SELECT count(*) FROM source_items si
			WHERE si.source_playlist_id = rp.source_playlist_id AND si.published_at IS NOT NULL),
		COALESCE(oc.verdict, '')
	FROM reverse_playlists rp
	JOIN source_playlists sp ON sp.id = rp.source_playlist_id
	LEFT JOIN LATERAL (
		SELECT verdict FROM order_checks
		WHERE source_playlist_id = rp.source_playlist_id ORDER BY id DESC LIMIT 1
	) oc ON true`

func scanReverse(row pgx.Row) (ReversePlaylist, error) {
	var rp ReversePlaylist
	err := row.Scan(&rp.ID, &rp.SourceID, &rp.SourceTitle, &rp.SourceChannel, &rp.YTPlaylistID, &rp.Status,
		&rp.CreatedAt, &rp.LastSyncedAt, &rp.Done, &rp.Total, &rp.Verdict)
	return rp, err
}

func (s *Store) GetReverse(ctx context.Context, id int64) (ReversePlaylist, error) {
	rp, err := scanReverse(s.db.QueryRow(ctx, reverseQuery+` WHERE rp.id = $1`, id))
	return rp, notFound(err)
}

func (s *Store) ReverseBySource(ctx context.Context, sourceID string) (ReversePlaylist, error) {
	rp, err := scanReverse(s.db.QueryRow(ctx, reverseQuery+` WHERE rp.source_playlist_id = $1`, sourceID))
	return rp, notFound(err)
}

func (s *Store) ListReverse(ctx context.Context, verdict, status string) ([]ReversePlaylist, error) {
	rows, err := s.db.Query(ctx, reverseQuery+`
		WHERE ($1 = '' OR oc.verdict = $1) AND ($2 = '' OR rp.status = $2)
		ORDER BY rp.id DESC`, verdict, status)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ReversePlaylist, error) { return scanReverse(row) })
}

func (s *Store) CreateReverse(ctx context.Context, sourceID string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO reverse_playlists (source_playlist_id) VALUES ($1)
		ON CONFLICT (source_playlist_id) DO UPDATE SET source_playlist_id = EXCLUDED.source_playlist_id
		RETURNING id`, sourceID).Scan(&id)
	return id, err
}

func (s *Store) SetReverseStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.Exec(ctx, `UPDATE reverse_playlists SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (s *Store) SetYTPlaylistID(ctx context.Context, id int64, ytPlaylistID string) error {
	_, err := s.db.Exec(ctx, `UPDATE reverse_playlists SET yt_playlist_id = $2 WHERE id = $1`, id, ytPlaylistID)
	return err
}

func (s *Store) ReverseItems(ctx context.Context, rpID int64) ([]Item, error) {
	rows, err := s.db.Query(ctx, `
		SELECT video_id, yt_item_id FROM reverse_playlist_items
		WHERE reverse_playlist_id = $1 ORDER BY position`, rpID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Item])
}

func (s *Store) InsertItemAt(ctx context.Context, rpID int64, pos int, videoID, ytItemID string) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE reverse_playlist_items SET position = position + 1
			WHERE reverse_playlist_id = $1 AND position >= $2`, rpID, pos)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO reverse_playlist_items (reverse_playlist_id, position, video_id, yt_item_id)
			VALUES ($1, $2, $3, $4)`, rpID, pos, videoID, ytItemID)
		return err
	})
}

func (s *Store) DeleteItemAt(ctx context.Context, rpID int64, pos int) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			DELETE FROM reverse_playlist_items WHERE reverse_playlist_id = $1 AND position = $2`, rpID, pos)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE reverse_playlist_items SET position = position - 1
			WHERE reverse_playlist_id = $1 AND position > $2`, rpID, pos)
		return err
	})
}

func (s *Store) MoveItem(ctx context.Context, rpID int64, from, to int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE reverse_playlist_items
		SET position = CASE WHEN position = $2 THEN $3 ELSE position + 1 END
		WHERE reverse_playlist_id = $1 AND position BETWEEN $3 AND $2`, rpID, from, to)
	return err
}

const jobCols = `id, reverse_playlist_id, type, trigger, status, run_after, error, created_at, updated_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.ReversePlaylistID, &j.Type, &j.Trigger, &j.Status, &j.RunAfter, &j.Error,
		&j.CreatedAt, &j.UpdatedAt)
	return j, err
}

func (s *Store) EnqueueJob(ctx context.Context, rpID int64, typ, trigger string) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO jobs (reverse_playlist_id, type, trigger) VALUES ($1, $2, $3)
			ON CONFLICT (reverse_playlist_id) WHERE status IN ('pending', 'running', 'waiting_quota') DO NOTHING`,
			rpID, typ, trigger)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE reverse_playlists SET status = 'queued' WHERE id = $1`, rpID)
		return err
	})
}

func (s *Store) ClaimJob(ctx context.Context) (*Job, error) {
	j, err := scanJob(s.db.QueryRow(ctx, `
		UPDATE jobs SET status = 'running', updated_at = now()
		WHERE id = (
			SELECT id FROM jobs
			WHERE status IN ('pending', 'waiting_quota') AND run_after <= now()
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1
		)
		RETURNING `+jobCols))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) FinishJob(ctx context.Context, job *Job, status string, runAfter time.Time, errMsg *string) error {
	rpStatus := status
	if status == "done" {
		rpStatus = "ready"
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE jobs SET status = $2, run_after = $3, error = $4, updated_at = now() WHERE id = $1`,
			job.ID, status, runAfter, errMsg)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE reverse_playlists SET status = $2,
				last_synced_at = CASE WHEN $2 = 'ready' THEN now() ELSE last_synced_at END
			WHERE id = $1`, job.ReversePlaylistID, rpStatus)
		return err
	})
}

func (s *Store) ActiveJob(ctx context.Context, rpID int64) (*Job, error) {
	j, err := scanJob(s.db.QueryRow(ctx, `
		SELECT `+jobCols+` FROM jobs
		WHERE reverse_playlist_id = $1 AND status IN ('pending', 'running', 'waiting_quota')`, rpID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) ResetRunningJobs(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `UPDATE jobs SET status = 'pending', updated_at = now() WHERE status = 'running'`)
	return err
}

func (s *Store) ListJobs(ctx context.Context) ([]Job, error) {
	rows, err := s.db.Query(ctx, `SELECT `+jobCols+` FROM jobs ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Job, error) { return scanJob(row) })
}

func (s *Store) SaveRefreshToken(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO oauth_token (id, refresh_token) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET refresh_token = EXCLUDED.refresh_token, updated_at = now()`, token)
	return err
}

func (s *Store) RefreshToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRow(ctx, `SELECT refresh_token FROM oauth_token WHERE id = 1`).Scan(&token)
	return token, notFound(err)
}
