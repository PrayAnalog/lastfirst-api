#!/usr/bin/env bash
set -euo pipefail

TAG=${1:?usage: DEPLOY_HOST=<droplet-ip> deploy/deploy.sh <tag>}
HOST=${DEPLOY_HOST:?set DEPLOY_HOST to the droplet IP}
IMAGE=ghcr.io/prayanalog/lastfirst:$TAG

repo=$(cd "$(dirname "$0")/.." && pwd)
if [ "$(basename "$repo")" != backend ]; then
  echo "deploy.sh builds backend/ next to frontend/; run it from the backend/ checkout, not $repo" >&2
  exit 1
fi
cd "$repo/.."

docker buildx build --platform linux/amd64 -f backend/Dockerfile -t "$IMAGE" --push .

ssh "root@$HOST" "cd /opt/lastfirst && sed -i 's#\(prayanalog/lastfirst:\).*#\1$TAG#' docker-compose.yml && docker compose pull && docker compose up -d"
