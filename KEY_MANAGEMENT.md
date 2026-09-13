# Managed client keys

This fork supports the matching `kv9898/Cli-Proxy-API-Management-Center`
management UI. The sidebar's **Key management** page and the configuration
page list legacy unrestricted keys, the legacy school key, and managed keys.

## Access

- Existing keys keep their permissions until explicitly edited.
- Restricted keys select exact runtime credential IDs. Provider API-key
  profiles (including separate OpenAI-compatible profiles) appear alongside
  OAuth auth files. Selecting one never selects the other profiles implicitly.
- An empty selection denies all models. Missing, disabled, or replaced
  credentials never cause fallback to an unselected credential.
- Restricted keys retain the text/Codex endpoint allowlist; media, realtime,
  Amp, and remote Home dispatch are not supported for restricted keys.
- Unrestricted keys may use all current and future credentials.
- Changes apply after configuration reload. Reconnect already-open WebSocket
  sessions to pick up changed permissions; in-flight work is not cancelled.

Managed keys use `client-keys`, indexed by SHA-256 of the client secret:

```yaml
client-keys:
  <64-character-sha256>:
    name: Friend
    all: false
    auth-ids:
      - school-auth.json
```

Editing a legacy key migrates its secret representation. Its original secret remains valid,
but only its hash is persisted afterward. Other legacy keys retain their secrets
and permissions; their display names are preserved when list indexes shift.
New secrets are generated in the UI,
must be copied before saving, and cannot be retrieved. The last key cannot be
deleted through this API, because empty legacy authentication means public access.

## Management API

All routes require the existing management credential, not a client API key.

- `GET /v0/management/client-keys`: unified `keys`, selectable `resources`,
  `recording`, and `usage` (`since`, `total_tokens`, and per-key `keys` totals).
- `POST /v0/management/client-keys`: `{key, name, all, auth_ids}`.
- `PUT /v0/management/client-keys/:sha256`: `{name, all, auth_ids}`.
- `DELETE /v0/management/client-keys/:sha256`: revoke a key.

The legacy `/api-keys` API still addresses only legacy unrestricted keys.
Never put a restricted secret into the legacy array on an older server.

## Token statistics

Counters are process-local, mutex-protected, and reset on restart/redeploy.
They do not expire when the 60-second usage queue expires and do not depend
on the browser staying open. Recording follows `usage-statistics-enabled`.
Totals use normalized provider-reported total tokens, without adding cached
or reasoning subsets a second time. Missing usage cannot be reconstructed.
Reported tokens from failed upstream attempts are included when available.

Percentage = this key's recorded tokens / all recorded client-key tokens.
Deleted keys' prior usage remains in the denominator until restart. This is
not subscription consumption or remaining allowance. There are no usage
limits, persistent statistics, or Codex `/status` budget integration.

## Build and rollout

1. Run backend tests and build; build the matching UI using `bun run verify`.
2. Publish the UI fork's single-file `dist/index.html` as `management.html`
   using its existing release workflow. Build the API image with the maintained
   `Scoped proxy image` workflow (default version `7.2.158-keys.1`).
3. When deployment is approved, update the saved configuration's
   `remote-management.panel-github-repository` to
   `https://github.com/kv9898/Cli-Proxy-API-Management-Center`, deploy the API
   image, and verify the portal uses the matching UI release.
4. Check both unrestricted keys' model lists and the school's model list;
   confirm the school key still denies Pro-only and both DeepSeek profiles.
   Make one small school request and verify its counter increments.

Keep API and UI releases paired. Do not auto-update from the upstream UI repo.
To roll back after keys have been edited, restore the prior key configuration
and the prior API/UI versions together. Older API versions do not understand
`client-keys` and could otherwise disable authentication if no legacy keys remain.

## Verification

Regression tests cover auth channels, restricted-key precedence, multi-ID
eligibility, model catalog union/empty selection, retries, missing/disabled
school auth, DeepSeek denial, HTTP/stream/WebSocket paths, concurrent token
counting, migration and deletion across YAML reload, and save failure rollback.
UI tests cover percentages, translations, and safe unified-list integration.
Browser verification uses synthetic local data, not production credentials.
