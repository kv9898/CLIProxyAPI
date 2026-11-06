#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
curl --fail --silent --show-error --max-time 10 http://127.0.0.1:8317/healthz >/dev/null
openssl x509 -in runtime/certs/origin.pem -noout -checkend 2592000
[[ $(df --output=pcent runtime | tail -1 | tr -dc '0-9') -lt 80 ]] || {
  echo 'Disk usage is at or above 80%.' >&2; exit 1;
}
docker compose ps
free -m
df -h runtime
docker stats --no-stream
# No credential/config contents are printed by this check.
