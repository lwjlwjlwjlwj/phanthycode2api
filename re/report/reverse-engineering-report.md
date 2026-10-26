# 逆向工程报告 — `https://code.phanthy.com`

里程碑：**M4（API 文档与报告）**，在 **M6b** 中扩展 · 语言：中文 · 状态：完成
校验（`2026-10-26`）：`json.load` → `openapi 3.1.0`、17 条路径、7 个可复用参数；`push_apifox.py --self-test`
→ 17 条路径 / 17 个唯一 operation / 13 个 schema / 44 个本地 `$ref` / `SELF-TEST OK`（exit 0）。
目标：`https://code.phanthy.com` + npm 包 `@phanthy/phanthy-codeo@0.2.11`
采集时间窗：M1 `2026-10-26T05:58:24+08:00` → `06:14:37+08:00`；M6 主采集 `08:18:27` → `08:27:24`
（操作者会话边界——工件自身时间戳为 `08:19:00` → `08:22:00`，最后一次写入 `08:27:09`）；
M6 安装页采集 `09:07:11`

锚点约定：`M0-*`、`M1-*`、`M6-*` 锚点解析到 `/workspace/re/evidence/`；`docs/*` 解析到
`/workspace/re/docs/`；`acceptance/*` 解析到 `/workspace/re/acceptance/`；Go/README 锚点解析到 `/workspace/`。
所有"线上"陈述均继承自 M1 与 M6 的匿名采集；**M4 对目标零请求，M6b 对任何对象零请求**。凡涉及登录态的
行为一律标注为 `PRIOR-ART` / `BLOCKED-CRED`，且对官方 CLI 的 M6 静态分析**从未执行**。

---

## 1. 执行摘要

`code.phanthy.com` 是一个 Next.js App Router 应用，前置 Kong 2.5.1 网关与 TencentEdgeOne CDN，在同一
origin 下暴露两个互不相同的 API 家族：**兼容 Claude Code 的 OAuth 2.0 + PKCE** 面（`/oauth/*`、
`/api/oauth/*`、`/.well-known/*`）与**兼容 Anthropic Messages 的推理**面（`POST /v1/messages`）。另有一
小块公开元数据/静态资源面（`/api/download/client`、`/api/public-assets/`、`/robots.txt`、`/sitemap.xml`）
以及 HTML 应用外壳，构成观测到的全部足迹。

已证实存在 17 个端点；6 条探测路径已证实不存在（返回 Next.js 的 HTML 404 外壳）
`[M1-index.md:76-80]`。OAuth 元数据内部自洽且完全可读：仅授权码模式、仅 PKCE `S256`、公开客户端
（`token_endpoint_auth_methods_supported: ["none"]`）、五个 scope、RS256 JWKS
`[M1-openid-configuration.json:7-26]` `[M1-jwks.json:22]`。

**M6 增量（`2026-10-26`）。** 又确认了两个端点——`GET /api/phanthy_code/models` 与
`GET /api/phanthy_code/models/stats`：二者对匿名请求返回 `OPTIONS 204` 且
`allow: GET, HEAD, OPTIONS`，匿名 `GET` 均返回 `401` 与
`WWW-Authenticate: Bearer realm="phanthy-code"`。其成功响应体为 `BLOCKED-CRED`
`[M6-new-endpoint-probes.txt:28-193]`。官方 CLI `@phanthy/phanthy-codeo@0.2.11` 仅做**静态分析**
（tarball 解包；从未安装、导入或执行），独立确认了上述两条路径、六项 scope 请求集合、七个请求头组成的
客户端头族，以及"客户端**删除 `x-api-key`**、仅用 Bearer 鉴权"这一结论
`[M6-cli-config.txt:31-99, 145-157]`。公开定价页宣传了 13 个模型别名及积分倍率；该清单属于
`PUBLIC-MARKETING`，**不是**仓库内的 Iris/Zeus/Gaia/Apollo/Metis 映射——两层在 §8 中严格分列。

核心局限由凭据决定：**`POST /v1/messages` 的鉴权方案与成功体从未被观测**。仓库内既有实现优先使用
`x-api-key: <api_key>`，并以 `Authorization: Bearer <token>` 作为回退
`[internal/upstream/client.go:344-350]`；而两条 API-key 铸造路由均返回 HTML 404 外壳
`[M1-anon-probe-matrix.txt:67-68]`，故回退方案是唯一有文档可依的可行凭据路径。本报告因此严格区分
`VERIFIED`（目标事实）、`PRIOR-ART`（线上假设）与 `INFERRED`（协议推理），绝不把推理响应当作已观测结果。

M4 产出的交付物：

| 交付物 | 路径 | 状态 |
|---|---|---|
| 可导入 Apifox 的 OpenAPI 3.1.0 包 | `/workspace/re/apifox/openapi.json` | 已校验：17 paths / 17 operations / 13 schemas / 7 个可复用参数 / 44 个本地 `$ref` |
| 导入与运维指南 | `/workspace/re/apifox/IMPORT.md` | 完整；真实租户推送仍为 UNVERIFIED |
| 可选推送/校验 CLI（仅标准库） | `/workspace/re/apifox/push_apifox.py` | `--self-test` 退出 0 |
| 本报告 | `/workspace/re/report/reverse-engineering-report.md` | 13 节 |

---

## 2. 范围、授权与治理

**授权。** 本次工作限定为对用户指定目标的**只读逆向**；仅当用户提供自有测试账号/token 时，才允许可选的
登录态探测。治理文件为 `docs/00-governance.md`，其中写明授权来源、凭据前置条件，以及禁止以猜测或第三方
凭据替代 `[docs/00-governance.md:8-20]`。实际执行中从未提供任何凭据：
`authenticated_test_account: NOT PROVIDED`、`apifox_credentials: ABSENT` `[M0-preflight.json:18-19]`。
因此所有登录态观测均归为 `BLOCKED-CRED`。

**遵守的硬性禁令。** 未绕过鉴权、未伪造 token、未爆破或撞库、未做 DoS/限流压测、未持久化真实凭据、未
利用漏洞、未做写操作——唯一例外是治理允许的、使用用户自有 token 的 `POST /v1/messages`
`[docs/00-governance.md:11-20]`。匿名探测上限为 ≤1 req/s、单端点 ≤20 次；登录态验证 ≤5 次调用
`[docs/00-governance.md:20]`。M1 共记录 54 次请求，间隔 ≥1.1 s，单端点最多 5 次
`[M1-anon-probe-matrix.txt:8, 13]`。

**不可信仓库。** `https://github.com/witt3rd/oh-my-hermes` 按 UNTRUSTED 处理：仅纯文本阅读，从未 clone、
安装或执行，也未采纳其中任何操作性指令 `[docs/00-governance.md:22-32]` `[M0-preflight.json:20]`。

**数据分级。** L0（公开元数据、状态码、响应头、路由）原样保留；L1（结构、错误码、模型名）脱敏后写入
报告/OpenAPI；L2（`Authorization`、`x-api-key`、`refresh_token`、cookie）**永不落盘**——只记录
"存在/不存在" `[docs/00-governance.md:34-40]`。本报告与 OpenAPI 只含 `<api_key>`、`<token>`、
`<access_token>` 占位符。Cookie 仅记录属性名与 `Max-Age`，值在证据中已脱敏
`[M1-anon-probe-matrix.txt:346-347]`。

**M4 写入纪律。** M4 仅创建允许的四个文件，未修改任何既有证据、文档、验收报告或源码。M4 仅访问
`docs.apifox.com` / `apifox.com` 以核对导入端点；对 `code.phanthy.com` **零请求**。

**M6b 写入纪律。** M6b 是文档集成轮：**未发起任何网络请求**，未执行任何对象。仅编辑允许的八个产物
（五个 `docs/*.md`、两个 `apifox/*`、本报告），未触碰 `re/acceptance/M1.md`–`M5.md`、
`docs/00-governance.md`、`docs/06-tdd-handbook.md` 及任何 M0/M1/M6 证据文件。其集成的 M6 事实均已在
M6 证据中固化并哈希，未新增任何观测。

---

## 3. 方法论

1. **匿名侦察。** 端到端通读九个 M1 工件：根响应头、OPTIONS 矩阵、OIDC 元数据、JWKS、JS 静态分析、JS
   chunk 溯源、匿名探测矩阵、索引、M0 预检。身份完全匿名——无 `Authorization`、无 `Cookie`、无
   `x-api-key` `[M1-anon-probe-matrix.txt:6]`。全部流量经沙箱代理 `http://127.0.0.1:18080` 以 HTTP/2
   出网；直连 origin 的 TCP 被阻断 `[M1-anon-probe-matrix.txt:7]`。每份 dump 开头的 `HTTP/1.1 200 OK`
   是代理 CONNECT 握手，不是 origin 响应 `[M1-anon-probe-matrix.txt:39-44]`。
2. **OPTIONS 存在性矩阵。** 由于 API 路由的 405 不带 `Allow`，`OPTIONS` 是唯一可靠的方法探针
   `[M1-anon-probe-matrix.txt:447-448]`。`204 + allow` 证明 handler 存在；`404 + text/html` 证明路由不
   存在 `[M1-options-matrix.txt:7-10]`。用实时 GET/HEAD 交叉核对，未发现矛盾
   `[M1-anon-probe-matrix.txt:441-448]`。
3. **OIDC 元数据。** 抓取 discovery 文档并与已存 JSON 做字段比对；因线上响应为压缩格式、存档为美化
   格式，只声明字段级相等 `[M1-anon-probe-matrix.txt:450-469]`。
4. **JS 静态分析。** 下载 12 个公开 Next.js chunk 并做文本扫描，不执行。其中只出现三个端点字符串
   （`/api/download/client`、`/api/web/events`、`/api/public-assets/`），对 `/oauth/`、`x-api-key`、
   `authorization`、`anthropic-version`、`anthropic-beta` 及任何模型家族均为**零命中**
   `[M1-js-static-analysis.txt:56-69, 84-96, 97-124]`。故前端不能作为协议探针。
5. **既有实现交叉核对。** 仓库内 Go 客户端（`cmd/login`、`internal/upstream`）作为登录态与仅 POST 契约
   的既有实现参考。它约束的是*客户端*行为，绝不代表*目标*行为。
6. **OpenAPI 建模与校验。** 每个 operation 都带明确的证据等级；共享结构用 `$ref`；只写观测到的状态码；
   未编造成功体。写本报告前已在本地完成校验（`json.load` + 标准库校验器）。
7. **官方包静态分析（M6）。** 链路为：npm registry 元数据 → tarball
   （`sha256 3134d33d4eba31ec0af543a8afa061d3b21c4df7259d49decc11928e2244d80c`，解包后 969 个文件 /
   76,873,667 B）→ `tar -xzf` 解到 `/tmp` → 按记录偏移做 Python 字节切片，并对两个被检 bundle 单独计算
   sha256 `[M6-npm-package.json:13-93]` `[M6-cli-config.txt:6-31]`。全程未 `npm install`、未 import、
   未转译、未执行；无二进制运行、无包内 socket 打开、未读取任何凭据文件。这是安全检视闭源一方客户端的
   唯一方法。
8. **公开营销页采集（M6）。** 对 `https://code.phanthy.com/token-plan` 做一次匿名 `GET`（49,225 B，
   sha256 `2abf943b…be8a`），不渲染 JavaScript、不执行任何东西；模型行与套餐档位仅从服务端 HTML 提取
   `[M6-public-model-catalog.txt:4-18]`。

**证据分级。** `VERIFIED` = M1 线上观测且有 file:line 锚点；`PRIOR-ART` = 仅来自仓库内代码/README；
`INFERRED` = 依元数据/RFC/命名推理且注明依据；`BLOCKED-CRED` = 需要用户自有 token，未编造任何内容
`[docs/01-endpoint-catalog.md §1.4]`。M6 增加三个更细等级，仅用于 M6 派生陈述：`VERIFIED-LIVE`
（M6 匿名窗口线上观测，`M6-new-endpoint-probes.txt`）、`VERIFIED-STATIC`（官方 npm bundle 内的精确字符串，
从未执行，`M6-cli-config.txt`）、`PUBLIC-MARKETING`（公开定价页——不是 API 契约，
`M6-public-model-catalog.txt`）。

**带入本报告的更正。** M3 验收发现 SDD 中两处统计被夸大，最终报告不得引用
`[acceptance/M3.md:34-42, 229-242]`。本报告采用独立重算的数字：**FR 完全覆盖 = 1（FR-10），部分覆盖 = 6，
`NO-TEST` = 10，覆盖率 7/17 = 41.2%**，以及**17 个硬错误标记、6 个会话失效标记**
`[acceptance/M3.md:179-192]`。SDD 原文的"3 个完全覆盖"与"16 个硬标记"在本文中**不予重复**。

---

## 4. 目标指纹

| 层次 | 观测 | 证据 |
|---|---|---|
| 边缘 / WAF | `server: TencentEdgeOne`、`eo-log-uuid`、`eo-cache-status` | `M1-root-headers.txt:16-20` |
| 网关 | `via: kong/2.5.1`、`x-kong-upstream-latency`、`x-kong-proxy-latency` | `M1-root-headers.txt:13-15` |
| 应用 | `x-powered-by: Next.js`；App Router（RSC `vary`、`x-nextjs-cache`、`/_next/static/*`） | `M1-root-headers.txt:10-12` `[M1-anon-probe-matrix.txt:89-100]` |
| 传输安全 | 仅 HSTS：`strict-transport-security: max-age=31536000; includeSubDomains` | `M1-root-headers.txt:9` |
| 缓存 | 根路径 `cache-control: private, must-revalidate, no-cache, no-store, max-age=0`；元数据 `x-nextjs-cache: HIT` | `M1-root-headers.txt:17` `[M1-anon-probe-matrix.txt:276, 285]` |
| 工件存储 | 腾讯 COS 桶 `agi-phanthy-dev-1252788780`、`cos.ap-beijing.myqcloud.com` | `M1-anon-probe-matrix.txt:345` |
| 语言 / 区域 | `html dir="ltr" lang="zh-CN"`；404 外壳使用同一 locale | `M1-anon-probe-matrix.txt:59, 67` |
| 错误信封 | 四个 JSON 错误体均为 `{"error":{"code":"...","message":"..."}}` | `M1-anon-probe-matrix.txt:60, 66, 76, 365` |

公开根文档为 56,923 字节的流式 HTML（无 `content-length`）
`[M1-root-headers.txt:27]` `[M1-anon-probe-matrix.txt:59]`。基础设施响应头集合（`x-kong-*`、`eo-*`、
`via`、`server`）是唯一的运维信息泄漏；从未观测到 `RateLimit-*`、`Retry-After` 或配额类头
`[M1-index.md:210-212]`。

---

## 5. AI 端点面

### 5.1 已证实端点清单（17 个 operation）

| # | 方法 | 路径 | 鉴权（观测 / 既有实现） | 用途 | 证据等级 | 锚点 |
|---|---|---|---|---|---|---|
| 1 | GET | `/` | 无 | Next.js 应用外壳（HTML） | VERIFIED | `M1-anon-probe-matrix.txt:59` |
| 2 | GET | `/oauth/authorize` | 到达 handler 无需鉴权；完成需浏览器登录 | 授权码 + PKCE 入口 | VERIFIED（存在性 + 错误文案）/ PRIOR-ART（参数集） | `M1-anon-probe-matrix.txt:60, 112` |
| 3 | POST | `/oauth/token` | 公开客户端，无 secret | 授权码兑换 + refresh 授予 | VERIFIED（存在性）/ PRIOR-ART（请求体）/ BLOCKED-CRED（兑换） | `M1-options-matrix.txt:16` |
| 4 | POST | `/oauth/revoke` | INFERRED 公开客户端 | RFC 7009 撤销 | VERIFIED（存在性）/ INFERRED（语义） | `M1-options-matrix.txt:17` |
| 5 | GET | `/oauth/code/success` | 无 | 静态"登录完成"落地页 | VERIFIED | `M1-anon-probe-matrix.txt:65` |
| 6 | GET | `/.well-known/openid-configuration` | 无 | OAuth/OIDC discovery | VERIFIED（字段级） | `M1-openid-configuration.json:2-26` |
| 7 | GET | `/.well-known/jwks.json` | 无 | RS256 签名密钥集 | VERIFIED | `M1-jwks.json:22` |
| 8 | GET | `/api/oauth/profile` | `Authorization: Bearer <access_token>` | 账号资料 | VERIFIED（守卫）/ BLOCKED-CRED（成功 schema） | `M1-anon-probe-matrix.txt:66, 154-163` |
| 9 | GET | `/api/oauth/claude_cli/roles` | `Authorization: Bearer <access_token>` | 角色/权益查询 | VERIFIED（守卫）/ BLOCKED-CRED（语义） | `M1-anon-probe-matrix.txt:69` |
| 10 | POST | `/v1/messages` | `Authorization: Bearer <access_token>`（官方 CLI，VERIFIED-STATIC）；`x-api-key` 仅作为未经证实的既有实现备选 | Anthropic Messages 推理（+ SSE） | VERIFIED（存在性）/ PRIOR-ART（线上格式）/ BLOCKED-CRED（服务端鉴权 + 实时响应体） | `M1-options-matrix.txt:23` `[M6-cli-config.txt:145-157]` |
| 11 | POST | `/api/web/events` | 未知 | 语义未知的事件摄取/流 | VERIFIED（存在性）/ 其余仅 INFERRED | `M1-options-matrix.txt:26` |
| 12 | GET | `/api/download/client` | 无 | 客户端工件下载；铸造访客 cookie | VERIFIED | `M1-anon-probe-matrix.txt:337-366` |
| 13 | GET | `/api/public-assets/`（308 → `/api/public-assets`） | 未知 | 公开资源面；目标未被跟随 | VERIFIED（仅重定向）/ INFERRED（目标） | `M1-anon-probe-matrix.txt:247-255` |
| 14 | GET | `/robots.txt` | 无 | 爬虫策略 | VERIFIED | `M1-anon-probe-matrix.txt:369-401` |
| 15 | GET | `/sitemap.xml` | 无 | SEO sitemap，841 个 URL | VERIFIED | `M1-anon-probe-matrix.txt:404-438` |
| 16 | GET | `/api/phanthy_code/models` | `Authorization: Bearer <access_token>` | 官方 CLI 选择器的模型目录 | VERIFIED-LIVE（存在性 + 守卫）/ BLOCKED-CRED（成功体） | `M6-new-endpoint-probes.txt:25-105` |
| 17 | GET | `/api/phanthy_code/models/stats` | `Authorization: Bearer <access_token>` | 按模型统计（best-effort，客户端 5 s 超时） | VERIFIED-LIVE（存在性 + 守卫）/ INFERRED（用途）/ BLOCKED-CRED（成功体） | `M6-new-endpoint-probes.txt:110-193` |

方法矩阵：四条仅 POST 路径（`/oauth/token`、`/oauth/revoke`、`/v1/messages`、`/api/web/events`）、五个
只读 handler、两个仅 GET/HEAD 的静态元数据文件 `[M1-options-matrix.txt:13-32]`。M6 新增的两条同样仅
GET/HEAD——`OPTIONS` 对二者均报 `allow: GET, HEAD, OPTIONS` `[M6-new-endpoint-probes.txt:42, 131]`。
端点编号遵循 `docs/01-endpoint-catalog.md`（`EP-01` … `EP-17`）；需注意仅凭 M1 证据只能支撑 13 条存在
路径，M6 的两条是唯一增量，"总数 17"是编辑性约定，无法仅由 M1 推出 `[M6-index.md §5]`。

### 5.2 已证实不存在（六条路径，HTML 404 外壳）

`/api/oauth/phanthy_cli/create_api_key`、`/api/oauth/claude_cli/create_api_key`、
`/v1/messages/count_tokens`、`/v1/models`、`/api/health`、`/healthz` `[M1-index.md:76-80]`。全部返回
`404 text/html`、约 26.5 KB、带 `x-powered-by: Next.js`、无 JSON 信封
`[M1-anon-probe-matrix.txt:67-79]`。这些路径**不写入 OpenAPI 文档**，而是在 `info.description` 中列为
"verified absent"（`x-verified-absent-paths`）。M6 修正：`/v1/models` 确实不存在，但模型发现能力**并未
缺失**——机器可读目录位于 `/api/phanthy_code/models`（§5.1 第 16–17 行），受 bearer 守卫
`[M6-index.md §3.1, §4]`。

### 5.3 OpenAPI 建模决策

- **文档：** OpenAPI `3.1.0`，server `https://code.phanthy.com`，七个 tag（`OAuth`、`OIDC`、`Account`、
  `Inference`、`Web`、`Assets`、`Meta`），**17 个 operation**，13 个共享 schema，7 个可复用请求头参数，
  44 个内部 `$ref`。
- **安全：** `apiKeyAuth`（`apiKey`，header，`x-api-key`）与 `bearerAuth`（`http`，`bearer`），二者仅
  在 `messagesCreate` 上作为并列备选方案（`security: [{apiKeyAuth: []}, {bearerAuth: []}]`，OpenAPI
  security 数组语义为 OR）；
  `profileGet`、`claudeCliRolesGet`、`phanthyModelsGet`、`modelStatsGet` 仅声明 `bearerAuth`。
  `apiKeyAuth` 被描述为**未经证实的既有实现可能性**，因为官方 CLI 会删除该头
  `[M6-cli-config.txt:145-157]`。
- **`phanthyModelsGet` / `modelStatsGet`（M6 新增）：** `401` 完整定义（观测到的 `ErrorEnvelope` 体 +
  `WWW-Authenticate: Bearer realm="phanthy-code"`），而 `200` 使用宽松的 `object` schema，其 description
  明示成功 schema 为 `BLOCKED-CRED`；未断言任何字段、示例或枚举 `[docs/02-usage-semantics.md §1B]`。
- **七个可复用参数：** `XPhanthyClient`、`XPhanthyClientSource`、`XPhanthyCliDevice`、
  `XPhanthyProduct`、`XPhanthyProject`、`XPhanthyRequest`、`XPhanthySession`，位于
  `components.parameters`，全部可选，挂在 `messagesCreate` 与两个模型 operation 上。它们以精确大小写
  复刻官方客户端的头族；属客户端行为（`VERIFIED-STATIC`），并非已证实的服务端要求。
- **`messagesCreate`：** Anthropic Messages 请求体（`model`、`max_tokens`、`messages[]`、`stream`、
  `system`、`temperature`、`top_p`、`tools`、`tool_choice`、`metadata`）；两个 200 media type——
  `application/json`（宽松的既有实现 schema，标注"never observed"）与 `text/event-stream`（具名 SSE
  示例）。未编造任何精确成功 JSON。
- **`webEventsCreate`：** 仅一个 `default` 响应，其 description 说明契约未经验证；无请求体、无安全声明，
  `x-contract-status: "unverified"`。
- **`clientDownloadGet`：** `platform` 枚举仅含已证实的 `linux`；观测到的 `darwin` 拒绝记录在参数说明与
  400 响应中。
- **`publicAssetsGet`：** 仅建模观测到的 308；不含下游响应体。
- **`profileGet` / `claudeCliRolesGet`：** 200 响应仅描述（无 schema、无示例）；观测到的 401
  `missing_login` 信封完整定义。
- **`/oauth/token` 与 `/oauth/revoke`：** `application/x-www-form-urlencoded` 请求体，字段集来自既有实现
  /RFC 7009；`grant_type` 取值与既有实现一致。
- **`/oauth/authorize`：** 七个 query 参数，`code_challenge_method` 枚举 `[S256]`，scope 示例
  `user:inference user:profile user:sessions:claude_code`。
- **错误信封：** 一个共享 `ErrorEnvelope`（`error.code`、`error.message`）加 `ErrorDetail`，供所有观测到
  JSON 错误的 operation 复用。未建模 JSON 404 分类，因为目标不存在此类响应。

---

## 5A. 官方 CLI 供应链静态分析（M6）

### 5A.1 方法与"未执行"声明

一方客户端以 **`@phanthy/phanthy-codeo@0.2.11`** 发布到 npm（`2026-10-21T08:56:55.362Z`；
`bin.phanthy-codeo`、`engines.bun >=1.3.14`、`type: module`、MIT、声明仓库
`github.com/yizhixu/phanthy-codeo`）`[M6-npm-package.json:13-37]`。分析严格为静态：

```
registry metadata (1 GET) → tarball (1 GET) → sha256 compare → tar -xzf into /tmp → Python byte slicing
```

Tarball `sha256 3134d33d4eba31ec0af543a8afa061d3b21c4df7259d49decc11928e2244d80c`；解包 969 个文件、
76,873,667 B；被检两个 bundle 为 `dist/tui-worker.js`
（`sha256 c38ca895230b827838cfebb19b2b9becdf8523379118868a0f9461956f9b0c6d`）与 `dist/cli.js`
（`sha256 61f503311058d5ec9e571db3f0a74bc3c883ea3a781b916b49619d16b8b27c52`）
`[M6-npm-package.json:39-93]`。证据中记录的声明为 `static_only: true`、`executed: false`、
`installed: false`、`imported: false`、`binaries_run: false`。**任何时刻均未运行包内代码**，故攻击者可控
代码无机会接触沙箱，也未读取任何凭据文件。

值得显式说明的"未发现项"：包内**不含**模型清单、不含 API key、不含 token、不含 `.credentials.json`；
也未发现任何能让未登录方铸造凭据的内容 `[M6-cli-config.txt:102-120, 206-208]`。

### 5A.2 Provider 配置（`VERIFIED-STATIC`）

从 `dist/tui-worker.js` 字节偏移 **7928736** 处逐字提取（`dist/cli.js` 中有同内容的重复块）
`[M6-cli-config.txt:31-99]`：

| 元素 | 值 |
|---|---|
| base URL | `https://code.phanthy.com` |
| client id | `phanthy-code-cli` |
| provider 名称 / ID | `phanthy` |
| authorize | `https://code.phanthy.com/oauth/authorize` |
| token | `https://code.phanthy.com/oauth/token` |
| profile | `https://code.phanthy.com/api/oauth/profile` |
| models | `https://code.phanthy.com/api/phanthy_code/models` |
| modelStats | `https://code.phanthy.com/api/phanthy_code/models/stats` |
| messages | `https://code.phanthy.com/v1/messages`（映射表条目，**从未被解引用**） |

client id 与仓库内 Go 值一致 `[cmd/login/main.go:28, 146]`，故 `phanthy-code-cli` 现由两个独立来源互证。
`messages` 映射条目零调用点；真实消息路径是一个兼容 Anthropic SDK 的 provider（`apiKey:"phanthy-oauth"`、
`baseURL: ${base}/v1`），其请求发往绝对路径 `/v1/messages`，故有效 URL 可能是 `/v1/messages` **或**
`/v1/v1/messages`——记为 `UNVERIFIED` `[M6-cli-config.txt:145-157]` `[M6-index.md §3.8]`。

### 5A.3 请求头族（`VERIFIED-STATIC`——精确大小写）

| 请求头（精确） | 映射字段 | 观测到的值 / 用途 |
|---|---|---|
| `x-phanthy-client` | `client` | 客户端标识 |
| `X-Phanthy-Client-Source` | `clientSource` | 消息路径为 `cli`，遥测为 `phanthy-codeo` |
| `x-phanthy-cli-device` | `device` | `base64url(JSON)` 主机指纹——隐私观察，见 §9 S13 |
| `X-Phanthy-Product` | `product` | `phanthy_code` / `phanthy_codeo` |
| `x-phanthy-project` | `project` | 会话所属 project id |
| `x-phanthy-request` | `request` | 每请求唯一 UUID 关联 id |
| `x-phanthy-session` | `session` | 会话 id |

其中两个为 `X-Phanthy` 大写、五个为小写 `x-phanthy-`；OpenAPI 的 `components.parameters` 块逐字保留该
大小写 `[M6-cli-config.txt:58-73]`。仅遥测上报时附带非命名空间的 `x-service-name: phanthy-codeo`。

### 5A.4 Scope 差集：客户端请求 6 项 vs 服务端公布 5 项

CLI 请求六个 scope：`org:create_api_key`、`user:profile`、`user:inference`、
`user:sessions:claude_code`、`user:mcp_servers`、`user:file_upload` `[M6-cli-config.txt:74-99]`。服务端
discovery 文档只公布五个 `[M1-openid-configuration.json:20-26]`。差集为 **`org:create_api_key`**——客户端
请求而服务端从未公布的唯一 scope。存在两种解读，且在无 token 时都无法证实：(a) 陈旧或共享的客户端库
请求了当前部署已不再公布的 scope；(b) 服务端实际接受但未列出。记为开放问题
`[M6-index.md §3.4]` `[docs/01-endpoint-catalog.md §6 R18]`。这**不**意味着存在可用的 API-key 路径：两条已
知的 `create_api_key` 路由都返回 HTML 404 外壳。

### 5A.5 凭据存储布局（`VERIFIED-STATIC`）

```
default dir   ~/.phanthy-code/                    (override: $PHANTHY_CONFIG_DIR)
credential    ~/.phanthy-code/.credentials.json   (path.join(dir, ".credentials.json"))
json key      "claudeAiOauth"                     (OAuth payload; absent ⇒ file treated as non-OAuth)
secondary cfg .config.json  →  else .phanthy-code.json
macOS         `security` CLI keychain entry, service `phanthy code-credentials[-<sha256(dir)[:8]>]`
identity      $USER ?? os.userInfo().username ?? "claude-code-user"
```

该布局为 Claude Code 形态。**包内不存在凭据值，也未读取任何凭据** `[M6-cli-config.txt:102-120]`。此段
仅用于接入说明；切勿粘贴真实的 `.credentials.json`。

### 5A.6 Bearer 与 `x-api-key`（关键更正）

CLI 的 provider `fetch` 会**删除 `x-api-key`**，随后设置 `Authorization: Bearer <access token>`，并附加
`X-Phanthy-Product: phanthy_code`、`X-Phanthy-Client-Source: cli`、`User-Agent` 与新的
`x-phanthy-request` UUID `[M6-cli-config.txt:145-157]`。

| 方案 | 状态 | 结论 |
|---|---|---|
| `Authorization: Bearer <token>` | 官方客户端**唯一**发送的方案（`VERIFIED-STATIC`） | Bearer 是首要且有文档依据的方案 |
| `x-api-key: <key>` | 被官方客户端显式移除；仅存于既有实现 | 仅作为**未经证实的可能性**保留，绝不写作"优先" |

这更正了早先的表述（既有实现"优先 `x-api-key`"）。线上 `/v1/*` 是否接受 Bearer 仍为 `BLOCKED-CRED`；
M6 在 `/api/phanthy_code/*` 上观测到的 `401` 挑战与"bearer 守卫的 API 面"一致，但不能证明 `/v1/*` 行为
相同。

### 5A.7 仅静态、未探测的面

bundle 中还含 `POST ${base}/api/event_logging/batch`（遥测批量上报，JSON `{events:[…]}`，Bearer +
`x-service-name: phanthy-codeo`）、构建标记主机 `dev.code.phanthy.com`、
`https://code.phanthy.com/console` 以及一个 web 搜索 base URL `[M6-cli-config.txt:123-166]`。这些均为
`VERIFIED-STATIC` / **线上未验证**：`/api/event_logging/batch` **未被探测**、**不计入** 17 个端点，且未经
治理修订不得探测。见 `docs/01-endpoint-catalog.md` §7。

---

## 6. 鉴权与 OAuth/OIDC 架构

### 6.1 流程（PKCE，公开客户端）

Discovery 文档实为 OIDC 文件名下的 OAuth 2.0 元数据：`response_types_supported=["code"]`、
`grant_types_supported=["authorization_code","refresh_token"]`、`code_challenge_methods_supported=["S256"]`、
`token_endpoint_auth_methods_supported=["none"]` `[M1-openid-configuration.json:7-19]`。**没有**
`userinfo_endpoint`、没有 `id_token_signing_alg_values_supported`、没有 `subject_types_supported`、也没有
`openid` scope——由完整字段清单证实 `[M1-openid-configuration.json:2-26]` `[M1-index.md:114-128]`。

观测/既有实现序列：

1. 客户端生成随机 verifier 与 `code_challenge = base64url(SHA-256(verifier))`
   `[cmd/login/main.go:51-56]`。
2. `GET /oauth/authorize`，携带 `client_id`、`response_type=code`、`redirect_uri`、`scope`、
   `code_challenge`、`code_challenge_method=S256`、`state` `[cmd/login/main.go:144-154]`。缺少
   `response_type` 会产生
   `400 {"error":{"code":"unsupported_response_type","message":"Only authorization code flow is supported."}}`
   `[M1-anon-probe-matrix.txt:60, 112]`。
3. 浏览器登录/授权——`BLOCKED-CRED`。
4. 重定向至 `https://code.phanthy.com/oauth/code/success?code=<code>&state=<state>`——`INFERRED`
   （RFC 6749 §4.1.2）`[docs/02-usage-semantics.md §1.2]`。
5. `POST /oauth/token`（form-urlencoded），`grant_type=authorization_code`、`code`、`code_verifier`、
   `redirect_uri`、`client_id=phanthy-code-cli` `[cmd/login/main.go:177-193]`。
6. `200 {access_token, refresh_token, expires_in}`——仅为既有实现形态；真实响应体 `BLOCKED-CRED`
   `[cmd/login/main.go:171-175, 208-218]`。
7. 刷新：`POST /oauth/token`，`grant_type=refresh_token`、`refresh_token`、`client_id`
   `[internal/upstream/client.go:238-245]`。

客户端身份 `phanthy-code-cli` 来自既有实现 `[cmd/login/main.go:28, 146]`；既有实现还会在 OAuth 调用中
发送 `anthropic-beta: oauth-2025-04-20` `[cmd/login/main.go:193]` `[internal/upstream/client.go:139, 243]`
——其必需性为 INFERRED，从未验证 `[docs/02-usage-semantics.md §1.3]`。

### 6.2 Scope

服务端公布五个 scope `[M1-openid-configuration.json:20-26]`；官方 CLI 请求**六个**（§5A.4）
`[M6-cli-config.txt:74-99]`：

| scope | 服务端公布 | 既有实现请求 | 官方 CLI 请求 | 用途 |
|---|---|---|---|---|
| `user:profile` | 是 | 是 | 是 | 资料访问 |
| `user:inference` | 是 | 是 | 是 | `/v1/messages` |
| `user:sessions:claude_code` | 是 | 是 | 是 | Claude Code 会话语义 |
| `user:mcp_servers` | 是 | 否 | 是 | MCP server 访问 |
| `user:file_upload` | 是 | 否 | 是 | 文件上传 |
| `org:create_api_key` | **否** | 否 | **是** | API-key 创建；即 6-vs-5 差集，原因不明 |

唯一未被公布的 scope `org:create_api_key`，是客户端请求而服务端从未公布的唯一项。它究竟被静默接受、
被忽略，还是陈旧客户端元数据，属 `BLOCKED-CRED`——但这不改变"两条 `create_api_key` 路由均返回 HTML 404
外壳"这一事实（§6.4）。

### 6.3 Profile 守卫、JWKS 与 token 格式

`GET /api/oauth/profile` 与 `GET /api/oauth/claude_cli/roles` 共用同一守卫，401 响应体字节完全相同、均为
79 B `[M1-anon-probe-matrix.txt:66, 69]`：

```
WWW-Authenticate: Bearer realm="phanthy-code", error="invalid_request",
                  error_description="Bearer access token is required."
{"error":{"code":"missing_login","message":"Bearer access token is required."}}
```

JWKS 仅含一个 RSA 密钥：`kty=RSA`、`e=AQAB`、`use=sig`、`alg=RS256`、`kid=configured-key`
`[M1-jwks.json:22, 29-32]`。密钥材料暗示 access token 可能是签名 JWT，但 token 格式、claims、`exp` 与
轮换均为 `BLOCKED-CRED`——`kid=configured-key` 属默认风格标识，且未展示密钥轮换
`[M1-index.md:126-128]`。

### 6.4 缺失的 API-key 铸造路由与 Bearer 回退

`/api/oauth/phanthy_cli/create_api_key` 与 `/api/oauth/claude_cli/create_api_key` 均返回 Next.js HTML 404
外壳 `[M1-anon-probe-matrix.txt:67-68]` `[M1-options-matrix.txt:20-21]`。既有实现把 `phanthy_cli` 路由的
404 视为"无可用 API key"而非硬错误 `[cmd/login/main.go:97, 221-251]`，随后以
`Authorization: Bearer <access_token>` **代替** `x-api-key` 调用 `POST /v1/messages`
`[internal/upstream/client.go:344-350]` `[cmd/login/main.go:103-104]`。若将来返回了密钥，既有实现期望它在
`data.raw_key`（回退 `data.api_key`、`api_key`）`[cmd/login/main.go:256-269]`。

**这是意图陈述，不是线上事实：** 在本目标上，access token 是 `/v1/messages` 的有文档凭据。`/v1/*` 是否
真的接受 Bearer 属 `BLOCKED-CRED`，故 OpenAPI 将两种方案并列建模，不断言其一。

---

## 7. 推理协议（`POST /v1/messages`）

### 7.1 请求契约（PRIOR-ART）

存在性已验证——`OPTIONS` 返回 `204` 且 `Allow: OPTIONS, POST`，GET/HEAD 返回 `405`
`[M1-options-matrix.txt:23]` `[M1-anon-probe-matrix.txt:70-71]`。POST 本身从未发出。

| 字段 | 必需 | 类型 / 行为 | 锚点 |
|---|---|---|---|
| `model` | 是 | 客户端别名或上游 ID；未知名称原样透传 | `internal/upstream/payload.go:75-97` |
| `max_tokens` | 实际必需 | 始终序列化；代理将 1..65536 之外的值钳制为 8192 | `internal/upstream/payload.go:47-60, 117-119` |
| `messages` | 是 | `{role, content}` 数组；`content` 可为字符串或块数组 | `internal/upstream/payload.go:47-60` |
| `system` | 否 | 字符串；既有实现会合并 system/developer 消息 | `internal/upstream/payload.go:141-147` |
| `temperature` / `top_p` | 否 | 数字，原样透传 | `internal/upstream/payload.go:121-126` |
| `stream` | 否 | 布尔；既有实现强制 `true` | `internal/upstream/payload.go:109` |
| `tools` | 否 | `{name, description, input_schema}` | `internal/upstream/payload.go:306-335` |
| `tool_choice` | 否 | `none` / `auto` / `any` / `{type:"tool",name}` | `internal/upstream/payload.go:337-380` |
| `stop_sequences` | 否 | 字符串数组 | `internal/upstream/payload.go:127-129` |
| `metadata` | 否 | 对象，既有实现设置 `{user_id: ...}` | `internal/upstream/payload.go:130-132` |

### 7.2 请求头（PRIOR-ART）

`Accept: application/json, text/event-stream`、`x-app: cli`、`User-Agent: phanthycode2api/1.0`，以及
`x-api-key: <api_key>` 或 `Authorization: Bearer <token>` 之一
`[internal/upstream/client.go:341-350]`。`X-Claude-Code-Session-Id` 属代理侧产物，非协议要求
`[internal/upstream/client.go:351-353]`。任何地方都未发送 `anthropic-version` 头，公开 JS 中也零命中
`[M1-js-static-analysis.txt:91-95]`——目标是否容忍其缺失为 INFERRED，未验证。

**M6 更正（`VERIFIED-STATIC`）。** 官方 CLI 的 provider fetch 在发送前**删除 `x-api-key`**，改用
`Authorization: Bearer <access token>` 鉴权；同时附加自己的七头族（`x-phanthy-client`、
`X-Phanthy-Client-Source`、`x-phanthy-cli-device`、`X-Phanthy-Product`、`x-phanthy-project`、
`x-phanthy-request`、`x-phanthy-session`）与 `User-Agent` `[M6-cli-config.txt:145-157]`（报告 §5A.3）。
因此 `x-api-key` 仅为来自既有实现的**未经证实可能性**，绝非推荐方案；服务端是否接受它属 `BLOCKED-CRED`。

### 7.3 流式（PRIOR-ART）

既有解析器按行读取 SSE，仅处理 `data: ` 行，完全忽略 `event:` 名称，并以 JSON 的 `type` 字段为准
`[internal/upstream/sse.go:148-168]`。理解的序列为：

`message_start` → `content_block_start`（text 或 `tool_use`）→ `content_block_delta`
（`text_delta` / `thinking_delta` / `input_json_delta`）→ `message_delta` → `message_stop`
`[internal/upstream/sse.go:188-276]` `[docs/02-usage-semantics.md §3.2]`。

`content_block_stop` 未被处理；`error` 事件虽被解析入结构体却**没有处理分支**，故流中途错误会被静默丢弃
`[internal/upstream/sse.go:44-47, 188-276]`。终止由 EOF 驱动而非 `message_stop`，之后代理自行追加
`[DONE]` `[internal/upstream/sse.go:162-184]`。消费方防御：把"无 `content_block_delta` 且无
`stop_reason`"的流视为失败，并只把流建立前的 HTTP 状态作为唯一可靠错误信号
`[docs/02-usage-semantics.md §3.4]`。

OpenAPI 的 `text/event-stream` 响应带具名示例（`messageStart`、`contentBlockStart`、`contentBlockDelta`、
`messageDelta`、`messageStop`、`errorEvent`），但明确标注该线上契约为既有实现。

### 7.4 错误分类：观测 vs 既有实现

目标上仅观测到四个 JSON 错误信封：
`unsupported_response_type` `[M1-anon-probe-matrix.txt:112]`、`missing_login` `[:163, :190]`、
`invalid_platform` `[:245, :365]`——形态均为 `{"error":{"code","message"}}`。**从未观测到 `402`、`429` 或
`5xx`** `[docs/02-usage-semantics.md §4.4]`。既有实现的分类器（402 → 硬额度；硬标记；会话标记；401；
429；404；≥500；≥400）描述的是代理，不是目标 `[internal/upstream/client.go:76-107]`。故 OpenAPI 只记录
观测到的错误信封与各状态码，不编造登录态错误码。

---

## 8. 模型清单——两个独立层，不可合并

模型 ID 仍无法从 HTTP 获得。`GET /v1/models` 返回 HTML 404 外壳
`[M1-anon-probe-matrix.txt:73]`；分析的 12 个公开 JS chunk 对所有探测过的家族**零命中**
`[M1-js-static-analysis.txt:97-123]`；新增的机器可读目录端点 `GET /api/phanthy_code/models` 与
`GET /api/phanthy_code/models/stats` 受凭据守卫，匿名仅返回 `401 missing_login`
`[M6-new-endpoint-probes.txt:28-193]`。因此记录中存在**两个不可合并的清单**：

- **A 层——`PUBLIC-MARKETING`：** 公开定价页宣传的 13 个展示别名。这是运营方*售卖*的东西，不是 API
  *接受*的东西 `[M6-public-model-catalog.txt:27-65]`。
- **B 层——`PRIOR-ART`：** 仓库内 Go 代理使用的别名 → 上游 ID 映射。这是*本仓库客户端*发送的东西
  `[internal/upstream/payload.go:75-89]` `[README.md:120-134]`。

**A 层与 B 层之间的映射并未建立。** 没有任何证据把定价页别名与既有实现的上游 ID 关联起来，也未观测到
登录态目录或推理响应。字符串相似不具证明力：A 层的 `kimi-k2.7-code` 与 B 层的 `Kimi-k2.7-code` 大小写
不同，且 A 层条目是带积分倍率的展示别名，而 B 层是映射到 `Apollo-1.1` 的客户端名。二者按"表面巧合"处理，
不视为已确认的关联 `[M6-public-model-catalog.txt:63-65]` `[docs/02-usage-semantics.md §6.3]`。

### 8.1 A 层——公开模型目录（`PUBLIC-MARKETING`）

来源：`https://code.phanthy.com/token-plan`，匿名 `GET 200 text/html`，49,225 B，sha256
`2abf943b…0be8a` `[M6-public-model-catalog.txt:4-8]`。展示名与倍率字符串按精确大小写与标点复录；
`N.NNx积分` 是**积分消耗倍率**，不是按 token 计价 `[M6-public-model-catalog.txt:104-106]`。

| # | 展示名 | 积分倍率（逐字） | 场景 |
|---|---|---|---|
| 1 | `Phanthy-fast` | `0.00x积分` | 快速高效 · 限时免费 |
| 2 | `Phanthy-pro` | `0.33x积分` | 全球前沿 · 强大通用 |
| 3 | `Phanthy-ultra` | `0.67x积分` | 全球顶尖智能 · 极致质量 |
| 4 | `glm-5.3-flash` | `0.04x积分` | 前沿智能 · 极致性价比 |
| 5 | `glm-5.3` | `0.38x积分` | 智谱旗舰 · 综合能力强 |
| 6 | `glm-5.2` | `0.38x积分` | 智谱上代旗舰 · 长程稳定 |
| 7 | `kimi-k3` | `1.00x积分` | Kimi 旗舰 · 擅长长上下文 |
| 8 | `kimi-k2.7-code` | `0.31x积分` | 编程专用 · 为 Coding Agent 优化 |
| 9 | `deepseek-v4.1-flash-1017` | `0.10x积分` | deepseek最新模型 · 高效推理 |
| 10 | `Phana-5.5o` | `0.67x积分` | 占位文案 `Available model` |
| 11 | `Phana-5.5s` | `0.33x积分` | 占位文案 `Available model` |
| 12 | `Phano-6a` | `1.67x积分` | 占位文案 `Available model` |
| 13 | `Phano-6.1s` | `0.33x积分` | 占位文案 `Available model` |

四行场景为字面占位 `Available model`；这是页面自身文本，不是脱敏
`[M6-public-model-catalog.txt:38-43]`。页面还公布四档套餐（体验版 ¥0/月 2,000 积分；标准版 ¥9.9/月
15,000；高级版 ¥139/月 30,000+2,000；旗舰版 ¥699/月 150,000+20,000）以及全站提示
"每 1,000 积分约等于 1 美元价值的 Token 额度"，属营销等价换算，不是价目表
`[M6-public-model-catalog.txt:68-88]`。

该页**明确缺失**的字段：**模型 id / API slug、上下文窗口、按 token 的输入/输出价格、限流、端点/协议
映射、模型→上游映射** `[M6-public-model-catalog.txt:91-102]`。`data-series`/logo 资源暗示厂商家族
（phana* → Anthropic logo，phano* → OpenAI logo），但页面未声明映射，logo 仅为呈现
`[M6-public-model-catalog.txt:45-65]`。

### 8.2 B 层——仓库内别名映射（`PRIOR-ART`）

| # | 客户端名 | 上游模型 ID | 家族 | 等级 |
|---|---|---|---|---|
| 1 | `DeepSeek-V4` | `Iris-1.0` | Iris | PRIOR-ART / 线上未验证 |
| 2 | `gpt-5.6-sol` | `Zeus-1.1-pro` | Zeus | PRIOR-ART / 线上未验证 |
| 3 | `gpt-5.6-terra` | `Zeus-1.1` | Zeus | PRIOR-ART / 线上未验证 |
| 4 | `gpt-5.6-luna` | `Zeus-1.1-fast` | Zeus | PRIOR-ART / 线上未验证 |
| 5 | `gpt-5.5` | `Zeus-1.0-pro` | Zeus | PRIOR-ART / 线上未验证 |
| 6 | `Claude Opus 4.8` | `Gaia-1.2` | Gaia | PRIOR-ART / 线上未验证 |
| 7 | `Claude Opus 4.7` | `Gaia-1.1` | Gaia | PRIOR-ART / 线上未验证 |
| 8 | `Claude Sonnet 4.6` | `Gaia-1.0` | Gaia | PRIOR-ART / 线上未验证 |
| 9 | `Kimi K3` | `Apollo-2.0` | Apollo | PRIOR-ART / 线上未验证 |
| 10 | `Kimi-k2.7-code` | `Apollo-1.1` | Apollo | PRIOR-ART / 线上未验证 |
| 11 | `Kimi K2.6` | `Apollo-1.0` | Apollo | PRIOR-ART / 线上未验证 |
| 12 | `GLM 5.2` | `Metis-1.1` | Metis | PRIOR-ART / 线上未验证 |
| 13 | `GLM-5.1` | `Metis-1.0` | Metis | PRIOR-ART / 线上未验证 |

能力说明同样未经证实：200,000 token 上下文数字来自代理自身的静态清单
`[internal/server/handler.go:93-108]`；工具调用由无条件的工具转换与解析推断；视觉**不**受代理支持（图像被
替换为字面文本 `"[image]"`）`[internal/upstream/payload.go:294-298]` `[docs/02-usage-semantics.md §6.4]`。
OpenAPI 的 model 字段是自由字符串，别名表写在其 description 中，未用 `enum`。

### 8.3 可与不可主张的事项

| 主张 | 状态 |
|---|---|
| 13 个展示别名及其积分倍率已公开宣传 | `PUBLIC-MARKETING`（A 层，页面哈希已固化） |
| 代理客户端对上述 13 个客户端名发送 `Iris-1.0` … `Metis-1.0` | `PRIOR-ART`（B 层，仓库源码） |
| API 接受任一具体别名作为线上 `model` 值 | `BLOCKED-CRED`——未观测到目录或推理响应 |
| A 层别名 ↔ B 层上游 ID | **未建立**——无文档或观测关联 |
| 任一别名的上下文窗口 / 按 token 价格 | `未公布`（A 层无此列；B 层 200k 为代理静态值） |

---

## 9. 安全观察与风险评级

所有发现均为**非利用性观察**；除一次无害匿名请求外未做任何实际操作。风险评级是相对目标自身部署而言，
不构成测试邀请。

| # | 观察 | 评级 | 理由 / 锚点 |
|---|---|---|---|
| S1 | 仅发送 HSTS；无 `Content-Security-Policy`、`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy`；`x-powered-by: Next.js` 泄漏技术栈 | **中** | 缺少纵深防御响应头会放大未来内容注入缺陷的影响；技术栈泄漏降低侦察成本。所有响应均如此 `[M1-root-headers.txt:7-20]` `[M1-index.md:185-189]`。 |
| S2 | API 路由的 `405` **不带 `Allow` 头**，不符合 RFC 9110 | **低** | 迫使客户端用 OPTIONS 探测可用方法；无直接安全影响 `[M1-anon-probe-matrix.txt:192-198, 447-448]`。 |
| S3 | 不存在的 API 路径返回约 26.5 KB 的 HTML 404 外壳而非 JSON | **低** | 降低客户端路由枚举精度，但未对 `robots.txt` 隐藏面；也导致无法建立机器可读 404 分类 `[M1-anon-probe-matrix.txt:67-79]`。 |
| S4 | `robots.txt` 显式枚举希望被跳过的私有面（`/api`、`/oauth`、`/v1`、`/admin`、`/account`、`/login`、`/payment`、`/app`、`/docs`、密码流程） | **低** | 等于公布攻击者待办清单，尽管这些路径本就可发现；属有意的 SEO 取舍 `[M1-anon-probe-matrix.txt:369-401]`。 |
| S5 | `sitemap.xml` 列出 841 个唯一内容 URL，且不含 API/OAuth/admin 位置 | **低（正向）** | sitemap 无敏感路径泄漏；已验证 `/api/`、`/oauth/`、`/v1/`、`/admin` 出现次数为 `0` `[M1-anon-probe-matrix.txt:404-438]`。 |
| S6 | `GET /api/download/client?platform=linux` 在匿名 GET 上铸造两个访客 cookie（`phanthy_vid` 90 天；`phanthy_vsid` 30 分钟；`Secure; HttpOnly; SameSite=lax`） | **低** | 读形态端点带会话副作用；cookie 有作用域与安全标志，但 GET 上的状态变更会复杂化缓存/CSRF 推理 `[M1-anon-probe-matrix.txt:346-354]`。 |
| S7 | JWKS 的 `kid` 为字面 `configured-key`，且仅一把 RSA 密钥 | **低** | 暗示默认部署标识；未展示密钥轮换，属运维卫生问题而非破绽 `[M1-jwks.json:22]`。 |
| S8 | 未登录 `401` 泄漏 `realm="phanthy-code"` 与 `error_description` | **低** | 常规且最小化；便于客户端，仅泄漏产品名 `[M1-anon-probe-matrix.txt:157]`。 |
| S9 | `POST /api/web/events` 存在但契约完全未知，且由公开站点外壳调用 | **中** | 输入面未知且其鉴权未验证（即可匿名到达）；存在四种互斥假设且无响应捕获 `[M1-options-matrix.txt:26]` `[M1-js-static-analysis.txt:63]`。 |
| S10 | 对任意第三方 `Origin` 无 CORS 响应头；`vary` 从不增加 `origin` | **低（正向）** | 跨源浏览器读取被阻断；是否存在白名单未确定。限制了浏览器集成，但阻止随意的跨源滥用 `[M1-anon-probe-matrix.txt:291-335]`。 |
| S11 | API-key 铸造路由缺失，而 `claude_cli/roles` 仍保留 bearer 守卫 | **低** | 面不一致；守卫仍然有效 `[M1-anon-probe-matrix.txt:67-69]`。 |
| S12 | 限流/配额响应形态未知（从未观测到 `RateLimit-*` 头） | **中** | 无法依据证据规定客户端退避；SDD 中任何冷却数字都只是既有实现策略 `[M1-index.md:210-212]` `[docs/02-usage-semantics.md §5.3]`。 |
| S13 | 官方 CLI 发送 `x-phanthy-cli-device: base64url(JSON)`，其 JSON 是**主机指纹**：hostname、用户名、平台、架构、OS 版本、最多 32 个 IPv4 候选、网络接口与默认路由——即每个请求都携带稳定的单机客户端标识 | **低–中** | 仅为隐私观察：这是合法的设备归因头，不是漏洞，且未做任何操作。它对用户不透明、可跨会话关联请求、且无公开文档。仅报告，未探测 `[M6-cli-config.txt:195-201]` `[docs/01-endpoint-catalog.md §5.7]`。 |
| S14 | 官方 CLI 引用一个**未文档化的遥测端点** `POST /api/event_logging/batch`（仅静态发现；从未调用） | **中** | 该客户端遥测汇聚点既不在 `robots.txt` 也不在任何公开文档中；载荷形态、鉴权与留存期未知。因探测遥测摄取路径超出匿名侦察范围，故**有意未探测**。风险反映的是未知数据处理，而非已证实的暴露 `[M6-cli-config.txt:145-157]` `[docs/01-endpoint-catalog.md §7]`。 |

本节无一项是已确认漏洞。S1/S9/S12/S14 评为中，因其涉及加固缺失、未知契约或未知数据处理；S13 因隐私观察
评为低–中；其余为低。此处没有任何内容足以支持利用，也未尝试利用。

---

## 10. 合规与伦理声明

本次工作为**只读**且**无凭据**。未绕过鉴权、未伪造 token、未猜解口令或 API key、未压测限流。匿名探测
遵守 ≤1 req/s 与单端点 ≤20 次 `[docs/00-governance.md:20]`；记录的采集使用 ≥1.1 s 间隔且任一路径最多 5 次
`[M1-anon-probe-matrix.txt:8, 13]`。未向磁盘写入任何 L2 值：cookie 值在采集时即脱敏
`[M1-anon-probe-matrix.txt:346-347]`，本报告与 OpenAPI 包仅含 `<api_key>` / `<token>` 占位符。未访问或
导出任何真实用户数据。UNTRUSTED 仓库仅作纯文本阅读，从未 clone、安装或执行
`[docs/00-governance.md:22-32]`。登录态行为未被猜测或模拟：全文一律标注 `PRIOR-ART` 或 `BLOCKED-CRED`。
所有发现均以非利用方式报告，且限于匿名观察者可及范围。

M6 增量保持同一纪律。官方 CLI 包 `@phanthy/phanthy-codeo@0.2.11` 取自公开 npm registry，**仅静态**检视
——元数据、tarball 哈希、字节切片字符串。从未安装、从未导入、包内二进制从未执行；未运行安装脚本，包内
代码未发起任何网络调用 `[M6-index.md §1]`。新端点探测为 9 次匿名 `OPTIONS`/`GET`/`HEAD` 请求，打向两条
公开路径，≤1 req/s，未使用凭据；仅静态发现的遥测路径被**有意未请求**。公开定价页仅匿名抓取一次，且只
使用其渲染后的表格文本。

---

## 11. 可复现性

### 11.1 环境与出网

沙箱内直连 origin 的 TCP 被阻断；所有请求必须经 M1 记录的 HTTP 代理
（`HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:18080`，HTTP/2 到 origin）`[M1-anon-probe-matrix.txt:7]`。
使用工具：curl 8.5.0、Python 3.14.8 `[M1-anon-probe-matrix.txt:9-12]`。不要把开头的代理
`HTTP/1.1 200 OK` 读作 origin 状态 `[M1-anon-probe-matrix.txt:39-44]`。

### 11.2 侦察命令（匿名、只读）

```bash
# app shell + infrastructure headers
curl -sS --http2 -D - -o /dev/null https://code.phanthy.com/

# method oracle (do not use 405 Allow headers on API routes)
curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/v1/messages

# OAuth/OIDC metadata
curl -sS --http2 https://code.phanthy.com/.well-known/openid-configuration
curl -sS --http2 https://code.phanthy.com/.well-known/jwks.json

# observed 401 guard
curl -sS --http2 -D - https://code.phanthy.com/api/oauth/profile

# download branch: 302 (linux) vs 400 (darwin)
curl -sS --http2 -i 'https://code.phanthy.com/api/download/client?platform=linux'
curl -sS --http2 -i 'https://code.phanthy.com/api/download/client?platform=darwin'

# static metadata
curl -sS --http2 https://code.phanthy.com/robots.txt
curl -sS --http2 https://code.phanthy.com/sitemap.xml | sha256sum
```

预期摘要：`robots.txt` sha256 `e9df3e77…bda64134`；`sitemap.xml` sha256 `1c1f5014…68b812`
`[M1-anon-probe-matrix.txt:371, 407]`。

M6 追加（匿名、两条新路径、≤1 req/s——省略登录态分支）：

```bash
# existence / method oracle on the new catalogue endpoints
curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/api/phanthy_code/models
curl -sS --http2 -X OPTIONS -D - -o /dev/null https://code.phanthy.com/api/phanthy_code/models/stats

# anonymous guard shape (expect 401 JSON, 79 B, WWW-Authenticate: Bearer realm="phanthy-code")
curl -sS --http2 -D - -o /dev/null https://code.phanthy.com/api/phanthy_code/models

# public marketing catalogue (table text only; no JS execution)
curl -sS --http2 -o /tmp/token-plan.html 'https://code.phanthy.com/token-plan'
sha256sum /tmp/token-plan.html   # expect 2abf943b…cb40be8a
```

预期摘要：`token-plan` sha256 `2abf943b6cab2090ed737618f76b03473319dc9e1704a90840f8c061cb40be8a`
`[M6-public-model-catalog.txt:4-8]`。M6 的 CLI 分析可无需安装、直接从公开 npm registry 复现：取
`https://registry.npmjs.org/@phanthy/phanthy-codeo` 元数据，下载 `0.2.11` tarball（应得 sha256
`3134d33d…2244d80c`），用 `tar -xzf` 解包，并以纯字节方式切片 `dist/cli.js` / `dist/tui-worker.js`——
**绝不**运行 `npm install`、`npx` 或任何解包出的二进制 `[M6-index.md §6]`。

### 11.3 OpenAPI 校验命令

```bash
python3 -c "import json; json.load(open('/workspace/re/apifox/openapi.json')); print('json ok')"
python3 /workspace/re/apifox/push_apifox.py --self-test
```

记录结果（M6b，`2026-10-26`）：`json ok 3.1.0 17`；self-test 报告 `openapi: 3.1.0`、`paths: 17`、
`operations: 17`、`schemas: 13`、`local $refs: 44`，tags 为
`Account, Assets, Inference, Meta, OAuth, OIDC, Web`，**17 个唯一 operationId**（M4 的 15 个加
`phanthyModelsGet`、`modelStatsGet`），以及 `SELF-TEST OK`（exit 0）。`components.parameters` 另有 **7**
个可复用 Phanthy 客户端头，均为可选 `$ref`。

### 11.4 证据索引

| 主题 | 证据 |
|---|---|
| 目标技术栈与响应头 | `M1-root-headers.txt:7-20`；`M1-anon-probe-matrix.txt:87-101` |
| 方法矩阵 | `M1-options-matrix.txt:13-32, 37-404` |
| 匿名状态码 | `M1-anon-probe-matrix.txt:55-81` |
| OIDC 元数据 | `M1-openid-configuration.json:2-26`；`M1-anon-probe-matrix.txt:450-469` |
| JWKS | `M1-jwks.json:5-35` |
| JS 分析 | `M1-js-static-analysis.txt:56-124`；`M1-js-chunks.tsv:2-13` |
| 下载分支 | `M1-anon-probe-matrix.txt:337-367` |
| robots/sitemap | `M1-anon-probe-matrix.txt:369-438` |
| 汇总索引（M1） | `M1-index.md:49-229` |
| 新目录端点——存在性、`allow`、401 形态 | `M6-new-endpoint-probes.txt:25-193` |
| 官方 CLI 静态提取（头、scope、端点、凭据、遥测） | `M6-cli-config.txt:31-99, 145-157` |
| npm 包元数据、哈希、未执行声明 | `M6-npm-package.json:1-120` |
| 公开 13 别名目录、倍率、套餐、缺失字段 | `M6-public-model-catalog.txt:4-130` |
| 汇总索引（M6）与相对 M1 的增量 | `M6-index.md`（整文件；按章节而非行范围阅读） |

---

## 12. 交付物索引与后续步骤

### 12.1 交付物

| 文件 | 用途 | 已验证状态 |
|---|---|---|
| `/workspace/re/apifox/openapi.json` | OpenAPI 3.1.0 包，17 个 operation、7 个 tag、13 个 schema、7 个可复用请求头参数、基于 `$ref` 的错误信封、`/v1/messages` 的两种安全方案 | JSON 可解析；版本 `3.1.0`；17 paths；17 个唯一 operationId；13 schemas；44 个本地 `$ref` 全部解析；无 `eyJ` 类密钥 |
| `/workspace/re/apifox/IMPORT.md` | Apifox 导入路径（UI 与脚本）、环境变量、鉴权配置、创建的目录、排除项、幂等重导、故障排查 | 完整；真实推送 UNVERIFIED（无 token） |
| `/workspace/re/apifox/push_apifox.py` | 仅用 Python 3 标准库的校验器与可选 `POST /v1/projects/{id}/import-openapi` 客户端；默认 dry-run | `--self-test` 退出 0 |
| `/workspace/re/report/reverse-engineering-report.md` | 本报告 | 13 节 |

17 个 operationId 为：`appShellGet`、`oauthAuthorize`、`oauthToken`、`oauthRevoke`、
`oauthCodeSuccessGet`、`openidConfigurationGet`、`jwksGet`、`profileGet`、`claudeCliRolesGet`、
`messagesCreate`、`webEventsCreate`、`clientDownloadGet`、`publicAssetsGet`、`robotsGet`、`sitemapGet`、
`phanthyModelsGet`、`modelStatsGet`。

### 12.2 仍未验证项

| 项目 | 状态 | 原因 |
|---|---|---|
| Apifox 真实推送（`import-openapi`） | UNVERIFIED | 环境中无 `APIFOX_ACCESS_TOKEN` `[M0-preflight.json:18]`；端点/请求形态已对 Apifox 公开文档核对，但未发起调用 |
| `/v1/messages` 鉴权方案（`x-api-key` vs Bearer） | BLOCKED-CRED（已部分收窄） | M6 表明**官方 CLI** 仅发 Bearer 并删除 `x-api-key`；*服务端*是否也接受 `x-api-key` 仍未被观测 `[docs/01-endpoint-catalog.md §5.5]` |
| `/v1/messages` 登录态 JSON 成功体与登录态错误信封 | BLOCKED-CRED | 从未观测；OpenAPI 将 JSON schema 标为既有实现且无示例 `[docs/01-endpoint-catalog.md §3.10]` |
| `GET /api/phanthy_code/models` 与 `.../models/stats` 的成功体 | BLOCKED-CRED | 匿名均返回 `401 missing_login`；成功 schema 仅作宽松 object 建模，不得读作已观测 `[M6-new-endpoint-probes.txt:28-193]` |
| A 层展示别名 → B 层上游 ID / 线上 `model` 值的映射 | 未建立 | 无登录态目录、无推理响应；字符串相似不具证明力（报告 §8） |
| 目标的精确 SSE 帧序 | PRIOR-ART | 来自代理解析器与其 fixture，而非实时流 `[docs/02-usage-semantics.md §3.2, §7.3 C4]` |
| `/api/oauth/profile` 成功 schema | BLOCKED-CRED | 既有实现返回无类型 map，且无调用点 `[docs/01-endpoint-catalog.md §3.8]` |
| `/api/oauth/claude_cli/roles` 语义/schema | BLOCKED-CRED | 仅有 401 守卫；用途由命名推断 `[docs/01-endpoint-catalog.md §3.9]` |
| `/api/web/events` 契约（请求体、media type、鉴权） | 仅 INFERRED | 四种互斥假设；无响应捕获 `[docs/01-endpoint-catalog.md §3.11]` |
| `POST /api/event_logging/batch`（遥测） | 仅 VERIFIED-STATIC | 官方 CLI bundle 内存在该字符串；载荷、鉴权与留存未知；**未探测** `[M6-cli-config.txt:145-157]` |
| `dev.code.phanthy.com`（构建标记主机） | 仅 VERIFIED-STATIC | 在 CLI 中作为编译期主机分支出现；未解析、未请求 `[M6-cli-config.txt:31-99]` |
| `/api/public-assets` 目标响应体/语义 | 未探测 | 有意未跟随 308 目标 `[M1-anon-probe-matrix.txt:247-255]` |
| `platform` 取值集合（`linux` 之外） | 仅 VERIFIED 否定 | `darwin` 被拒；未尝试更多取值 `[M1-anon-probe-matrix.txt:357-367]` |
| 模型清单的线上可用性 | PUBLIC-MARKETING / PRIOR-ART | `/v1/models` 404；两个目录端点 401；13 个宣传别名属营销，不是 API 契约（报告 §8） |
| Token TTL、JWT claims、refresh 轮换、密钥轮换 | BLOCKED-CRED | 未观测 token；`kid=configured-key` `[M1-jwks.json:22]` |
| 真实 API-key 铸造路径 | BLOCKED-CRED | 两条已知路由 404 `[M1-anon-probe-matrix.txt:67-68]`；CLI 的 `org:create_api_key` scope 并不与之矛盾（报告 §6.2） |
| 配额/限流响应形态 | INFERRED | 从未观测 `402`/`429`/`RateLimit-*` `[M1-index.md:210-212]` |

### 12.3 后续步骤

1. **凭据门控验证（≤5 次调用，仅用用户自有 token）。** 执行 M2 计划 C1–C5：profile、roles、一次非流式
   `/v1/messages`、一次流式调用、一次非法模型错误探测 `[docs/02-usage-semantics.md §7.3 C1–C5]`。只记录
   状态码、content type、响应头名与脱敏后的响应体结构。这样可在不违反治理的前提下补齐鉴权方案与成功
   schema 空白。
2. **模型发现（同一凭据预算）。** M6 计划已把两个目录端点排除在 5 次调用之外
   `[docs/02-usage-semantics.md §7.1 U8]`。若将来有 token，单次 `GET /api/phanthy_code/models` 即可同时
   敲定成功 schema **与** §8 留存的别名→线上 ID 问题；如需纳入同一 5 次预算，必须重新规划。
3. **Apifox 推送。** 拿到 `APIFOX_ACCESS_TOKEN` 与 `APIFOX_PROJECT_ID` 后，运行
   `python3 push_apifox.py --project-id <id> --push` 并记录返回的 `data.counters`。在此之前自动路径仍为
   UNVERIFIED `[apifox/IMPORT.md §0]`。
4. **`/api/web/events`。** 或在治理上限内取得契约观测，或永久标注"契约未验证"；不要基于四种假设中的
   任何一种实现客户端。
5. **`/api/public-assets`。** 对 308 目标做一次匿名 GET 即可确定资源面是否公开；M1 中有意未纳入范围。
6. **遥测与设备头。** `POST /api/event_logging/batch` 与 `x-phanthy-cli-device` 指纹仅作为已报告观察保留。
   不要探测遥测汇聚点；不要在本仓库客户端中实现该指纹头。
7. **模型清单。** 把 §8 的 A 层视为营销元数据、B 层视为既有实现；只有登录态目录或 `/v1/messages` 响应
   才能把二者关联。
8. **报告卫生。** 任何下游摘要都必须沿用更正后的需求统计（1 个完全覆盖、6 个部分、10 个 `NO-TEST`、
   7/17 = 41.2%；17 个硬标记）`[acceptance/M3.md:179-192]`。

---

*报告结束。对 `code.phanthy.com` 的仅有实时请求来自 M1 与 M6（全部匿名）；**M4 零请求，M6b 对任何对象零
请求**。npm 包 `@phanthy/phanthy-codeo@0.2.11` 仅静态阅读，从未安装或执行。未读取、复制或写入任何凭据、
cookie 值或 token。所有登录态主张仍为 `PRIOR-ART` 或 `BLOCKED-CRED`。*
