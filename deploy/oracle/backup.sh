#!/usr/bin/env bash
# Run on the operator's Linux computer, not on the VM.
set -euo pipefail
umask 077
: "${ORACLE_SSH_HOST:?Set the verified SSH host, e.g. ubuntu@203.0.113.1}"
: "${AGE_RECIPIENT:?Set your age public recipient; keep the identity off the VM}"
: "${BACKUP_DIR:?Set an absolute local backup directory}"
[[ $ORACLE_SSH_HOST =~ ^[a-zA-Z0-9][a-zA-Z0-9@._-]*$ && $BACKUP_DIR == /* ]] || exit 1
command -v age >/dev/null
mkdir -p "$BACKUP_DIR"
exec 9>"$BACKUP_DIR/.backup.lock"
flock -n 9 || { echo 'Backup already running.' >&2; exit 1; }
archive="$BACKUP_DIR/ai-api-$(date -u +%Y%m%dT%H%M%SZ).tar.gz.age"
partial="$archive.partial"
trap 'rm -f -- "$partial"' EXIT
# Quiesce the sole writer briefly. Its restart is guaranteed by the remote trap.
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes "$ORACLE_SSH_HOST" 'sudo bash -s' <<'REMOTE' | age --recipient "$AGE_RECIPIENT" --output "$partial"
set -euo pipefail
cd /opt/ai-api
running=$(docker compose ps --status running --services)
grep -qx proxy <<< "$running" || { echo 'Proxy is not running; backup aborted.' >&2; exit 1; }
trap 'docker compose start proxy >&2' EXIT
docker compose stop proxy >&2
tar -czf - .env compose.yaml Caddyfile runtime/app.env runtime/state runtime/certs runtime/caddy-data runtime/caddy-config
REMOTE
mv -- "$partial" "$archive"
# Rotate only archives created by this script, after a successful encrypted copy.
python3 - "$BACKUP_DIR" <<'PY'
from pathlib import Path
import re, sys
files=sorted(p for p in Path(sys.argv[1]).iterdir() if p.is_file() and re.fullmatch(r'ai-api-\d{8}T\d{6}Z\.tar\.gz\.age', p.name))
for p in files[:-7]: p.unlink()
PY
printf 'Encrypted backup saved: %s\n' "$archive"
