#!/usr/bin/env bash
set -euo pipefail

# Use only loopback networking and a private, disposable database.
image=${1:?image reference required}
container=$(docker run -d --network none --read-only --cap-drop ALL \
  --tmpfs /tmp:mode=1777 --tmpfs /data:uid=10001,gid=10001,mode=0700 \
  -e JWT_SECRET=finarch-isolated-image-smoke-test-only \
  -e FINARCH_ATTACHMENTS_DIR=/data/attachments "$image")
trap 'docker rm -fv "$container" >/dev/null' EXIT
base=http://127.0.0.1:8080

for attempt in {1..30}; do
  if docker exec "$container" wget -qO /dev/null "$base/api/v1/health"; then
    break
  fi
  sleep 1
done
docker exec "$container" wget -qO /dev/null "$base/api/v1/health"
html=$(docker exec "$container" wget -qO- "$base/")
asset=$(sed -nE 's/.*<script[^>]+src="(\/assets\/[^"]+\.js)".*/\1/p' <<< "$html" | head -1)
test -n "$asset"
docker exec "$container" wget -qO /dev/null "$base$asset"
worker=$(docker exec "$container" wget -qO- "$base/sw.js")
[[ "$worker" == *"${asset#/}"* ]]
