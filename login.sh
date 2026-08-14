#!/usr/bin/env bash
# login.sh — 半自动 OAuth 登录辅助脚本
#
# 用法:
#   ./login.sh              # 交互式：生成授权 URL → 粘贴 code → 自动交换落盘
#   ./login.sh -code=xxx    # 跳过第一步，直接用已有 code 交换（需先跑过第一步生成 verifier）
#
# 依赖: go（用于 go run ./cmd/login）。凭证落盘到 auths/，重启服务即生效。
set -euo pipefail

cd "$(dirname "$0")"

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
