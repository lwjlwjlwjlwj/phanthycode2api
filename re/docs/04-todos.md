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

## M6 官方 CLI 静态分析 + 新模型端点（增量）
- [x] T6.1 npm registry 元数据 + tarball 静态抽取（仅 `tar -xzf` + 字节切片，未安装/未执行/未导入）→ `evidence/M6-npm-package.json`、`evidence/M6-cli-config.txt`
- [x] T6.2 两条新端点匿名探测（`OPTIONS`/`GET`/`HEAD`，匿名、≥1.1 s 间隔）→ `evidence/M6-new-endpoint-probes.txt`
- [x] T6.3 公开定价页模型目录抓取（13 别名 + 积分倍率 + 4 档套餐）→ `evidence/M6-public-model-catalog.txt`
- [x] T6.4 证据索引、分级与 M1 差异说明 → `evidence/M6-index.md`
- [x] T6.5 M6b 文档集成：端点目录 EP-16/EP-17、调用语义、SDD 上游调用清单、里程碑、OpenAPI `phanthyModelsGet`/`modelStatsGet`、IMPORT、逆向报告；活动文档端点计数统一为 17
- [x] T6.6 独立验收（verifier 子 agent：PASS-WITH-GAPS，0 阻塞 / 6 非阻塞 D1–D6）→ `acceptance/M6.md`
- [x] T6.7 缺陷修复 + 独立复验（D1–D6 全部关闭，R1–R4 PASS；残留记账项 F1/F1a 已更正）→ `acceptance/M6-fixes.md`
- [x] T6.8 二次复验收尾（F1/F1a 复验通过；复验新发现的 6 项文档级问题 G1–G6 已更正：`M6-index` 锚点改为章节引用以消除行号漂移、时间窗与统计口径改为可证表述、计数范围与 docs 变更表述加限定、补充 `docs/06`）→ `acceptance/M6-fixes-final.md`
- [x] T6.9 G 轮复验的 5 项编辑级发现 H1–H5 已更正（错误章节号 2 处、会话时间窗加注、工件时间戳口径、`M6-fixes-final` 补入 M6 验收清单）→ `acceptance/M6-g-closure.md`
- [x] T6.10 Apifox 推送链路本地 mock 端到端验证（127.0.0.1，无外部流量；`input` 为字符串、`AUTO_MERGE`、bearer + API 版本头、exit 0）→ `evidence/M4-push-mock.txt`
- [x] T6.11 逆向报告按用户要求改为中文（内容等价、事实/锚点/数值不变，经独立 verifier 比对 HEAD 英文基线判定 PASS）→ `acceptance/M4-report-zh.md`

## 外部阻塞（需用户输入；2026-10-26 用户决定以可导入包结项，以下保留为已登记阻塞）
- [!] B1 Apifox 直推：需 `APIFOX_ACCESS_TOKEN` + `APIFOX_PROJECT_ID`（UI 导入当前可用）。推送链路已用本地 mock 端到端验证（`evidence/M4-push-mock.txt`），仅差真实凭据
- [!] B2 登录态验证：需自有测试账号 / token（`BLOCKED-CRED`）；M6 的 `GET /api/phanthy_code/models` 与 `/models/stats` 成功体同样依赖该 token
