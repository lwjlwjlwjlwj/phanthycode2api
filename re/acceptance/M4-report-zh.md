# M4 Report Translation Verification (EN → ZH)

**Verdict: PASS** — the Simplified-Chinese report is content-equivalent to the English baseline. All factual cells, numbers, identifiers, anchors and epistemic qualifiers survive; the only deviations found are cosmetic translations of status labels (K1–K6) and one intentional metadata edit (K7).

- **Verifier role:** independent, read-only verifier sub-agent. Scope limited to structural/factual/linguistic parity between the baseline and the translated report.
- **Artifact under review:** `/workspace/re/report/reverse-engineering-report.md` (773 lines) vs baseline `HEAD:re/report/reverse-engineering-report.md` (808 lines).
- **Timestamp:** `2026-10-26T02:07:55Z` = `2026-10-26T10:07:55+08:00` (Asia/Shanghai).
- **Attestation:** no network access was used (no curl/wget/DNS/socket/clone/install/package execution); `https://github.com/witt3rd/oh-my-hermes` was never fetched. No existing file was modified. The only write performed is this file, `/workspace/re/acceptance/M4-report-zh.md`.

---

## 1. Structure parity

Baseline: 1 level-1 title + 13 level-2 sections (1–12 plus 5A) + 28 level-3 subsections. Current: identical counts, identical ordering, identical placement (5A still sits between §5 and §6). Nothing dropped, added, merged or renumbered.

In addition, all 9 shell comments inside the §11.2 fenced blocks remain verbatim English in both (baseline L662–681 region / current L629–648 region).

### 1.1 Level-2 sections

| # | baseline heading (line) | current heading (line) | status |
|---|---|---|---|
| — | title `# Reverse-Engineering Report` (L1) | `# 逆向工程报告` (L1) | present (title translated) |
| 1 | `## 1. Executive summary` (L19) | `## 1. 执行摘要` (L18) | present |
| 2 | `## 2. Scope, authorization and governance` (L61) | `## 2. 范围、授权与治理` (L58) | present |
| 3 | `## 3. Methodology` (L99) | `## 3. 方法论` (L91) | present |
| 4 | `## 4. Target fingerprint` (L150) | `## 4. 目标指纹` (L136) | present |
| 5 | `## 5. AI endpoint surface` (L170) | `## 5. AI 端点面` (L156) | present |
| 5A | `## 5A. Official CLI supply-chain static analysis (M6)` (L250) | `## 5A. 官方 CLI 供应链静态分析（M6）` (L233) | present, same position |
| 6 | `## 6. Authentication and OAuth/OIDC architecture` (L363) | `## 6. 鉴权与 OAuth/OIDC 架构` (L342) | present |
| 7 | `## 7. Inference protocol (...)` (L444) | `## 7. 推理协议 (...)` (L423) | present |
| 8 | `## 8. Model roster — two independent layers, not joined` (L509) | `## 8. 模型清单——两个独立层，不可合并` (L486) | present |
| 9 | `## 9. Security observations and risk rating` (L600) | `## 9. 安全观察与风险评级` (L572) | present |
| 10 | `## 10. Compliance and ethics statement` (L628) | `## 10. 合规与伦理声明` (L599) | present |
| 11 | `## 11. Reproducibility` (L650) | `## 11. 可复现性` (L617) | present |
| 12 | `## 12. Deliverables index and next steps` (L741) | `## 12. 交付物索引与后续步骤` (L709) | present |

### 1.2 Level-3 subsections (28, all present, same order)

| baseline (line) | current (line) | baseline (line) | current (line) |
|---|---|---|---|
| 5.1 Verified endpoint inventory (L172) | 5.1 已证实端点清单（L158） | 7.1 Request contract (L446) | 7.1 请求契约（L425） |
| 5.2 Verified absent (L202) | 5.2 已证实不存在（L186） | 7.2 Headers (L464) | 7.2 请求头（L443） |
| 5.3 OpenAPI modelling decisions (L212) | 5.3 OpenAPI 建模决策（L196） | 7.3 Streaming (L479) | 7.3 流式（L457） |
| 5A.1 Method / non-execution (L252) | 5A.1 方法与"未执行"声明（L235） | 7.4 Error taxonomy (L497) | 7.4 错误分类（L475） |
| 5A.2 Provider configuration (L274) | 5A.2 Provider 配置（L256） | 8.1 Layer A (L530) | 8.1 A 层（L504） |
| 5A.3 Header family (L297) | 5A.3 请求头族（L278） | 8.2 Layer B (L564) | 8.2 B 层（L537） |
| 5A.4 Scope delta (L313) | 5A.4 Scope 差集（L293） | 8.3 What can and cannot be claimed (L588) | 8.3 可与不可主张的事项（L560） |
| 5A.5 Credential storage layout (L324) | 5A.5 凭据存储布局（L303） | 11.1 Environment and egress (L652) | 11.1 环境与出网（L619） |
| 5A.6 Bearer vs `x-api-key` (L338) | 5A.6 Bearer 与 `x-api-key`（L317） | 11.2 Recon commands (L659) | 11.2 侦察命令（L626） |
| 5A.7 Static-only, unprobed surface (L353) | 5A.7 仅静态、未探测的面（L332） | 11.3 OpenAPI validation commands (L708) | 11.3 OpenAPI 校验命令（L675） |
| 6.1 Flow (PKCE, public client) (L365) | 6.1 流程（PKCE，公开客户端）（L344） | 11.4 Evidence pointers (L720) | 11.4 证据索引（L688） |
| 6.2 Scopes (L395) | 6.2 Scope（L375） | 12.1 Deliverables (L743) | 12.1 交付物（L711） |
| 6.3 Profile guard, JWKS, token format (L413) | 6.3 Profile 守卫、JWKS 与 token 格式（L393） | 12.2 Still UNVERIFIED (L756) | 12.2 仍未验证项（L725） |
| 6.4 Missing API-key minting routes (L429) | 6.4 缺失的 API-key 铸造路由（L409） | 12.3 Next steps (L778) | 12.3 后续步骤（L747） |

Numbered-list structures also match: §3 has 8 items (both), §6.1 has 7 steps (both), §12.3 has 8 items (both).

---

## 2. Table parity (15 tables, row-by-row)

Method: every contiguous `|`-block was extracted from both files; row counts compared, then cell-by-cell comparison of (a) all backticked code spans and (b) all numeric tokens. Pure prose differences were ignored; identifier/number differences would have been reported. All tables have the same header rows and separator rows.

| table | baseline rows (header+data) | current rows | identical facts? | issues |
|---|---|---|---|---|
| §1 deliverables (B L52-57 / C L49-54) | 5 (4 data) | 5 (4 data) | yes | none |
| §4 fingerprint (B L152-161 / C L138-147) | 9 (8 data) | 9 (8 data) | yes | none |
| §5.1 endpoint inventory (B L174-192 / C L160-178) | 18 (17 data) | 18 (17 data) | yes | none — rows 1–17, methods (`GET`/`POST`), paths, grade mixes (`VERIFIED`/`PRIOR-ART`/`BLOCKED-CRED`/`VERIFIED-LIVE`/`INFERRED`/`VERIFIED-STATIC`) and anchors identical |
| §5A.2 provider config (B L279-289 / C L261-271) | 10 (9 data) | 10 (9 data) | yes | none |
| §5A.3 header family (B L299-307 / C L280-288) | 8 (7 data) | 8 (7 data) | yes | none — exact casing (`X-Phanthy-Client-Source`, `X-Phanthy-Product` vs 5 lowercase) preserved |
| §5A.6 scheme table (B L344-347 / C L323-326) | 3 (2 data) | 3 (2 data) | yes | none |
| §6.2 scope table (B L400-407 / C L380-387) | 7 (6 data) | 7 (6 data) | yes | none — `是/否/**否**/**是**` pattern preserved, `org:create_api_key` still row 6 |
| §7.1 request fields (B L451-462 / C L430-441) | 11 (10 data) | 11 (10 data) | yes | none — `1..65536`, `8192` preserved |
| §8.1 Layer A 13 models (B L537-551 / C L510-524) | 14 (13 data) | 14 (13 data) | yes | none — all 13 aliases, `N.NNx积分` strings and scenarios byte-identical |
| §8.2 Layer B 13 aliases (B L566-580 / C L539-553) | 14 (13 data) | 14 (13 data) | yes | none — `Iris-1.0`…`Metis-1.0`, 13 client names and `PRIOR-ART` retained ("unverified live" → "线上未验证") |
| §8.3 claims (B L590-596 / C L562-568) | 6 (5 data) | 6 (5 data) | yes | one label translated: `` `not published` `` → `` `未公布` `` (K1) |
| §9 S1–S14 risk (B L605-620 / C L577-592) | 15 (14 data) | 15 (14 data) | yes | ratings map 1:1: Medium→中 (S1,S9,S12,S14), Low→低, Low (positive)→低（正向） (S5,S10), Low–Medium→低–中 (S13) |
| §11.4 evidence pointers (B L722-737 / C L690-705) | 15 (14 data) | 15 (14 data) | yes | none — anchor sets identical |
| §12.1 deliverables (B L745-750 / C L713-718) | 5 (4 data) | 5 (4 data) | yes | none — `17 operations, 7 tags, 13 schemas, 7 parameters, 44 $refs` preserved |
| §12.2 still unverified (B L758-776 / C L727-745) | 18 (17 data) | 18 (17 data) | yes | statuses preserved; see §6 below for the 7 `BLOCKED-CRED` rows |

`§5.3` is a bullet list, not a table: 12 bullets in baseline (L214-246) and 12 in current (L198-229), in the same order, each with the same code spans (`3.1.0`, `security: [{apiKeyAuth: []}, {bearerAuth: []}]`, `ErrorEnvelope`, `x-contract-status: "unverified"`, …).

No row present in baseline is missing now; no row was added.

---

## 3. Numeric and identifier parity

### 3.1 Required tokens

Raw occurrence counts over the whole file (no stripping):

| token | baseline | current | meaning changed? |
|---|---|---|---|
| `17` | 26 | 27 | no — see 3.2 (a) |
| `13` | 29 | 29 | no |
| `44` | 8 | 8 | no |
| `7` | 39 | 39 | no |
| `12` | 12 | 13 | no — see 3.2 (b) |
| `9` | 18 | 18 | no |
| `6` | 120 | 121 | no — see 3.2 (c) |
| `5` | 62 | 62 | no |
| `969` | 2 | 2 | no |
| `76,873,667` | 2 | 2 | no |
| `56,923` | 1 | 1 | no |
| `49,225` | 2 | 2 | no |
| `26.5` | 2 | 2 | no |
| `79` | 4 | 4 | no |
| `841` | 2 | 2 | no |
| `41.2%` | 2 | 2 | no |
| `200,000` | 1 | 1 | no |
| `65536` | 1 | 1 | no |
| `8192` | 1 | 1 | no |
| `2,000` | 2 | 2 | no |
| `15,000` | 1 | 1 | no |
| `30,000` | 1 | 1 | no |
| `150,000` | 1 | 1 | no |

Document-wide numeric multiset (all digit tokens outside code spans and anchors): baseline 281, current 285. The complete difference set is: baseline-only `4`×1; current-only `12`×1, `17`×1, `404`×1, `405`×1, `6`×1. All five current-only items and the one baseline-only item are explained in 3.2 — no numeric value was altered, added as a new fact, or dropped as a fact.

### 3.2 Delta explanations (word↔numeral transliteration only)

| # | baseline | current | effect |
|---|---|---|---|
| (a) | "Seventeen endpoints are verified to exist" (B L27) | "已证实存在 17 个端点" (C L26) | spelled-out → digit; value 17 unchanged |
| (b) | "Twelve public Next.js chunks were downloaded" (B L114) | "下载 12 个公开 Next.js chunk" (C L104) | spelled-out → digit; value 12 unchanged |
| (c) | "six probed paths are verified absent" (B L27) | "6 条探测路径已证实不存在" (C L26) | spelled-out → digit; value 6 unchanged |
| (d) | "Because API-route 405s omit `Allow`" (B L107) | "由于 API 路由的 405 不带 `Allow`" (C L51) | plural "405s" → "405"; status code unchanged |
| (e) | "`/v1/models` 404s" (B L773) | "`/v1/models` 404" (C L337) | verb "404s" → "404"; status unchanged |
| (f) | "M4 created exactly the four permitted files" (B L87) | "M4 仅创建允许的四个文件" (C L81) | digit → Chinese numeral 四; value 4 unchanged |

### 3.3 Full sha256 values (5 occurrences, 3 distinct)

Baseline and current lists are element-wise identical:

1. `3134d33d4eba31ec0af543a8afa061d3b21c4df7259d49decc11928e2244d80c` (twice: B L125 / C L113; B L262 / C L245)
2. `c38ca895230b827838cfebb19b2b9becdf8523379118868a0f9461956f9b0c6d` (B L264 / C L247)
3. `61f503311058d5ec9e571db3f0a74bc3c883ea3a781b916b49619d16b8b27c52` (B L265 / C L248)
4. plan-page digest `2abf943b6cab2090ed737618f76b03473319dc9e1704a90840f8c061cb40be8a` (B L702 / C L669)

Short forms also identical, including the two pre-existing, mutually inconsistent abbreviations inherited from the baseline: `2abf943b…be8a` (B L130 / C L119) and `2abf943b…0be8a` (B L532 / C L507); plus `e9df3e77…bda64134`, `1c1f5014…68b812`, `3134d33d…2244d80c` (C L672 — matched after a line wrap, still present).

### 3.4 operationIds

All 17 names appear with identical counts: `appShellGet` 1/1, `oauthAuthorize` 1/1, `oauthToken` 1/1, `oauthRevoke` 1/1, `oauthCodeSuccessGet` 1/1, `openidConfigurationGet` 1/1, `jwksGet` 1/1, `profileGet` 3/3, `claudeCliRolesGet` 3/3, `messagesCreate` 4/4, `webEventsCreate` 2/2, `clientDownloadGet` 2/2, `publicAssetsGet` 2/2, `robotsGet` 1/1, `sitemapGet` 1/1, `phanthyModelsGet` 4/4, `modelStatsGet` 4/4. The §12.1 enumeration (B L752-754 / C L720-723) is identical in order and content.

### 3.5 Grade-label occurrence counts (whole document)

`VERIFIED-STATIC` 11/11, `VERIFIED-LIVE` 3/3, `VERIFIED` 18/18, `PRIOR-ART` 29/29, `BLOCKED-CRED` 28/28, `PUBLIC-MARKETING` 6/6, `UNVERIFIED` 6/5, `INFERRED` 11/12. The two apparent deltas are account-book artifacts, not lost/added claims:
- `UNVERIFIED` −1: the heading `### 12.2 Still UNVERIFIED` (B L756) became `### 12.2 仍未验证项` (C L725). The qualifier survives as "未验证" (unverified); all five other `UNVERIFIED` tokens are present at the same claims.
- `INFERRED` +1: the §12.2 status `INFERRED-only` (B L768) became `仅 INFERRED` (C L737), which now matches a bare-token regex. Same meaning.

---

## 4. Anchor diff

Anchors of the forms `M0-…`, `M1-…`, `M6-…`, `docs/…`, `acceptance/…`, `cmd/…`, `internal/…`, `README.md:…`, compared as multisets (anchor text including its `:line-range` suffix):

| family | baseline | current |
|---|---|---|
| `M1-*` | 99 | 99 |
| `M6-*` | 51 | 51 |
| `M0-*` | 3 | 3 |
| `docs/*` | 33 | 33 |
| `acceptance/*` | 4 | 4 |
| Go/README (`cmd/…`, `internal/…`, `README.md:…`) | 35 | 35 |
| **total** | **225** | **225** |

- **Missing:** none.
- **Extra:** none.
- **Changed file names or line ranges:** none — because the multiset is identical, every `file:line` range (e.g. `[M6-cli-config.txt:145-157]`, `[M1-anon-probe-matrix.txt:447-448]`, `[internal/upstream/payload.go:75-97]`, `[acceptance/M3.md:179-192]`) is character-for-character the same. One spurious extra match in the baseline (`M6-derived`, B L137, prose) has no counterpart because it was rendered as `M6 派生` (C L124); it is not an evidence anchor.

---

## 5. Claim-parity spot check (high-risk passages)

| passage | baseline (line) | current (line) | finding |
|---|---|---|---|
| §5A.4 6-vs-5 scope delta | "The CLI requests six scopes … The server's discovery document advertises five … The delta is **`org:create_api_key`** … Two readings are possible and **neither is provable without a token**: (a) stale/shared client … or (b) honoured but unlisted … It does **not** imply a working API-key path" (L315-322) | "CLI 请求六个 scope … 服务端 discovery 文档只公布五个 … 差集为 **`org:create_api_key`** … 存在两种解读，且**在无 token 时都无法证实**：(a) 陈旧或共享的客户端库 … (b) 服务端实际接受但未列出 … 这**不**意味着存在可用的 API-key 路径" (L295-301) | equivalent — same delta, same two readings, same "unprovable without a token" hedge, same explicit denial of an API-key path |
| §5A.6 Bearer vs `x-api-key` correction | "**deletes `x-api-key`** … Bearer is the primary documented scheme … `x-api-key` … retained as an **unverified possibility**, never as "preferred" … This corrects the earlier framing … remains `BLOCKED-CRED` … does not prove `/v1/*` behaves identically" (L340-351) | "**删除 `x-api-key`** … Bearer 是首要且有文档依据的方案 … 仅作为**未经证实的可能性**保留，绝不写作"优先" … 这更正了早先的表述 … 仍为 `BLOCKED-CRED` … 不能证明 `/v1/*` 行为相同" (L319-330) | equivalent — the correction, the demotion of `x-api-key` and the `/v1/*` non-generalisation are all intact |
| §6.4 "intent, not a live fact" | "**Statement of intent, not a live fact:** … Whether `/v1/*` actually accepts Bearer is `BLOCKED-CRED`" (L438-440) | "**这是意图陈述，不是线上事实：** … `/v1/*` 是否真的接受 Bearer 属 `BLOCKED-CRED`" (L418-419) | equivalent — the disclaimer is preserved verbatim in meaning before the modelling statement |
| §7.2 M6 correction | "**M6 correction (`VERIFIED-STATIC`).** … deletes `x-api-key` … `x-api-key` therefore remains only an **unverified possibility** carried from prior art, never the recommended scheme; whether the server honours it is `BLOCKED-CRED`" (L472-477) | "**M6 更正（`VERIFIED-STATIC`）。** … 删除 `x-api-key` … `x-api-key` 仅为来自既有实现的**未经证实的可能性**，绝非推荐方案；服务端是否接受它属 `BLOCKED-CRED`" (L451-455) | equivalent |
| §8 "layer A ↔ layer B NOT established" | "**The mapping between layer A and layer B is NOT established.** … Similar-looking strings **are not probative** … treated as coincidental surface similarity, not as a confirmed join" (L523-528) | "**A 层与 B 层之间的映射并未建立。** … 字符串相似**不具证明力** … 按"表面巧合"处理，不视为已确认的关联" (L499-502) | equivalent — bold emphasis retained; the capitalised "NOT" is rendered as "并未" (see K4) |
| §8.3 claim table | 5 claims: `PUBLIC-MARKETING`, `PRIOR-ART`, `BLOCKED-CRED` — no catalogue or inference response observed, **not established**, `not published` (L590-596) | 5 claims: `PUBLIC-MARKETING`, `PRIOR-ART`, `BLOCKED-CRED`——未观测到目录或推理响应, **未建立**, `未公布` (L562-568) | equivalent — no claim upgraded; only the two prose labels were translated (K1) |
| §9 S13 | "Low–Medium … Privacy observation only … not a vulnerability, and nothing was exercised … Reported, not probed" (L619) | "**低–中** … 仅为隐私观察 … 不是漏洞，且未做任何操作 … 仅报告，未探测" (L591) | equivalent — rating and all hedges preserved |
| §9 S14 | "Medium … deliberately **not probed** … Risk reflects unknown data handling rather than demonstrated exposure" (L620) | "**中** … **有意未探测** … 风险反映的是未知数据处理，而非已证实的暴露" (L592) | equivalent |
| §12.2 `BLOCKED-CRED` rows | 7 rows: auth scheme "(partially narrowed)", live JSON body, models success bodies, profile schema, roles schema, token TTL/claims, real API-key path (L761-763, L766-767, L774-775) | same 7 rows, same order, same qualifiers: "(已部分收窄)" and the rest plain (L730-732, L735-736, L743-744) | equivalent — no `BLOCKED-CRED` was downgraded to a softer grade, and no soft grade was upgraded |

Cross-cutting: the document-level grade counts in §3.5 confirm no qualifier was globally dropped or promoted. The four code blocks carrying observed header/JSON strings (§5A.5 layout, §6.3 401 body, §11.2 commands, §11.3 commands — all 6 fenced blocks) are byte-identical.

---

## 6. Language check

- **Predominance of Chinese:** 7,293 CJK characters vs. ASCII content that is almost entirely inside code spans (operationIds, header names, paths, hashes, JSON tokens, grade labels). Prose is Simplified Chinese throughout, including all table column headers.
- **Residual English runs:** exactly one 3+-word English sequence outside code/fences — "Next.js App Router" (C L20), a product/technology name that must stay. Zero 4+-word English sentences.
- **Permitted English retained:** all code blocks, shell commands, header names (`WWW-Authenticate`, `x-api-key`, `X-Phanthy-*`), paths, JSON/SSE tokens (`message_start` … `[DONE]`, `{"error":{"code","message"}}`), operationIds, evidence filenames, grade labels (`VERIFIED`, `PRIOR-ART`, `BLOCKED-CRED`, `PUBLIC-MARKETING`, `VERIFIED-STATIC`, `VERIFIED-LIVE`, `INFERRED`), tag names (`OAuth`, `OIDC`, `Account`, `Inference`, `Web`, `Assets`, `Meta`), and verbatim artifact strings (`"verified absent"`, `x-verified-absent-paths`, `x-contract-status: "unverified"`, `Available model`, `Bearer access token is required.`, `Only authorization code flow is supported.`, `"never observed"`).
- **Verdict:** pass. Only three short quoted/labelled strings were rendered in Chinese (K1–K3 below), none of which is a UI string from the target.

---

## 7. Regression results

| check | command | result |
|---|---|---|
| self-test | `cd /workspace/re/apifox && python3 push_apifox.py --self-test` | **exit 0**; `openapi: 3.1.0`, `paths: 17`, `operations: 17`, `schemas: 13`, `local $refs: 44`, tags `Account, Assets, Inference, Meta, OAuth, OIDC, Web`, 17 unique operationIds, `SELF-TEST OK` |
| working tree | `git status --short` | `M re/report/reverse-engineering-report.md` only (plus 9 pre-existing deletions under `.screenshots/.tmp/`, which are unrelated to a Markdown translation and were not touched by me) |
| acceptance files | `git status --short re/acceptance/` and `git diff --stat HEAD -- re/acceptance/` | empty — `re/acceptance/M1.md`–`M6.md` (and the M6-* closure files) are untouched |

Conclusion: the translation round touched exactly one tracked file, the report.

---

## 8. New findings (K-series)

All items below are **cosmetic**; none changes a fact, number, path, status code, grade, rating or anchor. No factual defects (`K-factual`) were found.

| ID | class | baseline text (line) | current text (line) | severity | one-line fix |
|---|---|---|---|---|---|
| K1 | cosmetic | `` `not published` `` (B L596) | `` `未公布` `` (C L568) | trivial | restore `not published` verbatim if status labels are to remain untranslated |
| K2 | cosmetic | `` "unverified contract" `` (B L792) | `` "契约未验证" `` (C L759) | trivial | keep the English label inside the quotes |
| K3 | cosmetic | `` "no API key available" `` (B L433) | `` "无可用 API key" `` (C L413) | trivial | keep the English quoted paraphrase |
| K4 | cosmetic | "**… is NOT established.**" — capitalised negation (B L523) | "**A 层与 B 层之间的映射并未建立。**" — bold retained, caps lost (C L499) | trivial | optionally re-add an explicit marker (e.g. 未建立／NOT) for emphasis parity |
| K5 | cosmetic | "both offered as alternatives on `messagesCreate` only" (B L217-219) | "二者仅在 `messagesCreate` 上作为互斥备选" (C L201-203) | minor wording | "互斥" (mutually exclusive) is stronger than OpenAPI's OR-semantics "alternatives"; consider "备选" alone |
| K6 | cosmetic | heading `### 12.2 Still UNVERIFIED` (B L756) | heading `### 12.2 仍未验证项` (C L725) | trivial | none needed; the qualifier survives in Chinese |
| K7 | intentional (not a defect) | header "Language: English" (B L3) | "语言：中文" (C L3) | none | correct and required for a translated artifact — this line now accurately describes the file |

Note: K7 is the only line whose truth value changed, and it changed from "English" to "中文" precisely because the file's language changed; it is a metadata correction, not a factual regression.

---

## 9. Final judgement

The Simplified-Chinese report is **content-equivalent** to the English baseline: structure, all 15 tables row-for-row, every number/hash/operationId, all 225 evidence anchors, and every `VERIFIED` / `PRIOR-ART` / `BLOCKED-CRED` / `PUBLIC-MARKETING` / `VERIFIED-STATIC` / `VERIFIED-LIVE` / `INFERRED` qualifier are preserved, with no claim upgraded or hedge removed; therefore `/workspace/re/report/reverse-engineering-report.md` (Chinese) **may replace the English version as the canonical deliverable**, with K1–K3 optionally reverted if strict verbatim status labels are required.
