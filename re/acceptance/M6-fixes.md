# M6 Defect-Closure Verification (D1–D6)

**Verdict: `PASS-WITH-GAPS`**

All six original non-blocking defects (D1–D6) are **closed**, and no functional regression was found
(R1–R4 pass). One **new, non-blocking bookkeeping inconsistency** remains in `evidence/M6-index.md`
(see §4). It does not invalidate any evidence and does not block acceptance.

- Verifier role: independent, read-only; no artifact modified; this report is the only write.
- Timestamp: `2026-10-26T01:13:19Z` = `2026-10-26T09:13:19+08:00` (`Asia/Shanghai`).
- Network used: **6 anonymous `GET`** requests through the sandbox egress proxy
  `http://127.0.0.1:18080` (3 discarded because the local output path did not yet exist, 3 retained),
  `>=1.2 s` apart; no login, token, cookie, POST, HEAD or OPTIONS.
- npm package: static byte-reading only; never installed, imported, transpiled or executed.
- Untrusted repository `github.com/witt3rd/oh-my-hermes`: not fetched/cloned/executed.

---

## 1. Defect closure table

| id | expected fix | observed | closed? | residual severity |
|---|---|---|---|---|
| **D1** | Remove bundle-wide “absence of any `x-api-key`”; scope the claim to the Phanthy message path; acknowledge `F.delete("x-api-key")`; prove other occurrences exist | `docs/01-endpoint-catalog.md:639` now says `x-api-key` “still appears elsewhere in the bundle only for generic third-party providers and redaction lists” and explicitly cites `F.delete("x-api-key")`; `:641-642` scopes the absence to “on that path”. Static grep of `dist/tui-worker.js`: **18** occurrences, including offset `673787` (redaction list `["authorization","cookie","set-cookie","x-api-key"]`), `1703806` / `5153429` (third-party provider auth), `5462701` / `5465515` / `5466584` (`authHeaders` catalogues), `7419748` (provider branch) and `13969765` (`F.delete("x-api-key")`). Scoped wording is factually correct. | **YES** | none |
| **D2** | Anchor install/login claims to a real M6 evidence capture; record HTTP/size/sha256 for `/cli`, `install.sh`, `install.ps1`; make hashes reproducible; include the four published commands verbatim | `evidence/M6-cli-install.txt` exists (1,874 B, sha256 `265f644d…ad382`). Re-fetched anonymously: `/cli` → `200`, 30,451 B, sha256 `422067f9…395ad2`; `install.sh` → `200`, 7,905 B, sha256 `5e0df605…ab2116`; `install.ps1` → `200`, 6,489 B, sha256 `b5c9b701…c9d12d` — **all three hash/size/HTTP values reproduce exactly**. The four commands appear verbatim in `/cli` and in the evidence: `curl -fsSL https://code.phanthy.com/install.sh \| bash`, `irm https://code.phanthy.com/install.ps1 \| iex`, `npm install -g @phanthy/phanthy-codeo`, `phanthy-codeo auth login`. Installer headers (`set -euo pipefail`, `PACKAGE_NAME=…`, `MIN_BUN_VERSION=…1.3.13`, `$ErrorActionPreference = 'Stop'`) also match. `docs/02-usage-semantics.md:94-105` now anchors the grade and URLs to this file. | **YES** | none (D2 itself); see §4 for index bookkeeping |
| **D3** | Point the keychain anchor to a real location; verify `lH9("security"` ≈1792690, `add-generic-password` ≈1793358, `find-generic-password` ≈1794461 in `dist/cli.js`; remove `Mv9`/tui-worker attribution | In `dist/cli.js` (sha256 `61f50331…27c52`): `lH9("security"` at **1792690** (1 occurrence), `add-generic-password` at **1793358**, `find-generic-password` at **1794461** — all three exact. `Mv9(` / `Mv9("security"` occur **0** times in `cli.js`; a repo-wide grep finds `Mv9` only inside the historical `acceptance/M6.md` defect row. `evidence/M6-cli-config.txt:117-120` now cites `dist/cli.js` / `lH9`. | **YES** | none |
| **D4** | Report S13 must list full fingerprint fields and anchor to `M6-cli-config.txt:195-201` | `report/reverse-engineering-report.md:617` now reads: “hostname, username, platform, architecture, OS release, up to 32 IPv4 candidates, network interfaces and default routes”, matching `evidence/M6-cli-config.txt:199-201` (`hostname`, `username`, `platform`, `arch`, `osRelease`, `ipCandidates` capped by `mv9=32`, `networkInterfaces`, `routes` capped by `uv9=32`). Anchor is `[M6-cli-config.txt:195-201]`. | **YES** | none |
| **D5** | One consistent client-id offset convention | `evidence/M6-cli-config.txt:49` now states: symbol `lq6` starts at `7928770`, quoted string starts at `7928775`. Static check: `lq6` = `7928770`, opening quote `"` = `7928774`, string content `phanthy-code-cli` = `7928775`. `:212` uses the same `7928775` (string-content convention). Consistent and correct. | **YES** | none |
| **D6** | Caveat must reconcile 17 = 13 probed + 2 `.well-known` (15) + 2 M6; must not claim 17 derives from M1’s matrix alone; add `M6-cli-install.txt` to inventory; inventory hash/size of `M6-cli-config.txt` must match current file | `evidence/M6-index.md:160-166` states 13 probed + 2 `.well-known` = 15, then M6 +2 = **17**, and explicitly says “neither is derivable from the M1 probe matrix alone.” Inventory `:37` now lists `M6-cli-install.txt` (sha256 `265f644d…ad382`, 1,874 B), matching the current file. Inventory `:34` lists `M6-cli-config.txt` as sha256 `c76ecf4b…b6b87`, **18,412 B** — recomputed identically on the current file. | **YES** | none |

---

## 2. Regression checks

| id | check | result | evidence |
|---|---|---|---|
| **R1** | `openapi.json` is 3.1.0, 17 paths, 17 unique operationIds, `$ref` resolve; `push_apifox.py --self-test` exits 0 | **PASS** | `json.load` → `openapi 3.1.0`; paths **17**; operations **17**; unique operationIds **17** (none missing); `$ref` **44** occurrences / **20** unique / **0** unresolved; 13 schemas; 7 reusable header parameters; 2 security schemes; 6 verified-absent paths absent from `paths`. `--self-test` → `SELF-TEST OK`, `EXIT=0`. |
| **R2** | Active docs still consistently say 17; historical M1–M5 keep 15 and are unchanged | **PASS** | Active: `docs/01:5,122`; `docs/02:197`; `docs/03:15` (“活动文档端点计数一致为 17”); `docs/04:56`; `docs/06:645`; `apifox/IMPORT.md:7,60,201,288`; `report:4-5,170,213,745`; `openapi.json` 17/17. Historical: `acceptance/M1–M5` mtimes `06:44`–`08:13`, all before the M6 window; `git status` shows them **not modified**; their 15-count statements intact (`M2:30,61,65,135`; `M4:55,56,77,329`; `M5:50,51,160`). |
| **R3** | No new secret leakage; npm package never executed | **PASS** | Regex scans over `evidence/M6-*`, `docs/0[1-6]-*.md`, `apifox/openapi.json`, `apifox/IMPORT.md`, `report/*.md`: zero JWT-shaped (`eyJ…`), zero `Bearer` + ≥16-char literal, zero `refresh_token: "…"`, zero `sk-[A-Za-z0-9]{16,}`, zero `claudeAiOauth: {` credential object. Only `<access_token>` / `<api_key>` placeholders. Package attestation `binaries_run:false`, `executed:false`, `installed:false`, `imported:false`; this verifier only byte-sliced files. |
| **R4** | No new claims beyond evidence (spot-check 5 new/anchor edits) | **PASS** | (1) D1 wording backed by 18 static occurrences; (2) D2 install/login claims backed by the reproduced capture and four verbatim commands; (3) D3 offsets backed byte-exactly; (4) D4 fingerprint fields backed by `M6-cli-config.txt:199-201`; (5) D6 caveat/inventory backed by arithmetic and recomputed hashes. Each edited claim carries a resolving anchor; no unsupported promotion to `VERIFIED` found. |

---

## 3. Commands executed (read-only)

```bash
# provenance / static slices (package already extracted under /tmp/m6, nothing executed)
python3 - <<'PY'   # find lH9("security"), add-generic-password, find-generic-password, Mv9(, x-api-key offsets
PY
sha256sum /workspace/re/evidence/M6-cli-install.txt /workspace/re/evidence/M6-cli-config.txt /workspace/re/evidence/M6-index.md
wc -c          /workspace/re/evidence/M6-cli-install.txt /workspace/re/evidence/M6-cli-config.txt

# anonymous GETs (6 total, proxy egress, >=1.2 s spacing)
curl -sS -o /tmp/m6-v/install.sh  https://code.phanthy.com/install.sh
curl -sS -o /tmp/m6-v/install.ps1 https://code.phanthy.com/install.ps1
curl -sS -o /tmp/m6-v/cli.html    https://code.phanthy.com/cli
sha256sum /tmp/m6-v/*; wc -c /tmp/m6-v/*

# verbatim command + installer-header assertions
python3 - <<'PY'   # membership tests for the four published commands and installer headers
PY

# OpenAPI regression
cd /workspace/re/apifox && python3 - <<'PY'   # version/paths/ops/unique ids/refs/schemas/params
PY
python3 /workspace/re/apifox/push_apifox.py --self-test; echo "EXIT=$?"

# count / immutability / secret checks
grep -rInE '15 paths|15 operations|15 endpoints|EP-01…EP-15|17 endpoints|17 operations|17 paths' /workspace/re
stat -c '%y %n' /workspace/re/acceptance/M{1,2,3,4,5}.md /workspace/re/docs/0[1-6]*.md ...
grep -rInE 'eyJ[A-Za-z0-9_-]{10,}|Bearer[[:space:]]+[A-Za-z0-9._-]{16,}|refresh_token[[:space:]]*:[[:space:]]*"[^<]|sk-[A-Za-z0-9]{16,}' ...
git status --short
```

---

## 4. Remaining defect (new, non-blocking)

| id | file:line | defect | severity | recommendation |
|---|---|---|---|---|
| **F1** | `evidence/M6-index.md:3,21,179,181` | The D2 fix added a sixth evidence artifact (`M6-cli-install.txt`) captured at `09:07:11` with **3 extra anonymous GETs**, but the index still says “complete (5 of 5 deliverables)” (`:3`), “request budget 9 of 10” (`:21`), “9 requests total (cap 10)” (`:179`) and “only the five M6 evidence files were written” (`:181`). The package now contains six evidence files and at least 12 anonymous requests across the M6/M6-fix captures. | Low (bookkeeping only) | Update the status line, request-budget row and governance bullet to state the six artifacts and the additional 3 `GET`s, or explicitly label the install capture as a separately budgeted follow-up capture. |
| F1a | `docs/03-milestones.md:15` | The M6 deliverables list omits the new `evidence/M6-cli-install.txt`. | Very Low | Add the file to the enumerated evidence list. |

No correctness, count, OpenAPI, privacy, or secret-handling regression was introduced by the fixes.
No D1–D6 defect remains open.

---

## 5. Acceptance confirmation

**M6 can be marked accepted.** D1–D6 are all closed with evidence-backed, reproducible fixes; R1–R4 pass;
historical acceptance files are untouched. The only residue is the editorial bookkeeping item F1/F1a above,
which is non-blocking and can be corrected in a single documentation touch-up without re-running any probes.
