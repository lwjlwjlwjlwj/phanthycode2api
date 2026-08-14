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
set -euo pipefail

cd "$(dirname "$0")"

# 工具链与依赖下载防护（不影响已缓存环境；无代理时这些变量为空也不会出错）
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
if [[ -z "${HTTP_PROXY:-}" && -z "${HTTPS_PROXY:-}" ]]; then
  # 仅当本机存在常用代理端口时默认启用（按需；不设也不会阻断已缓存场景）
  if [[ -f /dev/tcp/172.18.45.188/7891 ]] 2>/dev/null; then
    export HTTP_PROXY="http://172.18.45.188:7891"
    export HTTPS_PROXY="http://172.18.45.188:7891"
  fi
fi

CODE=""
for arg in "$@"; do
  case "$arg" in
    -code=*) CODE="${arg#-code=}" ;;
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
echo ">>> 步骤 2/2: 授权后粘贴 code（浏览器跳转页 URL 或裸 code 均可）"
read -r -p "code> " USER_CODE
[[ -z "$USER_CODE" ]] && { echo "未输入 code，退出" >&2; exit 1; }

go run ./cmd/login -step=exchange -code="$USER_CODE"

echo ""
echo "✅ 登录完成。凭证已写入 auths/，重启 phanthycode2api 容器/服务即生效。"
