#!/bin/bash
# 自动运行 go vet

source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 只对 .go 文件触发
if ! echo "$TOOL_INPUT" | grep -qE '\.go"'; then
  exit 0
fi

log "检测到 Go 文件修改，运行 go vet..."

# 运行 go vet
VET_OUTPUT=$(go vet ./... 2>&1)
VET_EXIT=$?

if [ $VET_EXIT -ne 0 ]; then
  # 截断过长输出
  if [ ${#VET_OUTPUT} -gt 1000 ]; then
    VET_OUTPUT="${VET_OUTPUT:0:1000}..."
  fi
  add_context "⚠️ go vet 发现问题:\n\`\`\`\n$VET_OUTPUT\n\`\`\`"
fi

exit 0
