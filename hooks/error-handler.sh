#!/bin/bash
# 错误处理

source "$(dirname "$0")/utils.sh"

TOOL_NAME="$1"
TOOL_OUTPUT="$2"

log "❌ 工具调用失败: $TOOL_NAME"
log "错误信息: $TOOL_OUTPUT"

exit 0
