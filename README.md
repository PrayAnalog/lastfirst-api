# lastfirst-api

Backend for [lastfirst.app](https://lastfirst.app) — a YouTube reverse-playlist
service. Given a playlist URL/ID, it fetches the playlist via the YouTube
Data API v3, reverses the video order, and returns "watch together" links
(`youtube.com/watch_videos`) so viewers can start from the first episode
instead of the last.

Also serves the built `lastfirst-web` frontend (React + Vite, kept in a
separate private repository) as static files.

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

Every response, API and static files alike, carries `Content-Security-Policy`,
`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` and
`Permissions-Policy` headers. The policy allows scripts only from this origin
and images only from this origin and `i.ytimg.com`, so the frontend must not
use inline scripts.

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
proxies to the app container, whose port is not published on the host.
[`deploy/docker-compose.yml`](deploy/docker-compose.yml) and
[`deploy/caddy/Caddyfile`](deploy/caddy/Caddyfile) describe that setup; `YT_API_KEY`
comes from an `.env` on the droplet.

Pushing a `v*` tag runs
[`.github/workflows/deploy.yml`](.github/workflows/deploy.yml), which builds
and pushes `ghcr.io/prayanalog/lastfirst:<tag>`, with the `main` branch of
`PrayAnalog/lastfirst-web` as the frontend, then connects to the droplet over
SSH with a key restricted to a single deploy command, which rolls the app
container to that tag:

```bash
git tag <tag> && git push origin <tag>
```

Once the deploy succeeds, the workflow publishes a GitHub Release for the tag
whose notes list the pull requests merged since the previous tag.

Use a tag that does not already exist in GHCR:
pushing an existing tag overwrites the image it names.

A tag deploy only changes the app image tag, which it writes into the
droplet's copy of the app `image` line; in `deploy/docker-compose.yml` that
line ends in `${APP_TAG}` instead, so keep the droplet's line when applying a
change. Changes to
`deploy/docker-compose.yml`, `deploy/caddy/Caddyfile` or `deploy/vector.yaml` are
applied to the droplet's copies by hand, before pushing the tag that depends
on them:
Compose stops a container with the `stop_grace_period` it was created
with, so a new grace period only covers containers created after the edit.
A tag deploy recreates only the app container, so a service added to the
compose file starts once it is brought up by hand with
`docker compose up -d <service>`. The same applies to a changed logging
driver: Docker fixes it when a container is created, so bring up
`vector` first, then recreate each service whose `logging` changed, such
as `caddy`, with `docker compose up -d <service>`.
Caddy mounts the `caddy/` directory next to the compose file at
`/etc/caddy`. On a droplet whose Caddyfile still sits next to the compose
file, move it before applying the new compose file, so the mount does not
start Caddy without a config, then recreate `caddy`:

```bash
mkdir caddy && mv Caddyfile caddy/Caddyfile
docker compose up -d caddy
```
