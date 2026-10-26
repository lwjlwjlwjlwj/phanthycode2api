# Apifox Import Guide — PhanthyCode AI API (reverse-engineered)

Companion files in this directory:

| file | purpose |
|---|---|
| `openapi.json` | OpenAPI 3.1.0 bundle, 15 operations, 13 shared schemas, 7 tags/folders. |
| `push_apifox.py` | Standard-library-only validator + optional Apifox Open API push client. |
| `IMPORT.md` | This guide. |

Target server: `https://code.phanthy.com` (production). Everything authenticated in the spec is
labelled **PRIOR-ART / BLOCKED-CRED**; see the `info.description` of `openapi.json`, which is the
authoritative statement of evidence grades.

---

## 0. What was verified against official Apifox documentation

Fetched on **2026-10-26**:

- <https://docs.apifox.com> — documentation index reachable.
- <https://apifox.com/apidoc/docs-site/4478210/api-173409873> — “导入 OpenAPI/Swagger 格式数据” (Apifox Open API).

Verified facts (quoted from that page):

| item | verified value |
|---|---|
| endpoint | `POST https://api.apifox.com/v1/projects/{projectId}/import-openapi` |
| required header | `X-Apifox-Api-Version: 2024-03-28` — “开放 API 版本号，必填，否则会报错” |
| auth header | `Authorization: Bearer {{access_token}}` (system-level access token) |
| body `input` | required; supports **URL object** (`{url, basicAuth?}`) or **string** payload |
| body `options` | optional; `endpointOverwriteBehavior` / `schemaOverwriteBehavior` enums include `AUTO_MERGE`, `OVERWRITE_EXISTING`, `KEEP_EXISTING`, `CREATE_NEW`; `deleteUnmatchedResources` default `false` |
| success response | `200 application/json` with `data.counters` (endpoint/schema/folder created/updated/failed/ignored counts) |

`push_apifox.py` implements exactly this shape: `input` is the raw spec text as a JSON string
(documented “字符串方式”), `options` uses conservative `AUTO_MERGE`, and it sends the
required `X-Apifox-Api-Version` header. `--api-base` / `--import-path` / `--api-version`
override the documented defaults for private deployments or endpoint changes.

**UNVERIFIED (runtime):** the push HTTP call itself has **never been executed** — no
`APIFOX_ACCESS_TOKEN` was available in this environment (`re/evidence/M0-preflight.json:18`:
`apifox_credentials: ABSENT`). The *documented endpoint shape* above is verified; the *live push*
remains **UNVERIFIED** until you supply a token and run `--push`. Local validation and `--dry-run`
are fully verified and exit 0.

---

## 1. Import path A — Apifox UI (recommended first import)

1. Open the target project in Apifox.
2. Go to **项目 → 设置 → 导入** (Project → Settings → Import), then choose **OpenAPI/Swagger**.
   The dedicated docs page is <https://docs.apifox.com/import-openapi-swagger>.
3. Upload `openapi.json` (or paste its contents). Apifox auto-detects `openapi: 3.1.0`.
4. In the import options:
   - **Endpoint overwrite behaviour** → `自动合并 / AUTO_MERGE` for re-imports.
   - **Schema overwrite behaviour** → `自动合并 / AUTO_MERGE`.
   - Leave **prepend base path** off; the server URL is carried by the collection environment
     instead (see §3).
   - Do **not** enable “delete unmatched resources”.
5. Confirm. Apifox creates one folder per tag and 15 operations (see §4).

### Tag → folder mapping

| tag / folder | operations |
|---|---|
| `OAuth` | `oauthAuthorize`, `oauthToken`, `oauthRevoke`, `oauthCodeSuccessGet` |
| `OIDC` | `openidConfigurationGet`, `jwksGet` |
| `Account` | `profileGet`, `claudeCliRolesGet` |
| `Inference` | `messagesCreate` |
| `Web` | `appShellGet`, `webEventsCreate` |
| `Assets` | `clientDownloadGet`, `publicAssetsGet` |
| `Meta` | `robotsGet`, `sitemapGet` |

---

## 2. Import path B — automated push

### 2.1 Environment variables

| variable | required | default | meaning |
|---|---|---|---|
| `APIFOX_ACCESS_TOKEN` | **yes for push** | — | System-level Apifox access token. Read **only** from the environment; never accepted as a CLI argument, so it does not enter shell history or `ps`. |
| `APIFOX_PROJECT_ID` | **yes for push** | — | Numeric/string project ID from the Apifox project URL or project settings. Can also be passed as `--project-id`. |
| `APIFOX_API_BASE` | no | `https://api.apifox.com` | Override for private deployment or a proxy. Also `--api-base`. |
| `APIFOX_API_VERSION` | no | `2024-03-28` | *Not read from env by the script*; use `--api-version` if Apifox bumps the version header. |

Get a token at **账号设置 → API 访问令牌** (<https://docs.apifox.com/api-access-token>).

### 2.2 Commands

```bash
cd /workspace/re/apifox

# 1) Local validation only — this is the DEFAULT; no network request.
python3 push_apifox.py --spec openapi.json

# 2) Self-test of the bundled spec; exits 0 only if valid.
python3 push_apifox.py --self-test

# 3) Real push (explicit opt-in).
export APIFOX_ACCESS_TOKEN='<apifox_system_token>'   # never pass on the command line
export APIFOX_PROJECT_ID='<project_id>'
python3 push_apifox.py --spec openapi.json --project-id "$APIFOX_PROJECT_ID" --push

# Optional overrides
python3 push_apifox.py --spec openapi.json --push \
  --api-base https://api.apifox.com \
  --import-path '/v1/projects/{project_id}/import-openapi' \
  --api-version 2024-03-28 \
  --timeout 60
```

Exit codes:

| code | meaning |
|---|---|
| `0` | validation passed (dry run) / HTTP 2xx (push) |
| `1` | validation failed, or Apifox returned a non-2xx response (body printed) |
| `2` | misuse: missing `APIFOX_ACCESS_TOKEN` / `--project-id`, or `--push` combined with `--dry-run` |
| `3` | transport failure (DNS, TLS, timeout) |

The dry run prints: spec path, OpenAPI version, path/operation/schema/`$ref` counts, tags, every
`operationId`, and any warnings. Validation checks:

- `openapi` is a `3.1.x` string;
- `info.title` and `info.version` are non-empty;
- every operation has `operationId` and a non-empty `responses` map;
- all `operationId`s are unique (the same check Apifox uses to avoid collisions);
- every `$ref` is a **local** pointer that resolves;
- no `eyJ…`-shaped JWT and no literal `Bearer <credential>` appears anywhere in the spec.

---

## 3. Auth configuration inside Apifox

The bundle declares two security schemes:

| scheme | type | placement | use |
|---|---|---|---|
| `apiKeyAuth` | `apiKey` | header `x-api-key` | Primary prior-art credential for `POST /v1/messages`. |
| `bearerAuth` | `http` / `bearer` | `Authorization: Bearer <token>` | Verified guard on `/api/oauth/profile` and `/api/oauth/claude_cli/roles`; fallback credential for `/v1/messages`. |

`messagesCreate` declares **both as alternatives** (`security: [{apiKeyAuth: []}, {bearerAuth: []}]`).
`profileGet` and `claudeCliRolesGet` declare only `bearerAuth`.

### 3.1 Environment variables

Create an Apifox environment (e.g. “PhanthyCode production”):

| variable | value | used by |
|---|---|---|
| `baseUrl` | `https://code.phanthy.com` | collection server / requests |
| `apiKey` | `<api_key>` | `apiKeyAuth` |
| `token` | `<access_token>` | `bearerAuth` |

Keep `baseUrl` as a variable rather than editing the server URL per request; that is also why the
bundle keeps `prependBasePath` off.

### 3.2 Wiring the auth

1. **Collection/folder Auth**: choose **Bearer Token**, value `{{token}}`.
2. For the `Inference` folder, set **API Key** auth: key name `x-api-key`, value `{{apiKey}}`,
   “Add to” = **Header**. Because the two schemes are alternatives, use one per request — do not
   send both. If no API key can be minted (both `create_api_key` paths are verified absent), switch
   that request to Bearer.
3. Never commit real values into the spec. `openapi.json` contains only placeholders:
   `<api_key>`, `<access_token>`, `<token>`, `<authorization_code>`, `<pkce_code_verifier>`,
   `<refresh_token>`, `<state>`, `<user_id>`.

**Security note:** the Apifox cloud environment stores whatever you type into it. Use a dedicated
account whose token you can revoke, and do not reuse a production credential.

---

## 4. What will be created

- **7 folders/tags**: `OAuth`, `OIDC`, `Account`, `Inference`, `Web`, `Assets`, `Meta`.
- **15 operations**, exactly one per `operationId`:

| # | operationId | method | path |
|---|---|---|---|
| 1 | `appShellGet` | GET | `/` |
| 2 | `oauthAuthorize` | GET | `/oauth/authorize` |
| 3 | `oauthToken` | POST | `/oauth/token` |
| 4 | `oauthRevoke` | POST | `/oauth/revoke` |
| 5 | `oauthCodeSuccessGet` | GET | `/oauth/code/success` |
| 6 | `openidConfigurationGet` | GET | `/.well-known/openid-configuration` |
| 7 | `jwksGet` | GET | `/.well-known/jwks.json` |
| 8 | `profileGet` | GET | `/api/oauth/profile` |
| 9 | `claudeCliRolesGet` | GET | `/api/oauth/claude_cli/roles` |
| 10 | `messagesCreate` | POST | `/v1/messages` |
| 11 | `webEventsCreate` | POST | `/api/web/events` |
| 12 | `clientDownloadGet` | GET | `/api/download/client` |
| 13 | `publicAssetsGet` | GET | `/api/public-assets/` |
| 14 | `robotsGet` | GET | `/robots.txt` |
| 15 | `sitemapGet` | GET | `/sitemap.xml` |

- **13 schemas**: `ErrorDetail`, `ErrorEnvelope`, `TokenResponse`, `OpenIDConfiguration`, `Jwks`,
  `Jwk`, `AnthropicMessagesRequest`, `MessageParam`, `ContentBlock`, `Tool`, `ToolChoice`, `Usage`,
  `MessageResponse`.
- **No JSON 404 response object** anywhere: a 404 on this target is a Next.js HTML shell.

### 4.1 Intentionally excluded

| excluded | reason |
|---|---|
| `GET /v1/models`, `POST /v1/messages/count_tokens`, `GET /api/health`, `GET /healthz`, `POST /api/oauth/phanthy_cli/create_api_key`, `POST /api/oauth/claude_cli/create_api_key` | **verified absent** — 404 Next.js HTML shell, not a JSON error (`re/evidence/M1-anon-probe-matrix.txt:67-68, 72-73, 78-79`). Also listed in `info.x-verified-absent-paths`. |
| a `200` response or body for `publicAssetsGet` | only the `308` redirect from `/api/public-assets/` was observed; the target was never followed (`re/docs/01-endpoint-catalog.md:391-406`). |
| a request body / success schema for `webEventsCreate` | contract unverified; the operation carries a single `default` response marked “UNVERIFIED” and `x-contract-status: unverified`. |
| platform values other than `linux` in `clientDownloadGet` | `linux` accepted, `darwin` rejected; wider vocabulary unproven, so the enum contains only `linux`. |
| a model catalogue / enum of model IDs | `/v1/models` is absent and the bundles contain no model strings; roster is PRIOR-ART / unverified live (`re/docs/02-usage-semantics.md:335-379`). |
| 402 / 429 / 5xx error responses | never observed; those classes belong to the proxy classifier, not the target (`re/docs/02-usage-semantics.md:283-290`). |

---

## 5. Re-importing idempotently

- **Stable identity is the `operationId`.** All 15 are fixed and unique, so Apifox matches existing
  endpoints rather than creating duplicates.
- Use **`AUTO_MERGE`** for both endpoints and schemas (the default in `push_apifox.py`). It updates
  matching items and preserves local edits Apifox cannot derive from the file.
- `deleteUnmatchedResources: false` ensures endpoints you added manually in Apifox are never
  removed by a re-import.
- `prependBasePath: false` keeps `https://code.phanthy.com` in the environment, not in every path.
- Because folder IDs are not pinned, a first import may create folders by tag; subsequent
  `AUTO_MERGE` imports reuse them. If a UI-driven re-import ever duplicates a folder, merge the
  folders manually once and keep using the same import options.

**Merge vs overwrite**

| behaviour | effect | when to use |
|---|---|---|
| `AUTO_MERGE` (chosen) | update matched fields, keep unmatched local edits/endpoints | routine re-imports |
| `OVERWRITE_EXISTING` (Apifox default) | replace matched endpoints wholesale | one-time clean import |
| `KEEP_EXISTING` | import only new items | adding without touching existing |
| `CREATE_NEW` | always duplicate matched items | never for this bundle |

---

## 6. Troubleshooting

| symptom | likely cause | fix |
|---|---|---|
| `401 Unauthorized` | `APIFOX_ACCESS_TOKEN` missing, expired, or revoked | re-issue at 账号设置 → API 访问令牌; export it again |
| `403 Forbidden` | token is not a **system-level** token, or has no permission on that project | use a system token with project access; confirm the project ID |
| `404 Not Found` | wrong `--project-id`, or private-deployment base URL | verify the project ID; set `--api-base` (and possibly `--import-path`) |
| error mentioning API version | `X-Apifox-Api-Version` rejected | pass `--api-version` with the version Apifox currently requires |
| “duplicate operationId” / validation aborts before push | two operations share an ID | run `--self-test`; fix the spec; never let Apifox guess |
| endpoints duplicated after import | UI import used `CREATE_NEW` or a changed `operationId` | re-import with `AUTO_MERGE`; keep IDs frozen |
| bodies/URLs look wrong | `prependBasePath` enabled | disable it and set `baseUrl` in the environment |
| empty/odd response from push | non-2xx; body printed by the script | read the printed body; the script exits non-zero |

If the documented endpoint ever changes, override `--import-path` (with `{project_id}`) and
`--api-version`; no code change is needed.

---

## 7. Status summary

| item | status |
|---|---|
| `openapi.json` parses as JSON; 15 paths / 15 unique operations / 13 schemas / 21 local `$ref`s resolve | **VERIFIED locally** |
| `python3 push_apifox.py --self-test` exits 0 | **VERIFIED locally** |
| Apifox documented endpoint, required headers, `input`/`options` shape | **VERIFIED against official docs on 2026-10-26** |
| Live Apifox push | **UNVERIFIED** — no `APIFOX_ACCESS_TOKEN` was available; only exercised when you supply one |
