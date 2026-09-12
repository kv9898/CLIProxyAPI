# Scoped client keys in this fork

This fork adds account-scoped client keys to upstream CLIProxyAPI v7.2.158.
The upstream repository remains `router-for-me/CLIProxyAPI`. Merge upstream
releases into this fork, run the isolation tests, and publish a new versioned
image with the **Scoped proxy image** workflow. Deploy a pinned image digest.

## Configuration

Existing `api-keys` remain unrestricted. Put restricted keys only in
`scoped-api-keys`, as SHA-256 digest to exact auth ID mappings:

```yaml
api-keys:
  - existing-unrestricted-key
scoped-api-keys:
  "<64-character SHA-256 digest of the restricted key>": "<auth-file-name.json>"
```

Never add the restricted plaintext key to `api-keys`. Only its digest is stored
in configuration. A stock build does not recognize restricted keys and rejects
them. Removing the digest revokes the key. Renaming/removing/disabling the
assigned auth makes requests fail rather than select another account.

Authentication metadata forces the exact account ID for every execution,
including retries, SSE bootstrap retries and Responses websocket turns. This
overrides session affinity and transport pins. A model with a matching name
under another provider still cannot use that provider's credential. The model
catalog is filtered by the assigned account, preserving OpenAI and Codex shapes.

Restricted keys support these routes:

- `GET /v1/models`
- `POST /v1/responses`, `/v1/responses/compact`, `/v1/chat/completions`, `/v1/completions`
- `POST /v1/messages`, `/v1/messages/count_tokens`
- `GET /v1/responses` (Responses WebSocket)
- The `/backend-api/codex/responses` HTTP/WebSocket and compact aliases

Other authenticated routes are denied, including direct alpha search, media,
realtime and Amp endpoints. Management authentication is separate; scoped keys
never grant management access. Built-in tools within a Responses request use
the selected account. Trusted server plugins are disabled in this deployment;
re-audit the scope before enabling plugins that replace routing or execution.

## Verification

```sh
go test ./internal/access/... ./internal/config ./internal/api ./sdk/api/handlers/... ./sdk/cliproxy/auth
go build -o CLIProxyAPI ./cmd/server
```

The `TestScopedKey` tests cover all supported authentication channels, mixed
credentials, unrestricted existing keys, stock-build rejection, route allowlists,
catalog filtering, overridden session pins, missing/disabled school credentials,
quota and stream failures, and both DeepSeek providers. Test deployment with
synthetic credentials before sending production verification requests. Never
commit production client keys, auth files, or deployment secrets to this fork.

## Usage statistics

The existing usage pipeline records the incoming scoped key like other client
keys. This feature introduces no request/token cap or persistent usage database.

## Rollback

Restore the previous Render image digest. Existing keys continue working;
restricted keys fail authentication on upstream builds. The configuration may
retain the scoped digests safely. Re-run isolation checks after any upstream
merge that changes authentication, request metadata, model catalogs or routing.
