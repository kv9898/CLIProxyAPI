#!/usr/bin/env bash
set -euo pipefail
: "${1:?Pass the immutable paired image reference}"
image=$1
here=$(cd "$(dirname "$0")" && pwd)
name="ai-api-smoke-$$"
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT
docker run -d --name "$name" -p 127.0.0.1::8317 \
  -v "$here/synthetic.yaml:/run/synthetic.yaml:ro" \
  "$image" --config /run/synthetic.yaml >/dev/null
address=$(docker port "$name" 8317/tcp)
url="http://$address"
ready=false
for attempt in {1..30}; do
  if curl -fsS --max-time 5 "$url/healthz" >/dev/null 2>&1; then ready=true; break; fi
  sleep 2
done
[[ $ready == true ]] || { echo 'Synthetic server failed readiness.' >&2; exit 1; }
[[ $(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$url/v1/models") == 401 ]]
curl -fsS --max-time 10 -H 'Authorization: Bearer synthetic-only-unrestricted-test-key' "$url/v1/models" >/dev/null
[[ $(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$url/v0/management/client-keys") == 401 ]]
curl -fsS --max-time 10 -H 'Authorization: Bearer synthetic-only-management-test-key' "$url/v0/management/client-keys" >/dev/null
curl -fsS --max-time 10 "$url/management.html" >/dev/null
docker restart "$name" >/dev/null
for attempt in {1..30}; do
  if curl -fsS --max-time 5 "$url/healthz" >/dev/null 2>&1; then
    echo 'Paired image passed synthetic HTTP, management, panel and restart checks.'
    exit 0
  fi
  sleep 2
done
exit 1
