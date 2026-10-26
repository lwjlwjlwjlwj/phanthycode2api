# M1 Evidence Index — `https://code.phanthy.com/` (anonymous, read-only)

Status: M1 evidence package complete (2 of 2 deliverables). All probes anonymous (GET/HEAD/OPTIONS only);
no credentials, no writes, no execution of retrieved code. Every claim below cites a file in
`/workspace/re/evidence/` plus a line anchor. Line anchors refer to the file as it stands at the
SHA-256 values in §2.

---

## 1. Capture metadata

| item | value | source |
|---|---|---|
| target | `https://code.phanthy.com/` | [M0-preflight.json:4] |
| capture window | `2026-10-26T05:58:24+08:00` (root fetch) → `2026-10-26T06:14:37+08:00` (last metadata probe) | [M1-root-headers.txt:1], [M1-anon-probe-matrix.txt:3] |
| index compiled | `2026-10-26T06:16:58+08:00` (+08:00 Asia/Shanghai) | this run (`date --iso-8601=seconds`) |
| tool: curl | `8.5.0 (x86_64-pc-linux-gnu) libcurl/8.5.0 OpenSSL/3.0.13 nghttp2/1.59.0` | [M1-anon-probe-matrix.txt:10] |
| tool: python3 | `3.14.8` | [M1-anon-probe-matrix.txt:11], [M0-preflight.json:7] |
| tool: sha256sum | `GNU coreutils 9.4` | [M1-anon-probe-matrix.txt:12] |
| egress path | `HTTP_PROXY/HTTPS_PROXY = http://127.0.0.1:18080`, HTTP/2 to origin; direct TCP blocked | [M1-anon-probe-matrix.txt:7] |
| rate discipline | 1.1 s spacing, ≤3 requests per endpoint, 33 requests total | [M1-anon-probe-matrix.txt:8], [M1-anon-probe-matrix.txt:13] |
| CDN / WAF observed | `server: TencentEdgeOne`, `via: kong/2.5.1`, `x-kong-upstream-latency`, `x-kong-proxy-latency`, `eo-log-uuid`, `eo-cache-status` | [M0-preflight.json:16], [M1-root-headers.txt:12-20] |
| upstream app | Next.js (`x-powered-by: Next.js`, `/_next/*` assets, app-router 404 shell) | [M1-root-headers.txt:12], [M1-anon-probe-matrix.txt:67] |
| identity used | anonymous; no Authorization, no Cookie, no x-api-key | [M1-anon-probe-matrix.txt:6] |
| response-header caveat | leading `HTTP/1.1 200 OK` in dumps is the proxy CONNECT handshake, not origin | [M1-anon-probe-matrix.txt:31-35] |

Environment constraint recorded for M2 planning: authenticated surfaces are unusable in M1 because no
test account or token exists (`authenticated_test_account: NOT PROVIDED`) [M0-preflight.json:19], and
governance forbids substituting guessed credentials [00-governance.md:11-19].

---

## 2. File inventory

| file | sha256 | bytes | what it proves | single-command reproduction |
|---|---|---|---|---|
| `M0-preflight.json` | `ab6ee8d3da198e7364ffa7c25fe22d898de9b327437fa386a5a3a2d192cf830b` | 1695 | target reachable (200/56923 B); stack = Kong 2.5.1 + TencentEdgeOne; no auth account/APIFOX token | `curl -sS --http2 -o /dev/null -w '%{http_code} %{size_download} %{content_type}\n' https://code.phanthy.com/` |
| `M1-root-headers.txt` | `98b0290f07358e3d06be48eac68005cf21072e6b99127a4268eea362302848e1` | 934 | `/` returns a 57 KB Next.js HTML app shell with HSTS + `cache-control: no-store` | `curl -sS --http2 -D - -o /dev/null https://code.phanthy.com/` |
| `M1-options-matrix.txt` | `c560c3cf16a4593498a48aa2d25c0401aeeec40e0d5155bc44153b78ddcf2e6b` | 14471 | OPTIONS-derived existence + method matrix for all 19 paths, raw headers per path | `curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/v1/messages` |
| `M1-openid-configuration.json` | `a4fae8e97ff3dbf9c8d15d833636a4e9ad159f841497f23c10a7b6a0f0bc7de5` | 771 | OAuth/OIDC discovery: 4 endpoints, code-only + PKCE S256, public client, 5 scopes | `curl -sS --http2 https://code.phanthy.com/.well-known/openid-configuration` |
| `M1-jwks.json` | `88a8ca9baa2920acb21ab6dda9575953b8429b0111b41cff24bacfa165b2cca2` | 1671 | exactly one RSA signing key, `kid=configured-key`, `alg=RS256`, plus its raw body | `curl -sS --http2 https://code.phanthy.com/.well-known/jwks.json` |
| `M1-js-chunks.tsv` | `0131dab81291c50c87dd0138be5def1d809701249ec2b6cb311d7f9a8efa7fe4` | 1456 | provenance (bytes + sha256) of the 12 public JS chunks analysed | `curl -sS --http2 -o /dev/null -w '%{size_download}\n' https://code.phanthy.com/_next/static/chunks/9894-c1c397f1dcfa0999.js` |
| `M1-js-static-analysis.txt` | `d65c63147ea5e58654994bb6f980c5d73fd443ed1059fa23245455f6fa91c1c7` | 7915 | public client bundles contain no `/oauth`, no `/v1/messages`, no auth header names, no model IDs; only 3 endpoint strings | `curl -sS --http2 https://code.phanthy.com/_next/static/chunks/app/layout-be0ffac7eea8e0cb.js` (text scan only) |
| `M1-anon-probe-matrix.txt` | `45a04077d23c1ecf2f673321ae9a389ce0ef7de67af4caf2607e09ffafbb7c56` | 26032 | GET/HEAD status + headers + body excerpt for all 19 endpoints; CORS probes; `platform=linux/darwin`; verbatim robots; sitemap stats; OIDC provenance fetch | `curl -sS --http2 -D - -o /dev/null https://code.phanthy.com/api/oauth/profile` |
| `M1-index.md` (this file) | not self-hashed (would be circular) | — | consolidated signal table, grades, findings, M2 questions | `n/a` |

---

## 3. Endpoint signal table

`methods` = authoritative list from `Allow` on OPTIONS [M1-options-matrix.txt:13-32]; `anon status` = what an
unauthenticated GET/HEAD actually returned [M1-anon-probe-matrix.txt:59-81].

| path | exists? | methods | anon status | auth required? | evidence (file:line) |
|---|---|---|---|---|---|
| `/` | yes (HTML page) | not derivable — OPTIONS returns 400 (anomaly) | GET 200, 56,923 B HTML | no | M1-options-matrix.txt:14 · M1-anon-probe-matrix.txt:59 · M1-root-headers.txt:7-20 |
| `/oauth/authorize` | yes | GET, HEAD, OPTIONS | GET 400 `unsupported_response_type` (JSON, 101 B) | no (validation precedes any login) | M1-options-matrix.txt:15 · M1-anon-probe-matrix.txt:60 |
| `/oauth/token` | yes | OPTIONS, POST | GET 405, HEAD 405, 0 B | not determinable anonymously → POST-only; `token_endpoint_auth_methods_supported: ["none"]` | M1-options-matrix.txt:16 · M1-anon-probe-matrix.txt:61-62 · M1-openid-configuration.json:17-19 |
| `/oauth/revoke` | yes | OPTIONS, POST | GET 405, HEAD 405, 0 B | not determinable anonymously | M1-options-matrix.txt:17 · M1-anon-probe-matrix.txt:63-64 |
| `/oauth/code/success` | yes | GET, HEAD, OPTIONS | GET 200, 1,769 B static HTML (`PhanthyCode 登录完成`) | no | M1-options-matrix.txt:18 · M1-anon-probe-matrix.txt:65 |
| `/api/oauth/profile` | yes | GET, HEAD, OPTIONS | GET 401, JSON 79 B, `WWW-Authenticate: Bearer realm="phanthy-code"` | YES — bearer access token | M1-options-matrix.txt:19 · M1-anon-probe-matrix.txt:66 |
| `/api/oauth/phanthy_cli/create_api_key` | no | — (OPTIONS 404) | GET 404, Next.js HTML shell 26,614 B | n/a | M1-options-matrix.txt:20 · M1-anon-probe-matrix.txt:67 |
| `/api/oauth/claude_cli/create_api_key` | no | — (OPTIONS 404) | GET 404, Next.js HTML shell 26,613 B | n/a | M1-options-matrix.txt:21 · M1-anon-probe-matrix.txt:68 |
| `/api/oauth/claude_cli/roles` | yes | GET, HEAD, OPTIONS | GET 401, identical 79 B body to profile | YES — same bearer guard | M1-options-matrix.txt:22 · M1-anon-probe-matrix.txt:69 |
| `/v1/messages` | yes | OPTIONS, POST | GET 405, HEAD 405, 0 B | presumed YES (unobservable without POST) | M1-options-matrix.txt:23 · M1-anon-probe-matrix.txt:70-71 |
| `/v1/messages/count_tokens` | no | — (OPTIONS 404) | GET 404, HTML shell 26,598 B | n/a | M1-options-matrix.txt:24 · M1-anon-probe-matrix.txt:72 |
| `/v1/models` | no | — (OPTIONS 404) | GET 404, HTML shell 26,579 B | n/a | M1-options-matrix.txt:25 · M1-anon-probe-matrix.txt:73 |
| `/api/web/events` | yes | OPTIONS, POST | GET 405, HEAD 405, 0 B | presumed YES (unobservable without POST) | M1-options-matrix.txt:26 · M1-anon-probe-matrix.txt:74-75 |
| `/api/download/client` | yes | GET, HEAD, OPTIONS | no query → 400 `invalid_platform`; `?platform=linux` → 302; `?platform=darwin` → 400 | NO — anonymous downloads work | M1-options-matrix.txt:27 · M1-anon-probe-matrix.txt:76, 337-366 |
| `/api/public-assets/` | yes | (308 before method evaluation) | 308 → `/api/public-assets`, 18 B body, `refresh: 0;url=` | n/a | M1-options-matrix.txt:28 · M1-anon-probe-matrix.txt:77 |
| `/api/health` | no | — (OPTIONS 404) | GET 404, HTML shell 26,580 B | n/a | M1-options-matrix.txt:29 · M1-anon-probe-matrix.txt:78 |
| `/healthz` | no | — (OPTIONS 404) | GET 404, HTML shell 26,573 B | n/a | M1-options-matrix.txt:30 · M1-anon-probe-matrix.txt:79 |
| `/robots.txt` | yes | GET;HEAD (OPTIONS 405) | GET 200, `text/plain`, 527 B | no | M1-options-matrix.txt:31 · M1-anon-probe-matrix.txt:80, 369-401 |
| `/sitemap.xml` | yes | GET;HEAD (OPTIONS 405) | GET 200, `application/xml`, 109,314 B, 841 `<loc>` | no | M1-options-matrix.txt:32 · M1-anon-probe-matrix.txt:81, 404-438 |

Aggregate over the 19 probed paths: **13 exist** (`/`, `/oauth/authorize`, `/oauth/token`, `/oauth/revoke`,
`/oauth/code/success`, `/api/oauth/profile`, `/api/oauth/claude_cli/roles`, `/v1/messages`,
`/api/web/events`, `/api/download/client`, `/api/public-assets*`, `/robots.txt`, `/sitemap.xml` — the last
two as static metadata) and **6 return the Next.js HTML 404 shell** (`/api/oauth/phanthy_cli/create_api_key`,
`/api/oauth/claude_cli/create_api_key`, `/v1/messages/count_tokens`, `/v1/models`, `/api/health`, `/healthz`).

---

## 4. Evidence grade (per endpoint)

Rule applied: `verified` = directly observed in this milestone's captures; `inferred` = deduced from
naming, prior art, or protocol convention and **never** promoted to verified.

| path | existence | auth/semantics | grade rationale |
|---|---|---|---|
| `/` | verified | verified (public shell) | 200 + 56,923 B body observed [M1-anon-probe-matrix.txt:59] |
| `/oauth/authorize` | verified | **partially inferred**: that it is the authorization-code entry point is *verified* by the JSON error text `Only authorization code flow is supported`; required/optional query params are **inferred** from `/oauth/...` convention | [M1-anon-probe-matrix.txt:60] |
| `/oauth/token` | verified | **inferred**: POST-only and public-client (PKCE, no secret) — POST itself not exercised | [M1-options-matrix.txt:16], [M1-openid-configuration.json:14-19] |
| `/oauth/revoke` | verified | **inferred**: RFC 7009 revoke semantics; body not exercised | [M1-options-matrix.txt:17] |
| `/oauth/code/success` | verified | verified: static HTML, title string captured, no dynamic data | [M1-anon-probe-matrix.txt:65] |
| `/api/oauth/profile` | verified | verified: bearer required (401 + `WWW-Authenticate`) | [M1-anon-probe-matrix.txt:66] |
| `/api/oauth/phanthy_cli/create_api_key` | verified absent | **inferred**: that an API-key minting route exists elsewhere (the `claude_cli` sibling 401s would otherwise be orphaned) | [M1-anon-probe-matrix.txt:67] |
| `/api/oauth/claude_cli/create_api_key` | verified absent | **inferred**: route was renamed/removed rather than never existed | [M1-anon-probe-matrix.txt:68] |
| `/api/oauth/claude_cli/roles` | verified | verified: 401 guard observed; response schema **inferred** | [M1-anon-probe-matrix.txt:69] |
| `/v1/messages` | verified | **inferred**: Anthropic Messages request/response schema, `x-api-key` vs `Authorization`, SSE streaming — none observed (GET/HEAD only) | [M1-anon-probe-matrix.txt:70-71], [M1-js-static-analysis.txt:84-96] |
| `/v1/messages/count_tokens` | verified absent | **inferred**: token counting may be folded into `/v1/messages` | [M1-anon-probe-matrix.txt:72] |
| `/v1/models` | verified absent | **inferred**: model catalogue is not exposed over HTTP | [M1-anon-probe-matrix.txt:73], [M1-js-static-analysis.txt:97-124] |
| `/api/web/events` | verified | **inferred**: POST-only eventing (SSE-like) — payload, media type and auth unverified | [M1-anon-probe-matrix.txt:74-75] |
| `/api/download/client` | verified | **verified** for: validation error without params, 302 for `linux`, 400 for `darwin`, visitor-cookie minting, COS redirect target. **Inferred**: full platform vocabulary | [M1-anon-probe-matrix.txt:76, 337-366] |
| `/api/public-assets/` | verified (redirect only) | **inferred**: that `/api/public-assets` serves a real asset listing; the 308 target was not followed | [M1-anon-probe-matrix.txt:77] |
| `/api/health`, `/healthz` | verified absent | verified: HTML 404 shell | [M1-anon-probe-matrix.txt:78-79] |
| `/robots.txt` | verified | verified: verbatim body incl. `Disallow: /api`, `/oauth`, `/v1` | [M1-anon-probe-matrix.txt:369-400] |
| `/sitemap.xml` | verified | verified: 200, 841 unique `<loc>`, zero API paths | [M1-anon-probe-matrix.txt:404-438] |

---

## 5. Key findings

**5.1 OAuth/OIDC metadata and scopes.** The discovery document at `/.well-known/openid-configuration`
returns `200 application/json` with a body that is field-for-field identical to
`M1-openid-configuration.json` (the served body is minified at 683 B; the stored file is the same JSON
pretty-printed at 771 B — byte strings differ, values do not) [M1-anon-probe-matrix.txt:450-469]. Fields: `issuer=https://code.phanthy.com`,
`authorization_endpoint=/oauth/authorize`, `token_endpoint=/oauth/token`,
`revocation_endpoint=/oauth/revoke`, `jwks_uri=/.well-known/jwks.json`;
`response_types_supported=["code"]`, `grant_types_supported=["authorization_code","refresh_token"]`,
`code_challenge_methods_supported=["S256"]`, `token_endpoint_auth_methods_supported=["none"]`
[M1-openid-configuration.json:7-19]. Supported scopes: `user:profile`, `user:inference`,
`user:sessions:claude_code`, `user:mcp_servers`, `user:file_upload` [M1-openid-configuration.json:20-26].
It is OAuth-2.0-with-PKCE metadata wearing an OIDC name: there is **no** `userinfo_endpoint`, **no**
`id_token_signing_alg_values_supported`, **no** `subject_types_supported`, and **no** `openid` scope
(absence verified by the full field list) [M1-openid-configuration.json:2-26]. JWKS has a single RSA
signing key with literal `kid=configured-key` [M1-jwks.json:22] — a default-style identifier, so key
rotation/rolling is not demonstrated.

**5.2 OPTIONS-derived method matrix.** Four paths are POST-only (`/oauth/token`, `/oauth/revoke`,
`/v1/messages`, `/api/web/events` → `Allow: OPTIONS, POST`), four are read-only handlers
(`/oauth/authorize`, `/oauth/code/success`, `/api/oauth/profile`, `/api/oauth/claude_cli/roles`,
`/api/download/client` → `Allow: GET, HEAD, OPTIONS`), and `/robots.txt`+`/sitemap.xml` expose a
two-line `Allow: GET` / `Allow: HEAD` form [M1-options-matrix.txt:13-32]. Cross-check against live
GET/HEAD is fully consistent [M1-anon-probe-matrix.txt:441-448].

**5.3 404-as-Next.js-shell.** Every non-existent API path returns **`404 text/html` with a ~26.5 KB
Next.js app-router document** (26,573–26,614 B across six paths) instead of a JSON error, and it carries
`x-powered-by: Next.js` plus a preload link to `/phanthy-code-logo-new.png`
[M1-anon-probe-matrix.txt:67-68, 72-73, 78-79]. Consequence: an HTTP client cannot distinguish "API route
absent" from "SPA catch-all served a page", and there is no JSON 404 taxonomy to document. A related
asymmetry: `405` responses carry **no `Allow` header at all**, contrary to RFC 9110, so method discovery
must rely on OPTIONS [M1-anon-probe-matrix.txt:447-448, 192-198].

**5.4 The `/` OPTIONS 400 anomaly.** `OPTIONS /` returns `400` with a 0-byte body while `GET /` returns
200/56,923 B [M1-options-matrix.txt:14, 37-52; M1-anon-probe-matrix.txt:59; M1-root-headers.txt:7-20].
It is inconsistent with the rest of the surface: `/oauth/code/success` (also a page-like route) answers
OPTIONS with 204, and `/robots.txt` answers 405. Recorded as an unresolved edge/app-router handling
artifact, not as an auth signal [M1-options-matrix.txt:7-9].

**5.5 robots / sitemap behaviour.** Both exist and are GET/HEAD-only with `x-nextjs-cache: HIT`
[M1-anon-probe-matrix.txt:80-81]. `robots.txt` is 527 B and enumerates the private surface it wants
crawlers to skip: `Disallow: /api`, `/oauth`, `/v1`, `/admin`, `/account`, `/login`, `/register`,
`/payment`, `/app`, `/feedback`, `/docs`, plus password flows [M1-anon-probe-matrix.txt:375-398];
it explicitly `Allow:`s `/api/public-assets/`. `sitemap.xml` is **not tiny** (109,314 B, 841 unique URLs,
0 duplicates) and contains no `/api`, `/oauth`, `/v1`, `/admin` locations — content-only SEO surface
[M1-anon-probe-matrix.txt:404-438]. Both reject OPTIONS with 405 (`Allow: GET`, `Allow: HEAD`)
[M1-options-matrix.txt:31-32].

**5.6 Client bundles carry almost no protocol information.** Across the 12 analysed chunks the only
endpoint strings are `/api/download/client`, `/api/web/events`, `/api/public-assets/`; there are **zero**
matches for `/oauth/`, `/api/oauth`, `x-api-key`, `authorization`, `anthropic-version`, `anthropic-beta`,
and **zero** for every model family probed (Iris/Zeus/Gaia/Apollo/Metis/DeepSeek/Claude/Kimi/GLM)
[M1-js-static-analysis.txt:56-69, 84-96, 97-124]. The only outbound host of note is `https://my.feishu.cn`
[M1-js-static-analysis.txt:132]. Method-surface discovery therefore cannot come from the frontend.
Provenance caveat carried over from the earlier capture: `M1-js-static-analysis.txt:4` states 524,648 B
for the 12 chunks, whereas the per-chunk byte counts in `M1-js-chunks.tsv:2-13` sum to 526,000 B
(Δ 1,352 B). No conclusion depends on the total (all scans are string-level), but the aggregate size
must not be quoted as a verified figure; the per-chunk values are the trustworthy primitive.

**5.7 Anonymous download path + session minting (new).** `GET /api/download/client` with no parameter is a
`400 invalid_platform` [M1-anon-probe-matrix.txt:76]. `?platform=linux` yields `302` to
`https://agi-phanthy-dev-1252788780.cos.ap-beijing.myqcloud.com/releases/latest/PhanthyCode-linux-x86_64.tar.gz`
and **mints two cookies** (`phanthy_vid`, 90 d; `phanthy_vsid`, 30 min; `Secure; HttpOnly; SameSite=lax`),
values redacted [M1-anon-probe-matrix.txt:338-355]. `?platform=darwin` yields the identical
`400 invalid_platform` [M1-anon-probe-matrix.txt:357-366]. So the accepted platform vocabulary is **not**
the conventional `{linux, darwin, win32}` — only `linux` is proven.

**5.8 No CORS surface.** With `Origin: https://example.com`, neither OPTIONS nor GET on
`/api/oauth/profile` or `/v1/messages` returns `access-control-allow-origin`, `-methods` or `-headers`,
`vary` never gains `origin`, and statuses are unchanged (204/401 and 204/405)
[M1-anon-probe-matrix.txt:291-335]. Cross-origin browser reads are therefore blocked for an arbitrary
origin; whether an allowlist exists is not established.

**5.9 Header posture and error envelope.** Only HSTS (`max-age=31536000; includeSubDomains`) is sent;
no CSP, no `X-Content-Type-Options`, no `X-Frame-Options`, no `Referrer-Policy`, and `x-powered-by`
is disclosed [M1-anon-probe-matrix.txt:87-102]. Errors use one envelope,
`{"error":{"code":"...","message":"..."}}`, observed with codes `missing_login`,
`unsupported_response_type`, `invalid_platform` [M1-anon-probe-matrix.txt:60, 66, 76].

**5.10 Auth scheme (partial).** The only observed scheme is `Authorization: Bearer` on the
`/api/oauth/*` surface, advertised via `WWW-Authenticate: Bearer realm="phanthy-code",
error="invalid_request"` [M1-anon-probe-matrix.txt:66, 69]. The `/v1/*` auth scheme is **unknown** —
`x-api-key` appears nowhere in bundles [M1-js-static-analysis.txt:84-96].

---

## 6. Open questions M2 must resolve

1. **Auth scheme on `/v1/messages`** — `Authorization: Bearer <oauth access token>` vs `x-api-key`; needs
   a user-supplied token, since M1 had none [M0-preflight.json:19] and governance forbids guessing
   [00-governance.md:11-19]. Current state: scheme unknown [M1-anon-probe-matrix.txt:70-71].
2. **Request/response schema of `POST /v1/messages`** and whether it is Anthropic-Messages-compatible
   (`model`/`max_tokens`/`messages`), and whether `count_tokens` is folded in — the path itself 404s
   [M1-anon-probe-matrix.txt:72]. Requires the single governance-sanctioned POST
   [00-governance.md:18-19].
3. **`/api/web/events` semantics** — POST-only but named like an event stream; need content type
   (`text/event-stream`?), request body, auth, and streaming/chunking behaviour
   [M1-options-matrix.txt:26].
4. **Error taxonomy** — only three codes observed; unknown whether invalid/expired token yields a
   distinct code, whether any `/api` path returns JSON 404, and how 429 / rate-limit failures look
   (no `RateLimit-*` headers observed in any capture) [M1-anon-probe-matrix.txt:87-97].
5. **Model IDs** — absent from bundles and `/v1/models` 404s [M1-js-static-analysis.txt:97-124,
   M1-anon-probe-matrix.txt:73]; must come from an authenticated response or from documentation.
6. **Token lifecycle** — JWT claims, `exp`/`refresh` lifetimes, whether the RS256 key behind
   `kid=configured-key` ever rotates [M1-jwks.json:22], and what scopes `user:sessions:claude_code` /
   `user:mcp_servers` unlock [M1-openid-configuration.json:20-26].
7. **Missing API-key minting route** — `phanthy_cli` and `claude_cli` `create_api_key` both 404
   [M1-anon-probe-matrix.txt:67-68]; locate the real minting path (or confirm removal) using an
   authenticated session, and explain why `claude_cli/roles` survives with a 401 guard
   [M1-anon-probe-matrix.txt:69].
8. **Full `platform` vocabulary** of `/api/download/client` and the artifact set in the COS bucket
   (only `linux` proven; `darwin` rejected) [M1-anon-probe-matrix.txt:338-366] — M1 deliberately did not
   enumerate further values.
9. **`/api/public-assets` (no trailing slash)** — the 308 target was not followed; whether it lists
   downloadable assets anonymously is open [M1-anon-probe-matrix.txt:77].
10. **`OPTIONS /` 400 anomaly and 405-without-Allow** — confirm at the app-router level whether these are
    framework artifacts or intentional edge rules [M1-options-matrix.txt:7-9, 37-52].

---

### Provenance of this index
All 19 required endpoints were probed anonymously with GET (HEAD substituted on the four POST-only paths),
plus the two mandated supplementary families (CORS; `platform=linux|darwin`) and the robots/sitemap body
fetches. No request failed, so no retry/abort path was exercised. The only deviation from the brief:
`/sitemap.xml` was assumed tiny but is 109,314 B, so its verbatim body is replaced by a byte-exact
digest, head/tail excerpts and a parse summary — the reproduction command is in
[M1-anon-probe-matrix.txt:408-412].
