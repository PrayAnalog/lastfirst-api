# lastfirst-api

Backend for [lastfirst.app](https://lastfirst.app) — a YouTube reverse-playlist
service. Given a playlist URL/ID, it fetches the playlist via the YouTube
Data API v3, reverses the video order, and returns "watch together" links
(`youtube.com/watch_videos`) so viewers can start from the first episode
instead of the last.

Also serves the built [lastfirst-web](https://github.com/PrayAnalog/lastfirst-web)
frontend as static files.

## Stack

- Go 1.26, standard `net/http`
- `google.golang.org/api/youtube/v3` for the YouTube Data API

## API

`POST /api/playlists`

```json
{ "input": "https://www.youtube.com/playlist?list=..." }
```

Accepts a playlist URL or a raw playlist ID. Returns the reversed video
order as a list of `watch_videos` links (chunked to 50 videos each), along
with source/reverse playlist titles and item counts.

## Configuration

| Env var | Default | Description |
|---|---|---|
| `YT_API_KEY` | — | YouTube Data API v3 key |
| `ADDR` | `:8080` | HTTP listen address |
| `STATIC_DIR` | `../frontend/dist` | Directory of built frontend assets to serve |

## Development

```bash
go run ./cmd/server
```

## Build & deploy

Built as a single Docker image containing both the compiled Go binary and
the Vite-built frontend (see `Dockerfile`, build context is the project
root containing both `backend/` and `frontend/`).

Production runs on a single DigitalOcean droplet with Docker Compose:
[Caddy](https://caddyserver.com) terminates TLS for `lastfirst.app` and
proxies to the app container, whose port is not published on the host. The
droplet keeps copies of [`deploy/docker-compose.yml`](deploy/docker-compose.yml)
and [`deploy/Caddyfile`](deploy/Caddyfile) in `/opt/lastfirst`, next to an
`.env` that provides `YT_API_KEY`.

To build, push `ghcr.io/prayanalog/lastfirst:<tag>`, and roll it out on the
droplet (run from the `backend/` checkout):

```bash
DEPLOY_HOST=<droplet-ip> deploy/deploy.sh <tag>
```
