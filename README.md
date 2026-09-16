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
root containing both `backend/` and `frontend/`):

```bash
docker buildx bake -f docker-bake.hcl --push
```

Kubernetes manifests for deployment (DigitalOcean Kubernetes) are in
[`deploy/`](deploy).

Per-IP rate limiting is keyed on `X-Real-IP`, which the nginx ingress sets to
the address it accepted the connection from. That key only identifies a
visitor if the cluster around this repository preserves and protects it, so
check these before exposing the service publicly:

```bash
# Must print "Local", unless the load balancer sends PROXY protocol and the
# ConfigMap below sets use-proxy-protocol: "true". Otherwise connections are
# SNATed, the ingress sees a node address for every visitor, and all public
# users share one rate-limit bucket.
kubectl -n ingress-nginx get svc ingress-nginx-controller \
  -o jsonpath='{.spec.externalTrafficPolicy}'

# Must not set use-forwarded-headers or enable-real-ip to "true". Either makes
# the ingress derive the address from headers the client sends.
kubectl -n ingress-nginx get configmap ingress-nginx-controller -o yaml
```

Anything that reaches the Service without going through the ingress can set
`X-Real-IP` itself. Restricting that needs a `NetworkPolicy` that selects the
ingress controller's actual namespace and Pod labels, which are not defined in
this repository.
