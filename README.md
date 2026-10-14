# phanthycode2api

> 将 PhanthyCode CLI 的 Anthropic Messages API 转换为 OpenAI 兼容 API，Go 实现。

多账号池管理 + 冷却与错误处理 + OpenAI ↔ Anthropic 协议转换，把 PhanthyCode 上游封装成
任意 OpenAI 兼容客户端（NextChat、ChatBox、OpenCat 等）可直接接入的标准接口。

## 功能特性

- 🔑 **可选鉴权** — 配置 `api_key` 后需 Bearer token 访问（空则无鉴权，便于本地调试）
- 🔄 **OpenAI 兼容** — 无痛接入任何支持 OpenAI 格式的客户端（NextChat、ChatBox、OpenCat 等）
- 👥 **多账号池管理** — 自动轮转账号、错误计数阈值冷却、禁用、持久化状态
- 🔁 **协议转换** — OpenAI 请求/响应 ↔ Anthropic Messages API 双向转换，流式 SSE 实时透传
- 🔓 **OAuth 登录** — 半自动 PKCE 授权码流程，一键获取凭证
- ⏰ **自动保活** — 后台定时检查 token，接近过期就自动刷新；被禁用的账号也会持续重试并在恢复后自动启用，全程无需手动干预
- 🗺 **模型映射** — 模型名统一归一化（大小写、空白、`[1m]` 上下文后缀），历史商业名自动映射到当前公开模型 ID
- 🛡 **模型被拒自动换号** — 上游拒绝某模型（套餐差异 / 阵发性风控）时换健康账号重试，不计错误、不触发冷却；全被拒才返回 `400 model_not_allowed`
- ⚡ **显式思考开关** — 上游缺省会自行开启扩展思考（首字 10 秒+），本服务总是显式下发 `thinking`，默认关闭，可切 `auto`/`on`
- 📋 **分级日志** — `debug` / `info` / `error` 三档，默认每个请求一行摘要（含账号、模型、状态、耗时、token），排障时切 `debug` 看换号细节；Windows 启动脚本会自动把日志落到 `logs\server.out.log`
- 🚀 **热路径不空等** — 上游的 `create_api_key` 已下线（实测挂几十秒才返回 404 页面），请求路径不再探测它，直接用 access_token 发请求
- 💠 **额度池视图** — 管理页按官网「套餐」页口径展示钱包总额与分池（套餐池 + 每日登录 / 活动等奖励池），过期批次自动剔除
- 🎁 **每日开工奖励自动领取** — 内置官方桌面端的 Ed25519 签名协议，自动登记安装身份并在每天北京时间 00:05 领取「每日登录奖励」（300→600→900→1200→1500 阶梯），启动即补领、接口幂等；管理页显示今日到账 / 下一档积分 / 下次可领时间，可手动补领
- 🧵 **请求上下文透传** — 下游取消请求时立即释放上游连接
- 🏗 **Go 单二进制** — 无第三方依赖，`go build` 即得

## 快速开始

### 方式一：下载 Release（推荐）

1. 打开 [Releases](https://github.com/guilinshanshui/phanthycode2api/releases)。
2. 下载你的系统压缩包：
   - Windows：`phanthycode2api-windows-amd64.zip`
   - Linux：`phanthycode2api-linux-amd64.tar.gz`
   - macOS Intel：`phanthycode2api-darwin-amd64.tar.gz`
   - macOS Apple Silicon：`phanthycode2api-darwin-arm64.tar.gz`
3. 解压到任意目录。
4. 双击 `phanthycode2api.exe`（Windows），或运行 `./phanthycode2api`。

   更省事的是双击同目录下的 `start-server.bat`：它会切到程序目录启动，运行日志**既打印在
   窗口里、又追加写进 `logs\server.out.log`**，忘了截窗口日志也能事后翻文件。服务已经在
   运行时再双击它，只会打开浏览器里的管理页、不会再起一份（避免撞端口）。
5. 浏览器打开管理页：

   ```text
   http://127.0.0.1:7864/admin
   ```

6. 使用默认密码 `admin123` 登录。
7. 在 **账号 → 添加账号** 里点击 **生成授权链接 → 打开链接**，登录后复制 `code`，粘贴回页面提交。

首次启动会自动生成：

- `config.json`
- `auths/`
- `data/`

程序会固定读写自己所在目录下的这些文件，所以可以放心双击运行。

重启与开机自启：

- **重启服务**：结束进程再启动即可（Windows 在任务管理器里结束 `phanthycode2api.exe`，Linux / macOS 按 `Ctrl+C` 或 `pkill phanthycode2api`）。管理页「设置」里改的配置、新添加的账号都是重启后生效
- **Windows 开机自启**：按 `Win + R` 输入 `shell:startup`，把 `start-server.bat` 的**快捷方式**放进打开的文件夹（比直接放 exe 更好：日志会落到 `logs\server.out.log`，崩了也能翻；服务已在运行时它只打开管理页，不会重复起进程）；想要后台静默运行就用「任务计划程序」新建一个「登录时」触发的任务，程序填 bat 路径、「起始位置」填程序目录
- **Linux 开机自启**：写一个 systemd 单元（`ExecStart` 指向二进制、`WorkingDirectory` 指向程序目录、`Restart=always`），再 `systemctl enable --now phanthycode2api`

### 方式二：源码构建

> 需要 Go 1.26.5+。

```bash
git clone https://github.com/guilinshanshui/phanthycode2api.git
cd phanthycode2api
go build -o phanthycode2api ./cmd/server
./phanthycode2api
```

命令行登录仍然可用：

```bash
go run ./cmd/login -step=url
go run ./cmd/login -step=exchange -code=<授权码>
```

服务默认监听 `:7864`，可用 `P2A_LISTEN` 环境变量或 `config.json` 覆盖。

### 验证

```bash
curl -s http://localhost:7864/healthz
```

## Web 管理界面

浏览器打开：

```text
http://127.0.0.1:7864/admin
```

默认密码为 `admin123`。登录后在 **设置 → 管理员密码** 里直接改即可，下次登录生效。

也可以在命令行用 `go run ./cmd/hash-password -password=你的强密码` 生成 PBKDF2 哈希后写回 `config.json`：

```json
"admin": {
  "enabled": true,
  "password_hash": "<PBKDF2 哈希>",
  "data_dir": "./data/admin"
}
```

管理页支持：

- 账号管理：生成 OAuth 授权链接、提交授权码、删除账号、手动领取开工奖励、手动刷新和「立即保活」（新账号默认昵称 `phanthy-MMDD`，同一天添加多个时自动补 uid 后四位区分）
- 密钥分发：每个下游独立密钥、独立次数上限、模型白名单
- 请求日志与统计
- 图形化编辑常用配置（API Key、目录、上游地址、超时、思考模式与预算、冷却与自动保活、每日开工奖励），并提供原始 JSON 高级编辑

### 账号积分 / 额度池

账号列表的「积分 / 额度池」列与官网「套餐」页（`https://code.phanthy.com/account`）同口径：

- 第一行是钱包：`剩余 / 总额`（对应官网「钱包积分」）。余额优先取上游 `activities/summary` 的 `credits.available`（与官网钱包余额逐分一致），取不到时退回按批次估算；下方进度条低于 30% 变黄、低于 10% 变红
- 下面每个小块是一个额度池：`套餐池（体验版 / Pro …）+ 每日登录奖励 + 活动奖励 …`，鼠标悬停可看该池的总额、已用、批次数与到期日
- 备注行给出 `官网余额`、`已用`、`最近到期`、`待发放`（推荐奖励等 `pending` 部分官网钱包同样不计入）

口径说明：

- 奖励按**批次**记账：一笔已发放的奖励就是一个批次，各有自己的发放时间与到期时间，过期未用的部分直接作废
- 奖励池的「已用」是**估算**（页面标 `≈`）。上游没有公开的批次级账本接口，只能由奖励台账（`/api/oauth/rewards`）加按天消耗明细（`/api/oauth/usage/summary`）按「先发放先扣、只在有效期内抵扣」还原，与官网会有几十分以内的偏差
- **每日开工奖励（每日登录奖励）已可自动领取**：官方桌面端用 Ed25519 签名调 `/api/oauth/activities/daily-login/claim`，本服务内置同一套协议 —— 首次启动自动生成安装身份存到 `data/desktop-keys/<uid>.json`（`0600`，勿外泄），启动即补领一次，之后每天北京时间 `claim_hour:claim_minute`（默认 00:05）自动领取；接口幂等，重复调用不会多领
  - 安装身份**每个账号一份**：上游的 installation id 全局唯一、登记后绑定首个账号，共用一份身份会让其他账号拿到 `desktop_installation_required`（实测 401）
  - 奖励阶梯：第 1 天 300、第 2 天 600、第 3 天 900、第 4 天 1200、第 5 天起 1500 积分；断签从第 1 天重算
  - 管理页「积分 / 额度池」列会多出一行：已到账显示 `开工奖励 今日已到账 +N · 连续 X 天`；未到账显示 `今日待领取 · 下一档 N 积分 · 下次可领 <时间>`；账号页操作列有「领取奖励」按钮，可随时手动补领
  - 到账当天运行日志输出一次 `开工奖励 <昵称>: <业务日> 已到账 +N 积分（连续 X 天，下一档 N 积分）`，同一天不会重复刷屏
  - 关闭自动领取：设置页「每日开工奖励 → 自动领取」选「关闭」，或改 `schedule.daily_reward: false`（关闭后仍可手动点「领取奖励」）
  - 状态口径来自上游 `activities/summary`（权威），历史累计来自 `/api/oauth/rewards` 台账，两者在管理页合并展示
- 一次刷新要打 3 个上游接口，额度结果有 5 分钟缓存，刚点过「刷新」再点可能仍是缓存值

## 配置说明

```json
{
  "listen": ":7864",
  "api_key": "***",
  "auth_dir": "./auths",
  "state_file": "./data/state.json",
  "base_url": "https://code.phanthy.com",
  "log_level": "info",
  "cooldown": {
    "hard_credit": "12h",
    "soft_rate": "60s",
    "err_threshold": 3,
    "err_cooldown": "10m"
  },
  "schedule": {
    "keepalive_interval": "30m",
    "keepalive_skew": "15m",
    "keepalive_hours": [],
    "auto_recover": true,
    "daily_reward": true,
    "claim_hour": 0,
    "claim_minute": 5
  },
  "upstream": {
    "timeout_seconds": 120
  },
  "thinking": {
    "mode": "off",
    "budget_tokens": 4096
  },
  "admin": {
    "enabled": true,
    "password_hash": "***",
    "data_dir": "./data/admin"
  }
}
```

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `listen` | `P2A_LISTEN` | `:7864` | 监听地址 |
| `api_key` | `P2A_API_KEY` | `""` | 服务端 API 密钥（空则无鉴权） |
| `auth_dir` | `P2A_AUTH_DIR` | `./auths` | 账号凭证目录 |
| `state_file` | `P2A_STATE_FILE` | `./data/state.json` | 账号状态持久化路径 |
| `base_url` | `P2A_BASE_URL` | `https://code.phanthy.com` | 上游 API 地址 |
| `log_level` | `P2A_LOG_LEVEL` | `info` | 日志级别：`debug`（排障）/ `info`（每请求一行）/ `error`（仅异常） |
| `cooldown.hard_credit` | `P2A_HARD_CREDIT` | `12h` | 积分不足冷却时长 |
| `cooldown.soft_rate` | `P2A_SOFT_RATE` | `60s` | 限流冷却时长 |
| `cooldown.err_threshold` | `P2A_ERR_THRESHOLD` | `3` | 连续错误阈值 |
| `cooldown.err_cooldown` | `P2A_ERR_COOLDOWN` | `10m` | 错误冷却时长 |
| `schedule.keepalive_interval` | `P2A_KEEPALIVE_INTERVAL` | `30m` | 自动保活检查间隔，填 `0` 关闭 |
| `schedule.keepalive_skew` | `P2A_KEEPALIVE_SKEW` | `15m` | 保活提前量：token 剩余寿命小于它就刷新 |
| `schedule.keepalive_hours` | — | `[]` | 额外的定点保活小时（可选，留空即关闭） |
| `schedule.auto_recover` | `P2A_AUTO_RECOVER` | `true` | 被禁用的账号也参与保活，刷新成功后自动重新启用 |
| `schedule.daily_reward` | `P2A_DAILY_REWARD` | `true` | 是否每天自动领取开工奖励 |
| `schedule.claim_hour` | `P2A_CLAIM_HOUR` | `0` | 领取时刻（北京时间小时，0–23） |
| `schedule.claim_minute` | `P2A_CLAIM_MINUTE` | `5` | 领取时刻（北京时间分钟，0–59） |
| `upstream.timeout_seconds` | `P2A_TIMEOUT_SECONDS` | `120` | 上游请求超时 |
| `thinking.mode` | `P2A_THINKING_MODE` | `off` | 扩展思考策略：`off` / `auto` / `on` |
| `thinking.budget_tokens` | `P2A_THINKING_BUDGET` | `4096` | 思考预算（token，1024–16384），仅 `auto` / `on` 生效 |
| `admin.enabled` | — | `true` | 是否启用 `/admin` Web 管理界面 |
| `admin.password_hash` | — | `""` | PBKDF2 管理密码哈希，空则使用默认密码 `admin123` |
| `admin.data_dir` | — | `./data/admin` | 管理数据目录 |

### 思考模式（响应速度）

上游在请求里**不带** `thinking` 字段时会自行开启扩展思考，正文首字延迟从 1~2 秒涨到 10 秒以上，
`max_tokens` 偏小时还会出现「预算全花在思考上、正文为空」。因此本服务总是显式下发该字段，
取值由 `thinking` 配置决定：

| `thinking.mode` | 行为 | 适用场景 |
|---|---|---|
| `off`（默认） | 始终下发 `{"type":"disabled"}` | 日常编码，追求首字速度 |
| `auto` | 读客户端推理档位：`none` / `minimal` / `low` 或未声明 → 关闭，`medium` 及以上 → 开启 | 想让客户端自己决定 |
| `on` | 始终开启，预算取 `thinking.budget_tokens` | 复杂推理 |

开启思考时，若客户端给的 `max_tokens` 不大于预算，会自动抬到 `budget_tokens + 1024`，
避免上游直接报参数错误。

以 Codex 接本服务为例，在 `~/.codex/config.toml` 里写：

```toml
model = "deepseek-v4.1-flash"
model_reasoning_effort = "low"
```

再把管理页的思考模式设为 `auto`，就能让低推理档位的请求自动走最快路径。

### 日志级别

服务日志写入标准错误：双击运行时看那个控制台窗口；Windows 用 `start-server.bat` 启动则
窗口显示的同时追加到 `logs\server.out.log`，其他平台可自行重定向，例如
`./phanthycode2api > server.err.log 2>&1`。`log_level` 控制详细程度：

| 级别 | 输出内容 |
|---|---|
| `debug` | 上面全部，外加每次换号、上游状态码、`ensure_api_key` 探测结果等逐次尝试细节 |
| `info`（默认） | 启动信息、keepalive 结果，以及每个请求一行摘要：`req key=... uid=... model=... status=200 1234ms tokens=123/45 stream=true` |
| `error` | 只输出失败与异常（上游报错、写盘失败等） |

排障时把 `log_level` 改成 `debug`，或设环境变量 `P2A_LOG_LEVEL=debug`；
管理页「设置 → 基础设置 → 日志级别」也能改，保存后重启生效。
摘要行里的 `uid` 和 `tokens` 同时会写进管理页的请求日志表格。

上游拒绝会额外打印一行 `stream uid=... kind=... code=...`，级别是 `error`，
不用开 `debug` 就能看到，便于直接从日志定位到具体原因。
`tokens` 只在上游真正下发该字段时才更新，所以不会出现 `tokens=0/<out>`
这种把 `input_tokens` 抹成 0 的记录。

### 可用模型

`/v1/models` 返回的即为下表模型（与上游公开目录一致），可直接作为 `model` 传入：

| 模型 ID | 上下文 | 说明 |
|---|---|---|
| `phanthy-fast` | 1.05M | 日常任务，最经济 |
| `phanthy-pro` | 1.05M | 较复杂任务 |
| `phanthy-ultra` | 1.05M | 高难度任务 |
| `glm-5.3-flash` | 1M | 日常任务 |
| `glm-5.3` | 1M | 日常任务 |
| `glm-5.2` | 1M | 日常任务 |
| `glm-5.1` | 200K | 一般任务 |
| `kimi-k3` | 1M | 较复杂任务 |
| `kimi-k2.7-code` | 256K | 日常任务 |
| `deepseek-v4.1-flash` | 1M | 日常任务 |

**模型被拒时先换号重试**：上游的「模型服务不可用」既可能来自套餐差异，也会
按账号阵发性出现——实测同一个账号同一个模型，5 分钟前被拒（`tokens=0/0`）、
之后又正常。所以本服务不会拿单个账号的结果下结论，而是换号再试一遍，只有
**池子里所有健康账号都被拒**才把错误交给客户端。

上游的拒绝有三种姿势，都会被识别成同一类错误：

| 上游表现 | 本服务行为 |
|---|---|
| `403 model_not_allowed`（HTTP 状态码） | 换号重试；全被拒才返回 `400 model_not_allowed` |
| `424 upstream_permission_denied`（HTTP 状态码） | 同上 |
| `200 OK` + SSE `event: error`，`code=upstream_permission_denied` | 同上；审计日志状态码记非 200，不再记成成功 |

换号只发生在**还没给下游写过任何字节**时；流式正文一旦开始下发，状态码已定局，
这时会把上游错误如实写进流里并把审计状态改成失败，不会出现「半截正文 + 悄悄换号」。

被拒**不会**让账号进入冷却（这是请求侧问题，不是账号故障），也**不会**扣积分。

> 上游拒绝服务时不一定返回 4xx：实测它会用 `HTTP 200 + text/event-stream`
> 承载错误，正文不到 200 字节，只在事件流里写一条
> `{"type":"error","error":{"code":"upstream_permission_denied",...}}`。
> 只认状态码的转发会把这种拒绝当成「正常结束的空回复」——客户端一直转圈，
> 管理台日志里留下一条 `status=200 tokens=0/0` 的假成功记录。

各账号套餐不同、放行的模型也不同（例如体验版只放行 `phanthy-fast`、`phanthy-pro`
和小尺寸的 `glm-5.3-flash`）。**多登录几个账号**能显著提高被拒时的成功率：
换号重试的预算会随健康账号数放宽，账号越多越不容易真正失败。

> 阵发性拒绝与「账号本身不可用」要分清：token 过期（`invalid_grant`）会被
> 直接禁用、`Refused` 的账号不再参与挑号，这种要回管理台重新登录；而
> `model_denied` 只跳过本次请求的这一个账号，账号仍然是健康的。

运行日志里会给出可直接定位的错误行，例如：

```
stream uid=phanthy-1790570942 kind=model_denied code=upstream_permission_denied committed=false msg="Model service access was denied. ..."
chat uid=phanthy-1790570942 model=phanthy-pro 上游未授权该模型（http 403 model_denied），换号重试 1/2
req key=默认密钥 uid=phanthy-1790570968 model=phanthy-pro status=200 21347ms tokens=106355/274 stream=true
```

只有所有账号都被拒时才会看到收尾的 `400 model_not_allowed`，消息里会列出
试过的账号数（`model "phanthy-pro" was rejected by all 2 tried account(s): ...`）。

### 名称兼容

模型名会先归一化（去首尾空白、转小写、剥离 `[1m]`/`[2m]`/`:1m` 之类的上下文后缀、内部空白折成 `-`），
再查别名表。因此下列写法都能正常工作：

| 客户端可用写法 | 实际发往上游 |
|---|---|
| `Kimi K3`、`Kimi-k3`、`kimi-k3[1m]` | `kimi-k3` |
| `GLM 5.2`、`glm-5.2` | `glm-5.2` |
| `DeepSeek-V4` | `deepseek-v4.1-flash` |
| `Claude Opus 4.8` | `claude-opus-4-8` |
| `auto` | `phanthy-fast` |
| `gpt-5.6-sol` | `phanthy-pro` |
| `Iris-1.0`、`Zeus-1.1-pro`、`Gaia-1.2`、`Apollo-2.0`、`Metis-1.1` | 上游已下线的旧内部代号，仅作入参兼容，自动转发到对应公开 ID |

## API

### `POST /v1/chat/completions`

OpenAI 兼容。支持 `stream`（SSE 流式）、`max_tokens`、`temperature`、`top_p`，以及
`tools` / `tool_choice`（函数调用，含 `none`/`auto`/`required` 三种字符串形式）与 `stop`。
请求体上限 32 MB，超出返回 `413 request_too_large`。

客户端声明的推理档位（Codex 的 `reasoning_effort`，或 OpenAI 新格式的 `reasoning.effort`）会被读取，
用于 `thinking.mode = auto` 时判断是否开启思考；其余模式下该字段仅作参考，不下发给上游。

### `GET /v1/models`

返回上游当前公开可用的模型列表（与官网定价页一致），含 `context_length`。
套餐未开放的模型仍会列出；调用时若池中所有健康账号都被上游拒绝，才返回 `400 model_not_allowed`。

### `GET /status`

账号池状态概览（每个账号的 UID、状态、冷却、错误计数）。

### `GET /healthz`

健康检查（无需鉴权）。

## 鉴权机制

启用方式：在 `config.json` 设置 `api_key`（或环境变量 `P2A_API_KEY`），服务端即开启 Bearer 鉴权。

- 受保护端点（`/v1/chat/completions`、`/v1/models`、`/status`）需携带请求头：
  ```
  Authorization: Bearer <your_api_key>
  ```
- 缺失或错误的 token → `401 {"error":{"code":"invalid_api_key",...}}`
- `/healthz` **不**参与鉴权（方便健康检查与容器探针）
- `api_key` 留空（默认）→ 不鉴权，任意请求可直接访问（适合本地开发 / 内网部署）

鉴权在 `internal/server/handler.go` 的 `withAuth` 中间件中实现：根据配置的 `APIKey`
统一拦截，匹配失败直接返回 401，零外部依赖。

## Docker 部署

### 1. 构建镜像 & 启动

```bash
# 可选：编辑 docker-compose.yml 设置 P2A_API_KEY 开启鉴权
docker compose up -d --build
```

服务默认监听 `7864`，通过 `docker-compose.yml` 的 `ports` 映射到宿主机。

### 2. 凭证与状态持久化

账号凭证（`auths/`）与运行状态（`data/`）已通过卷挂载持久化，容器重建不会丢失。
**注意**：这两目录含真实 token，已在 `.gitignore` / `.dockerignore` 中排除，切勿提交。

首次使用仍需在宿主机生成凭证后再挂载：

```bash
# 宿主机本地生成凭证（两步式 OAuth），写入 ./auths
./login.sh               # 或手动两步：-step=url 然后 -step=exchange -code=xxx
# 凭证就绪后再 docker compose up，容器直接复用 ./auths
```

### 3. 自定义配置

如需完全自定义，挂载本地 `config.json` 覆盖镜像默认的 `config.example.json`：

```yaml
# docker-compose.yml（取消注释）
volumes:
  - ./config.json:/app/config.json:ro
```

### 4. 验证

```bash
# 健康检查
curl -s http://localhost:7864/healthz

# 聊天（若已开启鉴权，加 -H "Authorization: Bearer <key>"）
curl -s http://localhost:7864/v1/chat/completions \
  -H "Authorization: Bearer your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"DeepSeek-V4","messages":[{"role":"user","content":"你好"}]}'
```

### 服务端口

| 服务 | 端口 | 说明 |
|------|------|------|
| phanthycode2api | `7864` | OpenAI 兼容 API 入口 |

> 国内网络环境：Dockerfile 使用官方 `golang:1.26-alpine` 基础镜像；若拉取缓慢，可改用国内镜像源（如阿里云 `registry.cn-hangzhou.aliyuncs.com`）或配置 Docker daemon 镜像加速。

## 目录结构

```
phanthycode2api/
├── cmd/
│   ├── server/          # 服务入口：配置 → auth → pool → upstream → scheduler → server
│   │   ├── main.go
│   │   └── config.go
│   └── login/           # 半自动 OAuth 登录工具
│       └── main.go
├── internal/
│   ├── auth/            # 账号解析（三种形态）+ 原子写入
│   ├── pool/            # 账号池状态机（冷却、禁用、错误计数阈值）
│   ├── upstream/        # 核心：OpenAI ↔ Anthropic 协议转换 + 思考开关 + SSE 流式转换 + 桌面端签名
│   ├── server/          # OpenAI 兼容 HTTP 服务器，带轮转与错误分类 + 鉴权中间件
│   ├── scheduler/       # 自动保活 + 每日开工奖励自动领取
│   ├── reward/          # 开工奖励口径换算（台账 + activities/summary 合并）
│   ├── admin/           # Web 管理台（PBKDF2 登录、账号 / 密钥 / 日志 / 配置）
│   └── logx/            # 分级日志（debug / info / error）
├── config.example.json  # 配置模板
├── start-server.bat     # Windows 双击启动（切到程序目录、日志落 logs\server.out.log、已在运行则只开管理页）
├── login.sh             # 半自动 OAuth 登录脚本（包装 cmd/login 两步流程）
├── credit.sh            # 账号池状态 / credit 查看
├── Dockerfile           # 多阶段构建
├── Dockerfile.local     # 离线构建（复用预编译 dist/ 二进制，网络受限环境用）
├── docker-compose.yml   # 容器编排（含 healthcheck）
├── .dockerignore
├── .gitignore
└── go.mod
```

## 免责声明

本项目仅供学习和研究使用。请遵守 PhanthyCode 平台服务条款，自行承担使用风险。
作者不对任何因使用本项目产生的直接或间接损失负责。

## License

MIT
