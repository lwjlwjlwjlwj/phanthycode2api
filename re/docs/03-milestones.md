# 03 · 项目里程碑

流程：`M0 治理 → M1 侦察 → M2 端点语义 → M3 SDD/TDD 手册 → M4 Apifox+报告 → M5 终验 → M6 CLI 供应链补全`

每个里程碑 = 1 个执行子 agent + 1 个只读验收子 agent。验收不通过则回到本里程碑修复后重验。

| ID | 里程碑 | 执行产物（确定性路径） | 验收标准 | 状态 |
|---|---|---|---|---|
| M0 | 治理与预检 | `re/docs/00-governance.md`、`re/evidence/M0-preflight.json` | 边界可执行、依赖可用、不可信仓库政策明确 | 已完成 |
| M1 | 侦察与证据固化 | `re/evidence/M1-*.{json,txt,md}`、`re/evidence/M1-index.md` | 抽样命令可复现；无 L2 数据落盘 | 已验收（PASS-WITH-GAPS） |
| M2 | 端点清单与调用语义 | `re/docs/01-endpoint-catalog.md`、`re/docs/02-usage-semantics.md` | 每个端点有方法/鉴权/参数/响应/错误码/证据引用；与 M1 证据一一对应 | 已验收（PASS-WITH-GAPS） |
| M3 | SDD + TDD 开发手册 | `re/docs/05-sdd-spec.md`、`re/docs/06-tdd-handbook.md` | 规格含验收标准与追踪矩阵；TDD 手册含红-绿-重构、测试金字塔、DoD，且与 `go test ./...` 基线一致 | 已验收（PASS-WITH-GAPS，已修复） |
| M4 | Apifox 导入包 + 逆向报告 | `re/apifox/openapi.json`、`re/apifox/IMPORT.md`、`re/apifox/push_apifox.py`、`re/report/reverse-engineering-report.md`；**M6 收尾补充**：`re/evidence/M4-push-mock.txt` | OpenAPI 3.1 通过结构校验；每条路径可回溯证据；报告含风险与合规声明；（补充项）推送链路本地 mock 端到端可跑通，真实租户仍需凭据 | 已验收（PASS-WITH-GAPS，已修复；补充项为 M6 收尾阶段新增，未参与 M4 当时验收） |
| M5 | 终验与交付审计 | `re/acceptance/M5.md` | 全链路一致性、可复现性、无密钥泄漏、导入包可用 | 已验收（PASS-WITH-GAPS） |
| M6 | 官方 CLI 供应链静态分析 + 新模型端点（增量） | `re/evidence/M6-npm-package.json`、`re/evidence/M6-cli-config.txt`、`re/evidence/M6-new-endpoint-probes.txt`、`re/evidence/M6-public-model-catalog.txt`、`re/evidence/M6-cli-install.txt`、`re/evidence/M6-index.md`；集成到 `docs/01`–`docs/06`、`apifox/openapi.json`、`apifox/IMPORT.md`、`report/reverse-engineering-report.md`；验收 `re/acceptance/M6.md`、`re/acceptance/M6-fixes.md`、`re/acceptance/M6-fixes-final.md`、`re/acceptance/M6-g-closure.md` | npm 包仅静态抽取（未安装/未执行/未导入），有 sha256 与偏移量证据；两条新端点匿名探测可复现且成功体标注 `BLOCKED-CRED`；模型目录标注 `PUBLIC-MARKETING` 且与 PRIOR-ART 映射分层不混用；活动文档端点计数一致为 17 且 OpenAPI 校验/自检通过 | 已验收（PASS-WITH-GAPS，D1–D6 已修复复验通过、记账项已更正） |

## 依赖关系

- M2 依赖 M1 证据；M3 依赖 M2 端点语义；M4 依赖 M2 + M3；M5 依赖全部。
- M6 是 M1/M2/M4 的增量补充（不重跑 M1 侦察），M6b 的文档集成以 M6 证据为前提。
- 登录态验证（`POST /v1/messages`、`GET /api/oauth/profile`）依赖用户提供自有 token；
  未提供时标记 `BLOCKED-CRED`，其余产物照常交付。
- M6 新增的 `GET /api/phanthy_code/models`、`/models/stats` 成功体同样依赖用户 token（`BLOCKED-CRED`）。

## 风险登记

| 风险 | 影响 | 处置 |
|---|---|---|
| 无 Apifox token | 无法直推 | 交付可导入 OpenAPI + 推送脚本，用户补 token 后一条命令同步 |
| 无测试账号 | 登录态语义只能靠公开元数据 + 既有实现推断 | 标注证据等级 `inferred`，不冒充 `verified` |
| 上游端点变更 | 报告过时 | 报告记录抓取时间戳；提供 `make recon` 复跑脚本 |
| 不可信仓库 | 供应链风险 | 只读文档，不执行 |
| 第三方 npm 包（官方 CLI） | 若安装或执行则可能触碰凭据/主机 | M6 只做 tarball 静态抽取（`tar -xzf` + 字节切片），`installed/executed/imported` 均为 false；凭据值不落盘 |

## 结项（用户决策）

- 2026-10-26：用户选择「以可导入包结项」。**Apifox 交付物 = 可一键导入的 OpenAPI 3.1 包 + IMPORT 指南 + 已用本地 mock 验证的推送脚本**，不再等待真实 token 直推。
- B1（Apifox 凭据）与 B2（测试账号/token）保留为**已登记的外部阻塞**，不作为未完成项；对应 `BLOCKED-CRED` 结论继续在端点目录与报告中如实标注。
- M0–M6 全部里程碑均已有执行产物与独立 verifier 验收记录（含 5 轮 M6 专项验收），无待验收项。
