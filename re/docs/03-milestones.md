# 03 · 项目里程碑

流程：`M0 治理 → M1 侦察 → M2 端点语义 → M3 SDD/TDD 手册 → M4 Apifox+报告 → M5 终验`

每个里程碑 = 1 个执行子 agent + 1 个只读验收子 agent。验收不通过则回到本里程碑修复后重验。

| ID | 里程碑 | 执行产物（确定性路径） | 验收标准 | 状态 |
|---|---|---|---|---|
| M0 | 治理与预检 | `re/docs/00-governance.md`、`re/evidence/M0-preflight.json` | 边界可执行、依赖可用、不可信仓库政策明确 | 已完成 |
| M1 | 侦察与证据固化 | `re/evidence/M1-*.{json,txt,md}`、`re/evidence/M1-index.md` | 抽样命令可复现；无 L2 数据落盘 | 已验收（PASS-WITH-GAPS） |
| M2 | 端点清单与调用语义 | `re/docs/01-endpoint-catalog.md`、`re/docs/02-usage-semantics.md` | 每个端点有方法/鉴权/参数/响应/错误码/证据引用；与 M1 证据一一对应 | 已验收（PASS-WITH-GAPS） |
| M3 | SDD + TDD 开发手册 | `re/docs/05-sdd-spec.md`、`re/docs/06-tdd-handbook.md` | 规格含验收标准与追踪矩阵；TDD 手册含红-绿-重构、测试金字塔、DoD，且与 `go test ./...` 基线一致 | 已验收（PASS-WITH-GAPS，已修复） |
| M4 | Apifox 导入包 + 逆向报告 | `re/apifox/openapi.json`、`re/apifox/IMPORT.md`、`re/apifox/push_apifox.py`、`re/report/reverse-engineering-report.md` | OpenAPI 3.1 通过结构校验；每条路径可回溯证据；报告含风险与合规声明 | 已验收（PASS-WITH-GAPS，已修复） |
| M5 | 终验与交付审计 | `re/acceptance/M5.md` | 全链路一致性、可复现性、无密钥泄漏、导入包可用 | 待启动 |

## 依赖关系

- M2 依赖 M1 证据；M3 依赖 M2 端点语义；M4 依赖 M2 + M3；M5 依赖全部。
- 登录态验证（`POST /v1/messages`、`GET /api/oauth/profile`）依赖用户提供自有 token；
  未提供时标记 `BLOCKED-CRED`，其余产物照常交付。

## 风险登记

| 风险 | 影响 | 处置 |
|---|---|---|
| 无 Apifox token | 无法直推 | 交付可导入 OpenAPI + 推送脚本，用户补 token 后一条命令同步 |
| 无测试账号 | 登录态语义只能靠公开元数据 + 既有实现推断 | 标注证据等级 `inferred`，不冒充 `verified` |
| 上游端点变更 | 报告过时 | 报告记录抓取时间戳；提供 `make recon` 复跑脚本 |
| 不可信仓库 | 供应链风险 | 只读文档，不执行 |
