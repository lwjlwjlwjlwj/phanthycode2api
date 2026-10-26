# 05 · Spec-Driven Development (SDD) Specification

**Milestone:** M3 · Author: SDD sub-agent · Language: English
Date: 2026-10-26

---

## 1. Vision, problem statement, target users, and non-goals

### 1.1 Vision
A pure Go reverse-proxy gateway that exposes a standard OpenAI-compatible API surface by translating requests to the upstream Anthropic Messages API at `https://code.phanthy.com`, with automatic multi-account rotation, cooldown management, OAuth PKCE login, token refresh, and SSE streaming pass-through — so that any existing OpenAI client (NextChat, ChatBox, OpenCat, etc.) can connect without modification.

### 1.2 Problem statement
The upstream PhanthyCode API speaks the Anthropic Messages protocol, which is incompatible with the OpenAI `/v1/chat/completions` format that most LLM client tools assume. Additionally:
- Users who authenticate via the web OAuth PKCE flow end up with long-lived credentials that can expire, requiring manual refresh.
- A single account can quickly exhaust its credit, hit rate limits, or expire, causing cascading failures.
- The upstream `/v1/messages` endpoint does not exist as a public OpenAI-compatible endpoint with model listing or health probes.

This gateway solves all of these locally, inside a single zero-dependency Go binary.

### 1.3 Target users
| user profile | use case |
|---|---|
| Self-hosters | Run the gateway on a local LAN behind `P2A_API_KEY` protection and point any OpenAI client at `http://localhost:7864` |
| Toolchain integrators | Drop the gateway between existing OpenAI-compatible automation scripts and the PhanthyCode upstream without modifying a single line of client code |
| Multi-account operators | Operate a pool of many independently logged-in accounts, and let the proxy transparently rotate healthy accounts per request |

### 1.4 Non-goals (explicitly out of scope)
1. This gateway does **not** provide a general-purpose reverse-proxy for arbitrary Anthropic backends — it is purpose-built exclusively for the PhanthyCode upstream at `https://code.phanthy.com`.
2. No persistent chat history, rate-limiting on the gateway side, logging of user prompts/responses, or custom billing logic is implemented.
3. No attempt to crack, leak, or otherwise obtain credentials from the upstream service. All authentication material comes exclusively from the user via the PKCE login flow.
4. The untrusted repository `https://github.com/witt3rd/oh-my-hermes` is never cloned, installed, or executed — per `00-governance.md:22-32`.
5. No outbound scanning or fuzzing of the upstream service beyond the ≤5 authenticated verification calls allowed by `00-governance.md`.

---

## 2. Scope and system context diagram

### 2.1 System context (ASCII)

```
                          ┌────────────────────────────────────┐
                          │  OpenAI-compatible clients         │
                          │  (NextChat / ChatBox / OpenCat)    │
                          └───────────────┬────────────────────┘
                                          │
                       Bearer api_key (optional) + OpenAI JSON
                                          ▼
  ┌──────────────────────────────────────────────────────────────────────┐
  │                          phanthycode2api gateway                     │
  │  ┌────────┐  ┌──────┐  ┌─────┐  ┌───────┐  ┌──────────┐  ┌───────┐  │
  │  │ server │→ │ pool │→│ auth │→│upstream│→│scheduler │  │ login  │  │
  │  └────────┘  └──────┘  └─────┘  └───────┘  └──────────┘  │ tool   │  │
  └──────────────────────────────────────────────────────────┘  └──┬───┘  │
                                                                   │      │
  auth_dir/*.json (persisted credentials) ←────────────────────────┘      │
  state_file (persisted cooldown/disabled status) ←───────────────────────┘
                                          │
                  x-api-key: or Bearer access_token + Anthropic SSE
                                          ▼
                          ┌────────────────────────────┐
                          │  https://code.phanthy.com  │
                          │  (PhanthyCode upstream)    │
                          └────────────────────────────┘
```

### 2.2 Boundaries
| element | inside scope? | note |
|---|---|---|
| OpenAI `/v1/chat/completions` (stream and non-stream) | yes | primary interface |
| OpenAI `/v1/models` listing | yes | static proxy-side model alias table |
| `/status` account-pool status endpoint | yes | operator diagnostics |
| `/healthz` no-auth health probe | yes | Kubernetes/docker healthcheck |
| PKCE login CLI tool | yes | `cmd/login/main.go` |
| OAuth token refresh | yes | proactive + scheduled keepalive |
| SSE translation (Anthropic → OpenAI) | yes | line-by-line pass-through + conversion |
| Per-request account rotation (up to 3 attempts) | yes | `handler.go:133` |
| `/v1/models` on the upstream itself | no | verified 404 (HTML shell), proxy serves its own static list |
| Upstream `/api/web/events` semantics | no | existence-only, no client code uses it |

---

## 3. Functional requirements (FR-01 … FR-17)

All acceptance criteria are binary (Given/When/Then) with no vague "should" language.

| ID | Description / Inputs / Outputs | Priority | Acceptance Criteria |
|---|---|---|---|
| **FR-01** | **OpenAI-compatible non-stream chat completions** — Accept `POST /v1/chat/completions` with standard OpenAI request fields (`model`, `messages`, `max_tokens`, `temperature`, `top_p`, etc.). The gateway forces stream=true on the upstream side, consumes the full SSE response locally (`Aggregate`), and returns a single well-formed OpenAI `chat.completion` JSON object. Input: valid OpenAI request body ≤ 8 MiB. Output: 200 `application/json` matching the OpenAI non-stream schema. | P0 | Given a healthy account in the pool, when a non-stream request arrives with `stream: false`, then the upstream receives a valid Anthropic request body with `stream: true`, the entire SSE response is aggregated in memory, and the client receives a 200 with `object: "chat.completion"` and no SSE framing. |
| **FR-02** | **OpenAI-compatible streaming (SSE) chat completions** — Accept the same `POST /v1/chat/completions` with `stream: true`. Translate the upstream Anthropic SSE stream line by line to OpenAI SSE format and flush each chunk immediately. Output: `text/event-stream` chunks, terminated by `data: [DONE]\n\n`. | P0 | Given a healthy account in the pool, when a streaming request arrives, then upstream SSE `data:` lines are parsed, converted, and written back as OpenAI `chat.completion.chunk` frames with immediate Flush, and the stream terminates with exactly one `data: [DONE]` marker. |
| **FR-03** | **Models listing endpoint** — `GET /v1/models` returns the full static list of 13 model aliases in standard OpenAI format (`object: "list"`, each item with `id`, `object: "model"`, `owned_by`, `context_length`). Input: none. Output: 200 JSON list. | P0 | Given no auth key is configured, when `GET /v1/models` is called, then a JSON 200 response is returned containing exactly 13 model entries, each with distinct client-facing names (`DeepSeek-V4`, `gpt-5.6-sol`, …). |
| **FR-04** | **Health endpoint** — `GET /healthz` returns 200 OK with plain text body `ok`, no authentication required. Input: none. Output: `200 text/plain`. | P0 | When `GET /healthz` is called on a running server, the response status code is 200 and the body equals the exact ASCII string `"ok"`. No authorization header is examined or required. |
| **FR-05** | **Pool status endpoint** — `GET /status` returns a JSON overview of every account currently in the pool: `uid`, `nickname`, `has_api_key`, `cooling`, `until`, `reason`, `disabled`, `err_count`. Input: authenticated bearer. Output: 200 JSON. | P1 | Given the pool has at least one registered account, when an authenticated caller hits `/status`, then every registered account appears in the `accounts` array with all seven status fields populated correctly, and the list is sorted by UID. |
| **FR-06** | **Auth middleware (Bearer protection)** — If `api_key` is non-empty in config, every protected endpoint (`/v1/chat/completions`, `/v1/models`, `/status`) must validate that the incoming `Authorization` header is exactly `Bearer <configured-api-key>`. On failure return 401 with standard OpenAI error shape `{"error":{"code":"invalid_api_key","message":"missing or invalid API key","type":"api_error"}}`. If `api_key` is empty, no auth check runs. | P0 | Given the server is started with a non-empty `P2A_API_KEY`, when a request to `/v1/chat/completions` arrives with no Authorization header or an incorrect Bearer token, then the server immediately returns HTTP 401 without forwarding to any upstream. When `api_key` is empty, the exact same unauthenticated request proceeds to handler logic with no error. |
| **FR-07** | **Account rotation across healthy accounts** — For one incoming chat completion request, try up to `MaxRotate` (default 3) distinct healthy accounts from the pool before failing. Skip any accounts already marked as `tried` for this request. Accounts are picked with a stable sort: prefer accounts that have a non-empty `APIKey`, then lexically by UID. | P0 | Given two or more healthy accounts exist, when the first selected account fails with a non-session-dead error, then the gateway automatically picks a second distinct healthy account that was not tried in the same loop and retries the upstream call. If all three rotation slots are exhausted with no success, the gateway stops rotation. |
| **FR-08** | **Cooldown — hard-credit (12h default)** — When the upstream classifier returns `ErrHardCredit`, the targeted account is marked as cooling until `now + HardCooldown` (default 12h). A cooling account is no longer returned by `Pick` / `PickExcluding`. | P0 | Given a healthy account exists, after calling `Cooldown(uid, CoolHard, 12*time.Hour, "test")`, then any subsequent call to `Pick()` returns `nil` for that UID for the full 12h simulated window (unless the wall-clock advances past `until`). |
| **FR-09** | **Cooldown — soft-rate (60s default)** — Classifier returns `ErrSoftRate` (HTTP 429) → cool for `SoftCooldown` (default 60s). Identical behaviour: account excluded from Pick. | P0 | Given a healthy account, after calling `Cooldown(uid, CoolSoft, 60*time.Second, "rate")`, then Pick() does not return that account, and 60 simulated seconds later (now > until) it is once again eligible. |
| **FR-10** | **Cooldown — consecutive-error threshold** — Every non-success / non-credit / non-rate / non-session-dead response increments that account's `errCount`. Once `errCount` reaches the configured `ErrThreshold` (default 3), the account enters `ErrCooldown` (default 10m). A single successful call resets `errCount` back to zero. | P1 | Given an account with threshold=3, after three consecutive `NoteError` calls and no intervening `NoteSuccess`, then that account is cooling. If `NoteSuccess` is called after two errors, `errCount` becomes zero and the account remains healthy. |
| **FR-11** | **Token expiry proactive refresh (handler path)** — Before using an account for an inference, check `NeedsRefresh(RefreshSkew)` (default 10 minutes). If the token will expire within that skew window, call `RefreshToken` against upstream, update in-memory fields, then atomically `SaveAtomic` the auth file to disk. If refresh returns `ErrSessionDead`, mark that account permanently disabled. | P0 | Given an account whose `expiresAt` timestamp is 5 minutes in the future (inside the 10-minute skew), when that account is selected for a chat completion, then `RefreshToken` is called, the new token values are written atomically to the auth JSON file, and the request proceeds with the refreshed credential. |
| **FR-12** | **PKCE login semi-autonomous flow** — The standalone `cmd/login` tool supports two steps: `-step=url` generates a 48-byte cryptographically random verifier, computes its SHA-256 S256 challenge, prints the full authorization URL, and saves the verifier to `.login-verifier` mode 0600. `-step=exchange -code=<code>` exchanges the code for an OAuth token, attempts to mint an API key, and writes the resulting credential JSON file into `auth_dir/phanthy-*.json` (single `os.WriteFile`, not crash-atomic). | P0 | Running `go run ./cmd/login -step=url` produces a valid `client_id=phanthy-code-cli` authorization URL containing the `code_challenge` parameter, writes the verifier to `.login-verifier`, and opens the browser (if supported). After a valid code is provided, `-step=exchange` produces a valid JSON auth file under `auth_dir/`. |
| **FR-13** | **API-key fallback — when APIKey is empty** — If an account has no populated APIKey, the gateway calls `EnsureAPIKey` on first use to attempt to retrieve one. If that call fails (which upstream 404s confirm), the gateway transparently falls back to using `Authorization: Bearer <access_token>` on the `/v1/messages` request header instead of the preferred `x-api-key`. | P1 | Given an account with non-empty access_token but empty APIKey, when the first chat call uses that account, `ChatStream` sends `Authorization: Bearer <access_token>` header instead of `x-api-key`, and the request does not fail due to missing APIKey. |
| **FR-14** | **Keepalive scheduler** — The scheduler runs immediately on startup (initial keepalive pass), then at each configured `KeepaliveHours` (default 22:00 local time every day). During each pass, every non-disabled account is scanned: if the token expires within the 30-minute skew window, it is refreshed and atomically persisted. Session-dead accounts are automatically marked disabled. | P1 | Given a running server with the scheduler started in background, when the simulated clock advances to 22:00 and the current time exactly matches one of the configured keepalive hours, then every expiring token in the pool is refreshed without any incoming HTTP request. |
| **FR-15** | **SSE translation (Anthropic → OpenAI)** — Every Anthropic SSE event type is translated correctly: `message_start` → emit role chunk, `content_block_delta/text_delta` → emit content delta, `content_block_delta/thinking_delta` → emit `reasoning_content`, `content_block_delta/input_json_delta` → stream tool argument partial JSON fragments, `message_delta` → capture stop_reason and usage counters. Unknown events are silently dropped. No Anthropic `event:` name field is parsed — only the JSON `type` field inside the `data:` payload is authoritative. | P0 | Given a valid upstream SSE fixture file matching the known event sequence, when passed through the `Stream` function, the emitted downstream SSE frames form a semantically identical OpenAI-compatible stream. No lines outside the `data: ` prefix are ever processed. |
| **FR-16** | **Error mapping — all 6 classification classes** — The `Classify` function evaluates status code and response body in the fixed priority order: 402 → hard markers → session markers → 401 → 429 → 404 → 5xx → generic client. Each class drives a distinct pool state-machine action. | P0 | Given the classifier receives status 402, it returns `ErrHardCredit`. Given status 401, returns `ErrSessionDead`. Given status 429, returns `ErrSoftRate`. Given status 404, returns `ErrNotFound`. Given status 503, returns `ErrServer`. Given status 400, returns `ErrClient`. |
| **FR-17** | **All-accounts-unavailable 503 response** — After exhausting all configured rotation attempts and every account in the pool is cooling/disabled, return a 503 Service Unavailable body in standard OpenAI error format: `{"error":{"code":"no_healthy_account","message":"all accounts unavailable (...)"}}`. | P0 | Given zero healthy accounts left in the pool, when a chat completion request arrives, no upstream call is made, and the gateway immediately returns HTTP status 503 with error code `no_healthy_account`. |

---

## 4. Non-functional requirements (NFR-01 … NFR-07)

| ID | Description |
|---|---|
| **NFR-01** | **Performance** — The gateway adds <5 ms of latency to each successful chat completion request on top of upstream network time. SSE chunk throughput is ≥ 1000 frames/sec per connection. Max upstream HTTP timeout is 120 seconds. Read-header server timeout is 30 seconds. |
| **NFR-02** | **Reliability** — No data loss on concurrent token refresh: credential files rewritten by the gateway (`internal/auth.SaveAtomic`) use `tmp + rename` under per-account mutex protection, so a crash mid-write never leaves a truncated JSON auth file. The standalone `cmd/login` tool writes its output with a single `os.WriteFile` (`cmd/login/main.go:124`) and is **not** crash-atomic. |
| **NFR-03** | **Security (no secret persistence)** — No real L2 secret (raw `access_token`, `refresh_token`, `api_key`, cookie value) is ever written to any file under `/workspace/re/` evidence, documentation, or test fixtures. All occurrences in this document and all spec files use `<api_key>` / `<token>` placeholders. The `.gitignore` and `.dockerignore` already exclude `auths/` and `data/` so secrets never reach version control. |
| **NFR-04** | **Observability** — Every account rotation decision, refresh event, classification outcome, and keepalive pass is logged at INFO level via Go `log.Printf`. No log message ever prints the full raw secret token string. |
| **NFR-05** | **Portability** — Single statically linked Go binary with zero external dependencies (`go build` produces a single 10–15 MB executable). Runs on any Linux/amd64, Linux/arm64, macOS, or Windows system without runtime installation steps. |
| **NFR-06** | **Compliance with upstream ToS** — No DoS, no rate pressure over 1 req/s, no credential brute-force, no exploitation of vulnerabilities. All usage must respect the PhanthyCode platform Terms of Service, user is solely responsible for their own conduct. |
| **NFR-07** | **Memory safety** — No unbounded buffer growth. SSE reader is capped at 64 KiB line buffer, chat request body is capped at 8 MiB. No unbounded read on upstream response bodies beyond 1 MiB via `io.LimitReader`. |

---

## 5. Architecture and component responsibilities (mapped to actual files)

```
cmd/
  server/main.go        # main entry: load config → load auths → build pool → start upstream client → start scheduler → start HTTP handler
  server/config.go      # JSON config load + environment variable overlay + duration parsing (P2A_* env vars)
  login/main.go         # standalone PKCE login CLI (two-step url / exchange)

internal/
  auth/
    auth.go             # Auth struct (normalized credential), Parse (3 disk formats), SaveAtomic (atomic tmp+rename), LoadDir
  pool/
    pool.go             # Account pool state machine: Pick, PickExcluding, Cooldown, Disable, NoteError, NoteSuccess, List, persistence
    pool_test.go        # Unit tests for the pool state machine
  upstream/
    client.go           # Upstream HTTP client, Classify error taxonomy, RefreshToken, CreateAPIKey, ChatStream
    payload.go          # OpenAI ↔ Anthropic request translation, model alias map, PrepareBody
    payload_test.go     # Payload conversion unit tests
    sse.go              # Anthropic SSE → OpenAI SSE streaming (Stream) / non-stream aggregation (Aggregate)
    sse_test.go         # SSE parser unit tests with golden fixtures
  server/
    handler.go          # HTTP mux, withAuth middleware, chatCompletions main loop, /v1/models, /status, /healthz
  scheduler/
    scheduler.go        # Keepalive cron loop: initial pass on startup, then hourly at configured KeepaliveHours
```

Component dependency direction:
```
server → pool → auth
server → upstream
server → scheduler
login → (imports no internal packages outside stdlib net/http)
```

---

## 6. Interface contracts

### 6.1 Gateway-facing API (OpenAI-compatible, this service)

| endpoint | method | auth | request | response | anchor |
|---|---|---|---|---|---|
| `/v1/chat/completions` | POST | optional Bearer | OpenAI chat completion JSON (stream/non-stream) | 200 JSON or SSE; 401 auth fail; 503 no healthy accounts | `internal/server/handler.go:58, 117-218` |
| `/v1/models` | GET | optional Bearer | empty | 200 `{"object":"list","data":[...]}` | `internal/server/handler.go:59, 93-115` |
| `/status` | GET | optional Bearer | empty | 200 `{"accounts":[...]}` | `internal/server/handler.go:60, 87-91` |
| `/healthz` | GET | no auth | empty | 200 `ok` | `internal/server/handler.go:61, 82-85` |

### 6.2 Upstream-facing calls (towards `https://code.phanthy.com`)

| path | method | auth | purpose | anchor |
|---|---|---|---|---|
| `/oauth/token` | POST | none (form) | code exchange / refresh token grant | `internal/upstream/client.go:232-259` |
| `/api/oauth/phanthy_cli/create_api_key` | POST | Bearer access_token | mint API key (PRIOR-ART, upstream returns 404) | `internal/upstream/client.go:262-308` |
| `/api/oauth/profile` | GET | Bearer access_token | fetch account profile (declared, no production call site) | `internal/upstream/client.go:369-394` |
| `/v1/messages` | POST | `x-api-key` (preferred) or `Authorization: Bearer` (fallback) | Anthropic Messages inference (SSE stream) | `internal/upstream/client.go:331-367` |

### 6.3 Auth file format (`auth_dir/phanthy-<uid>.json`)
Three disk formats are supported by `auth.Parse` (`internal/auth/auth.go:44-149`):

1. **CLI native format** (type-tagged):
```json
{
  "type": "oauth_token",
  "access_token": "<token>",
  "refresh_token": "<token>",
  "expires_at": 1234567890,
  "uid": "...",
  "nickname": "...",
  "api_key": "..."
}
```

2. **Nested format** (gateway own write-back):
```json
{
  "auth": {
    "accessToken": "<token>",
    "refreshToken": "<token>",
    "expiresAt": 1234567890
  },
  "account": {
    "uid": "...",
    "nickname": "..."
  },
  "api_key": "..."
}
```
Anchor: `internal/auth/auth.go:164-175`.

3. **Flat format** (hand-editable):
```json
{
  "accessToken": "<token>",
  "refreshToken": "<token>",
  "expiresAt": 1234567890,
  "uid": "...",
  "nickname": "...",
  "api_key": "..."
}
```

All three formats are normalized in memory to the single `auth.Auth` struct. Atomic write-back always writes the nested format.

### 6.4 State file format (`state_file` path, e.g. `./data/state.json`)
Persists only the cooling/disabled state of every account (never stores full credentials):
```json
{
  "accounts": {
    "phanthy-1730000000": {
      "disabled": false,
      "reason": "consecutive errors",
      "until": "2026-10-26T22:30:00Z"
    }
  }
}
```
Anchor: `internal/pool/pool.go:68-74`. The entire file is rewritten atomically via tmp+rename on every state change.

---

## 7. Data model and state machine

### 7.1 Account lifecycle state machine
```
          ┌───────────┐
          │  Healthy  │ ◀────────── NoteSuccess → errCount=0
          └─────┬─────┘
                │
                ▼
          ┌────────────┐
          │  Cooling   │  (CoolHard / CoolSoft / CoolErr)
          └─────┬──────┘
                │ time.Now() > until
                ▼
          ┌───────────┐
          │  Healthy  │
          └───────────┘

          (any state) → Disable() → ┌───────────┐
                                     │ Disabled  │
                                     └───────────┘
                                     (terminal; never picked until manual re-login removes the disabled record or the account is re-added)
```
State transitions:
- `Healthy → Cooling`: triggered by `Cooldown` (hard/soft) or by `NoteError` reaching the threshold.
- `Cooling → Healthy`: triggered automatically when the current wall-clock time passes the `until` deadline.
- Any state → `Disabled`: triggered by `Disable` (session death). This is a permanent state that survives process restarts because it is persisted to `state_file`.
- `Healthy → Healthy (errCount+1)`: triggered by `NoteError` without crossing threshold. `errCount` increments but the account remains eligible for selection.
- `Healthy → Healthy (errCount=0)`: triggered by `NoteSuccess`. Resets consecutive error counter to zero.

### 7.2 Token lifecycle
```
Fresh (valid, TTL >> 10m) ────> NeedsRefresh(10m) at T-10m ──> RefreshToken()
                                                                 │
                                                                 ▼
                                                        New access_token + rotated refresh_token
                                                                 │
                                                                 ▼
                                                        (persisted atomically)
```
If `RefreshToken` returns `ErrSessionDead`, the account transitions directly to permanent `Disabled`. The refresh grant itself never retries — one-shot call.

### 7.3 Pool persistence
On process startup, `Pool.New(stateFp)` loads any previously persisted state from `state_file` without overwriting the in-memory auth credential objects that were freshly loaded from `auth_dir`. This preserves cooling/disabled history across restarts even when the auth JSON files are unchanged. `Cooldown`, `Disable`, and `NoteError` trigger an immediate atomic save to disk, so state survives a hard process kill. `NoteSuccess` only resets the in-memory `errCount` and does **not** persist (`internal/pool/pool.go:207-213`).

---

## 8. Error taxonomy and mapping table

Priority order matches `internal/upstream/client.go:76-107` exactly (lower number = earlier evaluation, higher priority):

| Priority | Class | HTTP status | trigger marker or condition | Proxy action | Anchor |
|---|---|---|---|---|---|
| 1 | `ErrHardCredit` | 402 Payment Required (any 4xx with marker in body) | status == 402 OR body contains any of the 17 hard markers (English + Chinese: `"insufficient credit"`, `"no credit"`, `"积分不足"`, …) | Cool the account for `HardCooldown` (default 12 h) | `internal/upstream/client.go:77-85`; `handler.go:173-177` |
| 2 | `ErrSessionDead` | any | body contains any of the 6 session-dead markers (`"OAuth token has been revoked"`, `"missing_login"`, …) OR status == 401 | Permanently disable the account | `client.go:86-93`; `handler.go:182-185` |
| 3 | `ErrSoftRate` | 429 Too Many Requests | status == 429 | Cool for `SoftCooldown` (default 60 s) | `client.go:94-96`; `handler.go:178-181` |
| 4 | `ErrNotFound` | 404 Not Found | status == 404 | Short 60 s cooldown, **does NOT increment consecutive error counter** | `client.go:97-99`; `handler.go:186-189` |
| 5 | `ErrServer` | 5xx | status >= 500 | Increment error counter; at threshold, cool for 10 m | `client.go:100-102`; `handler.go:190-194` |
| 6 | `ErrClient` | 400, 403, 422, etc. | status >= 400 (not already matched above) | Same as ErrServer — increment consecutive error counter | `client.go:103-105` |
| 7 | `ErrNone` | <400 success | status < 400 | Reset consecutive error counter to zero (`NoteSuccess`) | `client.go:106`; `pool.go:206-213` |

**Note (PRIOR-ART label):** Every status in this table describes the proxy's classifier logic, not a verified live observation from the target. No authenticated call was ever issued to live target endpoints during M1/M2. All 402/429/5xx rows are the proxy's expected behaviour, not confirmed upstream responses.

---

## 9. Security and compliance constraints

Derived directly from `00-governance.md`:

1. **No unauthorized access.** No attempt to bypass upstream authentication, forge tokens, perform privilege escalation, or read data beyond what the user's own authenticated account permits.
2. **No brute force or DoS.** No password guessing, API key enumeration, rate pressure exceeding the governance cap of 5 authenticated calls total, or any traffic that could reasonably be classified as denial-of-service.
3. **No secret leakage.** All L2 secrets (raw access_token, refresh_token, x-api-key, cookie values) are never written to any documentation, evidence, or fixture file. The document only ever records the placeholder `<token>` or `<api_key>`. The `auths/` directory is `.gitignore`'d and must never be committed.
4. **No exploitation.** No use of discovered vulnerabilities, no webshell deployment, no lateral movement across target infrastructure.
5. **UnTRUSTED repo policy.** The GitHub repository `https://github.com/witt3rd/oh-my-hermes` is treated as read-only plain text documentation. It is never cloned, installed, executed, or allowed to run any code locally.
6. **Write operations cap.** The only permitted authenticated write operation against the target is `POST /v1/messages` inference, up to 5 total calls. No other POST/PUT/DELETE action against authenticated surfaces is executed unless the user explicitly amends the governance rules.
7. **No CORS / browser exposure.** The gateway is not intended for direct browser-to-gateway calls from arbitrary origins. It is a server-side helper for automation and LAN deployments, not a public edge service.

---

## 10. Acceptance criteria summary — traceability matrix

| FR/NFR ID | Implementation files | Test file :: test name | Evidence / endpoint reference |
|---|---|---|---|
| FR-01 | `internal/upstream/payload.go`, `internal/upstream/sse.go:278-331`, `internal/server/handler.go:199-211` | `internal/upstream/sse_test.go :: TestAggregate_Basic`; `::TestAggregate_ToolCalls`; `::TestAggregate_Empty` | `/v1/chat/completions` non-stream |
| FR-02 | `internal/upstream/sse.go:121-186`, `internal/server/handler.go:199-203` | NO-TEST (handler-level, no httptest fixture) | `/v1/chat/completions` stream |
| FR-03 | `internal/server/handler.go:93-115` | NO-TEST | `/v1/models` |
| FR-04 | `internal/server/handler.go:82-85` | NO-TEST | `/healthz` |
| FR-05 | `internal/pool/pool.go:216-264`, `internal/server/handler.go:87-91` | `internal/pool/pool_test.go :: TestStatePersistence` (exercises `Status`; `List` itself is untested) | `/status` |
| FR-06 | `internal/server/handler.go:69-80` | NO-TEST | withAuth middleware 401 |
| FR-07 | `internal/pool/pool.go:126-166`, `internal/server/handler.go:131-138` | `internal/pool/pool_test.go :: TestPick_PrefersAPIKey`; `::TestPickExcluding_SkipsTried` | account rotation |
| FR-08 | `internal/pool/pool.go:168-178` | `internal/pool/pool_test.go :: TestCooldown_MakesUnhealthy` | Cooldown (hard) |
| FR-09 | `internal/pool/pool.go:168-178` | `internal/pool/pool_test.go :: TestCooldown_MakesUnhealthy` (exclusion only; the 60 s duration is not asserted) | Cooldown (soft) |
| FR-10 | `internal/pool/pool.go:191-213` | `internal/pool/pool_test.go :: TestNoteError_Threshold`; `::TestNoteSuccess_ResetsErrors` | NoteError / NoteSuccess |
| FR-11 | `internal/upstream/client.go:232-260`, `internal/auth/auth.go:36-42` | NO-TEST (no httptest fake upstream server) | proactive refresh (handler path) |
| FR-12 | `cmd/login/main.go` | NO-TEST (no test files in cmd/login package) | PKCE login CLI |
| FR-13 | `internal/upstream/client.go:311-329, 344-350` | NO-TEST | EnsureAPIKey + Bearer fallback |
| FR-14 | `internal/scheduler/scheduler.go:49-101` | NO-TEST | keepalive scheduler |
| FR-15 | `internal/upstream/sse.go:121-186, 188-276` | `internal/upstream/sse_test.go :: TestAggregate_Basic`, `::TestAggregate_ToolCalls`, `::TestAggregate_Empty` (aggregate path only; `Stream` is 0% covered → NO-TEST) | SSE translation |
| FR-16 | `internal/upstream/client.go:76-107` | NO-TEST (Classify has no unit test) | Error classification |
| FR-17 | `internal/server/handler.go:213-218` | NO-TEST | 503 all accounts exhausted |
| NFR-01..07 | (cross-cutting, scattered across all packages) | NO-TEST (non-functional requirements — measured by external benchmark and inspection) | — |

Count summary: **17 FRs** and **7 NFRs**.
- FRs with at least one named Go test touching the code path: **7** — FR-01, FR-05, FR-07, FR-08, FR-09, FR-10, FR-15.
- Fully covered by tests against their stated acceptance criteria: **1** — FR-10.
- Partially covered (a secondary path or a stated criterion is unasserted): **6** — FR-01 (`Aggregate` only, no handler/response assembly), FR-05 (`Status` only, `List` untested), FR-07 (pick preference/`tried` skipping only; rotation/retry loop in handler at 0%), FR-08 (`Cooldown` ignores `kind`; test never exercises `CoolHard`), FR-09 (exclusion asserted, 60 s duration not), FR-15 (`Aggregate` only, `Stream` untested).
- FRs with **no** test (`NO-TEST`): **10** — FR-02, FR-03, FR-04, FR-06, FR-11, FR-12, FR-13, FR-14, FR-16, FR-17.
- FRs with tests: **7/17 ≈ 41%** (fully covered: 1/17 ≈ 6%).
- NFRs with tests: **0/7**.
- Overall requirement-level test reach: **7/24 ≈ 29%**.

---

## 11. Known gaps and out-of-scope items

### 11.1 `BLOCKED-CRED` upstream verification register
All items in this list require the user to supply their own authenticated PhanthyCode token before they can be verified. No live authenticated call was issued during M1–M3:
| # | unknown | dependency |
|---|---|---|
| R1 | Exact auth scheme on `/v1/messages`: is `Authorization: Bearer` accepted in production or is `x-api-key` the only valid header? | User token |
| R2 | Non-streaming JSON response shape from upstream | User token |
| R3 | Exact SSE frame sequence on live inference (line breaks, event naming, chunk boundaries) | User token |
| R4 | Live authenticated error envelope shape (402/429/4xx/5xx) | User token |
| R5 | `/api/oauth/profile` success schema | User token |
| R6 | `/api/oauth/claude_cli/roles` success schema | User token |
| R7 | Token TTL (`expires_in` actual value, refresh token rotation behaviour, JWT claims) | User token + governance amendment allowing refresh grant call |
| R8 | Real API-key minting endpoint — both known paths 404 in production | User token |
| R9 | `/api/web/events` semantics (content type, auth, request schema) | Existence only confirmed; no further investigation performed |

### 11.2 Other gaps
- No handler-level integration tests using `httptest.Server` for the full chat completions flow.
- No unit tests exist for the `internal/upstream/client.go` package (Classify, RefreshToken, CreateAPIKey, ChatStream are all 0% covered).
- No tests at all for `internal/auth/`, `internal/scheduler/`, `internal/server/`, `cmd/*`.
- `go vet ./...` exits 0. `gofmt -l .` is **not** clean at baseline: it lists all 13 Go files (missing trailing newline plus struct/comment alignment differences). Fixing this is a prerequisite for a CI gate (see the TDD handbook §5).
- Behavioural gaps in the existing test suite: mid-stream `error` events are dropped by `handleEvent` (no `case "error"`), and the aggregation path synthesises an ID when the upstream stream is empty, so a failed call can look successful.

---

Document generated 2026-10-26.
