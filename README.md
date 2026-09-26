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

`deploy/deploy.sh` does not copy those two files; it only rewrites the app
image tag in the droplet's `docker-compose.yml`. Before deploying, compare them
with the droplet's copies (the app image tag line is expected to differ):

```bash
ssh root@<droplet-ip> 'cat /opt/lastfirst/Caddyfile' | diff deploy/Caddyfile -
ssh root@<droplet-ip> 'cat /opt/lastfirst/docker-compose.yml' | diff deploy/docker-compose.yml -
```

If the Caddyfile differs, validate it locally, stage it on the droplet, then
write it into the live file and reload Caddy, restoring the previous contents
if the reload fails. The live file is bind-mounted as a single file, so it
must be written into rather than replaced, or the container keeps reading the
old one:

```bash
docker run --rm -v "$PWD/deploy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2 caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
ssh root@<droplet-ip> 'cat > /opt/lastfirst/Caddyfile.new' < deploy/Caddyfile
ssh root@<droplet-ip> 'cd /opt/lastfirst && cp Caddyfile Caddyfile.bak && cat Caddyfile.new > Caddyfile && { docker compose exec -T -w /etc/caddy caddy caddy reload || { cat Caddyfile.bak > Caddyfile; false; }; }'
```

If `docker-compose.yml` differs, apply the change to the droplet's copy by
hand, keeping its current app image tag, and run
`docker compose up -d` in `/opt/lastfirst`.

To build, push `ghcr.io/prayanalog/lastfirst:<tag>`, and roll it out on the
droplet (run from the `backend/` checkout):

```bash
DEPLOY_HOST=<droplet-ip> deploy/deploy.sh <tag>
```

This pulls and recreates only the `app` service, after checking that the
droplet's compose file now names that image; Caddy is left running.

Pushing a `v*` tag runs
[`.github/workflows/deploy.yml`](.github/workflows/deploy.yml), which builds
and pushes the same image, with the tag as the image tag and the `main`
branch of `PrayAnalog/lastfirst-web` as the frontend, then connects to the
droplet as `deploy` and sends only the tag:

```bash
git tag <tag> && git push origin <tag>
```

Use a tag that does not already exist in GHCR:
pushing an existing tag overwrites the image it names.

On the droplet, the `deploy` user's `authorized_keys` entry for the workflow
key is restricted to one command, which receives the tag and performs the
rollout as root through a sudo rule for that script only:

```
restrict,command="sudo /usr/local/bin/lastfirst-deploy \"$SSH_ORIGINAL_COMMAND\"" ssh-ed25519 AAAA... github-actions-deploy
```

The forced command has no terminal to type a password into, so the sudo
rule in `/etc/sudoers.d/lastfirst-deploy` must be passwordless:

```
deploy ALL=(root) NOPASSWD: /usr/local/bin/lastfirst-deploy
```

The workflow needs these repository secrets:

- `WEB_REPO_TOKEN`: a token with read access to `PrayAnalog/lastfirst-web`
- `DEPLOY_HOST`: the droplet IP
- `DEPLOY_SSH_KEY`: the private key of that `authorized_keys` entry
- `DEPLOY_KNOWN_HOSTS`: the droplet's host keys

`ssh-keyscan` does not verify the keys it collects. Compare its fingerprint
with the one read on the droplet itself, for example from the DigitalOcean
console, before storing it:

```bash
ssh-keyscan -t ed25519 <droplet-ip> | ssh-keygen -lf -
```

```bash
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

The `lastfirst` package on GHCR must grant this repository write access
under *Manage Actions access*.
