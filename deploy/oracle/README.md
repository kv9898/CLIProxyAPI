# AI-API on Oracle Always Free

Target: `ai.rubuky.com`, Cloudflare proxied, Oracle `uk-london-1`, Ubuntu 24.04
Arm, A1 Flex 1 OCPU / 2 GB, 50 GB Balanced boot volume. Never provision a paid
fallback or rely on trial credits. Verify home region, existing aggregate usage
and Always Free eligibility before creation. An image being marked Free only
means its OS license is free, not that the selected VM/storage is free.

## Release

Run **Scoped proxy image** on the reviewed commit with version `8.0.58-keys.1`.
Native amd64 and Arm runners run the isolation tests, build the pinned management
UI, publish architecture-specific API/UI images, and exercise the paired server
with synthetic credentials. The final job publishes multi-platform manifests.
Use the paired digest from the workflow summary, not the `-api` image. Verify its
manifest contains `linux/arm64`. Record the source/UI commits and both image
digests. Existing rollback tags are retained and never rebuilt by this workflow.

## Network and host

Use a dedicated VCN/public subnet. Before assigning a public address, replace any
broad default SSH ingress with TCP 22 from the operator's current public IPv4 /32.
Permit TCP 80 and 443 only from Cloudflare's published IPv4 ranges; retain required
ICMP and outbound connectivity. No public IPv6 is needed. Publish no other ports.
Do not rely on UFW alone: Docker's published ports can bypass its rules. Confirm
both OCI security lists and network security groups have no broader ingress.
Keep the current SSH session open while testing a second session.

Verify the host SSH fingerprint through the Oracle console before accepting it.
Run `sudo bash bootstrap.sh` on this VM only. It installs distro Docker/Compose
and unattended security upgrades; it does not start the service. Check pending
reboots during maintenance; automatic reboot is not enabled.

Copy this directory to `/opt/ai-api`. Use root ownership and mode 0700 for
`runtime/`, its subdirectories and `/opt/ai-api` itself. Copy `.env.example` to
`.env`, and `app.env.example` to `runtime/app.env`; set both to root-owned mode
0600. Use literal environment values, not shell commands. Pin the proxy and the
official Caddy image to verified Arm-capable digests. Do not print `docker compose
config` without `--quiet`, since its expanded output includes secrets.

The proxy's host port is loopback-only; Caddy talks to it over the Compose network.
The configuration's server host must be `0.0.0.0` and port 8317. Private callback
ports are deliberately not published. If a provider requires a local callback,
add a temporary loopback-only port mapping and use `ssh -L`, then remove it.

## TLS and Cloudflare

Create a Cloudflare Origin CA certificate for **only** `ai.rubuky.com`; generate
the private key and CSR on the VM so the private key never leaves it. Store the
certificate as `runtime/certs/origin.pem` and its root-owned mode-0600 key as
`runtime/certs/origin.key`. Record expiry and rotate before the 30-day alert.

Preserve the current Cloudflare DNS record and rule settings in the protected
migration record. Enforce Full (strict) for this hostname, using a hostname-scoped
rule if the zone setting differs. Do not change other applications. Add a cache
bypass rule for `http.host eq "ai.rubuky.com"`, retain WebSockets, and redirect
HTTP to HTTPS at the edge. Avoid browser challenges on API endpoints. Do not
disable existing security controls without reviewing the specific rule involved.

Before DNS cutover, test the origin via an SSH tunnel with the Cloudflare Origin
CA root supplied to `curl --cacert`; use `--resolve` for hostname/SNI. Do not use
`curl -k`. Origin CA certificates are trusted by Cloudflare, not ordinary browsers;
keep Cloudflare proxying enabled. Caddy flushes streaming responses immediately
and does not impose application response deadlines.

## Migration and acceptance

1. Confirm Render's active `GITSTORE_GIT_URL`, branch, management secret source,
   panel settings and any provider secrets outside Git. Check for Postgres/Object
   store variables that override Git storage. Reading repository file names alone
   does not establish what the running service uses.
2. Save Render's exact current image digest, Git store commit, DNS/rule settings
   and encrypted config/auth backup off the VM. Do not put secrets in this repo.
3. Run `bash smoke-image.sh <paired-digest>` using synthetic credentials. For TLS
   staging, run Caddy against that loopback-published synthetic server using a
   temporary staging configuration. Never point a staging server at live Git auth.
4. Suspend Render and confirm its background process is stopped. Take a final Git
   snapshot after it has stopped; only one service may write/refresh live auth.
5. Install a fine-grained token scoped to `kv9898/AI-API` with repository contents
   read/write. Set the verified Git branch in `runtime/app.env`. Reuse the existing
   management key; keep its source/override semantics unchanged.
6. Load the backed-up config with v8 and preserve client hashes, policies and auth
   IDs exactly. Disable automatic panel replacement (`management.disable-auto-update-panel`
   in v8; the legacy name is `remote-management.disable-auto-update-panel`). Keep
   plugins disabled as required by the existing isolation documentation. Any save
   must preserve both custom key maps. Do not silently create a default/public config
   when the Git store is missing; investigate the missing file before starting.
7. Run `sudo ./start.sh`. It rejects mutable image tags, placeholders, unsafe secret
   permissions, expiring/mismatched certificates and an invalid Compose definition.
   Validate origin health, authentication, catalogs, provider requests, streaming,
   WebSockets and management via the verified TLS tunnel.
8. Replace only the `ai` record with a proxied A record for the VM. Remove only
   conflicting `ai` origin records (save their originals first). Leave other records
   and the Render custom-domain binding intact for rollback.
9. Repeat the following checks through `https://ai.rubuky.com`: no key gets 401;
   unrestricted keys retain access; restricted/empty-selection keys never use
   unselected credentials, forbidden endpoints or management; model catalogs are
   filtered; missing/disabled credentials fail closed; HTTP, SSE and WebSocket
   provider switching retains isolation. Verify UI editing and token counters.
10. Verify a benign configuration write and OAuth refresh reach GitHub. Restart the
    containers, then reboot the VM and repeat the checks. Never deliberately force
    a production refresh failure; use synthetic tests for failure cases.

Keep Render suspended for seven days of successful observation, then remove it
only after checking backups and rollback readiness. Its 5 GB bandwidth history
makes it an emergency rollback, not the long-term destination. Token counters are
process-local and reset on restart; migrating hosts does not make them persistent.

## Monitoring and off-VM backups

Install `systemd/ai-api-check.*` in `/etc/systemd/system`, then enable
`ai-api-check.timer`. `check.sh` checks local health, certificate lifetime, disk,
container state and host memory. Review failures with `systemctl --failed` and
`journalctl -u ai-api-check.service`; inspect container/Git push errors locally,
redacting credentials before sharing any log. Also check external health and OCI
bandwidth/usage daily during the first week. Do not automate restarts on a failed
inference request, as that would interrupt active streams.

On the operator computer, install `age` from its official package source, create
an age identity in a protected file, and retain a recovery copy separately. Never
copy the private identity to Oracle. Install `backup.sh` at
`~/.local/lib/ai-api/backup.sh`. Set `ORACLE_SSH_HOST`, `AGE_RECIPIENT` (public key)
and an absolute `BACKUP_DIR` in mode-0600 `~/.config/ai-api/backup.env`. Preverify
SSH host keys. The Ubuntu operator must be able to run the fixed backup command
with noninteractive sudo.

Install `systemd/ai-api-backup.*` in `~/.config/systemd/user`, and enable
`ai-api-backup.timer` with `systemctl --user enable --now`. The local computer must
be online; missed runs execute when its user timer next runs. Each backup briefly
stops the sole proxy writer, streams the tar archive over SSH directly into age
encryption locally, resumes the proxy with a trap, and retains seven successful
archives. Test decryption and tar listing locally before accepting the backup.
No plaintext archive is stored. Certificate files, the Git working state and
host secrets are covered; rotated application logs are excluded.

## Rollback and recovery

Stop Oracle first. Restore the previous config **layout and permissions**, keeping
the latest valid auth files; do not blindly reset the whole Git repo and restore
stale refresh tokens. Publish the compatible config to the store, restore the saved
DNS record/rules, then resume Render at its saved digest. Revalidate authentication
and Git persistence. Never run the two credential writers concurrently.

To rebuild after loss/reclamation, provision the same free shape, restore the host
files from an encrypted backup with the proxy stopped, reconcile the newest Git
credential state, replace the Git token/certificate if compromised, and start the
pinned image. Update only `ai` to the new address after validation. Restore files
as root without exposing them to other users. If free capacity is unavailable,
stop and report it; do not upgrade or generate artificial load to avoid reclamation.
