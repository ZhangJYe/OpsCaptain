#!/bin/bash
# 保护 Go 依赖文件

source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 保护 Go 依赖文件
if echo "$TOOL_INPUT" | grep -qE '(go\.mod|go\.sum)'; then
  block "Go 依赖文件需要人工确认修改"
fi

# 保护 Makefile
if echo "$TOOL_INPUT" | grep -qE 'Makefile'; then
  block "Makefile 修改需要人工确认"
fi

# 保护 Dockerfile
if echo "$TOOL_INPUT" | grep -qE 'Dockerfile'; then
  block "Dockerfile 修改需要人工确认"
fi

allow
