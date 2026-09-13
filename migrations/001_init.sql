-- +goose Up
CREATE TABLE source_playlists (
    id text PRIMARY KEY,
    title text NOT NULL,
    channel_title text NOT NULL,
    item_count int NOT NULL,
    last_fetched_at timestamptz NOT NULL
);

CREATE TABLE source_items (
    source_playlist_id text NOT NULL REFERENCES source_playlists (id) ON DELETE CASCADE,
    position int NOT NULL,
    video_id text NOT NULL,
    title text NOT NULL,
    published_at timestamptz,
    PRIMARY KEY (source_playlist_id, position)
);

CREATE TABLE order_checks (
    id bigserial PRIMARY KEY,
    source_playlist_id text NOT NULL REFERENCES source_playlists (id) ON DELETE CASCADE,
    checked_at timestamptz NOT NULL DEFAULT now(),
    verdict text NOT NULL,
    basis text NOT NULL,
    title_signal jsonb NOT NULL,
    date_signal jsonb NOT NULL,
    anomalies jsonb NOT NULL
);

CREATE TABLE reverse_playlists (
    id bigserial PRIMARY KEY,
    source_playlist_id text NOT NULL UNIQUE REFERENCES source_playlists (id),
    yt_playlist_id text,
    status text NOT NULL DEFAULT 'queued',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_synced_at timestamptz
);

CREATE TABLE reverse_playlist_items (
    reverse_playlist_id bigint NOT NULL REFERENCES reverse_playlists (id) ON DELETE CASCADE,
    position int NOT NULL,
    video_id text NOT NULL,
    yt_item_id text NOT NULL,
    PRIMARY KEY (reverse_playlist_id, yt_item_id)
);

CREATE TABLE jobs (
    id bigserial PRIMARY KEY,
    reverse_playlist_id bigint NOT NULL REFERENCES reverse_playlists (id) ON DELETE CASCADE,
    type text NOT NULL,
    trigger text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    run_after timestamptz NOT NULL DEFAULT now(),
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX jobs_one_active ON jobs (reverse_playlist_id)
    WHERE status IN ('pending', 'running', 'waiting_quota');

CREATE TABLE oauth_token (
    id int PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    refresh_token text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
