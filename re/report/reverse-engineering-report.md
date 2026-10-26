# Reverse-Engineering Report — `https://code.phanthy.com`

Milestone: **M4 (API documentation & reporting)**, extended in **M6b** · Language: English · Status: complete
Validation (`2026-10-26`): `json.load` → `openapi 3.1.0`, 17 paths, 7 reusable parameters; `push_apifox.py --self-test`
→ 17 paths / 17 unique operations / 13 schemas / 44 local `$ref`s / `SELF-TEST OK` (exit 0).
Target: `https://code.phanthy.com` + npm package `@phanthy/phanthy-codeo@0.2.11`
Capture windows: M1 `2026-10-26T05:58:24+08:00` → `06:14:37+08:00`; M6 main capture `08:18:27` → `08:27:24`
(operator session bounds — the artifacts' own timestamps run `08:19:00` → `08:22:00`, last write `08:27:09`);
M6 install capture `09:07:11`

Anchor convention: `M0-*`, `M1-*` and `M6-*` anchors resolve under `/workspace/re/evidence/`; `docs/*` under
`/workspace/re/docs/`; `acceptance/*` under `/workspace/re/acceptance/`; Go/README anchors under `/workspace/`.
All live statements are inherited from the M1 and M6 anonymous captures; **M4 issued zero requests to the target
and M6b issued zero requests to anything**. Authenticated behaviour is `PRIOR-ART` / `BLOCKED-CRED` throughout,
and the M6 static analysis of the official CLI was **never executed**.

---

## 1. Executive summary

`code.phanthy.com` is a Next.js App Router application behind a Kong 2.5.1 gateway and TencentEdgeOne CDN that
exposes two distinct API families under one origin: a **Claude-Code-compatible OAuth 2.0 + PKCE** surface
(`/oauth/*`, `/api/oauth/*`, `/.well-known/*`) and an **Anthropic Messages-compatible inference** surface
(`POST /v1/messages`). A small public metadata/asset surface (`/api/download/client`, `/api/public-assets/`,
`/robots.txt`, `/sitemap.xml`) and the HTML app shell complete the observed footprint.

Seventeen endpoints are verified to exist; six probed paths are verified absent (Next.js HTML 404 shell)
`[M1-index.md:76-80]`. The OAuth metadata is internally consistent and fully readable: authorization-code only,
PKCE `S256` only, public client (`token_endpoint_auth_methods_supported: ["none"]`), five scopes, RS256 JWKS
`[M1-openid-configuration.json:7-26]` `[M1-jwks.json:22]`.

**M6 increment (`2026-10-26`).** Two further endpoints — `GET /api/phanthy_code/models` and
`GET /api/phanthy_code/models/stats` — were verified live and anonymously: both answer `OPTIONS 204` with
`allow: GET, HEAD, OPTIONS`, and both return an anonymous `401` with
`WWW-Authenticate: Bearer realm="phanthy-code"`. Their success bodies are `BLOCKED-CRED`
`[M6-new-endpoint-probes.txt:28-193]`. The official CLI `@phanthy/phanthy-codeo@0.2.11` was analysed
**statically only** (tarball extraction; never installed, imported or executed) and independently confirms both
paths, the six-scope request set, a seven-header client family, and the finding that the client **deletes
`x-api-key`** and authenticates with Bearer only `[M6-cli-config.txt:31-99, 145-157]`. The public pricing page
advertises 13 model aliases with credit multipliers; that catalogue is `PUBLIC-MARKETING` and is **not** the
prior-art Iris/Zeus/Gaia/Apollo/Metis mapping — the two layers are kept separate in §8.

The central limitation is credential-gated: **the auth scheme and the success bodies of `POST /v1/messages` were
never observed**. The prior-art implementation prefers `x-api-key: <api_key>` and falls back to
`Authorization: Bearer <token>` `[internal/upstream/client.go:344-350]`; both the API-key minting routes return
the HTML 404 shell `[M1-anon-probe-matrix.txt:67-68]`, so the fallback is the only documented viable credential
path. This report therefore separates `VERIFIED` target facts from `PRIOR-ART` wire assumptions and `INFERRED`
protocol reasoning, and never presents a live inference response as observed.

Deliverables produced in M4:

| deliverable | path | state |
|---|---|---|
| Apifox-importable OpenAPI 3.1.0 bundle | `/workspace/re/apifox/openapi.json` | validated: 17 paths / 17 operations / 13 schemas / 7 reusable parameters / 44 local `$ref`s |
| Import & operations guide | `/workspace/re/apifox/IMPORT.md` | complete; runtime push remains UNVERIFIED |
| Optional push/validator CLI (stdlib only) | `/workspace/re/apifox/push_apifox.py` | `--self-test` exits 0 |
| This report | `/workspace/re/report/reverse-engineering-report.md` | 13 sections |

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

**Milestone-6b write discipline.** M6b was a documentation-integration pass: it **issued zero network requests**
of any kind and executed nothing. It edited only the eight permitted artefacts (five `docs/*.md`, two
`apifox/*`, this report) and left `re/acceptance/M1.md`–`M5.md`, `docs/00-governance.md`, `docs/06-tdd-handbook.md`
and every M0/M1/M6 evidence file untouched. All M6 facts it integrated were already captured and hashed by the
M6 evidence files; it added no new observation.

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
7. **Official-package static analysis (M6).** The chain was: npm registry metadata → tarball
   (`sha256 3134d33d4eba31ec0af543a8afa061d3b21c4df7259d49decc11928e2244d80c`, 969 files / 76,873,667 B
   unpacked) → `tar -xzf` into `/tmp` → Python byte slicing at recorded offsets, with a per-file sha256 for the
   two bundles examined `[M6-npm-package.json:13-93]` `[M6-cli-config.txt:6-31]`. Nothing was `npm install`-ed,
   imported, transpiled or executed; no binary ran, no socket was opened by the package, and no credential file
   was read. This is the only method that can inspect a closed-source first-party client safely.
8. **Public marketing capture (M6).** One anonymous `GET` of `https://code.phanthy.com/token-plan` (49,225 B,
   sha256 `2abf943b…be8a`) rendered no JavaScript and executed nothing; model rows and plan tiers were extracted
   from the served HTML only `[M6-public-model-catalog.txt:4-18]`.

**Evidence grades.** `VERIFIED` = observed live in M1 with a file:line anchor; `PRIOR-ART` = derived only from
in-repo code/README; `INFERRED` = reasoned from metadata/RFC/naming with the basis stated; `BLOCKED-CRED` =
requires the user's own token, nothing fabricated `[docs/01-endpoint-catalog.md §1.4]`. M6 adds three finer
grades, used only for M6-derived statements: `VERIFIED-LIVE` (observed live in the M6 anonymous window,
`M6-new-endpoint-probes.txt`), `VERIFIED-STATIC` (exact string inside the official npm bundle, never executed,
`M6-cli-config.txt`), and `PUBLIC-MARKETING` (public pricing page — not an API contract,
`M6-public-model-catalog.txt`).

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

### 5.1 Verified endpoint inventory (17 operations)

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
| 10 | POST | `/v1/messages` | `Authorization: Bearer <access_token>` (official CLI, VERIFIED-STATIC); `x-api-key` retained only as an unverified prior-art alternative | Anthropic Messages inference (+ SSE) | VERIFIED (existence) / PRIOR-ART (wire) / BLOCKED-CRED (server-side auth + live body) | `M1-options-matrix.txt:23` `[M6-cli-config.txt:145-157]` |
| 11 | POST | `/api/web/events` | unknown | Event ingestion/stream of unknown semantics | VERIFIED (existence) / INFERRED only (everything else) | `M1-options-matrix.txt:26` |
| 12 | GET | `/api/download/client` | none | Client artifact download; mints visitor cookies | VERIFIED | `M1-anon-probe-matrix.txt:337-366` |
| 13 | GET | `/api/public-assets/` (308 → `/api/public-assets`) | unknown | Public-asset surface; target never followed | VERIFIED (redirect only) / INFERRED (target) | `M1-anon-probe-matrix.txt:247-255` |
| 14 | GET | `/robots.txt` | none | Crawler policy | VERIFIED | `M1-anon-probe-matrix.txt:369-401` |
| 15 | GET | `/sitemap.xml` | none | SEO sitemap, 841 URLs | VERIFIED | `M1-anon-probe-matrix.txt:404-438` |
| 16 | GET | `/api/phanthy_code/models` | `Authorization: Bearer <access_token>` | Model catalogue for the official CLI's picker | VERIFIED-LIVE (existence + guard) / BLOCKED-CRED (success body) | `M6-new-endpoint-probes.txt:25-105` |
| 17 | GET | `/api/phanthy_code/models/stats` | `Authorization: Bearer <access_token>` | Per-model statistics (best-effort, 5 s client timeout) | VERIFIED-LIVE (existence + guard) / INFERRED (purpose) / BLOCKED-CRED (success body) | `M6-new-endpoint-probes.txt:110-193` |

Method matrix: four POST-only paths (`/oauth/token`, `/oauth/revoke`, `/v1/messages`, `/api/web/events`), five
read-only handlers, and two GET/HEAD-only static metadata files `[M1-options-matrix.txt:13-32]`. The two M6
additions are GET/HEAD-only as well — `OPTIONS` reports `allow: GET, HEAD, OPTIONS` on both
`[M6-new-endpoint-probes.txt:42, 131]`. Endpoint numbering follows `docs/01-endpoint-catalog.md` (`EP-01` …
`EP-17`); note that the M1 evidence alone supports 13 existing paths, so the two M6 additions are the only
delta and the running total of 17 is an editorial convention, not a figure derivable from M1 by itself
`[M6-index.md §5]`.

### 5.2 Verified absent (six paths, HTML 404 shell)

`/api/oauth/phanthy_cli/create_api_key`, `/api/oauth/claude_cli/create_api_key`, `/v1/messages/count_tokens`,
`/v1/models`, `/api/health`, `/healthz` `[M1-index.md:76-80]`. All return `404 text/html`, ~26.5 KB, with
`x-powered-by: Next.js` and no JSON envelope `[M1-anon-probe-matrix.txt:67-79]`. These paths are **excluded from
the OpenAPI document** and listed in `info.description` as "verified absent" (`x-verified-absent-paths`).
M6 refinement: `/v1/models` stays absent, but model discovery is **not** absent from the site — the machine-readable
catalogue lives at `/api/phanthy_code/models` (§5.1 rows 16–17), behind a bearer guard
`[M6-index.md §3.1, §4]`.

### 5.3 OpenAPI modelling decisions

- **Document:** OpenAPI `3.1.0`, server `https://code.phanthy.com`, seven tags (`OAuth`, `OIDC`, `Account`,
  `Inference`, `Web`, `Assets`, `Meta`), **17 operations**, 13 shared schemas, 7 reusable header parameters,
  44 internal `$ref`s.
- **Security:** `apiKeyAuth` (`apiKey`, header, `x-api-key`) and `bearerAuth` (`http`, `bearer`), both offered as
  alternatives on `messagesCreate` only (`security: [{apiKeyAuth: []}, {bearerAuth: []}]`); `profileGet`,
  `claudeCliRolesGet`, `phanthyModelsGet` and `modelStatsGet` declare `bearerAuth` alone. `apiKeyAuth` is
  described as an **unverified prior-art possibility**, because the official CLI deletes that header
  `[M6-cli-config.txt:145-157]`.
- **`phanthyModelsGet` / `modelStatsGet` (added in M6):** the `401` is fully specified (observed `ErrorEnvelope`
  body plus `WWW-Authenticate: Bearer realm="phanthy-code"`), while the `200` carries a permissive
  `object` schema whose description states that the success schema is `BLOCKED-CRED`; no field, example or enum
  is asserted `[docs/02-usage-semantics.md §1B]`.
- **Seven reusable parameters:** `XPhanthyClient`, `XPhanthyClientSource`, `XPhanthyCliDevice`, `XPhanthyProduct`,
  `XPhanthyProject`, `XPhanthyRequest`, `XPhanthySession` live in `components.parameters`, all optional, and are
  attached to `messagesCreate` and the two model operations. They reproduce the official client's header family
  with exact casing; they are client behaviour (`VERIFIED-STATIC`), not proven server requirements.
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

## 5A. Official CLI supply-chain static analysis (M6)

### 5A.1 Method and non-execution attestation

The first-party client is published to npm as **`@phanthy/phanthy-codeo@0.2.11`** (`2026-10-21T08:56:55.362Z`;
`bin.phanthy-codeo`, `engines.bun >=1.3.14`, `type: module`, MIT, declared repository
`github.com/yizhixu/phanthy-codeo`) `[M6-npm-package.json:13-37]`. The analysis was strictly static:

```
registry metadata (1 GET) → tarball (1 GET) → sha256 compare → tar -xzf into /tmp → Python byte slicing
```

Tarball `sha256 3134d33d4eba31ec0af543a8afa061d3b21c4df7259d49decc11928e2244d80c`; 969 files, 76,873,667 B
unpacked; the two bundles examined are `dist/tui-worker.js`
(`sha256 c38ca895230b827838cfebb19b2b9becdf8523379118868a0f9461956f9b0c6d`) and `dist/cli.js`
(`sha256 61f503311058d5ec9e571db3f0a74bc3c883ea3a781b916b49619d16b8b27c52`)
`[M6-npm-package.json:39-93]`. The attestation recorded in the evidence is `static_only: true`, `executed: false`,
`installed: false`, `imported: false`, `binaries_run: false`. **No package code was run at any point**, so no
attacker-controlled code had the opportunity to touch the sandbox, and no credential file was read.

Non-findings worth stating explicitly: the package ships **no** model list, no API key, no token and no
`.credentials.json`; nothing was found that would let an unauthenticated party mint credentials
`[M6-cli-config.txt:102-120, 206-208]`.

### 5A.2 Provider configuration (`VERIFIED-STATIC`)

Extracted verbatim from `dist/tui-worker.js` at byte offset **7928736** (identical block duplicated in
`dist/cli.js`) `[M6-cli-config.txt:31-99]`:

| element | value |
|---|---|
| base URL | `https://code.phanthy.com` |
| client id | `phanthy-code-cli` |
| provider name / ID | `phanthy` |
| authorize | `https://code.phanthy.com/oauth/authorize` |
| token | `https://code.phanthy.com/oauth/token` |
| profile | `https://code.phanthy.com/api/oauth/profile` |
| models | `https://code.phanthy.com/api/phanthy_code/models` |
| modelStats | `https://code.phanthy.com/api/phanthy_code/models/stats` |
| messages | `https://code.phanthy.com/v1/messages` (map entry, **never dereferenced**) |

The client id matches the prior-art Go value `[cmd/login/main.go:28, 146]`, so `phanthy-code-cli` is now
corroborated by two independent sources. The `messages` map entry has zero call sites; the real message path is an
Anthropic-SDK-compatible provider (`apiKey:"phanthy-oauth"`, `baseURL: ${base}/v1`) that posts to the absolute path
`/v1/messages`, so the effective URL may be `/v1/messages` **or** `/v1/v1/messages` — recorded as `UNVERIFIED`
`[M6-cli-config.txt:145-157]` `[M6-index.md §3.8]`.

### 5A.3 Header family (`VERIFIED-STATIC` — exact casing)

| header (exact) | map field | value / purpose seen |
|---|---|---|
| `x-phanthy-client` | `client` | client discriminator |
| `X-Phanthy-Client-Source` | `clientSource` | `cli` on the message path, `phanthy-codeo` on telemetry |
| `x-phanthy-cli-device` | `device` | `base64url(JSON)` host fingerprint — privacy observation, §9 S13 |
| `X-Phanthy-Product` | `product` | `phanthy_code` / `phanthy_codeo` |
| `x-phanthy-project` | `project` | project id of the session |
| `x-phanthy-request` | `request` | per-request UUID correlation id |
| `x-phanthy-session` | `session` | session id |

Two names capitalise `X-Phanthy`, five use lowercase `x-phanthy-`; the casing is preserved verbatim in the
OpenAPI `components.parameters` block `[M6-cli-config.txt:58-73]`. A non-namespaced `x-service-name: phanthy-codeo`
accompanies telemetry flushes only.

### 5A.4 Scope delta: 6 requested vs 5 advertised

The CLI requests six scopes: `org:create_api_key`, `user:profile`, `user:inference`,
`user:sessions:claude_code`, `user:mcp_servers`, `user:file_upload` `[M6-cli-config.txt:74-99]`. The server's
discovery document advertises five `[M1-openid-configuration.json:20-26]`. The delta is **`org:create_api_key`** —
the only scope the client asks for that the server never advertises. Two readings are possible and neither is
provable without a token: (a) a stale or shared client library requests a scope the current deployment no longer
advertises, or (b) it is honoured but unlisted. Recorded as an open question
`[M6-index.md §3.4]` `[docs/01-endpoint-catalog.md §6 R18]`. It does **not** imply a working API-key path: both
known `create_api_key` routes return the HTML 404 shell.

### 5A.5 Credential storage layout (`VERIFIED-STATIC`)

```
default dir   ~/.phanthy-code/                    (override: $PHANTHY_CONFIG_DIR)
credential    ~/.phanthy-code/.credentials.json   (path.join(dir, ".credentials.json"))
json key      "claudeAiOauth"                     (OAuth payload; absent ⇒ file treated as non-OAuth)
secondary cfg .config.json  →  else .phanthy-code.json
macOS         `security` CLI keychain entry, service `phanthy code-credentials[-<sha256(dir)[:8]>]`
identity      $USER ?? os.userInfo().username ?? "claude-code-user"
```

The layout is Claude-Code-shaped. **No credential value exists in the package and none was read**
`[M6-cli-config.txt:102-120]`. This is documented for onboarding only; never paste a real `.credentials.json`.

### 5A.6 Bearer vs `x-api-key` (the corrective finding)

The CLI's provider `fetch` **deletes `x-api-key`** and then sets `Authorization: Bearer <access token>`, plus
`X-Phanthy-Product: phanthy_code`, `X-Phanthy-Client-Source: cli`, a `User-Agent` and a fresh `x-phanthy-request`
UUID `[M6-cli-config.txt:145-157]`.

| scheme | status | consequence |
|---|---|---|
| `Authorization: Bearer <token>` | the **only** scheme the official client sends (`VERIFIED-STATIC`) | Bearer is the primary documented scheme |
| `x-api-key: <key>` | explicitly removed by the official client; prior art only | retained as an **unverified possibility**, never as "preferred" |

This corrects the earlier framing (prior art "prefers `x-api-key`"). Wire-level acceptance of Bearer by `/v1/*`
remains `BLOCKED-CRED`; the M6 `401` challenge on `/api/phanthy_code/*` is consistent with a bearer-gated API
surface but does not prove `/v1/*` behaves identically.

### 5A.7 Static-only, unprobed surface

The bundle also contains `POST ${base}/api/event_logging/batch` (telemetry batch flush, JSON `{events:[…]}`,
Bearer + `x-service-name: phanthy-codeo`), a `dev.code.phanthy.com` build-flag host, `https://code.phanthy.com/console`
and a web-search base URL `[M6-cli-config.txt:123-166]`. These are `VERIFIED-STATIC` / **live-unverified**:
`/api/event_logging/batch` was **not** probed, is **not** counted among the 17 endpoints, and must not be probed
without a governance amendment. See `docs/01-endpoint-catalog.md` §7.

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
   (RFC 6749 §4.1.2) `[docs/02-usage-semantics.md §1.2]`.
5. `POST /oauth/token` (form-urlencoded) with `grant_type=authorization_code`, `code`, `code_verifier`,
   `redirect_uri`, `client_id=phanthy-code-cli` `[cmd/login/main.go:177-193]`.
6. `200 {access_token, refresh_token, expires_in}` — prior-art shape only; actual body `BLOCKED-CRED`
   `[cmd/login/main.go:171-175, 208-218]`.
7. Refresh: `POST /oauth/token` with `grant_type=refresh_token`, `refresh_token`, `client_id`
   `[internal/upstream/client.go:238-245]`.

The client identity `phanthy-code-cli` is prior art `[cmd/login/main.go:28, 146]`; prior art also sends
`anthropic-beta: oauth-2025-04-20` on OAuth calls `[cmd/login/main.go:193]` `[internal/upstream/client.go:139, 243]`
— requiredness is INFERRED, never verified `[docs/02-usage-semantics.md §1.3]`.

### 6.2 Scopes

Five scopes are advertised server-side `[M1-openid-configuration.json:20-26]`; the official CLI requests **six**
(§5A.4) `[M6-cli-config.txt:74-99]`:

| scope | advertised | requested by prior art | requested by official CLI | use |
|---|---|---|---|---|
| `user:profile` | yes | yes | yes | profile access |
| `user:inference` | yes | yes | yes | `/v1/messages` |
| `user:sessions:claude_code` | yes | yes | yes | Claude Code session semantics |
| `user:mcp_servers` | yes | no | yes | MCP server access |
| `user:file_upload` | yes | no | yes | file upload |
| `org:create_api_key` | **no** | no | **yes** | API-key creation; the 6-vs-5 delta, unexplained |

The single unadvertised scope, `org:create_api_key`, is the only scope the client requests that the server never
advertises. Whether it is silently honoured, ignored, or stale client metadata is `BLOCKED-CRED` — and it does not
change the fact that both `create_api_key` routes return the HTML 404 shell (§6.4).

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

**M6 correction (`VERIFIED-STATIC`).** The official CLI's provider fetch **deletes `x-api-key`** before sending and
authenticates with `Authorization: Bearer <access token>`; it also adds its own seven-header family
(`x-phanthy-client`, `X-Phanthy-Client-Source`, `x-phanthy-cli-device`, `X-Phanthy-Product`, `x-phanthy-project`,
`x-phanthy-request`, `x-phanthy-session`) alongside `User-Agent` `[M6-cli-config.txt:145-157]` (report §5A.3).
`x-api-key` therefore remains only an **unverified possibility** carried from prior art, never the recommended
scheme; whether the server honours it is `BLOCKED-CRED`.

### 7.3 Streaming (PRIOR-ART)

The prior-art parser reads line-oriented SSE, considers only `data: ` lines, ignores `event:` names entirely, and
treats the JSON `type` field as authoritative `[internal/upstream/sse.go:148-168]`. Event sequence understood:

`message_start` → `content_block_start` (text or `tool_use`) → `content_block_delta`
(`text_delta` / `thinking_delta` / `input_json_delta`) → `message_delta` → `message_stop`
`[internal/upstream/sse.go:188-276]` `[docs/02-usage-semantics.md §3.2]`.

`content_block_stop` is not handled; an `error` event is parsed into a struct but has **no handler branch**, so a
mid-stream error is silently discarded `[internal/upstream/sse.go:44-47, 188-276]`. Termination is driven by EOF,
not by `message_stop`, after which the proxy appends its own `[DONE]` `[internal/upstream/sse.go:162-184]`.
Consumer defence: treat a stream with no `content_block_delta` and no `stop_reason` as a failure and keep the
pre-stream HTTP status as the only reliable error signal `[docs/02-usage-semantics.md §3.4]`.

The OpenAPI `text/event-stream` response carries named examples (`messageStart`, `contentBlockStart`,
`contentBlockDelta`, `messageDelta`, `messageStop`, `errorEvent`) but explicitly marks the wire contract prior-art.

### 7.4 Error taxonomy: observed vs prior-art

Only four JSON error envelopes were ever observed on the target:
`unsupported_response_type` `[M1-anon-probe-matrix.txt:112]`, `missing_login` `[:163, :190]`, and
`invalid_platform` `[:245, :365]` — all shaped `{"error":{"code","message"}}`. **No `402`, `429`, or `5xx` was ever
observed** `[docs/02-usage-semantics.md §4.4]`. The prior-art classifier (402 → hard credit; hard markers;
session markers; 401; 429; 404; ≥500; ≥400) is a statement about the proxy, not the target
`[internal/upstream/client.go:76-107]`. The OpenAPI document therefore documents only the observed error envelope
and each observed status, with no invented authenticated error codes.

---

## 8. Model roster — two independent layers, not joined

Model IDs still cannot be obtained from HTTP. `GET /v1/models` returns the HTML 404 shell
`[M1-anon-probe-matrix.txt:73]`; the 12 analysed public JS chunks contain **zero** matches for every probed family
`[M1-js-static-analysis.txt:97-123]`; and the new machine-readable catalogue endpoints
`GET /api/phanthy_code/models` and `GET /api/phanthy_code/models/stats` are credential-gated, returning only a
`401 missing_login` anonymously `[M6-new-endpoint-probes.txt:28-193]`. There are therefore **two catalogues on
record that must not be merged**:

- **Layer A — `PUBLIC-MARKETING`**: the 13 display aliases advertised on the public pricing page. This is what the
  operator *sells*, not what the API *accepts* `[M6-public-model-catalog.txt:27-65]`.
- **Layer B — `PRIOR-ART`**: the in-repo alias → upstream-ID map used by the Go proxy. This is what *this
  repository's client* sends `[internal/upstream/payload.go:75-89]` `[README.md:120-134]`.

**The mapping between layer A and layer B is NOT established.** No evidence connects a pricing-page alias to a
prior-art upstream ID, and no authenticated catalogue or inference response was observed. Similar-looking strings
are not probative: layer A's `kimi-k2.7-code` and layer B's `Kimi-k2.7-code` differ in casing and layer A's entry
is a display alias with a credit multiplier, while layer B's is a client-facing name mapped to `Apollo-1.1`. They
are treated as coincidental surface similarity, not as a confirmed join `[M6-public-model-catalog.txt:63-65]`
`[docs/02-usage-semantics.md §6.3]`.

### 8.1 Layer A — published model catalogue (`PUBLIC-MARKETING`)

Source: `https://code.phanthy.com/token-plan`, anonymous GET `200 text/html`, 49,225 B, sha256
`2abf943b…0be8a` `[M6-public-model-catalog.txt:4-8]`. Display names and ratio strings are reproduced with exact
casing and punctuation; `N.NNx积分` is a **credit-consumption multiplier**, not a per-token price
`[M6-public-model-catalog.txt:104-106]`.

| # | display name | credit ratio (verbatim) | scenario |
|---|---|---|---|
| 1 | `Phanthy-fast` | `0.00x积分` | 快速高效 · 限时免费 |
| 2 | `Phanthy-pro` | `0.33x积分` | 全球前沿 · 强大通用 |
| 3 | `Phanthy-ultra` | `0.67x积分` | 全球顶尖智能 · 极致质量 |
| 4 | `glm-5.3-flash` | `0.04x积分` | 前沿智能 · 极致性价比 |
| 5 | `glm-5.3` | `0.38x积分` | 智谱旗舰 · 综合能力强 |
| 6 | `glm-5.2` | `0.38x积分` | 智谱上代旗舰 · 长程稳定 |
| 7 | `kimi-k3` | `1.00x积分` | Kimi 旗舰 · 擅长长上下文 |
| 8 | `kimi-k2.7-code` | `0.31x积分` | 编程专用 · 为 Coding Agent 优化 |
| 9 | `deepseek-v4.1-flash-1017` | `0.10x积分` | deepseek最新模型 · 高效推理 |
| 10 | `Phana-5.5o` | `0.67x积分` | placeholder `Available model` |
| 11 | `Phana-5.5s` | `0.33x积分` | placeholder `Available model` |
| 12 | `Phano-6a` | `1.67x积分` | placeholder `Available model` |
| 13 | `Phano-6.1s` | `0.33x积分` | placeholder `Available model` |

Four rows carry the literal placeholder scenario `Available model`; that is the page's own text, not a redaction
`[M6-public-model-catalog.txt:38-43]`. The page also publishes four plan tiers (体验版 ¥0/month 2,000 credits;
标准版 ¥9.9/month 15,000; 高级版 ¥139/month 30,000+2,000; 旗舰版 ¥699/month 150,000+20,000) and the site-wide
note "每 1,000 积分约等于 1 美元价值的 Token 额度", which is a marketing equivalence, not a rate card
`[M6-public-model-catalog.txt:68-88]`.

Fields explicitly absent from this page: **model id / API slug, context window, per-token input/output price,
rate limits, endpoint/protocol mapping, and model→upstream mapping** `[M6-public-model-catalog.txt:91-102]`. The
`data-series`/logo assets hint at vendor families (phana* → Anthropic logo, phano* → OpenAI logo) but the page
states no mapping and the logo is presentation only `[M6-public-model-catalog.txt:45-65]`.

### 8.2 Layer B — in-repo alias map (`PRIOR-ART`)

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
`[internal/upstream/payload.go:294-298]` `[docs/02-usage-semantics.md §6.4]`. The OpenAPI model field is a free
string with the alias table in its description, not an `enum`.

### 8.3 What can and cannot be claimed

| claim | status |
|---|---|
| 13 display aliases and their credit ratios are publicly advertised | `PUBLIC-MARKETING` (layer A, verified page hash) |
| the proxy's client sends `Iris-1.0` … `Metis-1.0` for these 13 client-facing names | `PRIOR-ART` (layer B, repo source) |
| the API accepts any specific alias as a `model` wire value | `BLOCKED-CRED` — no catalogue or inference response observed |
| layer A alias ↔ layer B upstream ID | **not established** — no documentary or observed link |
| context window / per-token price for any alias | `not published` (layer A has no such column; layer B's 200k is proxy static) |

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
| S12 | Rate-limit/quota response shapes are unknown (no `RateLimit-*` headers ever observed) | **Medium** | Client-side backoff cannot be specified from evidence; any cooldown numbers in the SDD are prior-art policy only `[M1-index.md:210-212]` `[docs/02-usage-semantics.md §5.3]`. |
| S13 | The official CLI sends `x-phanthy-cli-device: base64url(JSON)` where the JSON is a **host fingerprint**: hostname, username, platform, architecture, OS release, up to 32 IPv4 candidates, network interfaces and default routes — i.e. stable per-machine client identification on every request | **Low–Medium** | Privacy observation only: it is a legitimate device-attribution header, not a vulnerability, and nothing was exercised. It is opaque to the user, correlates requests across sessions, and is undocumented publicly. Reported, not probed `[M6-cli-config.txt:195-201]` `[docs/01-endpoint-catalog.md §5.7]`. |
| S14 | The official CLI references an **undocumented telemetry endpoint** `POST /api/event_logging/batch` (static-only discovery; never called) | **Medium** | A client-side telemetry sink that appears in neither `robots.txt` nor any public doc; payload shape, auth and retention are unknown. It was deliberately **not probed** because probing a telemetry ingest path is out of scope for anonymous recon. Risk reflects unknown data handling rather than demonstrated exposure `[M6-cli-config.txt:145-157]` `[docs/01-endpoint-catalog.md §7]`. |

No finding in this section is a confirmed vulnerability. Items S1/S9/S12/S14 are rated Medium because they concern
missing hardening, unknown contracts, or unknown data handling; S13 is rated Low–Medium as a privacy observation;
the remainder are Low. Nothing here justifies exploitation, and none was attempted.

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

The M6 increment kept the same discipline. The official CLI package `@phanthy/phanthy-codeo@0.2.11` was obtained
from the public npm registry and inspected **statically only** — metadata, tarball hash, and byte-sliced strings.
It was never installed, never imported, and no binary inside it was executed; no install script was run and no
network call was made from package code `[M6-index.md §1]`. The new-endpoint probes were 9 anonymous
`OPTIONS`/`GET`/`HEAD` requests to two public paths at ≤1 req/s, using no credential; the static-only telemetry
path was deliberately **not** requested. The public pricing page was fetched once, anonymously, and only its
rendered table text was used.

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

M6 additions (anonymous, two new paths, ≤1 req/s — omit the authenticated branches):

```bash
# existence / method oracle on the new catalogue endpoints
curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/api/phanthy_code/models
curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/api/phanthy_code/models/stats

# anonymous guard shape (expect 401 JSON, 79 B, WWW-Authenticate: Bearer realm="phanthy-code")
curl -sS --http2 -D - -o /dev/null https://code.phanthy.com/api/phanthy_code/models

# public marketing catalogue (table text only; no JS execution)
curl -sS --http2 -o /tmp/token-plan.html 'https://code.phanthy.com/token-plan'
sha256sum /tmp/token-plan.html   # expect 2abf943b…cb40be8a
```

Expected digests: `token-plan` sha256 `2abf943b6cab2090ed737618f76b03473319dc9e1704a90840f8c061cb40be8a`
`[M6-public-model-catalog.txt:4-8]`. The M6 CLI analysis is reproduced from the public npm registry without
install: fetch `https://registry.npmjs.org/@phanthy/phanthy-codeo` metadata, download the `0.2.11` tarball
(expect sha256 `3134d33d…2244d80c`), extract with `tar -xzf`, and slice `dist/cli.js` / `dist/tui-worker.js` as
plain bytes — **never** run `npm install`, `npx`, or any extracted binary `[M6-index.md §6]`.

### 11.3 OpenAPI validation commands

```bash
python3 -c "import json; json.load(open('/workspace/re/apifox/openapi.json')); print('json ok')"
python3 /workspace/re/apifox/push_apifox.py --self-test
```

Recorded results (M6b, `2026-10-26`): `json ok 3.1.0 17`; self-test reports `openapi: 3.1.0`, `paths: 17`,
`operations: 17`, `schemas: 13`, `local $refs: 44`, tags `Account, Assets, Inference, Meta, OAuth, OIDC, Web`,
**17 unique operationIds** (the 15 M4 ids plus `phanthyModelsGet`, `modelStatsGet`), and `SELF-TEST OK` (exit 0).
`components.parameters` additionally holds **7** reusable Phanthy client headers, all optional `$ref`s.

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
| consolidated index (M1) | `M1-index.md:49-229` |
| new catalogue endpoints — existence, `allow`, 401 shape | `M6-new-endpoint-probes.txt:25-193` |
| official CLI static extraction (headers, scopes, endpoints, credentials, telemetry) | `M6-cli-config.txt:31-99, 145-157` |
| npm package metadata, hashes, non-execution attestation | `M6-npm-package.json:1-120` |
| public 13-alias catalogue, ratios, plan tiers, absent fields | `M6-public-model-catalog.txt:4-130` |
| consolidated index (M6) and delta vs M1 | `M6-index.md` (whole file; read by section rather than by line range) |

---

## 12. Deliverables index and next steps

### 12.1 Deliverables

| file | purpose | validated state |
|---|---|---|
| `/workspace/re/apifox/openapi.json` | OpenAPI 3.1.0 bundle, 17 operations, 7 tags, 13 schemas, 7 reusable header parameters, `$ref`-based error envelope, both `/v1/messages` security alternatives | JSON parses; version `3.1.0`; 17 paths; 17 unique operationIds; 13 schemas; 44 local `$ref`s resolve; no `eyJ`-style secrets |
| `/workspace/re/apifox/IMPORT.md` | Apifox import paths (UI and script), environment variables, auth configuration, created folders, exclusions, idempotent re-import, troubleshooting | complete; live push UNVERIFIED (no token) |
| `/workspace/re/apifox/push_apifox.py` | Python 3 stdlib-only validator and optional `POST /v1/projects/{id}/import-openapi` client; dry-run by default | `--self-test` exits 0 |
| `/workspace/re/report/reverse-engineering-report.md` | this report | 13 sections |

The 17 operationIds are: `appShellGet`, `oauthAuthorize`, `oauthToken`, `oauthRevoke`, `oauthCodeSuccessGet`,
`openidConfigurationGet`, `jwksGet`, `profileGet`, `claudeCliRolesGet`, `messagesCreate`, `webEventsCreate`,
`clientDownloadGet`, `publicAssetsGet`, `robotsGet`, `sitemapGet`, `phanthyModelsGet`, `modelStatsGet`.

### 12.2 Still UNVERIFIED

| item | status | why |
|---|---|---|
| Apifox runtime push (`import-openapi`) | UNVERIFIED | no `APIFOX_ACCESS_TOKEN` in the environment `[M0-preflight.json:18]`; endpoint/shape verified against Apifox's public docs, but no call was made |
| `/v1/messages` auth scheme (`x-api-key` vs Bearer) | BLOCKED-CRED (partially narrowed) | M6 shows the **official CLI** sends Bearer only and deletes `x-api-key`; whether the *server* also accepts `x-api-key` is still unobserved `[docs/01-endpoint-catalog.md §5.5]` |
| `/v1/messages` live JSON success body and authenticated error envelope | BLOCKED-CRED | never observed; OpenAPI marks the JSON schema prior-art and example-free `[docs/01-endpoint-catalog.md §3.10]` |
| Success bodies of `GET /api/phanthy_code/models` and `.../models/stats` | BLOCKED-CRED | both answer `401 missing_login` anonymously; the success schema is modelled permissively as an object and must not be read as observed `[M6-new-endpoint-probes.txt:28-193]` |
| Mapping from layer-A display aliases to layer-B upstream IDs / wire `model` values | not established | no authenticated catalogue, no inference response; similar strings are not probative (report §8) |
| Exact SSE frame sequence from the target | PRIOR-ART | derived from the proxy parser and its fixture, not a live stream `[docs/02-usage-semantics.md §3.2, §7.3 C4]` |
| `/api/oauth/profile` success schema | BLOCKED-CRED | prior art returns an untyped map with no call site `[docs/01-endpoint-catalog.md §3.8]` |
| `/api/oauth/claude_cli/roles` semantics/schema | BLOCKED-CRED | 401 guard only; purpose inferred from naming `[docs/01-endpoint-catalog.md §3.9]` |
| `/api/web/events` contract (body, media type, auth) | INFERRED-only | four competing hypotheses; no response capture `[docs/01-endpoint-catalog.md §3.11]` |
| `POST /api/event_logging/batch` (telemetry) | VERIFIED-STATIC only | string exists in the official CLI bundle; payload, auth and retention unknown; **not probed** `[M6-cli-config.txt:145-157]` |
| `dev.code.phanthy.com` (build-flag host) | VERIFIED-STATIC only | appears as a compile-time host branch in the CLI; not resolved or requested `[M6-cli-config.txt:31-99]` |
| `/api/public-assets` target body/semantics | unprobed | the 308 target was deliberately not followed `[M1-anon-probe-matrix.txt:247-255]` |
| `platform` vocabulary beyond `linux` | VERIFIED negative only | `darwin` rejected; no further values tried `[M1-anon-probe-matrix.txt:357-367]` |
| Model roster live availability | PUBLIC-MARKETING / PRIOR-ART | `/v1/models` 404s; the two catalogue endpoints 401; the 13 advertised aliases are marketing, not an API contract (report §8) |
| Token TTL, JWT claims, refresh rotation, key rotation | BLOCKED-CRED | no token observed; `kid=configured-key` `[M1-jwks.json:22]` |
| Real API-key minting path | BLOCKED-CRED | both known routes 404 `[M1-anon-probe-matrix.txt:67-68]`; the CLI's `org:create_api_key` scope does not contradict this (report §6.2) |
| Quota/rate-limit response shapes | INFERRED | no `402`/`429`/`RateLimit-*` ever observed `[M1-index.md:210-212]` |

### 12.3 Next steps

1. **Credential-gated verification (≤5 calls, only with the user's own token).** Execute the M2 plan C1–C5:
   profile, roles, one non-streaming `/v1/messages`, one streaming call, and one invalid-model error probe
   `[docs/02-usage-semantics.md §7.3 C1–C5]`. Record only status, content type, header names, and redacted body
   structure. This closes the auth-scheme and success-schema gaps without violating governance.
2. **Model discovery (same credential budget).** The M6 plan already excluded the two catalogue endpoints from
   the 5-call envelope `[docs/02-usage-semantics.md §7.1 U8]`. If a token is ever available, a single
   `GET /api/phanthy_code/models` would settle the success schema **and** the alias→wire-ID question that §8
   leaves open; do not budget it inside the same 5 calls without re-planning.
3. **Apifox push.** When `APIFOX_ACCESS_TOKEN` and `APIFOX_PROJECT_ID` are supplied, run
   `python3 push_apifox.py --project-id <id> --push` and record the resulting `data.counters`. Until then the
   automated path remains UNVERIFIED `[apifox/IMPORT.md §0]`.
4. **`/api/web/events`.** Either obtain a contract observation under the governance cap or keep it permanently
   marked "unverified contract"; do not implement a client against any of the four hypotheses.
5. **`/api/public-assets`.** A single anonymous GET of the 308 target would settle whether the asset surface is
   public; it was intentionally out of scope in M1.
6. **Telemetry and device header.** Leave `POST /api/event_logging/batch` and the `x-phanthy-cli-device`
   fingerprint as reported observations only. Do not probe the telemetry sink; do not implement the fingerprint
   header in this repository's client.
7. **Model roster.** Treat §8 layer A as marketing metadata and layer B as prior art; only an authenticated
   catalogue or `/v1/messages` response can join them.
8. **Reporting hygiene.** Keep the corrected requirement statistics (1 fully covered, 6 partial, 10 `NO-TEST`,
   7/17 = 41.2%; 17 hard markers) in any downstream summary `[acceptance/M3.md:179-192]`.

---

*End of report. M1 and M6 issued the only live requests to `code.phanthy.com` (all anonymous); **M4 issued zero
requests and M6b issued zero requests to anything**. The npm package `@phanthy/phanthy-codeo@0.2.11` was read
statically and never installed or executed. No credential, cookie value, or token was read, copied, or written.
All authenticated claims remain `PRIOR-ART` or `BLOCKED-CRED`.*
