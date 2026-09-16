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

Requests are limited to an 8 KiB JSON body and playlists of at most 2,000
items. The service processes at most eight playlist requests concurrently
and gives each request two minutes for YouTube API work. Capacity and rate
limit responses include `Retry-After`.

## Configuration

| Env var | Default | Description |
|---|---|---|
| `YT_API_KEY` | — | YouTube Data API v3 key |
| `ADDR` | `:8080` | HTTP listen address |
| `STATIC_DIR` | `../frontend/dist` | Directory of built frontend assets to serve |
| `TRUST_PROXY_HEADERS` | `false` | Trust ingress-controlled `X-Real-IP` for rate limiting; enable only when direct access to the app is blocked |

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

The supplied Deployment removes a terminating pod from routing before a
5-second `preStop` delay, allows up to 135 seconds for connection draining, and
keeps another 10 seconds of Kubernetes termination headroom.

## Production security and capacity

The checked-in deployment runs one replica with a non-overlapping `Recreate`
rollout because the daily YouTube quota budget and per-IP rate limiter are
process-local. A restart resets that budget, and overlapping or additional pods
multiply it. This trades brief rollout unavailability for strict limits. Move
both controls to shared durable storage before enabling zero-downtime rolling
updates or scaling horizontally.

Before a public launch:

- Restrict `YT_API_KEY` in Google Cloud to the YouTube Data API and to the
  production server's egress address. Keep it only in the Kubernetes Secret,
  enable repository secret scanning, and rotate it if it has ever been exposed.
- Put an edge DDoS/WAF service in front of ingress and alert on request rate,
  429/503/504 responses, pod CPU/memory, restarts, and YouTube quota usage.
  Application and ingress limits protect capacity but cannot absorb a
  volumetric network attack.
- Publish images with immutable digests and deploy by digest rather than by the
  mutable `v1` tag currently shown in the manifest. Scan the final image and
  dependencies in CI.
- Keep `TRUST_PROXY_HEADERS=false` for direct/local deployments. The supplied
  Kubernetes manifest enables it because the ClusterIP service is reached
  through ingress, which owns `X-Real-IP`; do not expose the pod or service
  directly while that setting is enabled. The supplied NetworkPolicy assumes
  the controller namespace is `ingress-nginx` and uses the standard controller
  labels; adjust those selectors to the installed controller before applying
  the manifests if your cluster differs.

Report vulnerabilities through GitHub's private vulnerability reporting flow;
see [`SECURITY.md`](SECURITY.md).
