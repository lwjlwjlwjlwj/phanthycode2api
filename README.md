# phanthycode2api

> 把 PhanthyCode 的 Anthropic Messages API 转换成 OpenAI 兼容 API 的 Go 网关。

本项目**完全依据 `re/` 下的逆向证据重写**，不参考任何第三方同名实现：

- `re/evidence/M6-cli-config.txt` — 官方 CLI `@phanthy/phanthy-codeo@0.2.11` 的静态提取
  （请求头族、设备指纹字段、6 个 scope、端点映射、`/v1/messages` 上的 Bearer 语义）
- `re/evidence/M6-new-endpoint-probes.txt` — `/api/phanthy_code/models`、`/models/stats` 的匿名探测
- `re/docs/02-usage-semantics.md` — 请求字段、SSE 事件序列、错误分类优先级
- `re/docs/05-sdd-spec.md` — 功能需求（FR-01…FR-17）与接口契约

未验证的字段（成功响应 schema 等）在代码中一律按“防御性解析”处理，并在注释里标注证据出处。

## 功能特性

- **OpenAI 兼容入口** — `/v1/chat/completions`（流式 / 非流式）、`/v1/models`、`/status`、`/healthz`
- **官方指纹对齐** — 按 M6 证据注入 7 个 `x-phanthy-*` 请求头（含 `base64url(JSON)` 设备指纹与每请求刷新 UUID）。
  本项目没有真实会话 / 项目概念，因此 `x-phanthy-session` 取账号 UID，`x-phanthy-project` 默认省略
  （证据中该头也是条件注入）。
- **Bearer 优先** — `/v1/messages` 优先 `Authorization: Bearer <access_token>`，无 access token 时退回 `x-api-key`
- **账号池调度** — LRU（最近未用优先）+ 余额感知 + 冷却 / 禁用 / 连续错误阈值，状态持久化
- **单收集器 SSE** — 流式与非流式共用同一个事件状态机；上游 `error` 事件不再被丢弃，空流不会被伪造成成功
- **OAuth 登录** — PKCE（S256）+ 官方 CLI 的 6 个 scope + 随机 `state` 校验
- **定时 keepalive** — 启动即执行一次，之后按配置小时刷新即将过期的 token；可选周期余额探测
- **协议层复用 new-api 实现** — 请求转换与 SSE 状态机由 [relaykit]（`new-api/relaykit`）
  承载，token 估算由 [tokenkit]（`new-api/tokenkit`）承载，不在协议层自研车轮

[relaykit]: https://github.com/QuantumNous/new-api/tree/main/relaykit
[tokenkit]: https://github.com/QuantumNous/new-api/tree/main/tokenkit

## 目录结构

```
cmd/
  server/main.go      # 入口：配置 → 凭证 → 账号池 → 上游客户端 → 网关 → keepalive
  login/main.go       # PKCE 半自动登录（-step=url / -step=exchange）
internal/
  config/             # JSON 配置 + P2A_* 环境变量覆盖 + 校验
  credential/         # 4 种磁盘凭证格式解析 + 原子写（tmp + rename）
  pool/               # 账号池：LRU、余额降级、冷却、禁用、状态持久化
  phanthy/            # 上游客户端、7 个指纹头、错误分类、余额提取
  protocol/           # 薄封装：模型展示列表 + max_tokens 改写 + SSE 帧读写，复用 relaykit/tokenkit
  gateway/            # 路由、鉴权、轮转、流式/非流式转发
  keepalive/          # token 保活与余额探测
```

## 快速开始

要求 Go ≥ 1.26.5，构建依赖 relaykit / tokenkit 及其传递依赖（详见 `go.mod`）。

```bash
go build -o phanthycode2api ./cmd/server
cp config.example.json config.json
# 编辑 config.json：设置 api_key（留空 = 不鉴权）
```

### 1. 登录获取凭证

```bash
go run ./cmd/login -step=url
```

在浏览器打开输出的授权链接，完成登录后从回调地址复制 `code` 与 `state`：

```bash
go run ./cmd/login -step=exchange -code=<code> -state=<state>
```

凭证以 `auths/phanthy-<uid>.json` 落盘（权限 0600）。也可以用 `./login.sh` 交互式完成两步。

### 2. 启动服务

```bash
./phanthycode2api -config config.json
# 或纯环境变量
P2A_LISTEN=:7864 P2A_API_KEY=your-secret ./phanthycode2api
```

### 3. 验证

```bash
curl -s http://localhost:7864/healthz

curl -s http://localhost:7864/v1/models -H "Authorization: Bearer your-secret"

curl -s http://localhost:7864/v1/chat/completions \
  -H "Authorization: Bearer your-secret" \
  -H "Content-Type: application/json" \
  -d '{"model":"Phanthy-pro","messages":[{"role":"user","content":"你好"}]}'

curl -N http://localhost:7864/v1/chat/completions \
  -H "Authorization: Bearer your-secret" \
  -H "Content-Type: application/json" \
  -d '{"model":"Phanthy-pro","stream":true,"messages":[{"role":"user","content":"数到3"}]}'
```

## 配置说明

```json
{
  "listen": ":7864",
  "api_key": "",
  "auth_dir": "./auths",
  "state_file": "./data/state.json",
  "base_url": "https://code.phanthy.com",
  "cooldown": {
    "hard_credit": "12h",
    "soft_rate": "60s",
    "err_threshold": 3,
    "err_cooldown": "10m"
  },
  "schedule": {
    "keepalive_hours": [22],
    "probe_interval": "0"
  },
  "fingerprint": {
    "enabled": true,
    "cli_version": "0.2.11",
    "client": "cli",
    "client_source": "cli",
    "product": "phanthy_code",
    "user_agent": "",
    "device_json": ""
  },
  "upstream": {
    "timeout_seconds": 120
  }
}
```

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `listen` | `P2A_LISTEN` | `:7864` | 监听地址 |
| `api_key` | `P2A_API_KEY` | `""` | 非空时对 `/v1/*`、`/status` 启用 Bearer 鉴权 |
| `auth_dir` | `P2A_AUTH_DIR` | `./auths` | 凭证目录 |
| `state_file` | `P2A_STATE_FILE` | `./data/state.json` | 账号池状态文件 |
| `base_url` | `P2A_BASE_URL` | `https://code.phanthy.com` | 上游地址 |
| `cooldown.hard_credit` | `P2A_HARD_CREDIT` | `12h` | 积分不足冷却 |
| `cooldown.soft_rate` | `P2A_SOFT_RATE` | `60s` | 限流 / 404 冷却 |
| `cooldown.err_threshold` | `P2A_ERR_THRESHOLD` | `3` | 连续错误阈值 |
| `cooldown.err_cooldown` | `P2A_ERR_COOLDOWN` | `10m` | 达到阈值后的冷却 |
| `schedule.keepalive_hours` | `P2A_KEEPALIVE_HOURS` | `[22]` | 每日 keepalive 小时，逗号分隔（如 `3,9,22`） |
| `schedule.probe_interval` | `P2A_PROBE_INTERVAL` | `0` | 余额探测周期，`0` 关闭 |
| `fingerprint.enabled` | `P2A_FP_ENABLED` | `true` | 是否注入官方指纹头 |
| `fingerprint.cli_version` | `P2A_FP_CLI_VERSION` | `0.2.11` | 指纹里的 CLI 版本 |
| `fingerprint.client` | `P2A_FP_CLIENT` | `cli` | `x-phanthy-client` |
| `fingerprint.client_source` | `P2A_FP_CLIENT_SOURCE` | `cli` | `X-Phanthy-Client-Source` |
| `fingerprint.product` | `P2A_FP_PRODUCT` | `phanthy_code` | `X-Phanthy-Product` |
| `fingerprint.user_agent` | `P2A_FP_USER_AGENT` | `phanthy-codeo/<cli_version>` | `User-Agent` |
| `fingerprint.device_json` | `P2A_FP_DEVICE_JSON` | `""` | 自定义设备指纹 JSON；留空则自动采集 |
| `upstream.timeout_seconds` | `P2A_TIMEOUT_SECONDS` | `120` | 上游超时 |

## 模型名处理

请求中的 `model` **原样透传**到上游，不做别名映射、不设 allow-list（证据
`02-usage-semantics.md §6.3`）。旧版携带的 13 条「客户端名 → 上游模型」映射
（Layer B，PRIOR-ART / 线上未验证）来自不可信的仓库代码，本重写已删除。

`GET /v1/models` 返回的 13 个展示名逐字取自公开定价页 `/token-plan` 的「可用模型列表」
（PUBLIC-MARKETING 等级，`re/evidence/M6-public-model-catalog.txt:27-43`）：

| # | 展示名 |
|---|---|
| 1 | `Phanthy-fast` |
| 2 | `Phanthy-pro` |
| 3 | `Phanthy-ultra` |
| 4 | `glm-5.3-flash` |
| 5 | `glm-5.3` |
| 6 | `glm-5.2` |
| 7 | `kimi-k3` |
| 8 | `kimi-k2.7-code` |
| 9 | `deepseek-v4.1-flash-1017` |
| 10 | `Phana-5.5o` |
| 11 | `Phana-5.5s` |
| 12 | `Phano-6a` |
| 13 | `Phano-6.1s` |

这些是营销展示名而非已验证的 wire 模型 ID，仅用于展示；客户端可按任意名字请求
（原样透传），网关不校验模型名。

## API

### `POST /v1/chat/completions`

OpenAI 兼容。支持 `stream`、`max_tokens` / `max_completion_tokens`、`temperature`、`top_p`、
`stop`、`tools`、`tool_choice`、`user`。图片内容块会被降级为字面量 `[image]`。

上游始终以 `stream: true` 调用：
- 非流式请求在本地聚合完整 SSE 后返回 `chat.completion`；
- 流式请求逐帧转换为 `chat.completion.chunk`，以唯一的 `data: [DONE]` 结束。

### `GET /v1/models`

返回 13 个展示模型名（`object: "list"`，取自公开定价页，见「模型名处理」）。

### `GET /status`

返回账号池快照（`accounts` 数组）：`uid`、`nickname`、`has_api_key`、`cooling`、`until`、
`reason`、`disabled`、`err_count`、`balance`、`last_used`。排序固定按 UID。

### `GET /healthz`

无鉴权健康检查，返回纯文本 `ok`。

## 鉴权与错误语义

配置 `api_key` 后，受保护端点必须携带完全匹配的 `Authorization: Bearer <api_key>`，否则返回
`401 {"error":{"code":"invalid_api_key",...}}`；`api_key` 为空时不做任何鉴权。

上游错误按固定优先级分类（`internal/phanthy/errors.go`）：

```
402 → 余额/配额文案 → 登录态文案 → 401 → 429 → 404 → 5xx → 其他 4xx
```

对应的账号池动作：

| 分类 | 动作 |
|---|---|
| 积分不足 | 冷却 `hard_credit`，余额标记为 0，换号重试 |
| 登录态失效 | 永久禁用，直到人工重新登录 |
| 限流 / 404 | 冷却 `soft_rate`，换号重试 |
| 5xx / 连接失败 | 累加错误计数，达到阈值后冷却 |
| 其他 4xx | 直接返回 400（请求本身有问题，不惩罚账号） |

单个请求最多尝试 3 个账号；全部不可用时返回
`503 {"error":{"code":"no_healthy_account",...}}`。

流式响应在写出第一个 chunk 之前不会提交状态码，因此首个事件失败仍可自动换号。

## 凭证格式

`auth_dir/*.json` 兼容 4 种布局，写出统一为嵌套格式：

1. CLI 原生：`{"type":"oauth_token","access_token":...,"expires_at":<秒>,"uid":...}`
2. 嵌套：`{"auth":{"accessToken":...,"expiresAt":<秒>},"account":{"uid":...}}`
3. 扁平：`{"accessToken":...,"refreshToken":...,"expiresAt":<秒或毫秒>}`
4. 官方 CLI：`{"claudeAiOauth":{"accessToken":...,"refreshToken":...,"expiresAt":<毫秒>}}`

`expiresAt` 自动识别秒 / 毫秒。凭证刷新与状态写入均使用 `临时文件 + rename`，崩溃不会留下截断 JSON。

注意：加载时会跳过以 `.` 开头的文件，官方 CLI 的 `~/.phanthy-code/.credentials.json` 需重命名为
`credentials.json` 或 `phanthy-<uid>.json` 后放入 `auth_dir`。

账号池的 LRU 顺序在进程内维护；`last_used` 会随冷却 / 禁用 / 错误 / 余额等状态变化一并落盘，
仅「成功使用」不会单独触发写盘。

## Docker 部署

```bash
# 先在宿主机生成凭证到 ./auths（见上文登录步骤）
docker compose up -d --build
```

`auths/` 与 `data/` 通过卷挂载持久化，均已加入 `.gitignore` / `.dockerignore`，切勿提交。

离线环境可先本地构建 `dist/phanthycode2api`，再使用 `Dockerfile.local`：

```bash
go build -o dist/phanthycode2api ./cmd/server
docker build -f Dockerfile.local -t phanthycode2api:local .
```

## 免责声明

本项目仅供学习和研究使用。请遵守 PhanthyCode 平台服务条款，控制请求频率（≤ 1 req/s），
自行承担使用风险。作者不对任何因使用本项目产生的直接或间接损失负责。

## License

MIT

本项目链接了 `new-api/relaykit` 与 `new-api/tokenkit`（均为 **AGPL-3.0**）。
AGPL-3.0 具有传染性：以网络服务方式对外提供的、链接了受 AGPL 保护的库的衍生作品，
整体可能需按 AGPL-3.0 开源并公开对应源代码。使用前请自行评估许可合规风险；
本项目的 MIT 声明不改变上方 AGPL 传染条款对整体作品的适用性。
