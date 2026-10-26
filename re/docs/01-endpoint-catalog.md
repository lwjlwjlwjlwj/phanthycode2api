# 01 · Endpoint Catalog — `https://code.phanthy.com/`

Milestone: **M2 / T2.1** · Author: reverse-engineering analyst (M2) · Language: English
Document status: complete for the anonymous surface. Authenticated request/response bodies remain
`BLOCKED-CRED` (see §6).

---

## 1. Scope, target, capture date, methodology, evidence legend

### 1.1 Scope

This catalog enumerates every HTTP endpoint of `https://code.phanthy.com/` that is either (a) directly
observed in the M1 evidence package, or (b) asserted by the in-repo Go implementation as an upstream
contract. It records method, URL, auth scheme, parameters, response shape, error responses, observed
status codes and an evidence anchor for each row.

Out of scope: the marketing pages reachable from `/sitemap.xml` (content-only SEO surface, no protocol
information), and the untrusted repository `https://github.com/witt3rd/oh-my-hermes` (never read, run or
installed — policy at `00-governance.md:22-32`).

### 1.2 Capture date and provenance

| item | value | anchor |
|---|---|---|
| live capture window (M1) | `2026-10-26T05:58:24+08:00` → `2026-10-26T06:14:37+08:00` (= `2026-10-25T21:58:24Z` → `22:14:37Z`) | `M1-index.md:15`, `M1-anon-probe-matrix.txt:2-3` |
| M1 OPTIONS sweep | `2026-10-26T06:00:47+08:00` | `M1-options-matrix.txt:2` |
| M1 index compiled | `2026-10-26T06:16:58+08:00` | `M1-index.md:16` |
| M1 independent acceptance | `2026-10-26T06:42:00+08:00`, verdict `PASS-WITH-GAPS`, 0 blocking | `acceptance/M1.md:8-9, 16-21` |
| M2 authoring | `2026-10-26` (no new network capture) | this run |
| identity used for all live facts | anonymous: no `Authorization`, no `Cookie`, no `x-api-key` | `M1-anon-probe-matrix.txt:6` |

**M2 issued zero network requests.** Every `VERIFIED` statement below is inherited from M1 and carries an
M1 file:line anchor. Anything not in M1 is labelled `PRIOR-ART` (in-repo Go/README) or `INFERRED`
(protocol reasoning) and is never presented as live-verified.

### 1.3 Methodology

1. Read all nine M1 artifacts and the M1 acceptance report end to end.
2. Extract every endpoint signal from three independent M1 sources and cross-check them:
   the OPTIONS `Allow` matrix (`M1-options-matrix.txt:13-32`), the anonymous GET/HEAD status matrix
   (`M1-anon-probe-matrix.txt:59-81`), and the consolidated index (`M1-index.md:54-74`).
3. For each endpoint, attach the Go implementation's actual outbound call (or record that no call site
   exists), because the prior art is the only available evidence for authenticated and POST-only shapes.
4. For every claim, choose exactly one evidence grade and one anchor. Where existence and semantics have
   different confidence, both are stated separately rather than averaged.
5. Where the M1 acceptance report flagged a numeric inconsistency, the per-file value is used instead of
   the summary value (see §1.6).

### 1.4 Evidence grades

| grade | definition | allowed use |
|---|---|---|
| `VERIFIED` | Directly observed on the live target during M1; anchored to an evidence file:line. | May be stated as fact about the target on the capture date. |
| `PRIOR-ART` | Derived only from the in-repo Go implementation / README; anchored to `file:line` in `/workspace`. | Describes what this repository does, **not** what the target does. Never promoted to VERIFIED. |
| `INFERRED` | Reasoned from OIDC metadata, RFCs or naming convention; the basis is stated inline. | A hypothesis with an explicit foundation; must be re-tested with a token. |
| `BLOCKED-CRED` | Requires the user's own token / authorization code; no request has been or will be made by M2. | Recorded as unknown. No request/response payload is fabricated. |

### 1.5 Existence-status vocabulary

| code | meaning | how it is proven |
|---|---|---|
| `EX-200` | Route exists and returned a 200 to an anonymous GET | `M1-anon-probe-matrix.txt:59-81` |
| `EX-400` | Route exists; input validation reached and failed before any auth check | `M1-anon-probe-matrix.txt:46-49` |
| `EX-401` | Route exists and is guarded by a bearer check | `M1-anon-probe-matrix.txt:50` |
| `EX-405-POSTONLY` | Route exists, GET/HEAD rejected, POST is the advertised method | OPTIONS `Allow: OPTIONS, POST` |
| `EX-302` | Route exists and issued a redirect | `M1-anon-probe-matrix.txt:341-353` |
| `EX-308` | Route exists only as a trailing-slash normalisation | `M1-anon-probe-matrix.txt:247-255` |
| `ABSENT-404` | No handler at that path; Next.js app-router HTML shell returned | `M1-anon-probe-matrix.txt:51` |

### 1.6 Inherited caveats from the M1 acceptance report

These defects live in the inherited evidence, not in this document. They do not change any endpoint
finding; they change which numbers this catalog is allowed to quote.

| id | flagged issue | anchor | effect on this catalog |
|---|---|---|---|
| D1 | Index claims "33 requests total"; the cited file records 54 | `acceptance/M1.md:60`; `M1-index.md:21` vs `M1-anon-probe-matrix.txt:13` | No probe-count total is quoted here; per-endpoint observations are cited individually. |
| D2 | "≤3 requests per endpoint" contradicted by a recorded per-endpoint max of 5 (`/robots.txt`) | `acceptance/M1.md:61`; `M1-anon-probe-matrix.txt:8, 13` | Request counts are treated as 1–5 per path, never as a hard cap of 3. |
| D3 | OIDC body stated as 683 B; served body is 594 B | `acceptance/M1.md:32`; `M1-anon-probe-matrix.txt:465-466` | Only field-level equality is claimed for the discovery document (§3.6). No byte count is quoted. |
| D4 | "405 carries no `Allow` header at all" is over-general | `acceptance/M1.md:63`; `M1-anon-probe-matrix.txt:447-448` vs `M1-options-matrix.txt:370-375` | Restated correctly: GET/HEAD 405 on API routes omit `Allow`; OPTIONS 405 on `robots.txt`/`sitemap.xml` **do** send `Allow: GET` / `Allow: HEAD`. |
| D5 | "four read-only handlers" followed by five paths | `acceptance/M1.md:64`; `M1-index.md:130-133` | §2 lists the five read-only paths explicitly; no count word is used. |
| D6 | JS aggregate 524,648 B vs 526,000 B summed per chunk | `acceptance/M1.md:65`; `M1-js-static-analysis.txt:4` vs `M1-js-chunks.tsv:2-13` | Only per-chunk facts are cited (§3.11, §3.13); the aggregate is never quoted. |
| D7 | "only endpoint strings" omitted `/v1/wechat-contact-qr.png` | `acceptance/M1.md:66`; `M1-js-static-analysis.txt:38, 65-69` | That path is a static image, not an API; it is excluded but disclosed here. |
| D8 | M0 preflight timestamp reads as a placeholder | `acceptance/M1.md:67`; `M0-preflight.json:3` | M0 is not used as a time source anywhere in this catalog. |

Also carried forward (not a defect, a scope note): the untrusted repository was used as read-only plain
text only (`M0-preflight.json:20`; policy `00-governance.md:22-32`).

---

## 2. Summary table

`auth` gives the **exact header names** where known. `grade` separates existence evidence from semantic
evidence; where they differ both are shown.

| ID | method | path | auth | purpose | existence status | evidence grade | evidence anchor |
|---|---|---|---|---|---|---|---|
| EP-01 | GET | `/` | none | Next.js web app shell | `EX-200` | VERIFIED | `M1-anon-probe-matrix.txt:59, 87-101`; `M1-root-headers.txt:7-20` |
| EP-02 | GET | `/oauth/authorize` | none to reach handler (login happens in browser) | Authorization-code + PKCE entry point | `EX-400` (validation before auth) | VERIFIED (existence, error text); PRIOR-ART (param set); INFERRED (success redirect) | `M1-anon-probe-matrix.txt:60, 104-112`; `cmd/login/main.go:144-154` |
| EP-03 | POST | `/oauth/token` | none (public client; `client_id` in body) | Code→token and refresh-token grant | `EX-405-POSTONLY` | VERIFIED (existence only); PRIOR-ART (form/headers/response fields); BLOCKED-CRED (actual exchange) | `M1-options-matrix.txt:16`; `M1-anon-probe-matrix.txt:61-62`; `cmd/login/main.go:177-193`; `internal/upstream/client.go:238-259` |
| EP-04 | POST | `/oauth/revoke` | unknown | RFC 7009 token revocation | `EX-405-POSTONLY` | VERIFIED (existence); INFERRED (semantics); BLOCKED-CRED (body) | `M1-options-matrix.txt:17`; `M1-anon-probe-matrix.txt:63-64`; `M1-openid-configuration.json:5` |
| EP-05 | GET | `/oauth/code/success` | none | Static "login complete" landing page (also the `redirect_uri`) | `EX-200` | VERIFIED (200/static); INFERRED (query handling) | `M1-anon-probe-matrix.txt:65, 143-152`; `cmd/login/main.go:69-71, 148` |
| EP-06 | GET | `/.well-known/openid-configuration` | none | OAuth/OIDC discovery metadata | `EX-200` | VERIFIED (field-level) | `M1-anon-probe-matrix.txt:450-466`; `M1-openid-configuration.json:2-26` |
| EP-07 | GET | `/.well-known/jwks.json` | none | Public signing-key set (RS256) | `EX-200` | VERIFIED | `M1-jwks.json:22, 29-32` |
| EP-08 | GET | `/api/oauth/profile` | `Authorization: Bearer <access_token>` | Account profile of the token holder | `EX-401` | VERIFIED (existence + guard); PRIOR-ART (client call shape); BLOCKED-CRED (success schema) | `M1-anon-probe-matrix.txt:66, 154-163`; `internal/upstream/client.go:369-394` |
| EP-09 | GET | `/api/oauth/claude_cli/roles` | `Authorization: Bearer <access_token>` | Role/entitlement lookup for the claude_cli variant | `EX-401` | VERIFIED (existence + guard); BLOCKED-CRED (semantics) | `M1-anon-probe-matrix.txt:69, 184-190` |
| EP-10 | POST | `/v1/messages` | `x-api-key: <api_key>` **or** `Authorization: Bearer <access_token>` (PRIOR-ART) | Anthropic Messages API surface (chat/inference) | `EX-405-POSTONLY` | VERIFIED (existence); PRIOR-ART (wire contract); BLOCKED-CRED (auth scheme + live response) | `M1-options-matrix.txt:23`; `M1-anon-probe-matrix.txt:70-71`; `internal/upstream/client.go:331-367`; `internal/upstream/payload.go:47-60` |
| EP-11 | POST | `/api/web/events` | unknown (presumed required) | Web-event ingestion / stream of unknown semantics | `EX-405-POSTONLY` | VERIFIED (existence); INFERRED (everything else) | `M1-options-matrix.txt:26`; `M1-anon-probe-matrix.txt:74-75`; `M1-js-static-analysis.txt:63` |
| EP-12 | GET | `/api/download/client` | none | Client artifact download; issues visitor cookies | `EX-400` / `EX-302` | VERIFIED | `M1-anon-probe-matrix.txt:76, 337-366` |
| EP-13 | GET | `/api/public-assets` (via 308 from `/api/public-assets/`) | unknown | Public-asset surface (target not followed) | `EX-308` | VERIFIED (redirect only); INFERRED (target semantics) | `M1-anon-probe-matrix.txt:77, 247-255`; `M1-js-static-analysis.txt:59, 62` |
| EP-14 | GET | `/robots.txt` | none | Crawler policy | `EX-200` | VERIFIED | `M1-anon-probe-matrix.txt:80, 369-402`; `M1-options-matrix.txt:31` |
| EP-15 | GET | `/sitemap.xml` | none | SEO sitemap (841 URLs) | `EX-200` | VERIFIED | `M1-anon-probe-matrix.txt:81, 404-438`; `M1-options-matrix.txt:32` |

Note on method discovery: 405 responses on API routes carry **no** `Allow` header, so `OPTIONS` is the only
reliable method oracle (`M1-anon-probe-matrix.txt:447-448`). `OPTIONS /` is itself anomalous (400, empty
body) and yields no method list (`M1-options-matrix.txt:14, 37-52`).

---

## 3. Endpoint detail

### 3.1 EP-01 · `GET /` — web app shell

- **Method / full URL:** `GET https://code.phanthy.com/` (HEAD implied by the router; `OPTIONS` returns 400 and
  therefore does not enumerate methods).
- **Existence:** `EX-200`.
- **Auth:** none. No `Authorization`, no cookie required.
- **Parameters:** none.
- **Response:** `200`, `text/html; charset=utf-8`, 56,923 bytes transferred (no `content-length`; the body is
  streamed). Document is `<html dir="ltr" lang="zh-CN">`, a Next.js app shell with `/_next/static/*` assets and
  a preload link to `/phanthy-code-logo-icon.png`.
- **Headers observed:** `strict-transport-security: max-age=31536000; includeSubDomains`,
  `cache-control: private, must-revalidate, no-cache, no-store, max-age=0`, `vary: rsc, next-router-state-tree, ...`,
  `x-powered-by: Next.js`, `via: kong/2.5.1`, `server: TencentEdgeOne`.
- **Errors:** none observed. Unknown sub-paths return the 26 KB HTML 404 shell (§4), not a JSON error.
- **Observed status codes:** `200` (GET), `400` (OPTIONS, 0 B, no `Allow`).
- **Evidence:** `M1-anon-probe-matrix.txt:59, 87-101`; `M1-root-headers.txt:7-31`;
  `M1-options-matrix.txt:14, 37-52`; `M1-index.md:145-149`.
- **Grade:** VERIFIED. The `OPTIONS /` 400 is an app-router artifact recorded in M1; it is not an auth signal.

### 3.2 EP-02 · `GET /oauth/authorize` — authorization-code entry point

- **Method / full URL:** `GET https://code.phanthy.com/oauth/authorize`
  (`Allow: GET, HEAD, OPTIONS` — `M1-options-matrix.txt:15`).
- **Existence:** `EX-400`. Validation is reached without any authentication, which proves the handler is
  present and that parameter validation precedes login (`M1-anon-probe-matrix.txt:49`).
- **Auth:** none needed to reach the handler. A browser login session is required to complete the flow
  (`BLOCKED-CRED` for the success path).
- **Query parameters (PRIOR-ART, `cmd/login/main.go:144-154`):**

  | parameter | value sent by prior art | grade |
  |---|---|---|
  | `client_id` | `phanthy-code-cli` | PRIOR-ART (`cmd/login/main.go:28, 146`) |
  | `response_type` | `code` (only value accepted — see error text) | VERIFIED by server error message (`M1-anon-probe-matrix.txt:112`) |
  | `redirect_uri` | `https://code.phanthy.com/oauth/code/success` | PRIOR-ART (`cmd/login/main.go:148`) |
  | `scope` | `user:inference user:profile user:sessions:claude_code` (space-delimited) | PRIOR-ART (`cmd/login/main.go:149`); scope names VERIFIED (`M1-openid-configuration.json:20-26`) |
  | `code_challenge` | base64url(SHA-256(verifier)), unpadded | PRIOR-ART (`cmd/login/main.go:55-56`) |
  | `code_challenge_method` | `S256` (only supported method) | VERIFIED (`M1-openid-configuration.json:14-16`) |
  | `state` | `p2a-login` (static) | PRIOR-ART (`cmd/login/main.go:152`) |

- **Response shape (success):** not observed. `INFERRED` from RFC 6749 §4.1.2 plus the prior-art note that the
  browser lands on `/oauth/code/success` (`cmd/login/main.go:69-71`): `302` to `redirect_uri?code=<code>&state=<state>`.
- **Error response (observed):** `400 application/json`, 101 B,
  `{"error":{"code":"unsupported_response_type","message":"Only authorization code flow is supported."}}`
  — envelope is `{error:{code,message}}`, identical in structure to EP-08/EP-12 errors.
- **Errors not observed:** `invalid_request`, `invalid_scope`, `invalid_client` — convention only (`INFERRED`).
- **Observed status codes:** `400` (GET, no params), `204` (OPTIONS), `400` (OPTIONS on `/` is unrelated).
- **Evidence:** `M1-anon-probe-matrix.txt:60, 104-112`; `M1-options-matrix.txt:15`; `M1-openid-configuration.json:3, 7-16`.
- **Grade:** existence VERIFIED; parameter set PRIOR-ART (PKCE/`S256`/`code` parts VERIFIED); success redirect INFERRED.

### 3.3 EP-03 · `POST /oauth/token` — code and refresh-token exchange

- **Method / full URL:** `POST https://code.phanthy.com/oauth/token`
  (`Allow: OPTIONS, POST` — `M1-options-matrix.txt:16`). GET and HEAD return `405`, 0 B, no `Allow` header.
- **Existence:** `EX-405-POSTONLY`. The POST itself was never exercised (`BLOCKED-CRED`).
- **Auth:** public client. `token_endpoint_auth_methods_supported: ["none"]` is VERIFIED
  (`M1-openid-configuration.json:17-19`), i.e. no `Authorization` header, no client secret; `client_id` travels
  in the form body (`cmd/login/main.go:183`).
- **Request media type:** `application/x-www-form-urlencoded` (PRIOR-ART, `cmd/login/main.go:191`).
- **Body — grant `authorization_code` (PRIOR-ART, `cmd/login/main.go:178-183`):**
  `grant_type=authorization_code`, `code=<code>`, `code_verifier=<verifier>`, `redirect_uri=<same as authorize>`,
  `client_id=phanthy-code-cli`.
- **Body — grant `refresh_token` (PRIOR-ART, `internal/upstream/client.go:238-242`):**
  `grant_type=refresh_token`, `refresh_token=<refresh_token>`, `client_id=phanthy-code-cli`.
  No `code_verifier` is sent on this grant.
- **Headers (PRIOR-ART):** `Content-Type: application/x-www-form-urlencoded` (`cmd/login/main.go:191`),
  `Accept: application/json` (`:192`), `anthropic-beta: oauth-2025-04-20` (`:193`; also
  `internal/upstream/client.go:243` using `OAuthBetaHeader` defined at `:139`). Why an OAuth token endpoint
  requires an Anthropic beta header is unexplained by any capture — `INFERRED` that the upstream shares the
  Anthropic OAuth implementation.
- **Response shape (PRIOR-ART):** JSON object with `access_token` (required by prior art, `cmd/login/main.go:215-217`),
  `refresh_token`, `expires_in` (seconds) — struct at `cmd/login/main.go:171-175`; `expires_in` drives
  `expiresAt` at `:108` and is re-read on refresh at `internal/upstream/client.go:256-258`.
  Additional conventional fields (`token_type`, `scope`) are **not** referenced anywhere and are `UNKNOWN`.
- **Error response:** no HTTP error observed. Prior art assumes two error carriers:
  (a) HTTP ≥400 with an `{error:{code,message}}` body (`cmd/login/main.go:208-210`);
  (b) HTTP **200** carrying an `{error:{code,message}}` envelope, which `formPost` treats as an error
  (`internal/upstream/client.go:158-169`). Carrier (b) is a PRIOR-ART assumption, not an observation.
- **Named error:** `invalid_grant` is asserted by the login tool's hint text for an expired code
  (`cmd/login/main.go:94`). PRIOR-ART.
- **Observed status codes:** `405` (GET), `405` (HEAD), `204` (OPTIONS).
- **Evidence:** `M1-options-matrix.txt:16`; `M1-anon-probe-matrix.txt:61-62, 114-121`;
  `cmd/login/main.go:177-219`; `internal/upstream/client.go:230-260`.
- **Grade:** existence VERIFIED; request/response PRIOR-ART; errors PRIOR-ART/INFERRED; live exchange `BLOCKED-CRED`.

### 3.4 EP-04 · `POST /oauth/revoke` — token revocation

- **Method / full URL:** `POST https://code.phanthy.com/oauth/revoke`
  (`Allow: OPTIONS, POST` — `M1-options-matrix.txt:17`). GET/HEAD → `405`, 0 B, no `Allow`.
- **Existence:** `EX-405-POSTONLY`. Advertised by `revocation_endpoint` in the discovery document
  (`M1-openid-configuration.json:5`).
- **Auth:** unknown. No prior-art call site exists anywhere in `/workspace` (grep for `revoke` / `Revoke`
  returns only the M1 evidence and the discovery JSON). `INFERRED`: public client, `token` supplied in the body,
  no bearer header.
- **Parameters (INFERRED, RFC 7009 §2.1):** form-urlencoded `token=<token>` and optional
  `token_type_hint=access_token|refresh_token`; plausible but unproven `client_id`.
- **Response shape (INFERRED):** `200` with an empty body per RFC 7009 §2.2. Not observed.
- **Errors (INFERRED):** RFC 7009 §2.2.1 `unsupported_token_type`; not observed.
- **Observed status codes:** `405` (GET), `405` (HEAD), `204` (OPTIONS).
- **Evidence:** `M1-options-matrix.txt:17`; `M1-anon-probe-matrix.txt:63-64, 130-141`; `M1-openid-configuration.json:5`;
  `M1-index.md:94`.
- **Grade:** existence VERIFIED; all semantics INFERRED; body `BLOCKED-CRED`.

### 3.5 EP-05 · `GET /oauth/code/success` — login-complete landing page

- **Method / full URL:** `GET https://code.phanthy.com/oauth/code/success` (`Allow: GET, HEAD, OPTIONS`).
- **Existence:** `EX-200`.
- **Auth:** none.
- **Query parameters (INFERRED):** `code` and `state`, because this path is the registered `redirect_uri`
  (`cmd/login/main.go:148`) and the prior-art instructions say the page shows the authorization code for manual
  copying (`cmd/login/main.go:70-71`). The served HTML is static, so `code` must be read client-side.
- **Response shape:** `200 text/html; charset=utf-8`, 1,769 B, hand-written page, `<title>PhanthyCode 登录完成</title>`,
  inline `<style>`; no dynamic data in the server-side body.
- **Errors:** none observed.
- **Observed status codes:** `200` (GET), `204` (OPTIONS).
- **Evidence:** `M1-anon-probe-matrix.txt:65, 143-152`; `M1-options-matrix.txt:18`; `M1-index.md:95`.
- **Grade:** existence/static body VERIFIED; query handling INFERRED. Whether the page displays the code at all
  cannot be confirmed without a browser authorization (`BLOCKED-CRED`).

### 3.6 EP-06 · `GET /.well-known/openid-configuration` — discovery metadata

- **Method / full URL:** `GET https://code.phanthy.com/.well-known/openid-configuration`.
- **Existence:** `EX-200`; provenance fetch proves the stored JSON is served from this exact URL
  (`M1-anon-probe-matrix.txt:450-464`).
- **Auth:** none.
- **Parameters:** none.
- **Response shape:** `200 application/json`. Fields (`M1-openid-configuration.json:2-26`):
  `issuer=https://code.phanthy.com`; `authorization_endpoint=/oauth/authorize`; `token_endpoint=/oauth/token`;
  `revocation_endpoint=/oauth/revoke`; `jwks_uri=/.well-known/jwks.json`;
  `response_types_supported=["code"]`; `grant_types_supported=["authorization_code","refresh_token"]`;
  `code_challenge_methods_supported=["S256"]`; `token_endpoint_auth_methods_supported=["none"]`;
  `scopes_supported` = `user:profile`, `user:inference`, `user:sessions:claude_code`, `user:mcp_servers`,
  `user:file_upload`.
- **Important absences (VERIFIED by the complete field list):** no `userinfo_endpoint`,
  no `id_token_signing_alg_values_supported`, no `subject_types_supported`, and no `openid` scope.
  This is OAuth 2.0 + PKCE metadata under an OIDC file name.
- **Byte-size caveat:** the served body is minified; the stored file is pretty-printed. Only **field-level**
  equality is claimed — see `acceptance/M1.md:32` and `M1-anon-probe-matrix.txt:467-468` (caveat D3 in §1.6).
- **Errors:** none observed.
- **Observed status codes:** `200`.
- **Evidence:** `M1-anon-probe-matrix.txt:450-466`; `M1-openid-configuration.json:2-26`; `M1-index.md:114-128`.
- **Grade:** VERIFIED (field values).

### 3.7 EP-07 · `GET /.well-known/jwks.json` — signing keys

- **Method / full URL:** `GET https://code.phanthy.com/.well-known/jwks.json`.
- **Existence:** `EX-200`.
- **Auth:** none.
- **Parameters:** none.
- **Response shape:** `200 application/json`; `keys` contains exactly one RSA key:
  `kty=RSA`, `e=AQAB`, `use=sig`, `alg=RS256`, `kid=configured-key`. Public key material only — no private data.
- **Interpretation (`INFERRED`):** the presence of an RS256 JWKS implies the access token may be a signed JWT;
  no token was ever observed, so the claims, `exp` and signature are unverified. `kid=configured-key` is a
  default-style identifier, so key rotation is not demonstrated.
- **Errors:** none observed.
- **Observed status codes:** `200`.
- **Evidence:** `M1-jwks.json:5-22, 23-35`; `M1-index.md:126-128`.
- **Grade:** VERIFIED (key material); token format INFERRED.

### 3.8 EP-08 · `GET /api/oauth/profile` — account profile

- **Method / full URL:** `GET https://code.phanthy.com/api/oauth/profile` (`Allow: GET, HEAD, OPTIONS`).
- **Existence:** `EX-401`.
- **Auth (VERIFIED as required):** `Authorization: Bearer <access_token>`. The challenge header is
  `WWW-Authenticate: Bearer realm="phanthy-code", error="invalid_request", error_description="Bearer access token is required."`
- **Parameters:** none.
- **Error response (observed):** `401 application/json`, 79 B,
  `{"error":{"code":"missing_login","message":"Bearer access token is required."}}`.
- **Success response shape:** `BLOCKED-CRED`. The prior-art client returns an untyped `map[string]any`
  (`internal/upstream/client.go:369-394`), so **no field is known**. Note this client method is defined but has
  no call site anywhere in `/workspace` (only `client.go:370`), so the prior art does not constrain the schema.
- **Errors not observed:** expired/invalid token behaviour (expected `401` with a different `code`) is `UNKNOWN`.
- **Observed status codes:** `401` (GET), `204` (OPTIONS).
- **Evidence:** `M1-anon-probe-matrix.txt:66, 154-163`; `M1-options-matrix.txt:19`; `M1-index.md:96`.
- **Grade:** existence + guard VERIFIED; success schema `BLOCKED-CRED`.

### 3.9 EP-09 · `GET /api/oauth/claude_cli/roles` — roles / entitlements

- **Method / full URL:** `GET https://code.phanthy.com/api/oauth/claude_cli/roles` (`Allow: GET, HEAD, OPTIONS`).
- **Existence:** `EX-401` — identical guard and byte-identical 79 B body to EP-08
  (`M1-anon-probe-matrix.txt:69, 184-190`).
- **Auth:** `Authorization: Bearer <access_token>`, same realm.
- **Parameters:** none observed. `INFERRED`: a query parameter may select a product/tenant; no evidence exists.
- **Success response shape:** `BLOCKED-CRED`. `INFERRED` purpose from the path name only: a list of roles or
  plan entitlements for the `claude_cli` integration (basis: `claude_cli` sibling naming, and the fact that the
  `create_api_key` sibling was removed while this route retains a guard — `M1-index.md:97-99`).
- **Errors:** same `missing_login` envelope.
- **Observed status codes:** `401` (GET), `204` (OPTIONS).
- **Evidence:** `M1-anon-probe-matrix.txt:69, 184-190`; `M1-options-matrix.txt:22`.
- **Grade:** existence + guard VERIFIED; semantics `BLOCKED-CRED`.

### 3.10 EP-10 · `POST /v1/messages` — Anthropic Messages surface

- **Method / full URL:** `POST https://code.phanthy.com/v1/messages`
  (`Allow: OPTIONS, POST` — `M1-options-matrix.txt:23`). GET and HEAD → `405`, 0 B, no `Allow`.
- **Existence:** `EX-405-POSTONLY`. The POST was never issued (`BLOCKED-CRED`).
- **Auth (two candidate schemes, neither observed):**
  - `x-api-key: <api_key>` — preferred by prior art when an API key exists (`internal/upstream/client.go:344-345`).
  - `Authorization: Bearer <access_token>` — fallback when no API key exists (`internal/upstream/client.go:346-347`),
    and the documented fallback in the login tool when `create_api_key` 404s (`cmd/login/main.go:97-104`).
  - Additional prior-art headers: `x-app: cli` (`client.go:342`), `User-Agent: phanthycode2api/1.0` (`:343`),
    `Accept: application/json, text/event-stream` (`:341`), and `X-Claude-Code-Session-Id: <uid>` (`:351-353`).
  - No `anthropic-version` header is sent anywhere in the repository (`INFERRED`: the upstream tolerates its
    absence — basis: the prior art is in production use per `README.md:3-17`, but no capture proves it).
- **Body (PRIOR-ART schema, `internal/upstream/payload.go:47-60`):** `model` (string), `max_tokens` (int;
  the struct has no `omitempty`, so it is always serialized), `messages` (array of `{role,content[]}`),
  and optional `system`, `temperature`, `top_p`, `stream`, `tools[{name,description,input_schema}]`,
  `tool_choice`, `stop_sequences`, `metadata{user_id}`.
- **Proxy-side normalisation (PRIOR-ART):** `stream` is forced to `true` (`payload.go:109`);
  `max_tokens` outside 1..65536 becomes `8192` (`payload.go:117-119`).
- **Response shape:** never observed. For streaming, the prior art parses Anthropic SSE event names
  `message_start`, `content_block_start`, `content_block_delta`, `message_delta`, `message_stop`
  (`internal/upstream/sse.go:193-275`) — see `02-usage-semantics.md` §3. A non-streaming JSON response was never
  produced by this client because it always asks for a stream; its shape is `INFERRED` from the
  `message_start` payload (`sse.go:18-25`) and test fixtures (`internal/upstream/sse_test.go:11`).
- **Error responses:** not observed. Expected classes are enumerated in `02-usage-semantics.md` §4
  (classification logic at `internal/upstream/client.go:76-107`).
- **Related paths that do NOT exist:** `/v1/messages/count_tokens` → 404 HTML shell (§4), so token counting is not
  a separate route (`M1-index.md:101`).
- **Observed status codes:** `405` (GET), `405` (HEAD), `204` (OPTIONS).
- **Evidence:** `M1-options-matrix.txt:23`; `M1-anon-probe-matrix.txt:70-71, 192-204`;
  `internal/upstream/client.go:331-367`; `internal/upstream/payload.go:47-60`.
- **Grade:** existence VERIFIED; wire contract PRIOR-ART; auth scheme and live response `BLOCKED-CRED`.

### 3.11 EP-11 · `POST /api/web/events` — web event surface (unknown semantics)

- **Method / full URL:** `POST https://code.phanthy.com/api/web/events`
  (`Allow: OPTIONS, POST` — `M1-options-matrix.txt:26`). GET and HEAD → `405`, 0 B, no `Allow`.
- **Existence:** `EX-405-POSTONLY`. The string also appears verbatim in the public layout bundle, so a browser
  client really does call it (`M1-js-static-analysis.txt:60-63`).
- **Auth:** unknown. No prior-art call site exists (grep over `/workspace` finds only M1 evidence and the JS scan).
- **Parameters / body / response:** **not established.** This endpoint is labelled `INFERRED` and the following
  are hypotheses, listed without assertion:
  - H1 — telemetry/beacon endpoint: JSON body, `204`/`2xx`, unauthenticated or cookie-auth, no response body
    (basis: a POST-only `/api/web/*` route called from the site shell).
  - H2 — server-sent event channel: a POST-triggered stream returning `text/event-stream` (basis: "events" naming;
    counter-indication: `EventSource` is GET-only, and GET returns 405 here).
  - H3 — user activity feed mutation: bearer-auth `POST` with a JSON filter, returning a JSON list
    (basis: the site's authenticated `/app` area and the `Bearer realm="phanthy-code"` convention on `/api/oauth/*`).
  - H4 — CSRF-style action endpoint relying on visitor cookies (`phanthy_vid` / `phanthy_vsid`, minted by EP-12).
  None of H1–H4 is supported by a response capture. Do not implement against any hypothesis.
- **Errors:** none observed.
- **Observed status codes:** `405` (GET), `405` (HEAD), `204` (OPTIONS).
- **Evidence:** `M1-options-matrix.txt:26`; `M1-anon-probe-matrix.txt:74-75, 223-235`;
  `M1-js-static-analysis.txt:63`; `M1-index.md:103`.
- **Grade:** existence VERIFIED; semantics INFERRED-only.

### 3.12 EP-12 · `GET /api/download/client` — client artifact download

- **Method / full URL:** `GET https://code.phanthy.com/api/download/client?platform=<value>`
  (`Allow: GET, HEAD, OPTIONS`).
- **Existence:** `EX-400` (no/`invalid` platform) and `EX-302` (accepted platform).
- **Auth:** **none** — anonymous callers receive the redirect (`M1-index.md:104`).
- **Query parameters:** `platform` (required; absent → 400). Proven accepted value: `linux`.
  Proven rejected value: `darwin` (identical 400 to the missing-parameter case). Full vocabulary `UNKNOWN`;
  the conventional `{linux, darwin, win32}` is **disproved** for the pair tested.
- **Response (accepted value):** `302`, `location: https://agi-phanthy-dev-1252788780.cos.ap-beijing.myqcloud.com/releases/latest/PhanthyCode-linux-x86_64.tar.gz`,
  empty body, plus two `Set-Cookie` headers: `phanthy_vid` (`Max-Age=7776000`, 90 d) and
  `phanthy_vsid` (`Max-Age=1800`, 30 min), both `Secure; HttpOnly; SameSite=lax`. Cookie **values were redacted
  and never persisted** (`acceptance/M1.md:37`).
- **Error response:** `400 application/json`, 79 B,
  `{"error":{"code":"invalid_platform","message":"Download platform is invalid."}}` — the error envelope matches
  EP-02/EP-08 structurally.
- **Side effect worth noting:** this anonymous GET mints a session; it is not a pure read
  (`M1-index.md:171-177`). The `releases/latest/` path segment implies a moving "latest" alias (`INFERRED`).
- **Observed status codes:** `400` (no query), `302` (`linux`, redirect not followed), `400` (`darwin`), `204` (OPTIONS).
- **Evidence:** `M1-anon-probe-matrix.txt:76, 237-245, 337-367`; `M1-options-matrix.txt:27`.
- **Grade:** VERIFIED for every observed branch; platform vocabulary INFERRED beyond `linux`.

### 3.13 EP-13 · `GET /api/public-assets` — public asset surface (redirect only)

- **Method / full URL:** observed as `GET https://code.phanthy.com/api/public-assets/`, which returns
  `308` → `/api/public-assets`.
- **Existence:** `EX-308`. The redirect **target was deliberately not followed** (`acceptance/M1.md:47`),
  so the target's own status is unprobed.
- **Auth:** unknown; the target was never fetched.
- **Parameters:** none observed.
- **Response:** `308`, `location: /api/public-assets`, `refresh: 0;url=/api/public-assets`, 18 B body
  literally `/api/public-assets` (no `content-type`).
- **Semantics (`INFERRED`):** an asset listing or catalog served by the app. Basis: the string is called from
  two public bundles (`M1-js-static-analysis.txt:59, 62`) and `robots.txt` explicitly `Allow`s
  `/api/public-assets/` (`M1-anon-probe-matrix.txt:383`). Content type, auth and shape are `UNKNOWN`.
- **Observed status codes:** `308` (GET and OPTIONS — the redirect short-circuits method evaluation).
- **Evidence:** `M1-anon-probe-matrix.txt:77, 247-255`; `M1-options-matrix.txt:28`; `M1-index.md:105`.
- **Grade:** redirect VERIFIED; target semantics INFERRED.

### 3.14 EP-14 · `GET /robots.txt`

- **Method / full URL:** `GET https://code.phanthy.com/robots.txt`. `OPTIONS` → `405` **with**
  `Allow: GET` / `Allow: HEAD` (`M1-options-matrix.txt:31, 370-375`) — the correct RFC 9110 behaviour, unlike
  the API-route 405s.
- **Existence:** `EX-200`.
- **Auth:** none.
- **Response:** `200 text/plain; charset=UTF-8`, 527 B, `x-nextjs-cache: HIT`,
  sha256 `e9df3e770411dccc14595ac4610fa43e52f6175bedab25f75bd3c464bda64134`.
- **Content:** `Allow: /`, `/download`, `/token-plan`, `/skills`, `/changelog`, `/work`, `/prompts`, `/llms.txt`,
  `/api/public-assets/`; `Disallow: /admin`, `/account`, `/api`, `/oauth`, `/v1`, `/logout`, `/login`, `/register`,
  `/payment`, `/app`, `/feedback`, `/reset-password`, `/forgot-password`, `/verify-email`, `/docs`;
  plus `Host` and `Sitemap` directives.
- **Interpretation:** the disallow list is the site's own enumeration of private surfaces. It is a **hint**, not
  proof that those paths exist (`INFERRED`); none of them was probed except `/api` and `/v1` prefixes.
- **Evidence:** `M1-anon-probe-matrix.txt:80, 369-402`; `M1-options-matrix.txt:31`; `M1-index.md:151-158`.
- **Grade:** VERIFIED.

### 3.15 EP-15 · `GET /sitemap.xml`

- **Method / full URL:** `GET https://code.phanthy.com/sitemap.xml`. `OPTIONS` → `405` with
  `Allow: GET` / `Allow: HEAD` (`M1-options-matrix.txt:32, 390-395`).
- **Existence:** `EX-200`.
- **Auth:** none.
- **Response:** `200 application/xml`, 109,314 B, `x-nextjs-cache: HIT`,
  sha256 `1c1f5014e54850f1c77bf7e391fa3d3eb0202410c81f4defd653a0a77768b812`; **841** unique `<loc>` entries,
  0 duplicates, and **0** occurrences of `/api/`, `/oauth/`, `/v1/`, `/admin`.
- **Interpretation:** content-only SEO surface; it leaks no API paths (`M1-index.md:155-158`).
- **Deviation recorded by M1:** the file was assumed small but is 109 KB, so the verbatim body was replaced by a
  digest plus head/tail excerpts (`M1-anon-probe-matrix.txt:404-412`).
- **Evidence:** `M1-anon-probe-matrix.txt:81, 404-438`; `M1-options-matrix.txt:32`; `acceptance/M1.md:48`.
- **Grade:** VERIFIED.

---

## 4. Negative inventory

### 4.1 Probed and NOT FOUND (Next.js HTML 404 shell)

All six return `404 text/html; charset=utf-8`, `x-powered-by: Next.js`, a preload link to
`/phanthy-code-logo-new.png`, and **no** JSON error envelope. Sizes 26,573–26,614 B differ only by build hashes,
so size alone is not a reliable discriminator (`M1-index.md:137-143`).

| path | observed | body size | anchor | consequence |
|---|---|---|---|---|
| `/api/oauth/phanthy_cli/create_api_key` | GET 404, OPTIONS 404 | 26,614 B | `M1-anon-probe-matrix.txt:67`; `M1-options-matrix.txt:20` | The API-key minting route used by prior art does not exist in production. |
| `/api/oauth/claude_cli/create_api_key` | GET 404, OPTIONS 404 | 26,613 B | `M1-anon-probe-matrix.txt:68`; `M1-options-matrix.txt:21` | The renamed sibling is also absent. |
| `/v1/messages/count_tokens` | GET 404, OPTIONS 404 | 26,598 B | `M1-anon-probe-matrix.txt:72`; `M1-options-matrix.txt:24` | Token counting is not a separate endpoint. |
| `/v1/models` | GET 404, OPTIONS 404 | 26,579 B | `M1-anon-probe-matrix.txt:73`; `M1-options-matrix.txt:25` | No model catalogue over HTTP. |
| `/api/health` | GET 404, OPTIONS 404 | 26,580 B | `M1-anon-probe-matrix.txt:78`; `M1-options-matrix.txt:29` | No health endpoint under `/api`. |
| `/healthz` | GET 404, OPTIONS 404 | 26,573 B | `M1-anon-probe-matrix.txt:79`; `M1-options-matrix.txt:30` | No health endpoint at root. |

**Consequence for tooling:** an HTTP client cannot distinguish "API route absent" from "SPA catch-all page".
There is no JSON 404 taxonomy to document, and OpenAPI must not model a JSON 404 body
(`M1-index.md:137-143`).

### 4.2 Named by `robots.txt` but NOT probed

These appear only as `Disallow` rules (`M1-anon-probe-matrix.txt:384-398`): `/admin`, `/account`, `/logout`,
`/login`, `/register`, `/payment`, `/app`, `/feedback`, `/reset-password`, `/forgot-password`, `/verify-email`,
`/docs`; and as `Allow` rules: `/download`, `/token-plan`, `/skills`, `/changelog`, `/work`, `/prompts`,
`/llms.txt`. Their existence is **UNVERIFIED**; nothing in this catalog should imply they exist.

Additionally, `/v1/wechat-contact-qr.png` appears in the public bundles (`M1-js-static-analysis.txt:65-69`) but is
a static image path, not an API, and is excluded here (caveat D7 in §1.6).

---

## 5. Auth model

### 5.1 Client identity, PKCE and redirect

| element | value | grade | anchor |
|---|---|---|---|
| `client_id` | `phanthy-code-cli` | PRIOR-ART | `cmd/login/main.go:28, 146`; `internal/upstream/client.go:138` |
| client type | public — `token_endpoint_auth_methods_supported: ["none"]` (no secret) | VERIFIED | `M1-openid-configuration.json:17-19` |
| PKCE | `S256` only; verifier = 48 random bytes, base64url without padding; challenge = base64url(SHA-256(verifier)) | VERIFIED (S256) / PRIOR-ART (construction) | `M1-openid-configuration.json:14-16`; `cmd/login/main.go:51-56` |
| `redirect_uri` | `https://code.phanthy.com/oauth/code/success` (must be replayed verbatim at token exchange) | PRIOR-ART | `cmd/login/main.go:148, 182` |
| `state` | static string `p2a-login` | PRIOR-ART | `cmd/login/main.go:152` |
| response types | `code` only | VERIFIED | `M1-openid-configuration.json:7-9`; error text `M1-anon-probe-matrix.txt:112` |
| grant types | `authorization_code`, `refresh_token` | VERIFIED | `M1-openid-configuration.json:10-13` |
| issuer | `https://code.phanthy.com` | VERIFIED | `M1-openid-configuration.json:2` |

### 5.2 Scopes

| scope | status | anchor |
|---|---|---|
| `user:profile` | advertised; requested by prior art | `M1-openid-configuration.json:21`; `cmd/login/main.go:149` |
| `user:inference` | advertised; requested by prior art | `M1-openid-configuration.json:22`; `cmd/login/main.go:149` |
| `user:sessions:claude_code` | advertised; requested by prior art | `M1-openid-configuration.json:23`; `cmd/login/main.go:149` |
| `user:mcp_servers` | advertised; **never requested** by prior art — purpose unknown | `M1-openid-configuration.json:24` |
| `user:file_upload` | advertised; **never requested** by prior art — purpose unknown | `M1-openid-configuration.json:25` |

No `openid` scope exists, consistent with the absence of `userinfo_endpoint` and of ID-token metadata
(`M1-index.md:122-128`).

### 5.3 Token and refresh lifecycle

| element | behaviour | grade | anchor |
|---|---|---|---|
| access token | opaque to us; returned as `access_token`; expected lifetime comes from `expires_in` | PRIOR-ART | `cmd/login/main.go:171-175, 108` |
| refresh grant | `POST /oauth/token` with `grant_type=refresh_token`, `refresh_token`, `client_id`, plus `anthropic-beta: oauth-2025-04-20`; response may rotate `refresh_token` | PRIOR-ART | `internal/upstream/client.go:232-259` |
| proactive refresh window | 10 min before expiry (handler), 30 min (nightly keepalive) | PRIOR-ART | `internal/server/handler.go:54-56, 141`; `internal/scheduler/scheduler.go:79` |
| keepalive schedule | daily at 22:00 local, plus once at startup | PRIOR-ART | `internal/scheduler/scheduler.go:28-31, 50-66`; `config.example.json:13-15` |
| session death | `401` or markers `OAuth token has been revoked`, `missing_login`, `invalid_grant`, `session expired`, `unauthorized`, `invalid access token` → account permanently disabled until manual re-login | PRIOR-ART | `internal/upstream/client.go:69-73, 86-93`; `internal/server/handler.go:182-185`; `internal/pool/pool.go:180-189` |
| JWT claim set | unknown; RS256 key exists (`kid=configured-key`) | INFERRED (key presence) / `BLOCKED-CRED` (claims) | `M1-jwks.json:22, 29-32` |
| revocation | `POST /oauth/revoke` advertised; never called by prior art | VERIFIED (endpoint) / INFERRED (semantics) | `M1-openid-configuration.json:5`; `M1-options-matrix.txt:17` |

### 5.4 The two missing API-key routes and the documented fallback

Both `/api/oauth/phanthy_cli/create_api_key` and `/api/oauth/claude_cli/create_api_key` return the HTML 404 shell
(§4.1). The prior art handles this explicitly:

- the login tool calls the `phanthy_cli` variant with `Authorization: Bearer <access_token>` and treats a `404` as
  "not available" rather than an error (`cmd/login/main.go:221-251`, comment at `:97`);
- when no API key exists, `POST /v1/messages` is called with `Authorization: Bearer <access_token>` **in place of**
  `x-api-key` (`internal/upstream/client.go:344-350`; `cmd/login/main.go:103-104`);
- if a key ever is returned, the prior art expects it at `data.raw_key` (or `data.api_key` / `api_key` fallbacks)
  (`cmd/login/main.go:256-269`; `internal/upstream/client.go:292-307`).

Statement of intent, not a live fact: on this target the **access token is used as the `/v1/messages`
credential**. That is `PRIOR-ART` only; the live server may reject Bearer on `/v1/*` (`BLOCKED-CRED`, §6).

---

## 6. Open risks and `BLOCKED-CRED` register

| # | risk / unknown | impact on M3/M4 | status | anchor |
|---|---|---|---|---|
| R1 | Auth scheme on `/v1/messages` (`x-api-key` vs `Authorization: Bearer`) | Decides the OpenAPI security scheme; currently two alternatives must be modelled | `BLOCKED-CRED` | `M1-index.md:198-202`; `internal/upstream/client.go:344-350` |
| R2 | Live request/response body of `/v1/messages` (non-streaming JSON shape, error envelope) | Response schemas in OpenAPI would be speculative | `BLOCKED-CRED` | `M1-index.md:203-206` |
| R3 | Whether upstream ever returns HTTP 200 with an `error` envelope | Affects error handling; prior art assumes yes | PRIOR-ART assumption | `internal/upstream/client.go:158-169` |
| R4 | `/api/web/events` semantics — content type, auth, body, streaming | Cannot be specified; four competing hypotheses in §3.11 | INFERRED-only | `M1-index.md:207-209` |
| R5 | `/api/oauth/claude_cli/roles` and `/api/oauth/profile` success schemas | Two "verified existence, unknown body" endpoints | `BLOCKED-CRED` | `M1-anon-probe-matrix.txt:66, 69` |
| R6 | Real API-key minting path (both known paths 404) | Credential docs must describe the fallback, not a minting flow | `BLOCKED-CRED` | `M1-index.md:218-221` |
| R7 | Model identifiers unavailable over HTTP (`/v1/models` 404) and absent from bundles | Model table can only be `PRIOR-ART / unverified live` | PRIOR-ART | `M1-index.md:213-214`; `M1-js-static-analysis.txt:97-123` |
| R8 | Full `platform` vocabulary of EP-12 (only `linux` proven, `darwin` rejected) | Download docs must not advertise macOS/Windows | VERIFIED (negatives) / INFERRED (positives) | `M1-anon-probe-matrix.txt:337-367` |
| R9 | `/api/public-assets` target never followed | Asset API cannot be documented | unprobed | `acceptance/M1.md:47` |
| R10 | Token TTL, JWT claims, key rotation (`kid=configured-key`) | Auth docs remain behavioural, not cryptographic | `BLOCKED-CRED` | `M1-jwks.json:22` |
| R11 | No CORS response headers for an arbitrary `Origin`; `vary` never gains `origin` | Browser-based integrations from other origins will fail; server-side only | VERIFIED (absence) / unknown allowlist | `M1-anon-probe-matrix.txt:291-335` |
| R12 | Rate-limit / quota failure shapes unknown (no `RateLimit-*` header ever observed; only three error codes seen) | Cooldown policy in §5 of `02-usage-semantics.md` is policy, not observation | INFERRED | `M1-index.md:210-212` |
| R13 | `OPTIONS /` returns 400 while `GET /` returns 200; 405s on API routes omit `Allow` | Method discovery must use OPTIONS on API paths, never 405 headers | VERIFIED | `M1-options-matrix.txt:7-10, 14`; `M1-anon-probe-matrix.txt:447-448` |
| R14 | 404 is an HTML shell, not JSON | Clients must not parse 404 bodies; OpenAPI must omit a JSON 404 schema | VERIFIED | `M1-index.md:137-143` |

**`BLOCKED-CRED` summary (requires the user's own token, no fabrication):** R1, R2, R5, R6, R10, plus every
success response of EP-03, EP-04, EP-08, EP-09, EP-10, EP-11. The verification plan that closes them is in
`02-usage-semantics.md` §7.
