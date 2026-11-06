#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
[[ $EUID -eq 0 ]] || { echo 'Run with sudo on the Oracle VM.' >&2; exit 1; }
for file in .env runtime/app.env runtime/certs/origin.pem runtime/certs/origin.key; do
  [[ -s $file ]] || { echo "Missing required file: $file" >&2; exit 1; }
done
# Environment files must contain literal KEY=value entries, never shell code.
for name in CLI_PROXY_IMAGE CADDY_IMAGE; do
  value=$(sed -n "s/^${name}=//p" .env)
  [[ $value =~ ^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$ ]] || {
    echo "Set $name to an immutable image digest." >&2; exit 1;
  }
done
if grep -q 'REPLACE_WITH' runtime/app.env; then
  echo 'Complete app.env before starting.' >&2; exit 1
fi
for file in .env runtime/app.env runtime/certs/origin.key; do
  [[ $(stat -c '%a' "$file") == 600 && $(stat -c '%u' "$file") == 0 ]] || {
    echo "Require root ownership and mode 0600: $file" >&2; exit 1;
  }
done
openssl x509 -in runtime/certs/origin.pem -noout -checkhost ai.rubuky.com
openssl x509 -in runtime/certs/origin.pem -noout -checkend 2592000
cert_key=$(openssl x509 -in runtime/certs/origin.pem -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum)
private_key=$(openssl pkey -in runtime/certs/origin.key -pubout -outform DER | sha256sum)
[[ $cert_key == "$private_key" ]] || { echo 'Certificate/key mismatch.' >&2; exit 1; }
docker compose config --quiet
docker compose pull
docker compose run --rm --no-deps caddy validate --config /etc/caddy/Caddyfile
docker compose up -d --wait --wait-timeout 120
# Do not set timeouts on model inference streams; this is a readiness probe only.
for attempt in {1..30}; do
  if curl --fail --silent --max-time 5 http://127.0.0.1:8317/healthz >/dev/null; then
    echo 'Proxy health endpoint ready. Run authentication and isolation acceptance checks.'
    exit 0
  fi
  sleep 2
done
echo 'Proxy readiness failed; inspect logs locally without publishing secrets.' >&2
exit 1
