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

Request bodies are capped at 8 KiB and must contain exactly one JSON value;
anything else is answered with `400`. A request is given two minutes to
finish its YouTube calls.

`GET /healthz`

Returns an empty `200` as long as the process is serving. Used by the
Kubernetes readiness and liveness probes.

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
root containing both `backend/` and `frontend/`):

```bash
docker buildx bake -f docker-bake.hcl --push
```

Kubernetes manifests for deployment (DigitalOcean Kubernetes) are in
[`deploy/`](deploy).

On `SIGTERM` the server stops accepting connections and finishes the requests
already in flight before exiting. The `preStop` delay and
`terminationGracePeriodSeconds` in [`deploy/deployment.yaml`](deploy/deployment.yaml)
are sized for that drain; changing the request timeout in
`internal/httpapi` means resizing both.

Rate limiting is keyed on `X-Real-IP`, which assumes the ingress sets it and
that nothing reaches the Service without passing through the ingress.
