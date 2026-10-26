# 04 · 任务清单（按里程碑）

约定：`[ ]` 待办，`[~]` 进行中，`[x]` 完成，`[!]` 阻塞。每个任务的验收人 = 独立 verifier 子 agent。

## M0 治理与预检
- [x] T0.1 明确目标、授权来源与硬性禁止清单 → `docs/00-governance.md`
- [x] T0.2 环境体检（node/python/go/代理/连通性）→ `evidence/M0-preflight.json`
- [x] T0.3 不可信仓库政策（只读、不执行）→ `docs/00-governance.md` §3
- [x] T0.4 Apifox 凭据缺失登记 → `evidence/M0-preflight.json`
- [x] T0.5 建立里程碑与依赖 → `docs/03-milestones.md`

## M1 侦察与证据固化
- [x] T1.1 抓取根页面与响应头（HTTP/2、CDN、安全头）→ `evidence/`
- [x] T1.2 抓取 OAuth/OIDC 公开元数据 → `evidence/`
- [x] T1.3 建立 OPTIONS 方法矩阵（候选端点）→ `evidence/`
- [x] T1.4 下载并静态分析 Next.js chunks → `evidence/`
- [x] T1.5 robots/sitemap/常见匿名入口探测 → `evidence/`
- [x] T1.6 证据索引与复现命令 → `evidence/M1-index.md`
- [x] T1.7 独立验收（PASS-WITH-GAPS，0 阻塞）→ `acceptance/M1.md`

## M2 端点清单与调用语义
- [x] T2.1 端点目录（方法/路径/鉴权/参数/响应）→ `docs/01-endpoint-catalog.md`
- [x] T2.2 调用语义与最小示例（含流式 SSE）→ `docs/02-usage-semantics.md`
- [x] T2.3 错误码与冷却分类映射 → `docs/02-usage-semantics.md`
- [x] T2.4 模型名映射与能力矩阵 → `docs/02-usage-semantics.md`
- [!] T2.5 登录态验证（需自有 token）→ `BLOCKED-CRED`
- [x] T2.6 独立验收（PASS-WITH-GAPS，0 阻塞）→ `acceptance/M2.md`

## M3 SDD + TDD 手册
- [x] T3.1 SDD 规格（需求/架构/契约/验收标准/追踪矩阵）→ `docs/05-sdd-spec.md`
- [x] T3.2 TDD 手册（红-绿-重构、测试金字塔、fixture、DoD）→ `docs/06-tdd-handbook.md`
- [x] T3.3 与现有 Go 测试基线对齐（`go test ./...`）→ 手册附录
- [x] T3.4 独立验收（PASS-WITH-GAPS，0 阻塞）→ `acceptance/M3.md`
- [x] T3.5 验收修复：覆盖率口径（1 全/6 部分/10 无）、原子写与 `NoteSuccess`、硬标记 17

## M4 Apifox 导入包 + 报告
- [x] T4.1 OpenAPI 3.1 规范 → `apifox/openapi.json`
- [x] T4.2 导入说明与环境变量 → `apifox/IMPORT.md`
- [x] T4.3 推送脚本（token 就绪后一键同步）→ `apifox/push_apifox.py`
- [x] T4.4 逆向报告 → `report/reverse-engineering-report.md`
- [x] T4.5 独立验收（PASS-WITH-GAPS，0 阻塞）→ `acceptance/M4.md`
- [x] T4.6 验收修复：SSE 响应键合并、响应键校验、移除非法 input-mode

## M5 终验与交付审计
- [x] T5.1 证据抽样复现（3/3 一致）
- [x] T5.2 规格/目录/OpenAPI 三方一致性
- [x] T5.3 密钥泄漏扫描（0 真实泄漏）
- [x] T5.4 OpenAPI 结构校验 + 自检（均通过）
- [x] T5.5 交付审计报告 → `acceptance/M5.md`（PASS-WITH-GAPS，0 阻塞）

## 外部阻塞（需用户输入）
- [!] B1 Apifox 直推：需 `APIFOX_ACCESS_TOKEN` + `APIFOX_PROJECT_ID`（UI 导入当前可用）
- [!] B2 登录态验证：需自有测试账号 / token（`BLOCKED-CRED`）
