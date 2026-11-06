#!/usr/bin/env bash
# Run on the approved Always Free VM, not on the operator's computer.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo 'Run with sudo on the Oracle VM.' >&2; exit 1; }
# shellcheck source=/dev/null
source /etc/os-release
[[ $ID == ubuntu && $VERSION_ID == 24.04 && $(uname -m) == aarch64 ]] || {
  echo 'Expected Ubuntu 24.04 on Arm64; refusing to alter this host.' >&2; exit 1;
}
# OCI network rules must already restrict SSH to the operator and HTTP(S) to
# Cloudflare ranges. Docker published ports can bypass host UFW rules.
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y docker.io docker-compose-v2 unattended-upgrades curl openssl
systemctl enable --now docker
cat > /etc/apt/apt.conf.d/20auto-upgrades <<'APT'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT
install -d -m 0700 /opt/ai-api/runtime/{state,logs,certs,caddy-data,caddy-config}
echo 'Host prepared. Install deployment files and secrets; bootstrap does not start the app.'
