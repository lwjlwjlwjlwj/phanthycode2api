# 02 · Usage Semantics — `https://code.phanthy.com/`

Milestone: **M2 / T2.2–T2.4**, extended in **M6b** · Author: reverse-engineering analyst (M2), M6b
documentation-integration engineer · Language: English
Companion document: `01-endpoint-catalog.md` (endpoint IDs `EP-01` … `EP-17` used throughout).

Grade vocabulary (identical to the catalog): `VERIFIED` = observed live in M1 with an evidence anchor;
`PRIOR-ART` = derived from the in-repo Go implementation / README only; `INFERRED` = reasoned from metadata
or convention, basis stated; `BLOCKED-CRED` = needs the user's own token, nothing fabricated.
M6 adds three finer grades used only for M6-derived statements: `VERIFIED-LIVE` (observed live in the M6
anonymous window), `VERIFIED-STATIC` (exact string inside the official npm CLI bundle, never executed) and
`PUBLIC-MARKETING` (public web page, not an API contract). See `01-endpoint-catalog.md` §1.4.
**M2 issued no network requests**; **M6b issued no network requests**. Every live statement is inherited from the M1 capture window
`2026-10-26T05:58:24+08:00 → 06:14:37+08:00` (`M1-index.md:15`) or the M6 window
`2026-10-26T00:20:25Z → 00:20:33Z` (`M6-new-endpoint-probes.txt:4`).

---

## 1. End-to-end authentication flow (PKCE)

### 1.1 Shape of the flow

```
(local)  verifier ──SHA-256/base64url──► challenge
   │
   ├─2─ GET  /oauth/authorize?client_id&response_type=code&redirect_uri&scope&code_challenge&code_challenge_method=S256&state
   │        ↳ 400 unsupported_response_type if response_type is missing   [VERIFIED]
   ├─3─ browser login + consent                                           [BLOCKED-CRED]
   ├─4─ 302 Location: https://code.phanthy.com/oauth/code/success?code=<code>&state=<state>  [INFERRED]
   ├─5─ POST /oauth/token  (grant_type=authorization_code, code, code_verifier, redirect_uri, client_id)
   ├─6─ 200 {access_token, refresh_token, expires_in}                     [PRIOR-ART shape]
   ├─7─ (optional) POST /api/oauth/phanthy_cli/create_api_key → 404 → fall back to access_token
   ├─8─ later: POST /oauth/token (grant_type=refresh_token, refresh_token, client_id)
   └─9─ use: POST /v1/messages with x-api-key OR Authorization: Bearer
```

### 1.2 Step-by-step messages

| # | actor | HTTP message | grade | anchor |
|---|---|---|---|---|
| 1 | client | generate 48 random bytes, base64url-encode **without padding** = `code_verifier`; `code_challenge = base64url(SHA-256(verifier))` | PRIOR-ART | `cmd/login/main.go:51-56` |
| 2 | client → server | `GET https://code.phanthy.com/oauth/authorize?client_id=phanthy-code-cli&response_type=code&redirect_uri=https%3A%2F%2Fcode.phanthy.com%2Foauth%2Fcode%2Fsuccess&scope=user%3Ainference%20user%3Aprofile%20user%3Asessions%3Aclaude_code&code_challenge=<code_challenge>&code_challenge_method=S256&state=<state>` | route + `response_type` requirement VERIFIED; parameter set PRIOR-ART | `cmd/login/main.go:144-154`; `M1-anon-probe-matrix.txt:60, 112` |
| 2a | server → client | if `response_type` absent: `400 application/json`, 101 B, `{"error":{"code":"unsupported_response_type","message":"Only authorization code flow is supported."}}` | VERIFIED | `M1-anon-probe-matrix.txt:60, 112` |
| 3 | user ↔ browser | interactive login + consent on the PhanthyCode site | `BLOCKED-CRED` | — |
| 4 | server → browser | `302` to `redirect_uri` with `code` and (returned) `state` | INFERRED (RFC 6749 §4.1.2 + the target `redirect_uri` naming) | `cmd/login/main.go:69-71, 148` |
| 5 | client → server | `POST https://code.phanthy.com/oauth/token`, `Content-Type: application/x-www-form-urlencoded`, body `grant_type=authorization_code&code=<code>&code_verifier=<code_verifier>&redirect_uri=https%3A%2F%2Fcode.phanthy.com%2Foauth%2Fcode%2Fsuccess&client_id=phanthy-code-cli`, headers `Accept: application/json` and `anthropic-beta: oauth-2025-04-20` | PRIOR-ART (POST route VERIFIED to exist) | `cmd/login/main.go:177-193`; `M1-options-matrix.txt:16` |
| 6 | server → client | `200 application/json` `{ "access_token": "<token>", "refresh_token": "<token>", "expires_in": <seconds> }` (+ fields not referenced by prior art) | PRIOR-ART shape; actual body `BLOCKED-CRED` | `cmd/login/main.go:171-175, 208-218` |
| 7 | client → server | `POST https://code.phanthy.com/api/oauth/phanthy_cli/create_api_key` with `Authorization: Bearer <access_token>`, `anthropic-beta: oauth-2025-04-20`, `Content-Type: application/json`, `Accept: application/json` | PRIOR-ART; the path returns 404 to GET and OPTIONS (VERIFIED), POST-404 asserted by prior art | `cmd/login/main.go:221-249`; `internal/upstream/client.go:262-308`; `M1-anon-probe-matrix.txt:67` |
| 7a | server → client | `404` (HTML shell) → client treats as "no API key", continues | VERIFIED for GET/OPTIONS; POST path asserted | `M1-anon-probe-matrix.txt:67`; `cmd/login/main.go:97, 245-249` |
| 8 | client → server | `POST /oauth/token` with `grant_type=refresh_token&refresh_token=<refresh_token>&client_id=phanthy-code-cli` + `anthropic-beta: oauth-2025-04-20` | PRIOR-ART | `internal/upstream/client.go:238-245` |
| 9 | client → server | `POST /v1/messages` with `x-api-key: <api_key>` if one exists, else `Authorization: Bearer <access_token>` | PRIOR-ART (scheme itself `BLOCKED-CRED`) | `internal/upstream/client.go:344-350` |

### 1.3 Exact header set used by prior art, and what a direct client should copy

| header | where it is used | value | grade |
|---|---|---|---|
| `Content-Type: application/x-www-form-urlencoded` | `/oauth/token` | fixed | PRIOR-ART `cmd/login/main.go:191` |
| `Accept: application/json` | `/oauth/token`, `create_api_key`, profile | fixed | PRIOR-ART `cmd/login/main.go:192`; `internal/upstream/client.go:184, 276` |
| `anthropic-beta: oauth-2025-04-20` | `/oauth/token` (both grants), `create_api_key` | constant `oauth-2025-04-20` | PRIOR-ART `cmd/login/main.go:193`; `internal/upstream/client.go:139, 243, 274` |
| `Authorization: Bearer <token>` | `/api/oauth/*`, `create_api_key`, fallback on `/v1/messages`; the **only** scheme the official CLI sends on `/v1/messages` (§1.6) | token | PRIOR-ART; guard VERIFIED on `/api/oauth/*`; CLI intent VERIFIED-STATIC |
| `x-api-key: <api_key>` | prior art only; **not used by the official CLI**, which deletes it | key | PRIOR-ART `internal/upstream/client.go:345` — unverified possibility (see §1.6) |
| `x-app: cli` | `/v1/messages` | literal `cli` | PRIOR-ART `internal/upstream/client.go:342` |
| `X-Claude-Code-Session-Id` | `/v1/messages` | local account UID | PRIOR-ART `internal/upstream/client.go:351-353` — a **proxy-side artifact**, not required by the protocol; omit for a clean client |
| `anthropic-version` | **never sent anywhere** | — | PRIOR-ART by absence (repo-wide grep); see §2.4 |
| `User-Agent` | upstream calls | `phanthycode2api/1.0` | PRIOR-ART `internal/upstream/client.go:185, 343` |

### 1.4 Where the `anthropic-beta` header matters

The OAuth endpoints are reached only with `anthropic-beta: oauth-2025-04-20` in the prior art. No capture
demonstrates that the header is **required**, because no POST to `/oauth/token` was ever made. Two readings are
possible and neither is provable without a token: (a) the target's OAuth implementation is a fork of Anthropic's
beta OAuth and rejects requests without the header; (b) the header is inert. Recommendation for M4/M5 tooling:
**send it** on `/oauth/token` and `/api/oauth/*` calls; it is cheap and matches the only known working client.
Grade for "required": INFERRED, unverified.

### 1.5 Verifier and state hygiene (deviation from prior art)

Prior art writes the verifier to `.login-verifier` with mode `0600` (`cmd/login/main.go:34, 63-66`) and deletes it
on success (`:131`). For this project's tooling, keep the verifier in process memory only — do not add a new
on-disk secret, per `00-governance.md:36-40` (L2 must never be persisted). Also note that prior art uses a
**static** `state=p2a-login` (`cmd/login/main.go:152`): it provides no CSRF binding. Use a per-flow random
`state` and verify it on return; the target's own validation of `state` is unverified.

---

## 1A. Official CLI: install, login, credentials, headers (M6)

This section describes the **official** first-party client
`@phanthy/phanthy-codeo@0.2.11` (npm, published `2026-10-21T08:56:55.362Z`). It was obtained as a tarball
(sha256 `3134d33d4eba31ec0af543a8afa061d3b21c4df7259d49decc11928e2244d80c`) and inspected **statically only**:
`tar -xzf` into `/tmp`, then byte slicing. Nothing was imported, installed, transpiled or executed
(`M6-npm-package.json:83-93`; `M6-cli-config.txt:6-7`).

### 1A.1 Install and login (VERIFIED-PUBLIC via `/cli`)

```bash
npm install -g @phanthy/phanthy-codeo
phanthy-codeo auth login
```

Installer scripts published by the operator: `https://code.phanthy.com/install.sh` and
`https://code.phanthy.com/install.ps1` (both HTTP 200, hashes and verbatim command extraction in
`evidence/M6-cli-install.txt`). Package metadata: `bin.phanthy-codeo`, `engines.bun >=1.3.14`,
`type: module`, MIT, repository `github.com/yizhixu/phanthy-codeo` (which is **not** the untrusted
`github.com/witt3rd/oh-my-hermes` repository) (`M6-npm-package.json:13-37`).

### 1A.2 Provider configuration (VERIFIED-STATIC)

Extracted verbatim from `dist/tui-worker.js` at byte offset **7928736** (file sha256
`c38ca895230b827838cfebb19b2b9becdf8523379118868a0f9461956f9b0c6d`); the identical block is duplicated in
`dist/cli.js` (`M6-cli-config.txt:31-99`).

| element | value |
|---|---|
| base URL | `https://code.phanthy.com` (no trailing slash; call sites concatenate `${base}/…`) |
| client id | `phanthy-code-cli` |
| provider name / ID | `phanthy` |
| `authorize` | `https://code.phanthy.com/oauth/authorize` |
| `token` | `https://code.phanthy.com/oauth/token` |
| `profile` | `https://code.phanthy.com/api/oauth/profile` |
| `models` | `https://code.phanthy.com/api/phanthy_code/models` |
| `modelStats` | `https://code.phanthy.com/api/phanthy_code/models/stats` |
| `messages` | `https://code.phanthy.com/v1/messages` (map entry — **never dereferenced**, see below) |
| scopes requested (6) | `org:create_api_key`, `user:profile`, `user:inference`, `user:sessions:claude_code`, `user:mcp_servers`, `user:file_upload` |

The client id `phanthy-code-cli` therefore matches the prior art's value and is now confirmed on the official
client as well. The scope list is **6**, while the server's discovery document advertises **5** — the extra one,
`org:create_api_key`, is not advertised server-side (see `01-endpoint-catalog.md` §5.2).

### 1A.3 Credential storage layout (VERIFIED-STATIC)

```
default dir   ~/.phanthy-code/                       (override: $PHANTHY_CONFIG_DIR)
credential    ~/.phanthy-code/.credentials.json      (path.join(dir, ".credentials.json"))
json key      "claudeAiOauth"                        (OAuth payload; if absent the file is treated as non-OAuth)
secondary cfg .config.json  →  else .phanthy-code.json
macOS         `security` CLI keychain entry, service `phanthy code-credentials[-<sha256(dir)[:8]>]`
identity      $USER ?? os.userInfo().username ?? "claude-code-user"
```

The layout is Claude-Code-shaped (path, JSON key, `claudeAiOauth`). **No credential value exists in the package
and none was read** (`M6-cli-config.txt:102-120, 206-208`). When documenting onboarding, describe only this file
layout and use `<access_token>` placeholders — never paste a real `.credentials.json`.

The endpoint map contains `messages: https://code.phanthy.com/v1/messages`, but `nFA.messages` is **never
dereferenced** (0 occurrences). The real message path is an Anthropic-SDK-compatible provider with
`apiKey:"phanthy-oauth"` and `baseURL: ${base}/v1`; because the bundled SDK posts to the absolute path
`/v1/messages`, the effective URL may be `/v1/messages` **or** `/v1/v1/messages`. M1 proved `/v1/messages`
exists; this is recorded as **UNVERIFIED** (`M6-cli-config.txt:145-157`; `M6-index.md` §3.8).

### 1A.4 Request-header family (VERIFIED-STATIC — exact casing matters)

| header (exact) | map field | purpose / value seen |
|---|---|---|
| `x-phanthy-client` | `client` | client discriminator |
| `X-Phanthy-Client-Source` | `clientSource` | origin marker; `cli` on the message path, `phanthy-codeo` on telemetry |
| `x-phanthy-cli-device` | `device` | `base64url(JSON)` host fingerprint — privacy observation, see §1A.5 |
| `X-Phanthy-Product` | `product` | `phanthy_code` / `phanthy_codeo` |
| `x-phanthy-project` | `project` | project id of the session, when present |
| `x-phanthy-request` | `request` | per-request UUID correlation id (regenerated per message request) |
| `x-phanthy-session` | `session` | session id |

Two names deliberately capitalise `X-Phanthy`; five use lowercase `x-phanthy-`. Preserve the casing verbatim when
modelling them (`M6-cli-config.txt:58-73`). A non-namespaced `x-service-name: phanthy-codeo` accompanies telemetry
flushes only. On the two discovery endpoints only `X-Phanthy-Product: phanthy_code` is sent
(`M6-cli-config.txt:172, 180`).

### 1A.5 Bearer vs `x-api-key` on `/v1/messages` (the M6 finding)

The CLI's provider `fetch` **deletes `x-api-key`** and **sets `Authorization: Bearer <access token>`**, then sets
`x-phanthy-product: phanthy_code`, `X-Phanthy-Client-Source: cli`, a `User-Agent` and a fresh `x-phanthy-request`
UUID (`M6-cli-config.txt:145-157`).

| scheme | status | consequence |
|---|---|---|
| `Authorization: Bearer <token>` | the only scheme the official client sends (VERIFIED-STATIC) | document Bearer as the primary scheme |
| `x-api-key: <key>` | explicitly removed by the official client; prior art only | keep as an **unverified possibility**, never as "preferred" |

Wire-level acceptance of Bearer by `/v1/*` remains `BLOCKED-CRED`. The M6 probes of EP-16/EP-17 received a
`WWW-Authenticate: Bearer realm="phanthy-code"` challenge, which is consistent with a bearer-gated
`/api/phanthy_code/*` surface but does not prove `/v1/*` behaves the same way.

### 1A.6 Privacy observation — `x-phanthy-cli-device`

The value is `base64url(JSON.stringify({…}))` covering hostname, username, platform, architecture, OS release,
up to 32 IPv4 candidates, network interfaces and default routes (max 32)
(`M6-cli-config.txt:195-201`). It is a host-fingerprint/telemetry design choice, **not** a vulnerability and not
an access-control mechanism. Record it non-exploitatively: an operator running the official CLI transmits host
identity and network topology metadata on each message request. Nothing was replayed or exercised, and no value
is reproduced here.

### 1A.7 Static-only, unverified surface (do not probe)

The bundle also contains `POST ${base}/api/event_logging/batch` (telemetry batch flush, JSON `{events:[…]}`,
Bearer + `x-service-name: phanthy-codeo`), a `dev.code.phanthy.com` build-flag host, `https://code.phanthy.com/console`
and a web-search base URL. These are **VERIFIED-STATIC / UNVERIFIED live**; `/api/event_logging/batch` was **not**
probed and is **not** counted in the 17 endpoints (`M6-cli-config.txt:123-166`; `01-endpoint-catalog.md` §7).

---

## 1B. Model discovery — `GET /api/phanthy_code/models` and `/models/stats` (M6)

Added in M6b. These two routes are new relative to M1 and are catalogued as `EP-16` / `EP-17`
(`01-endpoint-catalog.md` §3.16–§3.17). They are the routes the official CLI uses to populate its model
picker, so they are the only known machine-readable model surface — and they are credential-gated.

### 1B.1 Existence and method surface (VERIFIED-LIVE)

| probe | EP-16 `/api/phanthy_code/models` | EP-17 `/api/phanthy_code/models/stats` |
|---|---|---|
| `OPTIONS` | `204`, `allow: GET, HEAD, OPTIONS` | identical |
| `GET` anonymous | `401` `application/json`, 79 B | identical, byte-identical body |
| `HEAD` anonymous | `401`, no body | identical |

No `access-control-*` and no `ratelimit-*` headers were returned, and `content-length` was absent (chunked
encoding). Gateway headers are the same Kong 2.5.1 / TencentEdgeOne set seen on every other route
(`M6-new-endpoint-probes.txt:18-22, 28-58`).

### 1B.2 Credential requirement (VERIFIED-LIVE for the challenge)

```
HTTP/2 401
content-type: application/json
www-authenticate: Bearer realm="phanthy-code"

{"error":{"code":"missing_login","message":"Bearer access token is required."}}
```

- The challenge names **Bearer** explicitly, so the intended scheme is `Authorization: Bearer <access_token>`
  (`bearerAuth` in the OpenAPI) — not `x-api-key`.
- The message text is a server-side statement of the requirement, which is stronger evidence than the prior-art
  fallback in §1.2 step 9.
- Body is 79 B, sha256 `abeec43bb27c7cbb7c6cd3fa3bbdee2bb9f953de4f6d18d648a416d92116ebe7`, **identical on both
  paths**. That identity is the main structural finding: one shared guard sits in front of the whole
  `/api/phanthy_code/*` prefix (`M6-new-endpoint-probes.txt:60-140`).
- The envelope matches the `{"error":{"code","message"}}` shape already documented in §4 (`missing_login`
  joins `unsupported_response_type` and `invalid_platform`).

### 1B.3 Usage recipe (once a token exists)

```bash
curl -sS --http2 -D - -m 20 \
  -H 'authorization: Bearer <access_token>' \
  -H 'accept: application/json' \
  -H 'x-phanthy-product: phanthy_code' \
  https://code.phanthy.com/api/phanthy_code/models

curl -sS --http2 -D - -m 20 \
  -H 'authorization: Bearer <access_token>' \
  -H 'accept: application/json' \
  -H 'x-phanthy-product: phanthy_code' \
  https://code.phanthy.com/api/phanthy_code/models/stats
```

`X-Phanthy-Product: phanthy_code` is the only Phanthy-namespaced header the official CLI attaches to these two
calls (§1A.2); that is VERIFIED-STATIC as a **client** behaviour, not verified as a server requirement. Per
§1A.4 preserve the exact casing if it is ever sent.

### 1B.4 Success schema: BLOCKED-CRED

Nothing about the `200` body is known. Do **not** publish a fabricated schema, example or field list. The
OpenAPI operations therefore carry a permissive `object` schema whose description states that the success
schema is `BLOCKED-CRED`. What can be stated with an anchor:

| statement | basis |
|---|---|
| response is expected to be `application/json` | CLI call sites send `accept: application/json` (`M6-cli-config.txt:160-186`) |
| `models` feeds the model picker; `modelStats` is best-effort with a **5 s** `AbortSignal` | `M6-cli-config.txt:160-186` |
| display aliases and credit ratios are published for 13 models | PUBLIC-MARKETING, `/token-plan` (§6.1) |
| no `model` wire ID, context window, per-token price or endpoint mapping is published anywhere public | `M6-public-model-catalog.txt:91-102` |

### 1B.5 What this changes for the earlier `/v1/models` finding (§6.5 caveat)

`GET /v1/models` still returns the Next.js HTML 404 shell (verified absent, catalog §4.1). M6 refines rather
than contradicts that: a catalogue **does** exist, but at `/api/phanthy_code/models` behind a bearer guard, not
on the Anthropic-style `/v1/models` path. `POST /v1/messages/count_tokens` remains verified absent.

---

## 2. Minimal call recipes for `POST /v1/messages`

### 2.1 Request fields (PRIOR-ART from `internal/upstream/payload.go:47-60`)

| field | required | type | supported values / behaviour | grade |
|---|---|---|---|---|
| `model` | yes (always serialized) | string | client-facing alias or upstream ID; see §6. Unmapped names pass through unchanged (`payload.go:91-97`) | PRIOR-ART |
| `max_tokens` | yes in practice — the struct has no `omitempty`, so it is always emitted; upstream requirement INFERRED | integer | proxy accepts 1..65536; anything ≤0 or >65536 is rewritten to `8192` (`payload.go:117-119`) | PRIOR-ART |
| `messages` | yes | array of `{role, content}` | `role` ∈ `user`, `assistant`; `content` may be a string or an array of blocks (`payload.go:62-65, 259-304`) | PRIOR-ART |
| `system` | no | string | prior art joins all `system`/`developer` messages with a blank line (`payload.go:141-147, 208-210`) | PRIOR-ART |
| `temperature` | no | number | passed through unmodified (`payload.go:121-123`) | PRIOR-ART |
| `top_p` | no | number | passed through unmodified (`payload.go:124-126`) | PRIOR-ART |
| `stream` | no | boolean | prior art **forces `true`** (`payload.go:109`); see §2.2 | PRIOR-ART |
| `tools` | no | array `{name, description, input_schema}` | OpenAI `function` objects are converted; a missing schema becomes `{"type":"object","properties":{}}` (`payload.go:212-218, 306-335`) | PRIOR-ART |
| `tool_choice` | no | object | `none` / `auto` / `any` (from OpenAI `required`) / `{type:"tool",name}` (`payload.go:337-380`) | PRIOR-ART |
| `stop_sequences` | no | array of strings | from OpenAI `stop` (`payload.go:127-129`) | PRIOR-ART |
| `metadata` | no | object | prior art sets `{"user_id": <openai user>}` (`payload.go:130-132`) | PRIOR-ART |
| image / vision blocks | — | — | OpenAI `image_url` parts are flattened to the literal text `"[image]"` (`payload.go:294-298`); no image passthrough | PRIOR-ART |

### 2.2 Non-streaming recipe

`curl` against the upstream. Prior-art form, using an API key (**not** what the official client does — see §1A.5):

```bash
curl -sS --http2 https://code.phanthy.com/v1/messages \
  -H 'content-type: application/json' \
  -H 'accept: application/json, text/event-stream' \
  -H 'x-api-key: <api_key>' \
  -d '{
        "model": "Iris-1.0",
        "max_tokens": 64,
        "messages": [{"role": "user", "content": "Reply with exactly: ok"}]
      }'
```

Bearer variant — the **only** scheme the official CLI sends on this path (§1A.5), and the documented prior-art
fallback when no API key can be minted (`cmd/login/main.go:97-104`):

```bash
curl -sS --http2 https://code.phanthy.com/v1/messages \
  -H 'content-type: application/json' \
  -H 'authorization: Bearer <access_token>' \
  -d '{"model":"Iris-1.0","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}'
```

**Critical caveat.** Both recipes express a client-side intent only. The prior-art proxy never sends
`stream:false`: `PrepareBody` sets `req.Stream = true` unconditionally (`internal/upstream/payload.go:109`) and
the non-streaming path is produced by locally aggregating the SSE stream (`internal/upstream/sse.go:278-331`,
called from `internal/server/handler.go:205-210`). Therefore:

- the **upstream non-streaming JSON response has never been produced by the prior art** — its shape is `INFERRED`
  from the `message_start` payload (`sse.go:18-25`) and the test fixture (`sse_test.go:11`);
- whether the target even honours `"stream": false` is `BLOCKED-CRED`.
- Grade for the recipes above: PRIOR-ART (fields), unverified live (behaviour).

### 2.3 Streaming recipe

```bash
curl -N -sS --http2 https://code.phanthy.com/v1/messages \
  -H 'content-type: application/json' \
  -H 'accept: text/event-stream' \
  -H 'x-api-key: <api_key>' \
  -d '{
        "model": "Iris-1.0",
        "max_tokens": 128,
        "stream": true,
        "messages": [{"role": "user", "content": "Count to three."}]
      }'
```

Expected frames and their handling are in §3. `-N` disables curl buffering; `head -c` should be used when
capturing, and the command must not be pointed at a file that persists secrets. To mirror the official client,
replace the `x-api-key` line with `-H 'authorization: Bearer <access_token>'` (§1A.5).

### 2.4 `Anthropic-version` handling

- Repo-wide grep for `anthropic-version` returns **zero** hits in Go code and **zero** in the analysed public JS
  chunks (`M1-js-static-analysis.txt:91-95`). The prior art therefore works without it (`PRIOR-ART by absence`).
- `INFERRED`: the target tolerates its absence, or defaults internally. Basis: the prior art is a working
  production proxy per `README.md:3-17`, but no capture confirms it.
- Guidance: **omit** `anthropic-version` first. If (and only if) a live 400 names a version requirement, retry once
  with `anthropic-version: 2023-06-01` — the conventional Anthropic value — and record the outcome. Do not
  pre-emptively send it: adding an unverified header risks a 400 on a gateway that validates allowed headers.

---

## 3. Streaming semantics

### 3.1 What the parser actually does

`internal/upstream/sse.go:148-168` (stream) and `:287-295` (aggregate):

- reads **line by line** via `bufio.NewReaderSize(r, 64*1024)` (`sse.go:132, 282`), `ReadString('\n')`;
- only lines starting with the literal `data: ` are considered (`sse.go:152, 287`);
- the `event: ` name is **never read**; the JSON `type` field is authoritative (`sse.go:155-157, 290-292`);
- a payload equal to `[DONE]` is skipped (`sse.go:154, 289`);
- anything that does not unmarshal as JSON is silently dropped (`sse.go:156-158`);
- blank lines and comment lines (`:`) are ignored implicitly.

Consequence: the client-side SSE emitter only needs `data:` lines; `event:` lines are optional decoration for
observability, not for parsing.

### 3.2 Event sequence understood by the prior art

| order | JSON `type` | what the parser reads | effect on the downstream (OpenAI-shaped) stream | anchor |
|---|---|---|---|---|
| 1 | `message_start` | `message.id`, `message.model`, `message.usage.input_tokens` | emits first chunk `delta.{"role":"assistant"}`; records id/model/input tokens | `sse.go:193-205`, struct `:18-25` |
| 2 | `content_block_start` (`content_block.type == "text"`) | nothing | no output (text arrives in deltas) | `sse.go:207-213` |
| 2' | `content_block_start` (`type == "tool_use"`, at `index=i`) | `content_block.id`, `.name` | registers tool `i` and emits `tool_calls[i]` with `id`, `type:"function"`, `function.name`, `arguments:""` | `sse.go:214-231` |
| 3 | `content_block_delta` (`delta.type == "text_delta"`) | `delta.text` | emits `delta.content` = text fragment | `sse.go:237-244` |
| 3' | `content_block_delta` (`delta.type == "thinking_delta"`) | `delta.thinking` | emits `delta.reasoning_content` (reasoning is surfaced separately, **not** appended to text) | `sse.go:245-250` |
| 3'' | `content_block_delta` (`delta.type == "input_json_delta"`, index `i`) | `delta.partial_json` | appends to the tool buffer and emits `tool_calls[i].function.arguments` fragment | `sse.go:251-263` |
| 4 | `content_block_stop` | — | **not handled**; falls through the switch silently | `sse.go:188-276` (no case) |
| 5 | `message_delta` | `delta.stop_reason`, `delta.stop_sequence`, `usage.output_tokens`, `usage.input_tokens` | stores stop reason and final usage (usage recorded only when `output_tokens > 0`) | `sse.go:265-272`; helpers `:74-100` |
| 6 | `message_stop` | — | no-op | `sse.go:273-275` |
| — | `error`, `ping` | `error.type`, `error.message` are *parsed* into the struct | **no handler branch → the event is silently discarded** | `sse.go:44-47` vs `:188-276` |

A representative upstream fixture, preserved in the repo's tests, matches this sequence and ends with
`data: [DONE]` (`internal/upstream/sse_test.go:10-30`). Grade: PRIOR-ART (fixture authored by the repo, not a live
capture).

### 3.3 Termination, chunking and `[DONE]`

- Termination is driven by **EOF of the upstream body**, not by `message_stop` (`sse.go:162-168`).
- On EOF the parser always appends a final downstream chunk carrying `finish_reason`, then writes
  `data: [DONE]\n\n` (`sse.go:169-184`).
- `finish_reason` is `stop_reason` if one was seen, else `"tool_calls"` when any tool block was registered, else
  `"stop"` (`sse.go:170-177`; same logic for aggregation at `:307-314`).
- Because the aggregation path synthesises an id when none arrived (`sse.go:304-306`), an empty upstream stream
  still yields a well-formed (empty-content) completion — a failure can look like success.
- Chunking: the reader is line-oriented over a 64 KiB buffer, so a `data:` line split across TCP segments is
  reassembled before parsing. However, a **multi-line** SSE event (several consecutive `data:` lines forming one
  payload) would be treated as several independent payloads and each would fail to unmarshal and be dropped.
  `INFERRED` from code structure (`sse.go:148-168`); the target's framing conventions are unverified.
- `[DONE]` from the upstream is tolerated but not expected from an Anthropic-style upstream; the fixture includes
  it (`sse_test.go:30`), so the code is defensive here rather than evidence of target behaviour.

### 3.4 Error frames — a real gap worth documenting

`anthroEvent.Error` is declared (`sse.go:44-47`) but `handleEvent` has **no `case "error"`** (`sse.go:188-276`).
Therefore a mid-stream error event is dropped, the loop ends at EOF, and the client receives a normal
`finish_reason:"stop"` plus `[DONE]`. Recommended consumer defence (not a claim about the target):

- treat a stream that produced **no** `content_block_delta` and no `stop_reason` as a failure;
- compare `usage.output_tokens` against whether text arrived;
- keep the pre-stream HTTP status, which is the only reliable error signal the proxy forwards.

Grade for "upstream may send an `error` event": INFERRED (Anthropic SSE convention; the struct field proves the
author expected it).

---

## 4. Error taxonomy

### 4.1 Classification algorithm (PRIOR-ART, `internal/upstream/client.go:76-107`)

Evaluation order matters and is counter-intuitive:

1. `status == 402` → `hard_credit` (short-circuits everything) — `client.go:77-79`.
2. body contains any **hard marker** (case-insensitive, plus raw-match) → `hard_credit` — `client.go:61-67, 80-85`.
3. body contains any **session-dead marker** → `session_dead` — `client.go:69-73, 86-90`.
4. `status == 401` → `session_dead` — `:91-93`.
5. `status == 429` → `soft_rate` — `:94-96`.
6. `status == 404` → `not_found` — `:97-99`.
7. `status >= 500` → `server` — `:100-102`.
8. `status >= 400` → `client` — `:103-105`.
9. otherwise → `none` — `:106`.

Two consequences to state explicitly: a `401` whose body says `billing` is classified `hard_credit` (markers run
before the status test), and `Classify(200, body)` is used for OAuth error envelopes (`client.go:164`), where an
unmatched code degrades to `client` (`:165-168`).

### 4.2 Taxonomy table

| condition | HTTP status | upstream markers used | internal class | recommended client behaviour | anchor |
|---|---|---|---|---|---|
| payment required | `402` | none needed | `hard_credit` | stop immediately; do not retry; surface "credit exhausted"; re-check balance out of band | `client.go:77-79` |
| credit/quota wording in body | any (commonly 402/403) | `insufficient credit`, `no credit`, `credit exhausted`, `out of credit`, `quota exceeded`, `quota exhaust`, `payment required`, `credit not enough`, `not enough credit`, `credit balance is too low`, `billing`, `积分不足`, `额度不足`, `余额不足`, `积分用完`, `额度用尽`, `没有积分` | `hard_credit` | same as above; the mixed English/Chinese list shows the exact upstream wording is **not** known | `client.go:61-67, 80-85` |
| token/session invalid | any | `OAuth token has been revoked`, `missing_login`, `invalid_grant`, `session expired`, `unauthorized`, `invalid access token` | `session_dead` | re-authenticate (new PKCE flow); do not retry the same credential | `client.go:69-73, 86-90` |
| unauthenticated | `401` | — | `session_dead` | same; a repeated 401 must stop the sequence | `client.go:91-93` |
| throttled | `429` | — (no `RateLimit-*` header ever observed: `M1-index.md:210-212`) | `soft_rate` | back off ≥ 60 s; rotate to another account if one exists | `client.go:94-96` |
| model/route missing | `404` | — | `not_found` | rotate account once, short cooldown, do **not** count as a consecutive error | `client.go:25-26, 97-99`; `internal/server/handler.go:186-189` |
| upstream fault | `5xx` | — | `server` | bounded retry with exponential backoff; count toward error threshold | `client.go:100-102`; `handler.go:190-194` |
| malformed request / other 4xx | `400`, `403`, `422`, … | — | `client` | fix the request; never blind-retry unchanged | `client.go:103-105` |
| success | `<400` | — | `none` | proceed | `client.go:106` |

### 4.3 How the prior-art proxy reacts to each class

| class | proxy action | anchor |
|---|---|---|
| `hard_credit` | cool the account for `HardCooldown` (default 12 h) and try the next account | `internal/server/handler.go:173-177` |
| `soft_rate` | cool for `SoftCooldown` (default 60 s) and try the next account | `handler.go:178-181` |
| `session_dead` | permanently disable the account until manual re-login | `handler.go:182-185`; `internal/pool/pool.go:180-189` |
| `not_found` | short cool (60 s) via the *soft* cooldown, and continue | `handler.go:186-189` |
| other 4xx/5xx | increment the error counter; at threshold, cool for `ErrCooldown` (10 m) | `handler.go:190-194`; `pool.go:191-204` |
| transport failure | increment the error counter | `handler.go:166-170` |
| success | reset the error counter | `handler.go:196-197`; `pool.go:206-213` |
| all accounts exhausted | return `503` with `{"error":{"message":…,"type":"api_error","code":"no_healthy_account"}}` | `handler.go:212-217, 231-239` |

Important behavioural note for consumers of the **proxy** (not the target): upstream status codes are never
passed through. `chatCompletions` either returns a successful body or the `503` envelope above
(`handler.go:117-218`); the upstream status appears only inside the message string. Applications must therefore
key on `503 + code=no_healthy_account`, not on 402/429.

### 4.4 What is actually evidenced vs assumed

- Evidence for the **existence** of an error taxonomy on the target: only three envelope codes were ever observed —
  `unsupported_response_type` (`M1-anon-probe-matrix.txt:112`), `missing_login` (`:163, :190`),
  `invalid_platform` (`:245, :365`). All share the shape `{"error":{"code","message"}}`.
- Evidence for `402`, `429`, `5xx`: **none**. No request in M1 could reach an authenticated inference path.
- Therefore every row above is a statement about the *proxy's classifier*, with the target-side statuses and
  marker strings being PRIOR-ART assumptions.

---

## 5. Rate-limit / credit / cooldown behaviour

### 5.1 Configured policy (PRIOR-ART)

| trigger | class → cooldown kind | duration | source |
|---|---|---|---|
| `402` or credit/quota markers | `hard_credit` → `CoolHard` | **12 h** | `README.md:89-94`; `config.example.json:8`; `cmd/server/config.go:51`; `handler.go:42-44, 173-177` |
| `429` | `soft_rate` → `CoolSoft` | **60 s** | `README.md:91`; `config.example.json:9`; `cmd/server/config.go:52`; `handler.go:45-47, 178-181` |
| `401` / session markers | `session_dead` → `Disable` (permanent) | until manual re-login | `handler.go:182-185`; `pool.go:180-189` |
| `404` | `not_found` → `CoolSoft` | **60 s** (not the error cooldown) | `handler.go:186-189` |
| 3 consecutive other errors | `client`/`server` → `NoteError` | threshold **3**, cooldown **10 m** | `README.md:92-93`; `config.example.json:10-11`; `pool.go:191-204` |
| success | resets the counter to 0 | — | `pool.go:206-213` |
| per-request account rotation | up to 3 accounts | — | `handler.go:39-41, 133` |
| proactive token refresh | 10 min before expiry | — | `handler.go:54-56, 141`; `internal/auth/auth.go:36-42` |
| nightly keepalive refresh | 22:00 local, refresh if expiring within 30 min | — | `internal/scheduler/scheduler.go:28-31, 79` |

### 5.2 Which parts are observations

**Observed, indirectly:** the existence of a rate-limit *concept* is only suggested by the presence of a `429`
branch and the word `soft_rate` in the source. Nothing in M1 shows a 429, a 402, or any `RateLimit-*`,
`Retry-After`, or quota header (`M1-index.md:210-212`; header dumps `M1-anon-probe-matrix.txt:87-289` contain
only infrastructure headers: `x-kong-*`, `eo-*`, `via`, `server`).

**Assumptions, not observations:**

- that 402 is the credit signal (Anthropic convention; `client.go:77`);
- that 429 is the throttle signal;
- that the marker strings match the target's real wording — the marker list is long, heterogeneous and mixes
  English with Chinese (`client.go:61-67`), which is itself evidence that the exact upstream text was guessed;
- that a 12 h / 60 s / 10 m policy is well-matched to the target's actual quota reset window;
- that `404` on `/v1/messages` is transient (`client.go:25-26` describes it as "上游偶发"), while the only 404s
  M1 observed were *route-absence* HTML shells (§4 of the catalog) — these are different phenomena.

### 5.3 Consequence for M3/M4

Treat §5.1 as this repository's policy, not as target behaviour. Any normative document (SDD) must mark the
cooldown numbers `PRIOR-ART` and keep them configurable; no test may assert that the target returns a specific
status for quota exhaustion until the plan in §7 is executed.

---

## 6. Model roster — two independent layers

M6 established that two model vocabularies circulate and that **they are not the same list**. They are
presented as separate layers here and must not be merged, sorted together or cross-indexed without new evidence.

| layer | source | grade | what it tells you |
|---|---|---|---|
| **A** — public display catalogue | operator's `/token-plan` pricing page (M6) | PUBLIC-MARKETING | which model names and credit multipliers the operator advertises |
| **B** — prior-art internal mapping | in-repo `README.md`, `payload.go`, `handler.go` | PRIOR-ART / unverified live | which upstream IDs the *proxy* sends for its own client-facing names |

**No mapping between A and B has been established.** Layer A never states a wire `model` value; layer B never
mentions any `Phanthy-*`, `Phana-*` or `Phano-*` alias. Do not assume `Phanthy-pro` = `Iris-1.0`, or any other
pairing.

### 6.1 Layer A — public display catalogue (PUBLIC-MARKETING, M6)

Source: `https://code.phanthy.com/token-plan`, anonymous GET, 49,225 B, sha256
`2abf943b6cab2090ed737618f76b03473319dc9e1704a90840f8c061cb40be8a` (`M6-public-model-catalog.txt:4-8,
27-43`). Names and ratios are reproduced verbatim, including the literal `x积分` suffix; the trailing `o`/`s`
characters in the `Phana`/`Phano` rows are U+006F/U+0073 and must not be "corrected"
(`M6-public-model-catalog.txt:124-130`).

| # | display name | credit ratio (verbatim) | scenario (summary) |
|---|---|---|---|
| 1 | `Phanthy-fast` | `0.00x积分` | fast/cheap Q&A, summary, translation, rewriting |
| 2 | `Phanthy-pro` | `0.33x积分` | general strong model: office, coding, analysis, agent |
| 3 | `Phanthy-ultra` | `0.67x积分` | top-tier reasoning, deep research, hardest agent work |
| 4 | `glm-5.3-flash` | `0.04x积分` | cost-optimised GLM, coding/agent/multimodal |
| 5 | `glm-5.3` | `0.38x积分` | Zhipu flagship |
| 6 | `glm-5.2` | `0.38x积分` | previous Zhipu flagship, long-context |
| 7 | `kimi-k3` | `1.00x积分` | Kimi flagship, long context |
| 8 | `kimi-k2.7-code` | `0.31x积分` | coding-specialised Kimi |
| 9 | `deepseek-v4.1-flash-1017` | `0.10x积分` | DeepSeek fast/balanced |
| 10 | `Phana-5.5o` | `0.67x积分` | placeholder scenario text (`Available model`) |
| 11 | `Phana-5.5s` | `0.33x积分` | placeholder scenario text (`Available model`) |
| 12 | `Phano-6a` | `1.67x积分` | placeholder scenario text (`Available model`) |
| 13 | `Phano-6.1s` | `0.33x积分` | placeholder scenario text (`Available model`) |

Reading rules for this table:

- A ratio is a **credit multiplier**, printed as `N.NNx积分`. It is *not* a price per 1k tokens, and no
  per-token price can be derived from it (`M6-public-model-catalog.txt:104-106`).
- Four of the 13 rows carry the literal placeholder `Available model` instead of a scenario — the operator
  appears to have staged these entries ahead of their descriptions. Do not infer capabilities from them.
- `data-series` / logo assets are presentation only: `phana*` rows use `anthropic.svg`, `phano*` rows use
  `openai.svg`, `phanthy*` use `phanthy.jpg`. The page does **not** state an upstream mapping
  (`M6-public-model-catalog.txt:45-65`).
- Not published on the page: wire model IDs, context windows / max tokens, per-token prices, rate limits,
  endpoint mapping, credit-balance API (`M6-public-model-catalog.txt:91-102`).

Plan tiers referenced by the same page (verbatim prices, PUBLIC-MARKETING): 体验版 ¥0/月 · 2,000 积分/月;
标准版 ¥9.9/月 (struck 原价 ¥99) · 15,000 积分/月; 高级版 ¥139/月 (struck 原价 ¥199) · 30,000+2,000 积分/月;
旗舰版 ¥699/月 (struck 原价 ¥999) · 150,000+20,000 积分/月. Value note: `每 1,000 积分约等于 1 美元`
(`M6-public-model-catalog.txt:71-85`).

### 6.2 Layer B — prior-art internal mapping table (PRIOR-ART / unverified live)

Label for the whole table: **PRIOR-ART / unverified live.** Sources: `README.md:120-134`,
`internal/upstream/payload.go:75-89`, tests `internal/upstream/payload_test.go:167-194`,
proxy-side model list `internal/server/handler.go:93-108`.

| # | client-facing name | upstream model ID | family | capability notes | grade |
|---|---|---|---|---|---|
| 1 | `DeepSeek-V4` | `Iris-1.0` | Iris | reasoning/chat; suffix-free base version | PRIOR-ART / unverified live |
| 2 | `gpt-5.6-sol` | `Zeus-1.1-pro` | Zeus | `-pro` tier, highest quality of the Zeus line (`INFERRED` from suffix) | PRIOR-ART / unverified live |
| 3 | `gpt-5.6-terra` | `Zeus-1.1` | Zeus | standard tier | PRIOR-ART / unverified live |
| 4 | `gpt-5.6-luna` | `Zeus-1.1-fast` | Zeus | `-fast` tier, latency-optimised (`INFERRED` from suffix) | PRIOR-ART / unverified live |
| 5 | `gpt-5.5` | `Zeus-1.0-pro` | Zeus | previous generation, pro tier | PRIOR-ART / unverified live |
| 6 | `Claude Opus 4.8` | `Gaia-1.2` | Gaia | Anthropic-compatible top tier | PRIOR-ART / unverified live |
| 7 | `Claude Opus 4.7` | `Gaia-1.1` | Gaia | previous point release | PRIOR-ART / unverified live |
| 8 | `Claude Sonnet 4.6` | `Gaia-1.0` | Gaia | mid tier | PRIOR-ART / unverified live |
| 9 | `Kimi K3` | `Apollo-2.0` | Apollo | newest Kimi generation | PRIOR-ART / unverified live |
| 10 | `Kimi-k2.7-code` | `Apollo-1.1` | Apollo | `-code` variant, coding-tuned (`INFERRED` from suffix) | PRIOR-ART / unverified live |
| 11 | `Kimi K2.6` | `Apollo-1.0` | Apollo | previous generation | PRIOR-ART / unverified live |
| 12 | `GLM 5.2` | `Metis-1.1` | Metis | Zhipu GLM line, newest | PRIOR-ART / unverified live |
| 13 | `GLM-5.1` | `Metis-1.0` | Metis | previous point release | PRIOR-ART / unverified live |

### 6.3 Why layer A and layer B cannot be joined

- Vocabulary overlap is suggestive but not probative: `kimi-k2.7-code` appears as a display alias (§6.1 #8) and
  `Kimi-k2.7-code` as a prior-art client name (§6.2 #10); `glm-5.2` / `GLM 5.2` likewise. Similar strings do
  **not** establish that the upstream IDs are the same.
- Layer A's naming scheme (`Phanthy-*`, `Phana-*`, `Phano-*`) has **zero** occurrences in the in-repo Go code or
  README, and layer B's family IDs (`Iris`, `Zeus`, `Gaia`, `Apollo`, `Metis`) have **zero** occurrences on the
  public page.
- The machine-readable catalogue (EP-16) that could settle it returns `401` anonymously (§1B), and the CLI's
  `models` call was never executed. Until one authenticated `GET /api/phanthy_code/models` is performed, the
  mapping is `BLOCKED-CRED`.
- Consequence for tooling: accept **both** vocabularies as pass-through names. The proxy already forwards
  unknown model names unchanged (`payload.go:91-97`), so no allow-list should be enforced.

### 6.4 Capability statements and their basis

| capability | statement | grade / basis |
|---|---|---|
| context window | 200,000 tokens declared for every model | PRIOR-ART (`handler.go:95-107` — the proxy's own static list). No upstream evidence. |
| tool/function calling | all 13 prior-art models accept `tools` and emit `tool_use` streaming blocks | INFERRED: the proxy converts tools for every request (`payload.go:212-223`) and parses `tool_use` + `input_json_delta` (`sse.go:207-231, 251-263`); it applies no per-model gating |
| extended thinking | at least one upstream model emits `thinking_delta` blocks | INFERRED: a handler exists (`sse.go:245-250`). Which model, and whether it must be requested explicitly, is **unknown** (no `thinking` request field exists in `payload.go:47-60`) |
| vision / images | **not supported through the proxy** | PRIOR-ART: image parts are replaced by the text `"[image]"` (`payload.go:294-298`) |
| upstream model naming | `/v1/messages` expects the real IDs (`Iris-1.0`, …), not the commercial names | PRIOR-ART comment `payload.go:73-74`; no live confirmation |
| unknown names | passed through unchanged (no 400 from the proxy) | PRIOR-ART `payload.go:91-97` |

### 6.5 Availability caveats (why neither table can be tightened without a token)

- `GET /v1/models` returns the **HTML 404 shell** (26,579 B), i.e. no catalogue on the Anthropic-style path
  (`M1-anon-probe-matrix.txt:73`; `M1-options-matrix.txt:25`). **Refined by M6:** a catalogue does exist at
  `GET /api/phanthy_code/models` (EP-16), but anonymously it answers `401` (§1B), so it is not readable either.
- The 12 analysed public JS chunks contain **zero** matches for `Iris`, `Zeus`, `Gaia`, `Apollo`, `Metis`,
  `DeepSeek`, `Claude`, `Kimi`, `GLM` (`M1-js-static-analysis.txt:46-54, 97-123`).
- The public pricing page publishes display aliases only, no wire IDs (§6.1). The official CLI bundle contains
  the endpoint URLs but no model list, so the CLI cannot fill the gap statically either.
- Model IDs are therefore obtainable only from (a) an authenticated `GET /api/phanthy_code/models` (EP-16), or
  (b) an authenticated `/v1/messages` response. `M1-index.md:213-214` records exactly this gap.
- Do not present either table as a live catalogue. Use `PUBLIC-MARKETING` for layer A and
  `PRIOR-ART / unverified live` for layer B in the OpenAPI description and in the report.

---

## 7. Unknowns and verification plan (≤ 5 calls, when the user supplies a token)

### 7.1 Unknowns this plan targets

| id | unknown | needs |
|---|---|---|
| U1 | `/v1/messages` auth scheme: `x-api-key` vs `Authorization: Bearer` | token (and/or key). **Narrowed by M6:** the official CLI sends Bearer only (§1A.5); wire acceptance still unproven |
| U2 | non-streaming JSON response shape | token |
| U3 | exact SSE frame sequence, headers, termination | token |
| U4 | error envelope of an authenticated 4xx | token |
| U5 | `/api/oauth/profile` schema | token |
| U6 | `/api/oauth/claude_cli/roles` schema | token |
| U7 | token TTL / `expires_in` / refresh rotation | token + `POST /oauth/token` — **excluded below** (governance) |
| U8 | success bodies of `GET /api/phanthy_code/models` and `/models/stats` (EP-16/EP-17), and the alias→wire-ID mapping | token — **excluded from the 5-call plan below**; needs its own budget line |

### 7.2 Governance constraint on the plan

`00-governance.md:18-20` permits **one** write operation — `POST /v1/messages` — and caps authenticated
verification at **5 calls total**. Consequently `POST /oauth/token` (refresh), `POST /oauth/revoke` and
`POST /api/web/events` are **not** in this plan; U7 needs an explicit governance amendment from the user. All
GET calls below are allowed; all are single-shot, no retries, ≥ 1 s apart.

### 7.3 Call plan

| # | method + URL | purpose | expected observation | abort condition |
|---|---|---|---|---|
| C1 | `GET /api/oauth/profile` | U5 | `200 application/json`; record field **names** only | `401` → token invalid, stop everything; `text/html` → path changed |
| C2 | `GET /api/oauth/claude_cli/roles` | U6 | `200 application/json`; array or object of role names | same as C1 |
| C3 | `POST /v1/messages` (non-streaming, Bearer) | U1, U2, U4 | `200` JSON with `type:"message"`, `content[]`, `usage{}` — **or** a `4xx` envelope that itself answers U1 | `402`/`429`/`5xx` → stop all remaining calls; `401` → record, then stop (no key exists to test `x-api-key`) |
| C4 | `POST /v1/messages` (streaming, same scheme as C3 worked with) | U3 | `200`, SSE frames `message_start` → `content_block_*` → `message_delta` → `message_stop` | non-2xx before the first frame → stop |
| C5 | `POST /v1/messages` with `"model": "___invalid_model___"`, `max_tokens: 8` | U4 (clean 4xx envelope + code) | `400`/`404` JSON envelope, no credit consumed for output tokens | `5xx` or `402` → stop |

### 7.4 Exact commands

```bash
# C1 — profile
curl -sS --http2 -D - -m 20 \
  -H 'authorization: Bearer <access_token>' \
  -H 'accept: application/json' \
  https://code.phanthy.com/api/oauth/profile

# C2 — roles
curl -sS --http2 -D - -m 20 \
  -H 'authorization: Bearer <access_token>' \
  -H 'accept: application/json' \
  https://code.phanthy.com/api/oauth/claude_cli/roles

# C3 — non-streaming inference (Bearer variant; minimum viable body)
curl -sS --http2 -D - -m 60 \
  -H 'authorization: Bearer <access_token>' \
  -H 'content-type: application/json' \
  -H 'accept: application/json, text/event-stream' \
  -H 'x-app: cli' \
  -d '{"model":"Iris-1.0","max_tokens":16,"messages":[{"role":"user","content":"Reply with exactly: ok"}]}' \
  https://code.phanthy.com/v1/messages

# C4 — streaming, only if C3 returned 2xx
curl -N -sS --http2 -D - -m 60 \
  -H 'authorization: Bearer <access_token>' \
  -H 'content-type: application/json' \
  -H 'accept: text/event-stream' \
  -H 'x-app: cli' \
  -d '{"model":"Iris-1.0","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"Count to three."}]}' \
  https://code.phanthy.com/v1/messages

# C5 — error-envelope probe, only if C3 and C4 succeeded
curl -sS --http2 -D - -m 30 \
  -H 'authorization: Bearer <access_token>' \
  -H 'content-type: application/json' \
  -d '{"model":"___invalid_model___","max_tokens":8,"messages":[{"role":"user","content":"x"}]}' \
  https://code.phanthy.com/v1/messages
```

If C3 returns `401` while C1 succeeded, the split is informative rather than a failure: `/api/oauth/*` accepts
Bearer and `/v1/*` does not. Record it and stop — testing `x-api-key` requires an API key, and both minting
pathways 404 (catalog §4.1).

### 7.5 Recording rules and abort discipline

- Record only: status, content type, response headers **except** `authorization`/`set-cookie` values, and body
  **structure** with credential-like values replaced by `<token>`. Never write the token, cookie value, or
  `refresh_token` to disk (`00-governance.md:36-40`).
- One request per step; no retry loop; no concurrency; ≥ 1 s spacing; hard stop at 5 calls.
- Abort immediately on `402`/`429`/`5xx`, on two consecutive `401`s, or on any response larger than ~1 MB.
- Results belong in a new evidence file for the milestone that executes them (M5 re-verification), **not** in this
  document, which must stay byte-identical to what M2 delivered.

---

## 8. Cross-reference table (endpoint → catalog § → evidence → OpenAPI operationId)

Operation IDs reserved here for M4. `webEventsCreate` is reserved but **must be marked as unspecified** in the
spec because its contract is unknown (catalog §3.11).

| endpoint | catalog § | primary evidence anchor | OpenAPI `operationId` | M4 notes |
|---|---|---|---|---|
| `GET /` | §3.1 | `M1-anon-probe-matrix.txt:59, 87-101` | `appShellGet` | Describes HTML, not JSON; consider excluding from the API collection or tagging `web`. |
| `GET /oauth/authorize` | §3.2 | `M1-anon-probe-matrix.txt:60, 112` | `oauthAuthorize` | Query params from PRIOR-ART; `code_challenge_method` enum `[S256]`; document the observed `400` body. |
| `POST /oauth/token` | §3.3 | `M1-options-matrix.txt:16`; `M1-anon-probe-matrix.txt:61-62` | `oauthToken` | `application/x-www-form-urlencoded`; `grant_type` enum `[authorization_code, refresh_token]`; security: none. Success schema PRIOR-ART. |
| `POST /oauth/revoke` | §3.4 | `M1-options-matrix.txt:17`; `M1-openid-configuration.json:5` | `oauthRevoke` | Body shape RFC 7009 `INFERRED`; mark "contract inferred". |
| `GET /oauth/code/success` | §3.5 | `M1-anon-probe-matrix.txt:65` | `oauthCodeSuccessGet` | `text/html`; query `code`/`state` inferred. |
| `GET /.well-known/openid-configuration` | §3.6 | `M1-anon-probe-matrix.txt:450-466` | `openidConfigurationGet` | Full field list is known; safest schema in the whole spec. |
| `GET /.well-known/jwks.json` | §3.7 | `M1-jwks.json:22` | `jwksGet` | `keys[]` with `kty/e/use/alg/kid`. |
| `GET /api/oauth/profile` | §3.8 | `M1-anon-probe-matrix.txt:66, 154-163` | `profileGet` | `bearerAuth`; success schema `BLOCKED-CRED` → use a loose object + example-free description. |
| `GET /api/oauth/claude_cli/roles` | §3.9 | `M1-anon-probe-matrix.txt:69` | `claudeCliRolesGet` | Same `bearerAuth`; schema unknown. |
| `POST /v1/messages` | §3.10 | `M1-options-matrix.txt:23`; `M1-anon-probe-matrix.txt:70-71` | `messagesCreate` | Two alternative security schemes (`apiKeyAuth` header `x-api-key`, `bearerAuth`); request schema PRIOR-ART; two response variants: `application/json` and `text/event-stream`. |
| `POST /api/web/events` | §3.11 | `M1-options-matrix.txt:26`; `M1-js-static-analysis.txt:63` | `webEventsCreate` | Existence only. Emit a placeholder `description: "Semantics unverified (M2)"`, no request schema. |
| `GET /api/download/client` | §3.12 | `M1-anon-probe-matrix.txt:76, 337-366` | `clientDownloadGet` | Required query `platform`; only `linux` proven → do **not** publish an enum containing `darwin`; document `302` + `Set-Cookie` names. |
| `GET /api/public-assets` | §3.13 | `M1-anon-probe-matrix.txt:77, 247-255` | `publicAssetsGet` | Model the `308` from the slash form; target unprobed → no response schema. |
| `GET /robots.txt` | §3.14 | `M1-anon-probe-matrix.txt:80, 369-402` | `robotsGet` | `text/plain`. |
| `GET /sitemap.xml` | §3.15 | `M1-anon-probe-matrix.txt:81, 404-438` | `sitemapGet` | `application/xml`; 841 URLs; do not inline the body. |
| `GET /api/phanthy_code/models` | §3.16 | `M6-new-endpoint-probes.txt:25-105` | `phanthyModelsGet` | **Added M6.** `bearerAuth`; success schema `BLOCKED-CRED`; `401` uses the shared error envelope + `WWW-Authenticate: Bearer realm="phanthy-code"`. |
| `GET /api/phanthy_code/models/stats` | §3.17 | `M6-new-endpoint-probes.txt:110-190` | `modelStatsGet` | **Added M6.** Same guard and same `401` body as EP-16; tag `Inference`. |

Endpoints that must **not** appear as operations (verified absent, HTML 404): `/v1/models`,
`/v1/messages/count_tokens`, `/api/health`, `/healthz`, `/api/oauth/phanthy_cli/create_api_key`,
`/api/oauth/claude_cli/create_api_key` (catalog §4.1). Their absence is itself a documented finding: the
access-token fallback (`internal/upstream/client.go:344-350`) is the only viable `/v1/messages` credential path
until a token proves otherwise. **M6 refinement:** `/v1/models` stays absent, but model discovery is not absent
from the site — it lives at `EP-16` / `EP-17` (§1B), behind a bearer guard.

---

### Document provenance

Authored `2026-10-26` from the M1 evidence package only; zero network requests; no credential, cookie or token
value read, copied or written. The M1 acceptance report's numeric caveats (D1–D8, catalog §1.6) are honoured
throughout: no probe totals, no byte-for-byte OIDC comparison, no JS aggregate size, and the 405/`Allow`
statement is scoped to API routes.

Extended in **M6b** (`2026-10-26`) with the M6 evidence package: §1A (official CLI install/login, credentials,
header family, Bearer finding), §1B (model discovery, EP-16/EP-17), §6 (two-layer roster), §7.1 U8 and the
§8 operationIds `phanthyModelsGet` / `modelStatsGet`. M6b itself issued **zero** network requests; it inherits
the M6 anonymous probes (`M6-new-endpoint-probes.txt`) and the M6 static extraction
(`M6-npm-package.json`, `M6-cli-config.txt`), in which the official npm package was inspected as a tarball and
**never installed or executed**.
