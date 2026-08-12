#!/usr/bin/env bash
# v_shell MCP 启动脚本 —— 命令行参数直接透传。
# 用法:
#   ./run.sh -h http://your-vshell-server:8082 -u admin -p '你的密码'
#   ./run.sh -help
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -x ./vshell-mcp ]]; then
  echo "[run.sh] 未找到 vshell-mcp 二进制,正在编译..."
  go build -o vshell-mcp .
fi

if [[ $# -eq 0 ]]; then
  ./vshell-mcp -help
  exit 1
fi

exec ./vshell-mcp "$@"
