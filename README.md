# phanthycode2api

> 将 PhanthyCode CLI 的 Anthropic Messages API 转换为 OpenAI 兼容 API，提供多账号池管理、冷却与错误处理。

## 功能特性

- **OpenAI 兼容 API** — 无痛接入任何支持 OpenAI 格式的客户端（NextChat、ChatBox、OpenCat 等）
- **多账号池管理** — 自动轮转账号、错误计数阈值冷却、禁用、持久化状态
- **协议转换** — OpenAI 请求/响应 ↔ Anthropic Messages API 双向转换，流式 SSE 实时透传
- **OAuth 登录** — 半自动 PKCE 授权码流程，一键获取凭证
- **定时 keepalive** — 保持 token 活跃，接近过期时自动刷新
- **模型映射** — 客户端使用商业名（DeepSeek-V4、gpt-5.6-sol 等），自动映射到上游真实模型

## 架构

```
phanthycode2api/
├── cmd/
│   ├── server/          # 服务入口：配置 → auth → pool → upstream → scheduler → server
│   │   ├── main.go
│   │   └── config.go
│   └── login/           # 半自动 OAuth 登录工具
│       └── main.go
├── internal/
│   ├── auth/            # 账号解析（三种形态：CLI 原生 / 嵌套 / 扁平）+ 原子写入
│   ├── pool/            # 账号池状态机（冷却、禁用、错误计数阈值）
│   ├── upstream/        # 核心：OpenAI ↔ Anthropic 协议转换 + SSE 流式转换
│   ├── server/          # OpenAI 兼容 HTTP 服务器，带轮转与错误分类
│   └── scheduler/       # 定时 keepalive 任务
├── config.example.json  # 配置模板
├── .gitignore
└── go.mod
```

## 快速开始

### 1. 配置

```bash
cp config.example.json config.json
# 编辑 config.json 按需调整
```

### 2. 登录获取凭证

```bash
go run ./cmd/login
```

浏览器打开授权链接 → 登录 PhanthyCode 账号 → 授权后粘贴 code 到终端，凭证自动保存到 `auths/` 目录。

### 3. 启动服务

```bash
go run ./cmd/server
```

默认监听 `:7864`，可通过 `P2A_LISTEN=:8080` 环境变量覆盖。

### 4. 使用

```bash
curl http://localhost:7864/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"DeepSeek-V4","messages":[{"role":"user","content":"你好"}]}'
```

## 可用模型

| 客户端名 | 上游模型 |
|---|---|
| DeepSeek-V4 | Iris-1.0 |
| gpt-5.6-sol | Zeus-1.1-pro |
| gpt-5.6-terra | Zeus-1.1 |
| gpt-5.6-luna | Zeus-1.1-fast |
| gpt-5.5 | Zeus-1.0-pro |
| Claude Opus 4.8 | Gaia-1.2 |
| Claude Opus 4.7 | Gaia-1.1 |
| Claude Sonnet 4.6 | Gaia-1.0 |
| Kimi K3 | Apollo-2.0 |
| Kimi-k2.7-code | Apollo-1.1 |
| Kimi K2.6 | Apollo-1.0 |
| GLM 5.2 | Metis-1.1 |
| GLM-5.1 | Metis-1.0 |

## API 端点

| 端点 | 方法 | 说明 |
|---|---|---|
| `/v1/chat/completions` | POST | OpenAI 兼容聊天补全（流式 + 非流式） |
| `/v1/models` | GET | 列出可用模型 |
| `/status` | GET | 账号池状态概览 |
| `/healthz` | GET | 健康检查 |

## 配置

全部配置项及环境变量覆盖（前缀 `P2A_`）：

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `listen` | `P2A_LISTEN` | `:7864` | 监听地址 |
| `api_key` | `P2A_API_KEY` | `""` | 服务端 API 密钥（空则无鉴权） |
| `auth_dir` | `P2A_AUTH_DIR` | `./auths` | 账号凭证目录 |
| `state_file` | `P2A_STATE_FILE` | `./data/state.json` | 账号状态持久化路径 |
| `base_url` | `P2A_BASE_URL` | `https://code.phanthy.com` | 上游 API 地址 |
| `cooldown.hard_credit` | — | `12h` | 积分不足冷却时长 |
| `cooldown.soft_rate` | — | `60s` | 限流冷却时长 |
| `cooldown.err_threshold` | — | `3` | 连续错误阈值 |
| `cooldown.err_cooldown` | — | `10m` | 错误冷却时长 |
| `schedule.keepalive_hours` | — | `[22]` | 定时 keepalive 小时 |
| `upstream.timeout_seconds` | — | `120` | 上游请求超时 |

## 技术栈

- **语言**: Go 1.26+
- **依赖**: 无第三方依赖（纯标准库）

## 许可证

[MIT](LICENSE)