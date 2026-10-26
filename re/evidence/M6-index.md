# M6 Evidence Index — official CLI package + new model endpoints (read-only)

Status: M6 evidence package complete (**6 of 6 deliverables** — five from the main capture plus
`M6-cli-install.txt`, added during the D2 defect fix). Method chain:
**npm registry metadata → tarball static extraction → public pricing page → anonymous live probes →
published installer/CLI-page capture.**
No package file was imported, installed, transpiled or executed; no login or token was used; no
repository was cloned. Every claim below is reproducible from the command recorded for it.

---

## 1. Capture metadata

| item | value |
|---|---|
| work window | main capture operator session `2026-10-26T00:18:27Z` → `00:27:24Z` (command-session bounds, not artifact-derived: the artifacts' own timestamps run `00:19:00Z` → `00:22:00Z`, and the last artifact write is `00:27:09Z`); D2 install capture `2026-10-26T01:07:11Z` |
| local timezone | `Asia/Shanghai` (+08:00), i.e. `08:18` → `08:27`, then `09:07` |
| target | `https://code.phanthy.com/` and npm package `@phanthy/phanthy-codeo` |
| npm latest | `0.2.11`, published `2026-10-21T08:56:55.362Z` |
| tarball sha256 | `3134d33d4eba31ec0af543a8afa061d3b21c4df7259d49decc11928e2244d80c` (recomputed, matches) |
| egress | `HTTP_PROXY=HTTPS_PROXY=http://127.0.0.1:18080` (sandbox egress), HTTP/2 to origin |
| identity | anonymous; no `Authorization`, no `Cookie`, no `x-api-key`, no token |
| request budget | **12 anonymous requests across two captures**: main capture 9 (1 registry metadata, 1 tarball, 1 public page, 6 endpoint probes) + D2 install capture 3 (`/cli`, `install.sh`, `install.ps1`); main-capture spacing `>=1.1 s` per `M6-new-endpoint-probes.txt`, install-capture spacing not individually timestamped; no single endpoint exceeded 20 hits. The main capture's self-imposed 10-request cap was exceeded by 2 when the install capture was added; recorded here rather than hidden |
| methods | `GET`, `HEAD`, `OPTIONS` only — no `POST`, no writes, no body submitted |
| tools | `curl`, `python3`, `sha256sum`, `tar` (extraction only) |
| execution | `executed: false`, `imported: false`, `installed: false`, `binaries_run: false` |
| untrusted repo | `github.com/witt3rd/oh-my-hermes` — **not** fetched, cloned or executed. The package's declared repo is `github.com/yizhixu/phanthy-codeo` |

---

## 2. File inventory

| file | sha256 | bytes | what it proves | reproduction |
|---|---|---|---|---|
| `M6-npm-package.json` | `2bd1c9a543cd78eda2d404c23a4246b40f4555b19fc9bd1ac7d750c32a74a3a2` | 4612 | package identity, publish date, integrity, 969 files / 76,873,667 B unpacked, static-only attestation | `curl -sS 'https://registry.npmjs.org/@phanthy%2Fphanthy-codeo'` + `sha256sum` on the tarball |
| `M6-cli-config.txt` | `c76ecf4b26ff661d159db3667a8d91e4247f3c058d002d76b7f1958f356b6b87` | 18412 | the Phanthy provider block in `dist/tui-worker.js` with byte offsets and a file sha256; credential path pattern; other `code.phanthy.com` strings | `python3 -c "b=open('/tmp/m6/pkg/package/dist/tui-worker.js','rb').read();print(b[7928736:7929506].decode())"` |
| `M6-new-endpoint-probes.txt` | `73caed0bc5b401a8efc4de164ef30f95d9e57befb0b3493558d3bffd8b57be56` | 10691 | both new endpoints exist, are anonymous-401 guarded, `allow: GET, HEAD, OPTIONS`, with full raw dumps | `curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/api/phanthy_code/models` |
| `M6-public-model-catalog.txt` | `aca853674f6adc589047a998235be14119fb46850eedc02b38e12bf21eda668d` | 9043 | the 13-row public model table (display names + ratios) and 4 plan tiers | `curl -sS --http2 -o /tmp/m6/token-plan.html https://code.phanthy.com/token-plan` |
| `M6-cli-install.txt` | `265f644d01b6928396d65e06e43dabfaa117e788c0ca663def9964f81b0ad382` | 1874 | `/cli` install+login commands, `install.sh` / `install.ps1` HTTP status, sizes and sha256, verbatim installer headers (scripts read as text, never executed) | `curl -sS -o /tmp/m6/cli.html https://code.phanthy.com/cli` + `curl -sS -o /tmp/m6/install.sh https://code.phanthy.com/install.sh` |
| `M6-index.md` (this file) | not self-hashed (circular) | — | consolidated findings, grades, delta vs M1 | `n/a` |

Extraction commands (all under `/tmp`, nothing written outside `/workspace/re/evidence/`):

```bash
curl -sS -o /tmp/m6/phanthy-codeo-0.2.11.tgz \
  https://registry.npmjs.org/@phanthy/phanthy-codeo/-/phanthy-codeo-0.2.11.tgz
sha256sum /tmp/m6/phanthy-codeo-0.2.11.tgz      # 3134d33d...4d80c
mkdir -p /tmp/m6/pkg && tar -xzf /tmp/m6/phanthy-codeo-0.2.11.tgz -C /tmp/m6/pkg
find /tmp/m6/pkg -type f | wc -l                # 969
du -sb /tmp/m6/pkg                              # 76873667
sha256sum /tmp/m6/pkg/package/dist/tui-worker.js
```

During the main capture no M0/M1 evidence file was modified and no documentation under `/workspace/re/docs`
was touched. The docs were updated later in M6b and in the subsequent bookkeeping passes
(`docs/04-todos.md` T6.5/T6.7).

---

## 3. New findings

**3.1 Two new endpoints exist live and are bearer-guarded (this is the M6 headline).**
`GET /api/phanthy_code/models` and `GET /api/phanthy_code/models/stats` both answer
`OPTIONS → 204 allow: GET, HEAD, OPTIONS`, `GET/HEAD → 401 application/json` with the M1 envelope
`{"error":{"code":"missing_login","message":"Bearer access token is required."}}` (79 B, byte-identical
on both paths) and `www-authenticate: Bearer realm="phanthy-code", error="invalid_request",
error_description="Bearer access token is required."`. No `content-length` is sent (chunked), no
`access-control-*`, no rate-limit headers, `eo-cache-status: MISS`.
→ `M6-new-endpoint-probes.txt`

**3.2 The official CLI hard-codes both paths, plus four older ones.**
`package/dist/tui-worker.js` (sha256 `c38ca895…b0c6d`) at byte offset **7928736** holds
`https://code.phanthy.com` (`Kh`), `phanthy-code-cli` (`lq6`), provider name `phanthy` (`a2A`), the
7-entry header map (`iw`), the 6-entry scope array (`qKj`) and the 6-entry endpoint map (`nFA`).
The identical block is duplicated in `dist/cli.js` under different minified symbols. The two new
paths therefore are not a guess: they are the exact strings the official CLI calls.
→ `M6-cli-config.txt` §1, §2

**3.3 New header family (7 namespaced headers + one telemetry header).**
Exact casing, as extracted:
`x-phanthy-client`, `X-Phanthy-Client-Source`, `x-phanthy-cli-device`, `X-Phanthy-Product`,
`x-phanthy-project`, `x-phanthy-request`, `x-phanthy-session`. Two of the seven deliberately capitalise
`X-Phanthy`; five use lowercase `x-phanthy-`. Header **values** observed in the bundle are
`phanthy_code`, `phanthy_codeo` and `cli`. A separate telemetry flush adds `x-service-name: phanthy-codeo`.
`x-phanthy-cli-device` carries `base64url(JSON.stringify(...))` of hostname, username, platform, arch,
OS release, up-to-32 IPv4 candidates, interfaces and default routes — a host fingerprint header. Recorded
as a privacy observation, not a vulnerability claim.
→ `M6-cli-config.txt` §2(iv), §5.2

**3.4 Scope delta: +1 client-requested scope beyond the server's advertised set.**
The CLI requests 6 scopes (`org:create_api_key`, `user:profile`, `user:inference`,
`user:sessions:claude_code`, `user:mcp_servers`, `user:file_upload`). The server's OIDC document (M1)
advertises only 5 and omits `org:create_api_key`. This is notable because M1 found both
`/api/oauth/phanthy_cli/create_api_key` and `/api/oauth/claude_cli/create_api_key` returning 404, yet
`roles` survived with a 401 guard — i.e. the CLI asks for a capability the discovery document does not
advertise. Whether the AS accepts `org:create_api_key` is **not** established (the probe budget and the
no-login rule forbid testing the authorize endpoint).
→ `M6-cli-config.txt` §2(v), §6; cross-ref `M1-openid-configuration.json`

**3.5 Credential storage is Claude-Code-shaped.**
`~/.phanthy-code/.credentials.json` (override `PHANTHY_CONFIG_DIR`), OAuth payload under the key
`claudeAiOauth`, secondary config `.config.json` / `.phanthy-code.json`, macOS keychain via the
`security` CLI with a dir-salted service name. No credential value exists in the package and none was read.
→ `M6-cli-config.txt` §3

**3.6 Public model catalogue: 13 aliases, 4 of them placeholder rows.**
`https://code.phanthy.com/token-plan` (sha256 `2abf943b…be8a`, verified) publishes 13 display names with
credit ratios: `Phanthy-fast` 0.00x, `Phanthy-pro` 0.33x, `Phanthy-ultra` 0.67x, `glm-5.3-flash` 0.04x,
`glm-5.3` 0.38x, `glm-5.2` 0.38x, `kimi-k3` 1.00x, `kimi-k2.7-code` 0.31x,
`deepseek-v4.1-flash-1017` 0.10x, `Phana-5.5o` 0.67x, `Phana-5.5s` 0.33x, `Phano-6a` 1.67x,
`Phano-6.1s` 0.33x — all suffixed `x积分`. Four rows (`Phana-*`, `Phano-*`) print the untranslated
placeholder scenario **"Available model"** instead of a description; their logos point at
`anthropic.svg` and `openai.svg` respectively, which is presentation only, not a stated mapping.
No model IDs, context windows, per-token prices or endpoint mapping are published.
→ `M6-public-model-catalog.txt`

**3.7 Additional `code.phanthy.com` strings not in the M1 inventory.**
(a) `POST ${Kh}/api/event_logging/batch` — telemetry batch flush with `Bearer` token
(`PhanthyTelemetry.flush`), static-only, **not probed**;
(b) the web-search base URL is `https://code.phanthy.com` with a fail-closed token resolver;
(c) `https://code.phanthy.com/console` and a build-flag `https://dev.code.phanthy.com` in `cli.js`;
(d) `HTTP-Referer: https://code.phanthy.com/` / `X-Title` metadata injected into third-party provider
autoloads (llmgateway, openrouter, nvidia, vercel, zenmux, gitlab, kilo, snowflake-cortex) — branding on
other vendors' APIs, not a Phanthy endpoint.
→ `M6-cli-config.txt` §4

**3.8 Static inconsistency around the message path (flagged, not resolved).**
The endpoint map contains `messages: https://code.phanthy.com/v1/messages`, but `nFA.messages` is
**never dereferenced** (0 occurrences). The live path is the Anthropic-SDK-compatible provider
`{apiKey:"phanthy-oauth", baseURL: `${Kh}/v1`, fetch: …}` whose custom fetch deletes `x-api-key`, sets
`Authorization: Bearer`, `x-phanthy-product: phanthy_code`, `X-Phanthy-Client-Source: cli` and a
regenerated `x-phanthy-request`. Because the bundled SDK posts to the absolute path `/v1/messages`,
the effective URL may be `/v1/messages` or `/v1/v1/messages`. M1 proved `/v1/messages` exists; M6 did not
POST, so this cannot be settled by observation here. Recorded as **UNVERIFIED**.
→ `M6-cli-config.txt` §4.3

---

## 4. Evidence grade table

| grade | meaning | items in M6 |
|---|---|---|
| **VERIFIED-LIVE** | directly observed in this milestone's anonymous captures | `/api/phanthy_code/models` and `/api/phanthy_code/models/stats` exist (`OPTIONS 204`, `allow: GET, HEAD, OPTIONS`); both are bearer-guarded (`GET` and `HEAD` → `401`, same 79 B body, `WWW-Authenticate: Bearer realm="phanthy-code"`) |
| **VERIFIED-STATIC** | exact byte strings inside the official CLI bundle, with file sha256 + offset | base URL, client id `phanthy-code-cli`, provider name `phanthy`, 7 header names, 6 scopes, 6-endpoint map; credential path `~/.phanthy-code/.credentials.json`, `PHANTHY_CONFIG_DIR`, key `claudeAiOauth`; `POST /api/event_logging/batch`; web-search base URL; `dev.code.phanthy.com`; `/console` |
| **PUBLIC-MARKETING** | rendered on the public site, no auth, not an API contract | 13 model aliases + credit ratios, 4 plan tiers, "1,000 积分 ≈ USD 1" note, announcement banner (page sha256 `2abf943b…be8a`) |
| **UNVERIFIED / INFERRED** | never observed; explicitly *not* promoted | success schemas of both model endpoints; meaning/shape of "model stats"; ratio↔API linkage; acceptance of `org:create_api_key`; whether the message path is `/v1/messages` or `/v1/v1/messages`; `/api/event_logging/batch` live existence; full model ID ↔ display alias mapping |

Rule: 401 is the only application-level response body M6 observes. **No success-response schema is
claimed anywhere in this milestone.**

---

## 5. Delta vs M1

| item | M1 | M6 | delta |
|---|---|---|---|
| endpoints | 19 paths probed, 13 existed | + `/api/phanthy_code/models`, + `/api/phanthy_code/models/stats` | **+2 endpoints** (brief's running count: **17 total**) |
| scopes | 5 advertised server-side (`user:profile`, `user:inference`, `user:sessions:claude_code`, `user:mcp_servers`, `user:file_upload`) | 6 requested client-side | **+1 (`org:create_api_key`)**, not server-advertised |
| headers | only `Authorization: Bearer` on `/api/oauth/*` witnessed | 7 `x-phanthy-*` names + `x-service-name` + `x-phanthy-cli-device` fingerprint | **new header family** |
| model IDs | absent from public JS; `/v1/models` 404 | display aliases published on `/token-plan` (no wire IDs) | +13 aliases, still 0 model IDs |
| auth scheme on `/v1/messages` | unknown (`x-api-key` nowhere) | `x-api-key` is explicitly **deleted**; `Authorization: Bearer` set | scheme clarified statically, not observed live |
| client provenance | none | official npm CLI pinned at 0.2.11 with recomputed hashes | new independently verifiable source |

**Numbering caveat (honest bookkeeping).** The brief states "+2 endpoints (17 total)". Re-deriving from
the M1 artifacts: the M1 anonymous matrix **probed 19 paths of which 13 existed**, and two additional
`.well-known` endpoints (`openid-configuration`, `jwks.json`) were captured as separate M1 evidence files
rather than rows in that matrix. 13 + 2 = **15**, which is the pre-M6 active inventory. M6 adds exactly
two newly confirmed API paths (`/api/phanthy_code/models`, `/api/phanthy_code/models/stats`), giving
**17**. Both totals are consistent once the `.well-known` pair is counted; neither is derivable from the
M1 probe matrix alone, so the provenance above must be quoted with the number.

Additional static-only path found in M6 (`/api/event_logging/batch`) is **not** counted in either total
because it was not probed.

---

## 6. Governance / compliance statement

- Static inspection only: `tar -xzf` + Python byte slicing / string search. No `node`, no `npm install`,
  no `bun`, no binary from the package was run; nothing was imported.
- Anonymous only: no login, no token, no authenticated call, no guessed credential; no Authorization
  header was ever sent.
- 12 anonymous requests across two captures (9 main + 3 install), `>=1.1 s` apart in the main capture
  (install-capture spacing not individually timestamped), `GET`/`HEAD`/`OPTIONS` only. This is a
  capture-pass count only; the independent acceptance and fix-verification sub-agents issued their own
  requests, recorded in `acceptance/M6.md` and `acceptance/M6-fixes.md`. The install capture was budgeted
  separately and pushed the main capture's self-imposed cap of 10 to 12 in total.
- Untrusted `github.com/witt3rd/oh-my-hermes` was neither cloned nor executed.
- Downloads and extraction confined to `/tmp/m6`; only the six M6 evidence files were written to
  `/workspace/re/evidence/`. M0/M1 evidence was untouched during the capture; the active docs under
  `/workspace/re/docs` were updated separately in M6b and in the later bookkeeping passes.
- No credential value was found or logged; the artifact contains no secrets.

---

## 7. Open questions handed to M6b / M7

1. Success payload of `GET /api/phanthy_code/models` and `/models/stats` — shape, fields, whether ratios
   appear there (requires a user-supplied credential; M6 had none).
2. Is the message path `/v1/messages` or `/v1/v1/messages`? (Anthropic SDK baseURL vs absolute path.)
3. Does the authorization server accept `org:create_api_key`, and where does API-key minting actually
   happen (M1's two `create_api_key` routes both 404)?
4. Live existence and auth of `POST /api/event_logging/batch` (not probed here).
5. Model ID ↔ display alias mapping; the four "Available model" placeholder rows suggest an
   incomplete publication.
6. Expired/invalid-token error taxonomy (`missing_login` is the only code seen on this surface).
