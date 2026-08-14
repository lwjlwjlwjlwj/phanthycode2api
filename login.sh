#!/usr/bin/env bash
# login.sh — 半自动 OAuth 登录辅助脚本
#
# 用法:
#   ./login.sh              # 交互式：生成授权 URL → 粘贴 code → 自动交换落盘
#   ./login.sh -code=xxx    # 跳过第一步，直接用已有 code 交换（需先跑过第一步生成 verifier）
#
# 依赖: go（用于 go run ./cmd/login）。凭证落盘到 auths/，重启服务即生效。
#
# 注: go.mod 要求 go >= 1.26.5。若本机未缓存该工具链，go 会自动下载；
#     这里预置 GOPROXY 与代理，避免内网/受限网络下 DNS 超时导致下载失败。
#
# 关于 code 输入: 浏览器跳转后地址栏形如
#   https://code.phanthy.com/oauth/code/success?code=XXXX#p2a-login
# 你可以粘贴整段 URL、?code= 之后的部分、或裸 code，脚本会自动清洗提取。
set -euo pipefail

cd "$(dirname "$0")"

# 工具链与依赖下载防护（不影响已缓存环境；无代理时这些变量为空也不会出错）
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
if [[ -z "${HTTP_PROXY:-}" && -z "${HTTPS_PROXY:-}" ]]; then
  if [[ -f /dev/tcp/172.18.45.188/7891 ]] 2>/dev/null; then
    export HTTP_PROXY="http://172.18.45.188:7891"
    export HTTPS_PROXY="http://172.18.45.188:7891"
  fi
fi

# 清洗 code：去掉 URL fragment(#...) 与 query(?) 之外无关部分，只留裸 code
clean_code() {
  local c="$1"
  c="${c%%#*}"          # 去掉 #fragment（含 #p2a-login）
  c="${c##*code=}"     # 若含 ?code=xxx 或 code=xxx，取其后部分
  c="${c##*\?}"        # 兜底：若有 ? 但无 code=，取 ? 之后
  c="$(echo -n "$c" | tr -d '[:space:]')"   # 去空白
  echo "$c"
}

CODE=""
for arg in "$@"; do
  case "$arg" in
    -code=*) CODE="$(clean_code "${arg#-code=}")" ;;
    *) echo "未知参数: $arg" >&2; exit 1 ;;
  esac
done

if [[ -n "$CODE" ]]; then
  echo ">>> 用已有 code 交换 token ..."
  go run ./cmd/login -step=exchange -code="$CODE"
  exit 0
fi

echo ">>> 步骤 1/2: 生成授权 URL ..."
go run ./cmd/login -step=url

echo ""
echo ">>> 步骤 2/2: 授权后粘贴 code（支持：裸 code / ?code=xxx / 整段跳转 URL，自动清洗）"
read -r -p "code> " USER_CODE
CLEAN="$(clean_code "$USER_CODE")"
if [[ -z "$CLEAN" ]]; then
  echo "未检测到有效 code，退出。可稍后重试: ./login.sh -code=<你的code>" >&2
  exit 1
fi

go run ./cmd/login -step=exchange -code="$CLEAN"

echo ""
echo "✅ 登录完成。凭证已写入 auths/，重启 phanthycode2api 容器/服务即生效。"
