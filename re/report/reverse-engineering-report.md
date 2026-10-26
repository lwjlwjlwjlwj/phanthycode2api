# Reverse-Engineering Report — `https://code.phanthy.com`

Milestone: **M4 (API documentation & reporting)** · Language: English · Status: complete
Target: `https://code.phanthy.com` · Capture window (inherited): `2026-10-26T05:58:24+08:00` → `2026-10-26T06:14:37+08:00`

Anchor convention: `M0-*` and `M1-*` anchors resolve under `/workspace/re/evidence/`; `docs/*` under
`/workspace/re/docs/`; `acceptance/*` under `/workspace/re/acceptance/`; Go/README anchors under `/workspace/`.
All live statements are inherited from the M1 anonymous capture; **M4 issued zero requests to the target**.
Authenticated behaviour is `PRIOR-ART` / `BLOCKED-CRED` throughout.

---

## 1. Executive summary

`code.phanthy.com` is a Next.js App Router application behind a Kong 2.5.1 gateway and TencentEdgeOne CDN that
exposes two distinct API families under one origin: a **Claude-Code-compatible OAuth 2.0 + PKCE** surface
(`/oauth/*`, `/api/oauth/*`, `/.well-known/*`) and an **Anthropic Messages-compatible inference** surface
(`POST /v1/messages`). A small public metadata/asset surface (`/api/download/client`, `/api/public-assets/`,
`/robots.txt`, `/sitemap.xml`) and the HTML app shell complete the observed footprint.

Fifteen endpoints are verified to exist; six probed paths are verified absent (Next.js HTML 404 shell)
`[M1-index.md:76-80]`. The OAuth metadata is internally consistent and fully readable: authorization-code only,
PKCE `S256` only, public client (`token_endpoint_auth_methods_supported: ["none"]`), five scopes, RS256 JWKS
`[M1-openid-configuration.json:7-26]` `[M1-jwks.json:22]`.

The central limitation is credential-gated: **the auth scheme and the success bodies of `POST /v1/messages` were
never observed**. The prior-art implementation prefers `x-api-key: <api_key>` and falls back to
`Authorization: Bearer <token>` `[internal/upstream/client.go:344-350]`; both the API-key minting routes return
the HTML 404 shell `[M1-anon-probe-matrix.txt:67-68]`, so the fallback is the only documented viable credential
path. This report therefore separates `VERIFIED` target facts from `PRIOR-ART` wire assumptions and `INFERRED`
protocol reasoning, and never presents a live inference response as observed.

Deliverables produced in M4:

| deliverable | path | state |
|---|---|---|
| Apifox-importable OpenAPI 3.1.0 bundle | `/workspace/re/apifox/openapi.json` | validated: 15 paths / 15 operations / 13 schemas / 21 local `$ref`s |
| Import & operations guide | `/workspace/re/apifox/IMPORT.md` | complete; runtime push remains UNVERIFIED |
| Optional push/validator CLI (stdlib only) | `/workspace/re/apifox/push_apifox.py` | `--self-test` exits 0 |
| This report | `/workspace/re/report/reverse-engineering-report.md` | 12 sections |

---

## 2. Scope, authorization and governance

**Authorization.** The engagement is limited to **read-only reverse engineering** of the user-designated target,
with optional authenticated probing only when the user supplies their own test account/token. The governing
document is `docs/00-governance.md`, which states the authorization source, the credential precondition, and the
prohibition on substituting guessed or third-party credentials `[docs/00-governance.md:8-20]`. In practice no
credential was ever provided: `authenticated_test_account: NOT PROVIDED` and `apifox_credentials: ABSENT`
`[M0-preflight.json:18-19]`. Every authenticated observation therefore resolves to `BLOCKED-CRED`.

**Hard prohibitions observed.** No auth bypass, no token fabrication, no brute force or credential stuffing, no
DoS/rate stress, no persistence of real credentials, no exploitation, and no write operations — with a single
governance-sanctioned exception for `POST /v1/messages` using the user's own token `[docs/00-governance.md:11-20]`.
Anonymous probing was capped at ≤1 req/s and ≤20 requests per endpoint; authenticated verification at ≤5 calls
`[docs/00-governance.md:20]`. M1 recorded 54 total requests with ≥1.1 s spacing and a per-endpoint maximum of 5
`[M1-anon-probe-matrix.txt:8, 13]`.

**Untrusted repository.** `https://github.com/witt3rd/oh-my-hermes` was treated as UNTRUSTED: plain-text reading
only, never cloned, installed, or executed, and none of its operational instructions adopted
`[docs/00-governance.md:22-32]` `[M0-preflight.json:20]`.

**Data grading.** L0 (public metadata, status codes, headers, routes) is retained verbatim; L1 (structures,
error codes, model names) is redacted into report/OpenAPI; L2 (`Authorization`, `x-api-key`, `refresh_token`,
cookies) is **never persisted** — only "present/absent" is recorded `[docs/00-governance.md:34-40]`. The report
and OpenAPI contain only `<api_key>`, `<token>`, `<access_token>` placeholders. Cookie attributes are documented
by name/`Max-Age` only; values were redacted in the evidence `[M1-anon-probe-matrix.txt:346-347]`.

**Milestone-4 write discipline.** M4 created exactly the four permitted files and modified no existing
evidence, document, acceptance report, or source file. M4 contacted only `docs.apifox.com` / `apifox.com` to
verify the import endpoint; it issued **zero** requests to `code.phanthy.com`.

---

## 3. Methodology

1. **Anonymous recon.** Nine M1 artifacts were read end to end: root headers, OPTIONS matrix, OIDC metadata,
   JWKS, JS static analysis, JS chunk provenance, anonymous probe matrix, index, and M0 preflight. Identity was
   fully anonymous — no `Authorization`, no `Cookie`, no `x-api-key` `[M1-anon-probe-matrix.txt:6]`. All traffic
   egressed through the sandbox proxy `http://127.0.0.1:18080` over HTTP/2; direct TCP to the origin was blocked
   `[M1-anon-probe-matrix.txt:7]`. The leading `HTTP/1.1 200 OK` in each dump is the proxy CONNECT handshake, not
   an origin response `[M1-anon-probe-matrix.txt:39-44]`.
2. **OPTIONS existence matrix.** Because API-route 405s omit `Allow`, `OPTIONS` is the only reliable method
   oracle `[M1-anon-probe-matrix.txt:447-448]`. `204 + allow` proves a handler exists; `404 + text/html` proves
   route absence `[M1-options-matrix.txt:7-10]`. Cross-checking the matrix against live GET/HEAD produced no
   contradictions `[M1-anon-probe-matrix.txt:441-448]`.
3. **OIDC metadata.** The discovery document was fetched and field-compared against the stored JSON; only
   field-level equality is claimed because the served body is minified and the stored file is pretty-printed
   `[M1-anon-probe-matrix.txt:450-469]`.
4. **Static JS analysis.** Twelve public Next.js chunks were downloaded and text-scanned without execution. They
   contain only three endpoint strings (`/api/download/client`, `/api/web/events`, `/api/public-assets/`) and
   **zero** matches for `/oauth/`, `x-api-key`, `authorization`, `anthropic-version`, `anthropic-beta`, or any
   model family `[M1-js-static-analysis.txt:56-69, 84-96, 97-124]`. The frontend therefore cannot be used as a
   protocol oracle.
5. **Prior-art cross-check.** The in-repo Go client (`cmd/login`, `internal/upstream`) was read as prior art for
   the authenticated and POST-only contracts. It constrains the *client's* behaviour, never the *target's*.
6. **OpenAPI modelling and validation.** Every operation was modelled with an explicit evidence grade; shared
   structures use `$ref`; observed status codes only; no success body was invented. The bundle was validated
   locally (`json.load` + a stdlib validator) before this report was written.

**Evidence grades.** `VERIFIED` = observed live in M1 with a file:line anchor; `PRIOR-ART` = derived only from
in-repo code/README; `INFERRED` = reasoned from metadata/RFC/naming with the basis stated; `BLOCKED-CRED` =
requires the user's own token, nothing fabricated `[docs/01-endpoint-catalog.md:50-58]`.

**Corrections carried into this report.** The M3 acceptance review found two overstated statistics in the SDD
that a final report must not quote `[acceptance/M3.md:34-42, 229-242]`. This report uses the independently
computed figures: **FRs fully covered = 1 (FR-10), partial = 6, `NO-TEST` = 10, reach = 7/17 = 41.2%**, and
**17 hard error markers, 6 session-dead markers** `[acceptance/M3.md:179-192]`. The SDD's "3 fully covered" and
"16 hard markers" are **not** repeated.

---

## 4. Target fingerprint

| layer | observed | evidence |
|---|---|---|
| edge / WAF | `server: TencentEdgeOne`, `eo-log-uuid`, `eo-cache-status` | `M1-root-headers.txt:16-20` |
| gateway | `via: kong/2.5.1`, `x-kong-upstream-latency`, `x-kong-proxy-latency` | `M1-root-headers.txt:13-15` |
| application | `x-powered-by: Next.js`; App Router (RSC `vary`, `x-nextjs-cache`, `/_next/static/*`) | `M1-root-headers.txt:10-12` `[M1-anon-probe-matrix.txt:89-100]` |
| transport security | HSTS only: `strict-transport-security: max-age=31536000; includeSubDomains` | `M1-root-headers.txt:9` |
| caching | root `cache-control: private, must-revalidate, no-cache, no-store, max-age=0`; metadata `x-nextjs-cache: HIT` | `M1-root-headers.txt:17` `[M1-anon-probe-matrix.txt:276, 285]` |
| artifact storage | Tencent COS bucket `agi-phanthy-dev-1252788780`, `cos.ap-beijing.myqcloud.com` | `M1-anon-probe-matrix.txt:345` |
| language / locale | `html dir="ltr" lang="zh-CN"`; 404 shell carries the same locale | `M1-anon-probe-matrix.txt:59, 67` |
| error envelope | `{"error":{"code":"...","message":"..."}}` in all four JSON error bodies | `M1-anon-probe-matrix.txt:60, 66, 76, 365` |

The public root document is 56,923 bytes of streamed HTML (no `content-length`)
`[M1-root-headers.txt:27]` `[M1-anon-probe-matrix.txt:59]`. The infrastructure header set (`x-kong-*`, `eo-*`,
`via`, `server`) is the only operational disclosure; no `RateLimit-*`, `Retry-After`, or quota headers were ever
observed `[M1-index.md:210-212]`.

---

## 5. AI endpoint surface

### 5.1 Verified endpoint inventory (15 operations)

| # | method | path | auth (observed / prior art) | purpose | evidence grade | anchor |
|---|---|---|---|---|---|---|
| 1 | GET | `/` | none | Next.js app shell (HTML) | VERIFIED | `M1-anon-probe-matrix.txt:59` |
| 2 | GET | `/oauth/authorize` | none to reach handler; browser login to complete | Authorization-code + PKCE entry | VERIFIED (existence + error text) / PRIOR-ART (param set) | `M1-anon-probe-matrix.txt:60, 112` |
| 3 | POST | `/oauth/token` | public client, no secret | Code exchange + refresh grant | VERIFIED (existence) / PRIOR-ART (body) / BLOCKED-CRED (exchange) | `M1-options-matrix.txt:16` |
| 4 | POST | `/oauth/revoke` | INFERRED public client | RFC 7009 revocation | VERIFIED (existence) / INFERRED (semantics) | `M1-options-matrix.txt:17` |
| 5 | GET | `/oauth/code/success` | none | Static "login complete" landing page | VERIFIED | `M1-anon-probe-matrix.txt:65` |
| 6 | GET | `/.well-known/openid-configuration` | none | OAuth/OIDC discovery | VERIFIED (field level) | `M1-openid-configuration.json:2-26` |
| 7 | GET | `/.well-known/jwks.json` | none | RS256 signing-key set | VERIFIED | `M1-jwks.json:22` |
| 8 | GET | `/api/oauth/profile` | `Authorization: Bearer <access_token>` | Account profile | VERIFIED (guard) / BLOCKED-CRED (success schema) | `M1-anon-probe-matrix.txt:66, 154-163` |
| 9 | GET | `/api/oauth/claude_cli/roles` | `Authorization: Bearer <access_token>` | Role/entitlement lookup | VERIFIED (guard) / BLOCKED-CRED (semantics) | `M1-anon-probe-matrix.txt:69` |
| 10 | POST | `/v1/messages` | `x-api-key` **or** `Authorization: Bearer` (PRIOR-ART) | Anthropic Messages inference (+ SSE) | VERIFIED (existence) / PRIOR-ART (wire) / BLOCKED-CRED (auth + live body) | `M1-options-matrix.txt:23` `[internal/upstream/client.go:344-350]` |
| 11 | POST | `/api/web/events` | unknown | Event ingestion/stream of unknown semantics | VERIFIED (existence) / INFERRED only (everything else) | `M1-options-matrix.txt:26` |
| 12 | GET | `/api/download/client` | none | Client artifact download; mints visitor cookies | VERIFIED | `M1-anon-probe-matrix.txt:337-366` |
| 13 | GET | `/api/public-assets/` (308 → `/api/public-assets`) | unknown | Public-asset surface; target never followed | VERIFIED (redirect only) / INFERRED (target) | `M1-anon-probe-matrix.txt:247-255` |
| 14 | GET | `/robots.txt` | none | Crawler policy | VERIFIED | `M1-anon-probe-matrix.txt:369-401` |
| 15 | GET | `/sitemap.xml` | none | SEO sitemap, 841 URLs | VERIFIED | `M1-anon-probe-matrix.txt:404-438` |

Method matrix: four POST-only paths (`/oauth/token`, `/oauth/revoke`, `/v1/messages`, `/api/web/events`), five
read-only handlers, and two GET/HEAD-only static metadata files `[M1-options-matrix.txt:13-32]`.

### 5.2 Verified absent (six paths, HTML 404 shell)

`/api/oauth/phanthy_cli/create_api_key`, `/api/oauth/claude_cli/create_api_key`, `/v1/messages/count_tokens`,
`/v1/models`, `/api/health`, `/healthz` `[M1-index.md:76-80]`. All return `404 text/html`, ~26.5 KB, with
`x-powered-by: Next.js` and no JSON envelope `[M1-anon-probe-matrix.txt:67-79]`. These paths are **excluded from
the OpenAPI document** and listed in `info.description` as "verified absent" (`x-verified-absent-paths`).

### 5.3 OpenAPI modelling decisions

- **Document:** OpenAPI `3.1.0`, server `https://code.phanthy.com`, seven tags (`OAuth`, `OIDC`, `Account`,
  `Inference`, `Web`, `Assets`, `Meta`), 15 operations, 13 shared schemas, 21 internal `$ref`s.
- **Security:** `apiKeyAuth` (`apiKey`, header, `x-api-key`) and `bearerAuth` (`http`, `bearer`), both offered as
  alternatives on `messagesCreate` only (`security: [{apiKeyAuth: []}, {bearerAuth: []}]`).
- **`messagesCreate`:** Anthropic Messages request body (`model`, `max_tokens`, `messages[]`, `stream`, `system`,
  `temperature`, `top_p`, `tools`, `tool_choice`, `metadata`); two 200 media types — `application/json` (loose
  prior-art schema marked "never observed") and `text/event-stream` (named SSE examples). No exact success JSON was
  invented.
- **`webEventsCreate`:** one `default` response whose description states the contract is unverified; no request
  body, no security, `x-contract-status: "unverified"`.
- **`clientDownloadGet`:** `platform` enum contains only the proven `linux`; the observed `darwin` rejection is
  documented in the parameter description and the 400 response.
- **`publicAssetsGet`:** models only the observed 308; no downstream body.
- **`profileGet` / `claudeCliRolesGet`:** 200 responses are description-only (no schema, no example); the observed
  401 `missing_login` envelope is fully specified.
- **`/oauth/token` and `/oauth/revoke`:** `application/x-www-form-urlencoded` bodies with the prior-art/RFC 7009
  field sets; exact prior-art `grant_type` values.
- **`/oauth/authorize`:** seven query parameters, `code_challenge_method` enum `[S256]`, scope example
  `user:inference user:profile user:sessions:claude_code`.
- **Error envelope:** one shared `ErrorEnvelope` (`error.code`, `error.message`) plus `ErrorDetail`, used by every
  operation that has an observed JSON error. No JSON 404 taxonomy is modelled, because none exists.

---

## 6. Authentication and OAuth/OIDC architecture

### 6.1 Flow (PKCE, public client)

The discovery document is OAuth 2.0 metadata under an OIDC file name: `response_types_supported=["code"]`,
`grant_types_supported=["authorization_code","refresh_token"]`, `code_challenge_methods_supported=["S256"]`,
`token_endpoint_auth_methods_supported=["none"]` `[M1-openid-configuration.json:7-19]`. There is **no**
`userinfo_endpoint`, no `id_token_signing_alg_values_supported`, no `subject_types_supported`, and no `openid`
scope — verified by the complete field list `[M1-openid-configuration.json:2-26]` `[M1-index.md:114-128]`.

Observed/prior-art sequence:

1. Client generates a random verifier and `code_challenge = base64url(SHA-256(verifier))`
   `[cmd/login/main.go:51-56]`.
2. `GET /oauth/authorize` with `client_id`, `response_type=code`, `redirect_uri`, `scope`, `code_challenge`,
   `code_challenge_method=S256`, `state` `[cmd/login/main.go:144-154]`. Missing `response_type` produces
   `400 {"error":{"code":"unsupported_response_type","message":"Only authorization code flow is supported."}}`
   `[M1-anon-probe-matrix.txt:60, 112]`.
3. Browser login/consent — `BLOCKED-CRED`.
4. Redirect to `https://code.phanthy.com/oauth/code/success?code=<code>&state=<state>` — `INFERRED`
   (RFC 6749 §4.1.2) `[docs/02-usage-semantics.md:40]`.
5. `POST /oauth/token` (form-urlencoded) with `grant_type=authorization_code`, `code`, `code_verifier`,
   `redirect_uri`, `client_id=phanthy-code-cli` `[cmd/login/main.go:177-193]`.
6. `200 {access_token, refresh_token, expires_in}` — prior-art shape only; actual body `BLOCKED-CRED`
   `[cmd/login/main.go:171-175, 208-218]`.
7. Refresh: `POST /oauth/token` with `grant_type=refresh_token`, `refresh_token`, `client_id`
   `[internal/upstream/client.go:238-245]`.

The client identity `phanthy-code-cli` is prior art `[cmd/login/main.go:28, 146]`; prior art also sends
`anthropic-beta: oauth-2025-04-20` on OAuth calls `[cmd/login/main.go:193]` `[internal/upstream/client.go:139, 243]`
— requiredness is INFERRED, never verified `[docs/02-usage-semantics.md:62-69]`.

### 6.2 Scopes

Five scopes are advertised `[M1-openid-configuration.json:20-26]`:

| scope | use |
|---|---|
| `user:profile` | requested by prior art; profile access |
| `user:inference` | requested by prior art; `/v1/messages` |
| `user:sessions:claude_code` | requested by prior art; Claude Code session semantics |
| `user:mcp_servers` | advertised, never requested — purpose unknown |
| `user:file_upload` | advertised, never requested — purpose unknown |

### 6.3 Profile guard, JWKS and token format

`GET /api/oauth/profile` and `GET /api/oauth/claude_cli/roles` share the same guard and a byte-identical 79-byte
401 body `[M1-anon-probe-matrix.txt:66, 69]`:

```
WWW-Authenticate: Bearer realm="phanthy-code", error="invalid_request",
                  error_description="Bearer access token is required."
{"error":{"code":"missing_login","message":"Bearer access token is required."}}
```

JWKS contains exactly one RSA key: `kty=RSA`, `e=AQAB`, `use=sig`, `alg=RS256`, `kid=configured-key`
`[M1-jwks.json:22, 29-32]`. The key material implies a possible signed-JWT access token, but the token format,
claims, `exp`, and rotation are `BLOCKED-CRED` — `kid=configured-key` is a default-style identifier and key
rotation is not demonstrated `[M1-index.md:126-128]`.

### 6.4 Missing API-key minting routes and the Bearer fallback

Both `/api/oauth/phanthy_cli/create_api_key` and `/api/oauth/claude_cli/create_api_key` return the Next.js HTML 404
shell `[M1-anon-probe-matrix.txt:67-68]` `[M1-options-matrix.txt:20-21]`. The prior art treats a 404 on the
`phanthy_cli` route as "no API key available" rather than a hard error `[cmd/login/main.go:97, 221-251]`, and then
calls `POST /v1/messages` with `Authorization: Bearer <access_token>` **instead of** `x-api-key`
`[internal/upstream/client.go:344-350]` `[cmd/login/main.go:103-104]`. If a key were ever returned, prior art
expects it at `data.raw_key` (fallbacks `data.api_key`, `api_key`) `[cmd/login/main.go:256-269]`.

**Statement of intent, not a live fact:** on this target the access token is the documented `/v1/messages`
credential. Whether `/v1/*` actually accepts Bearer is `BLOCKED-CRED`, and the OpenAPI document therefore models
both schemes as alternatives rather than asserting one.

---

## 7. Inference protocol (`POST /v1/messages`)

### 7.1 Request contract (PRIOR-ART)

Existence is verified — `OPTIONS` returns `204` with `Allow: OPTIONS, POST`, and GET/HEAD return `405`
`[M1-options-matrix.txt:23]` `[M1-anon-probe-matrix.txt:70-71]`. The POST itself was never issued.

| field | required | type / behaviour | anchor |
|---|---|---|---|
| `model` | yes | client alias or upstream ID; unknown names pass through | `internal/upstream/payload.go:75-97` |
| `max_tokens` | yes in practice | always serialized; proxy clamps outside 1..65536 to 8192 | `internal/upstream/payload.go:47-60, 117-119` |
| `messages` | yes | array of `{role, content}`; `content` may be a string or block array | `internal/upstream/payload.go:47-60` |
| `system` | no | string; prior art joins system/developer messages | `internal/upstream/payload.go:141-147` |
| `temperature` / `top_p` | no | number, passed through | `internal/upstream/payload.go:121-126` |
| `stream` | no | boolean; prior art forces `true` | `internal/upstream/payload.go:109` |
| `tools` | no | `{name, description, input_schema}` | `internal/upstream/payload.go:306-335` |
| `tool_choice` | no | `none` / `auto` / `any` / `{type:"tool",name}` | `internal/upstream/payload.go:337-380` |
| `stop_sequences` | no | string array | `internal/upstream/payload.go:127-129` |
| `metadata` | no | object, prior art sets `{user_id: ...}` | `internal/upstream/payload.go:130-132` |

### 7.2 Headers (PRIOR-ART)

`Accept: application/json, text/event-stream`, `x-app: cli`, `User-Agent: phanthycode2api/1.0`, and either
`x-api-key: <api_key>` or `Authorization: Bearer <token>` `[internal/upstream/client.go:341-350]`. An
`X-Claude-Code-Session-Id` header is a proxy-side artifact, not protocol requirement `[internal/upstream/client.go:351-353]`.
No `anthropic-version` header is sent anywhere, and the public JS contains zero occurrences of it
`[M1-js-static-analysis.txt:91-95]` — whether the target tolerates its absence is INFERRED, not verified.

### 7.3 Streaming (PRIOR-ART)

The prior-art parser reads line-oriented SSE, considers only `data: ` lines, ignores `event:` names entirely, and
treats the JSON `type` field as authoritative `[internal/upstream/sse.go:148-168]`. Event sequence understood:

`message_start` → `content_block_start` (text or `tool_use`) → `content_block_delta`
(`text_delta` / `thinking_delta` / `input_json_delta`) → `message_delta` → `message_stop`
`[internal/upstream/sse.go:188-276]` `[docs/02-usage-semantics.md:183-198]`.

`content_block_stop` is not handled; an `error` event is parsed into a struct but has **no handler branch**, so a
mid-stream error is silently discarded `[internal/upstream/sse.go:44-47, 188-276]`. Termination is driven by EOF,
not by `message_stop`, after which the proxy appends its own `[DONE]` `[internal/upstream/sse.go:162-184]`.
Consumer defence: treat a stream with no `content_block_delta` and no `stop_reason` as a failure and keep the
pre-stream HTTP status as the only reliable error signal `[docs/02-usage-semantics.md:216-227]`.

The OpenAPI `text/event-stream` response carries named examples (`messageStart`, `contentBlockStart`,
`contentBlockDelta`, `messageDelta`, `messageStop`, `errorEvent`) but explicitly marks the wire contract prior-art.

### 7.4 Error taxonomy: observed vs prior-art

Only four JSON error envelopes were ever observed on the target:
`unsupported_response_type` `[M1-anon-probe-matrix.txt:112]`, `missing_login` `[:163, :190]`, and
`invalid_platform` `[:245, :365]` — all shaped `{"error":{"code","message"}}`. **No `402`, `429`, or `5xx` was ever
observed** `[docs/02-usage-semantics.md:283-290]`. The prior-art classifier (402 → hard credit; hard markers;
session markers; 401; 429; 404; ≥500; ≥400) is a statement about the proxy, not the target
`[internal/upstream/client.go:76-107]`. The OpenAPI document therefore documents only the observed error envelope
and each observed status, with no invented authenticated error codes.

---

## 8. Model roster — PRIOR-ART / unverified live

Model IDs cannot be obtained from HTTP: `GET /v1/models` returns the HTML 404 shell
`[M1-anon-probe-matrix.txt:73]`, and the 12 analysed public JS chunks contain **zero** matches for every probed
family `[M1-js-static-analysis.txt:97-123]`. The table below is therefore **not a live catalogue**; it is the
in-repo alias map `[internal/upstream/payload.go:75-89]` cross-checked against `README.md:120-134`.

| # | client-facing name | upstream model ID | family | grade |
|---|---|---|---|---|
| 1 | `DeepSeek-V4` | `Iris-1.0` | Iris | PRIOR-ART / unverified live |
| 2 | `gpt-5.6-sol` | `Zeus-1.1-pro` | Zeus | PRIOR-ART / unverified live |
| 3 | `gpt-5.6-terra` | `Zeus-1.1` | Zeus | PRIOR-ART / unverified live |
| 4 | `gpt-5.6-luna` | `Zeus-1.1-fast` | Zeus | PRIOR-ART / unverified live |
| 5 | `gpt-5.5` | `Zeus-1.0-pro` | Zeus | PRIOR-ART / unverified live |
| 6 | `Claude Opus 4.8` | `Gaia-1.2` | Gaia | PRIOR-ART / unverified live |
| 7 | `Claude Opus 4.7` | `Gaia-1.1` | Gaia | PRIOR-ART / unverified live |
| 8 | `Claude Sonnet 4.6` | `Gaia-1.0` | Gaia | PRIOR-ART / unverified live |
| 9 | `Kimi K3` | `Apollo-2.0` | Apollo | PRIOR-ART / unverified live |
| 10 | `Kimi-k2.7-code` | `Apollo-1.1` | Apollo | PRIOR-ART / unverified live |
| 11 | `Kimi K2.6` | `Apollo-1.0` | Apollo | PRIOR-ART / unverified live |
| 12 | `GLM 5.2` | `Metis-1.1` | Metis | PRIOR-ART / unverified live |
| 13 | `GLM-5.1` | `Metis-1.0` | Metis | PRIOR-ART / unverified live |

Capability notes are equally unverified: the 200,000-token context figure is the proxy's own static list
`[internal/server/handler.go:93-108]`; tool calling is inferred from unconditional tool conversion and parsing;
vision is explicitly **not** supported through the proxy (images become the literal text `"[image]"`)
`[internal/upstream/payload.go:294-298]` `[docs/02-usage-semantics.md:359-368]`. The OpenAPI model field is a free
string with the alias table in its description, not an `enum`.

---

## 9. Security observations and risk rating

All findings are **non-exploitative observations**; none was exercised beyond a harmless anonymous request. Risk
is rated relative to the target's own deployment, not as an invitation to test.

| # | observation | rating | rationale / anchor |
|---|---|---|---|
| S1 | Only HSTS is sent; no `Content-Security-Policy`, `X-Content-Type-Options`, `X-Frame-Options`, or `Referrer-Policy`; `x-powered-by: Next.js` discloses the stack | **Medium** | Missing defence-in-depth headers widen the impact of any future content-injection bug; stack disclosure lowers reconnaissance cost. Observed across every response `[M1-root-headers.txt:7-20]` `[M1-index.md:185-189]`. |
| S2 | `405` on API routes carries **no `Allow` header**, contrary to RFC 9110 | **Low** | Causes clients to probe OPTIONS to discover methods; no direct security impact `[M1-anon-probe-matrix.txt:192-198, 447-448]`. |
| S3 | Non-existent API paths return a ~26.5 KB HTML 404 shell instead of JSON | **Low** | Makes route enumeration less precise for clients but does not hide the surface from `robots.txt`; also prevents a machine-readable 404 taxonomy `[M1-anon-probe-matrix.txt:67-79]`. |
| S4 | `robots.txt` explicitly enumerates the private surface it wants skipped (`/api`, `/oauth`, `/v1`, `/admin`, `/account`, `/login`, `/payment`, `/app`, `/docs`, password flows) | **Low** | Publishes an attacker's to-do list, though these paths are already discoverable; deliberate SEO choice `[M1-anon-probe-matrix.txt:369-401]`. |
| S5 | `sitemap.xml` lists 841 unique content URLs and contains no API/OAuth/admin locations | **Low (positive)** | No sensitive-path leakage in the sitemap; verified `0` occurrences of `/api/`, `/oauth/`, `/v1/`, `/admin` `[M1-anon-probe-matrix.txt:404-438]`. |
| S6 | `GET /api/download/client?platform=linux` mints two visitor cookies (`phanthy_vid`, 90 d; `phanthy_vsid`, 30 min; `Secure; HttpOnly; SameSite=lax`) on an anonymous GET | **Low** | A read-shaped endpoint has a session side effect; cookies are scoped and flagged, but a state change on GET complicates caching/CSRF reasoning `[M1-anon-probe-matrix.txt:346-354]`. |
| S7 | JWKS `kid` is the literal `configured-key`, with a single RSA key | **Low** | Suggests a default deployment identifier; key rotation/rolling is not demonstrated, which is an operational hygiene concern, not a break `[M1-jwks.json:22]`. |
| S8 | Unauthenticated `401` discloses `realm="phanthy-code"` and an `error_description` | **Low** | Conventional and minimal; helps clients, reveals only the product name `[M1-anon-probe-matrix.txt:157]`. |
| S9 | `POST /api/web/events` exists with an entirely unknown contract, called from the public site shell | **Medium** | Unknown input surface reachable anonymously in the sense that its auth is unverified; four competing hypotheses and no response capture `[M1-options-matrix.txt:26]` `[M1-js-static-analysis.txt:63]`. |
| S10 | No CORS response headers for an arbitrary third-party `Origin`; `vary` never gains `origin` | **Low (positive)** | Cross-origin browser reads are blocked; whether an allowlist exists is not established. Limits browser integrations but prevents casual cross-origin abuse `[M1-anon-probe-matrix.txt:291-335]`. |
| S11 | API-key minting routes are absent while `claude_cli/roles` retains a bearer guard | **Low** | Surface inconsistency; the guard still holds `[M1-anon-probe-matrix.txt:67-69]`. |
| S12 | Rate-limit/quota response shapes are unknown (no `RateLimit-*` headers ever observed) | **Medium** | Client-side backoff cannot be specified from evidence; any cooldown numbers in the SDD are prior-art policy only `[M1-index.md:210-212]` `[docs/02-usage-semantics.md:294-331]`. |

No finding in this section is a confirmed vulnerability. Items S1/S9/S12 are rated Medium because they concern
missing hardening or unknown contracts; the remainder are Low. Nothing here justifies exploitation, and none was
attempted.

---

## 10. Compliance and ethics statement

This work was **read-only** and **credential-free**. No authentication was bypassed, no token was fabricated, no
password or API key was guessed, and no rate limits were stressed. Anonymous probing respected ≤1 req/s and
≤20 requests per endpoint `[docs/00-governance.md:20]`; the recorded capture used ≥1.1 s spacing and at most 5
requests on any path `[M1-anon-probe-matrix.txt:8, 13]`. No L2 value was written to disk: cookie values were
redacted at capture time `[M1-anon-probe-matrix.txt:346-347]`, and this report and the OpenAPI bundle contain only
`<api_key>` / `<token>` placeholders. No real user data was accessed or exported. The UNTRUSTED repository was
read as plain text only and never cloned, installed, or executed `[docs/00-governance.md:22-32]`. Authenticated
behaviour was not guessed or simulated: it is labelled `PRIOR-ART` or `BLOCKED-CRED` everywhere. All findings are
reported non-exploitatively and are limited to what an anonymous observer could see.

---

## 11. Reproducibility

### 11.1 Environment and egress

Direct TCP to the origin is blocked in the sandbox; all requests must traverse the HTTP proxy recorded by M1
(`HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:18080`, HTTP/2 to origin) `[M1-anon-probe-matrix.txt:7]`. Tools used:
curl 8.5.0, Python 3.14.8 `[M1-anon-probe-matrix.txt:9-12]`. Do not read the leading proxy `HTTP/1.1 200 OK` as
an origin status `[M1-anon-probe-matrix.txt:39-44]`.

### 11.2 Recon commands (anonymous, read-only)

```bash
# app shell + infrastructure headers
curl -sS --http2 -D - -o /dev/null https://code.phanthy.com/

# method oracle (do not use 405 Allow headers on API routes)
curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/v1/messages

# OAuth/OIDC metadata
curl -sS --http2 https://code.phanthy.com/.well-known/openid-configuration
curl -sS --http2 https://code.phanthy.com/.well-known/jwks.json

# observed 401 guard
curl -sS --http2 -D - https://code.phanthy.com/api/oauth/profile

# download branch: 302 (linux) vs 400 (darwin)
curl -sS --http2 -i 'https://code.phanthy.com/api/download/client?platform=linux'
curl -sS --http2 -i 'https://code.phanthy.com/api/download/client?platform=darwin'

# static metadata
curl -sS --http2 https://code.phanthy.com/robots.txt
curl -sS --http2 https://code.phanthy.com/sitemap.xml | sha256sum
```

Expected digests: `robots.txt` sha256 `e9df3e77…bda64134`; `sitemap.xml` sha256 `1c1f5014…68b812`
`[M1-anon-probe-matrix.txt:371, 407]`.

### 11.3 OpenAPI validation commands

```bash
python3 -c "import json; json.load(open('/workspace/re/apifox/openapi.json')); print('json ok')"
python3 /workspace/re/apifox/push_apifox.py --self-test
```

Recorded results: `json ok 3.1.0 15`; self-test reports `openapi: 3.1.0`, `paths: 15`, `operations: 15`,
`schemas: 13`, `local $refs: 21`, tags `Account, Assets, Inference, Meta, OAuth, OIDC, Web`, all 15 operationIds,
and `SELF-TEST OK` (exit 0).

### 11.4 Evidence pointers

| topic | evidence |
|---|---|
| target stack and headers | `M1-root-headers.txt:7-20`; `M1-anon-probe-matrix.txt:87-101` |
| method matrix | `M1-options-matrix.txt:13-32, 37-404` |
| anonymous statuses | `M1-anon-probe-matrix.txt:55-81` |
| OIDC metadata | `M1-openid-configuration.json:2-26`; `M1-anon-probe-matrix.txt:450-469` |
| JWKS | `M1-jwks.json:5-35` |
| JS analysis | `M1-js-static-analysis.txt:56-124`; `M1-js-chunks.tsv:2-13` |
| download branch | `M1-anon-probe-matrix.txt:337-367` |
| robots/sitemap | `M1-anon-probe-matrix.txt:369-438` |
| consolidated index | `M1-index.md:49-229` |

---

## 12. Deliverables index and next steps

### 12.1 Deliverables

| file | purpose | validated state |
|---|---|---|
| `/workspace/re/apifox/openapi.json` | OpenAPI 3.1.0 bundle, 15 operations, 7 tags, 13 schemas, `$ref`-based error envelope, both `/v1/messages` security alternatives | JSON parses; version `3.1.0`; 15 paths; 15 unique operationIds; 21 local `$ref`s resolve; no `eyJ`-style secrets |
| `/workspace/re/apifox/IMPORT.md` | Apifox import paths (UI and script), environment variables, auth configuration, created folders, exclusions, idempotent re-import, troubleshooting | complete; live push UNVERIFIED (no token) |
| `/workspace/re/apifox/push_apifox.py` | Python 3 stdlib-only validator and optional `POST /v1/projects/{id}/import-openapi` client; dry-run by default | `--self-test` exits 0 |
| `/workspace/re/report/reverse-engineering-report.md` | this report | 12 sections |

The 15 operationIds are: `appShellGet`, `oauthAuthorize`, `oauthToken`, `oauthRevoke`, `oauthCodeSuccessGet`,
`openidConfigurationGet`, `jwksGet`, `profileGet`, `claudeCliRolesGet`, `messagesCreate`, `webEventsCreate`,
`clientDownloadGet`, `publicAssetsGet`, `robotsGet`, `sitemapGet`.

### 12.2 Still UNVERIFIED

| item | status | why |
|---|---|---|
| Apifox runtime push (`import-openapi`) | UNVERIFIED | no `APIFOX_ACCESS_TOKEN` in the environment `[M0-preflight.json:18]`; endpoint/shape verified against Apifox's public docs, but no call was made |
| `/v1/messages` auth scheme (`x-api-key` vs Bearer) | BLOCKED-CRED | requires a real credential; both schemes modelled as alternatives `[docs/01-endpoint-catalog.md:537]` |
| `/v1/messages` live JSON success body and authenticated error envelope | BLOCKED-CRED | never observed; OpenAPI marks the JSON schema prior-art and example-free `[docs/01-endpoint-catalog.md:538]` |
| Exact SSE frame sequence from the target | PRIOR-ART | derived from the proxy parser and its fixture, not a live stream `[docs/02-usage-semantics.md:196-198]` |
| `/api/oauth/profile` success schema | BLOCKED-CRED | prior art returns an untyped map with no call site `[docs/01-endpoint-catalog.md:290-292]` |
| `/api/oauth/claude_cli/roles` semantics/schema | BLOCKED-CRED | 401 guard only; purpose inferred from naming `[docs/01-endpoint-catalog.md:298-311]` |
| `/api/web/events` contract (body, media type, auth) | INFERRED-only | four competing hypotheses; no response capture `[docs/01-endpoint-catalog.md:353-367]` |
| `/api/public-assets` target body/semantics | unprobed | the 308 target was deliberately not followed `[M1-anon-probe-matrix.txt:247-255]` |
| `platform` vocabulary beyond `linux` | VERIFIED negative only | `darwin` rejected; no further values tried `[M1-anon-probe-matrix.txt:357-367]` |
| Model roster live availability | PRIOR-ART | `/v1/models` 404s; IDs absent from bundles `[M1-js-static-analysis.txt:97-123]` |
| Token TTL, JWT claims, refresh rotation, key rotation | BLOCKED-CRED | no token observed; `kid=configured-key` `[M1-jwks.json:22]` |
| Real API-key minting path | BLOCKED-CRED | both known routes 404 `[M1-anon-probe-matrix.txt:67-68]` |
| Quota/rate-limit response shapes | INFERRED | no `402`/`429`/`RateLimit-*` ever observed `[M1-index.md:210-212]` |

### 12.3 Next steps

1. **Credential-gated verification (≤5 calls, only with the user's own token).** Execute the M2 plan C1–C5:
   profile, roles, one non-streaming `/v1/messages`, one streaming call, and one invalid-model error probe
   `[docs/02-usage-semantics.md:404-457]`. Record only status, content type, header names, and redacted body
   structure. This closes the auth-scheme and success-schema gaps without violating governance.
2. **Apifox push.** When `APIFOX_ACCESS_TOKEN` and `APIFOX_PROJECT_ID` are supplied, run
   `python3 push_apifox.py --project-id <id> --push` and record the resulting `data.counters`. Until then the
   automated path remains UNVERIFIED `[apifox/IMPORT.md §0]`.
3. **`/api/web/events`.** Either obtain a contract observation under the governance cap or keep it permanently
   marked "unverified contract"; do not implement a client against any of the four hypotheses.
4. **`/api/public-assets`.** A single anonymous GET of the 308 target would settle whether the asset surface is
   public; it was intentionally out of scope in M1.
5. **Model roster.** Treat the table as prior-art marketing metadata; only an authenticated `/v1/messages`
   response can confirm an upstream ID.
6. **Reporting hygiene.** Keep the corrected requirement statistics (1 fully covered, 6 partial, 10 `NO-TEST`,
   7/17 = 41.2%; 17 hard markers) in any downstream summary `[acceptance/M3.md:179-192]`.

---

*End of report. M4 issued zero requests to `code.phanthy.com`. No credential, cookie value, or token was read,
copied, or written. All authenticated claims remain `PRIOR-ART` or `BLOCKED-CRED`.*
