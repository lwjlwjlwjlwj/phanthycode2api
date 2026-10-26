# 02 · Usage Semantics — `https://code.phanthy.com/`

Milestone: **M2 / T2.2–T2.4** · Author: reverse-engineering analyst (M2) · Language: English
Companion document: `01-endpoint-catalog.md` (endpoint IDs `EP-01` … `EP-15` used throughout).

Grade vocabulary (identical to the catalog): `VERIFIED` = observed live in M1 with an evidence anchor;
`PRIOR-ART` = derived from the in-repo Go implementation / README only; `INFERRED` = reasoned from metadata
or convention, basis stated; `BLOCKED-CRED` = needs the user's own token, nothing fabricated.
**M2 issued no network requests.** Every live statement is inherited from the M1 capture window
`2026-10-26T05:58:24+08:00 → 06:14:37+08:00` (`M1-index.md:15`).

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
| `Authorization: Bearer <token>` | `/api/oauth/*`, `create_api_key`, fallback on `/v1/messages` | token | PRIOR-ART; guard VERIFIED on `/api/oauth/*` |
| `x-api-key: <api_key>` | `/v1/messages` (preferred) | key | PRIOR-ART `internal/upstream/client.go:345` |
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

`curl` against the upstream, with an API key:

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

Bearer variant (the documented fallback when no API key can be minted — `cmd/login/main.go:97-104`):

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
capturing, and the command must not be pointed at a file that persists secrets.

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

## 6. Model capability matrix

### 6.1 Mapping table

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

### 6.2 Capability statements and their basis

| capability | statement | grade / basis |
|---|---|---|
| context window | 200,000 tokens declared for every model | PRIOR-ART (`handler.go:95-107` — the proxy's own static list). No upstream evidence. |
| tool/function calling | all 13 models accept `tools` and emit `tool_use` streaming blocks | INFERRED: the proxy converts tools for every request (`payload.go:212-223`) and parses `tool_use` + `input_json_delta` (`sse.go:207-231, 251-263`); it applies no per-model gating |
| extended thinking | at least one upstream model emits `thinking_delta` blocks | INFERRED: a handler exists (`sse.go:245-250`). Which model, and whether it must be requested explicitly, is **unknown** (no `thinking` request field exists in `payload.go:47-60`) |
| vision / images | **not supported through the proxy** | PRIOR-ART: image parts are replaced by the text `"[image]"` (`payload.go:294-298`) |
| upstream model naming | `/v1/messages` expects the real IDs (`Iris-1.0`, …), not the commercial names | PRIOR-ART comment `payload.go:73-74`; no live confirmation |
| unknown names | passed through unchanged (no 400 from the proxy) | PRIOR-ART `payload.go:91-97` |

### 6.3 Availability caveats (why this table cannot be tightened without a token)

- `GET /v1/models` returns the **HTML 404 shell** (26,579 B), i.e. no model catalogue over HTTP
  (`M1-anon-probe-matrix.txt:73`; `M1-options-matrix.txt:25`).
- The 12 analysed public JS chunks contain **zero** matches for `Iris`, `Zeus`, `Gaia`, `Apollo`, `Metis`,
  `DeepSeek`, `Claude`, `Kimi`, `GLM` (`M1-js-static-analysis.txt:46-54, 97-123`).
- Model IDs are therefore obtainable only from (a) an authenticated `/v1/messages` response, or (b) vendor
  documentation. `M1-index.md:213-214` records exactly this gap.
- Do not present this matrix as a live catalogue in M4. Use the label `PRIOR-ART / unverified live` in the
  OpenAPI description and in the report.

---

## 7. Unknowns and verification plan (≤ 5 calls, when the user supplies a token)

### 7.1 Unknowns this plan targets

| id | unknown | needs |
|---|---|---|
| U1 | `/v1/messages` auth scheme: `x-api-key` vs `Authorization: Bearer` | token (and/or key) |
| U2 | non-streaming JSON response shape | token |
| U3 | exact SSE frame sequence, headers, termination | token |
| U4 | error envelope of an authenticated 4xx | token |
| U5 | `/api/oauth/profile` schema | token |
| U6 | `/api/oauth/claude_cli/roles` schema | token |
| U7 | token TTL / `expires_in` / refresh rotation | token + `POST /oauth/token` — **excluded below** (governance) |

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

Endpoints that must **not** appear as operations in M4 (verified absent, HTML 404): `/v1/models`,
`/v1/messages/count_tokens`, `/api/health`, `/healthz`, `/api/oauth/phanthy_cli/create_api_key`,
`/api/oauth/claude_cli/create_api_key` (catalog §4.1). Their absence is itself a documented finding: the
access-token fallback (`internal/upstream/client.go:344-350`) is the only viable `/v1/messages` credential path
until a token proves otherwise.

---

### Document provenance

Authored `2026-10-26` from the M1 evidence package only; zero network requests; no credential, cookie or token
value read, copied or written. The M1 acceptance report's numeric caveats (D1–D8, catalog §1.6) are honoured
throughout: no probe totals, no byte-for-byte OIDC comparison, no JS aggregate size, and the 405/`Allow`
statement is scoped to API routes.
