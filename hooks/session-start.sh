#!/bin/bash
# 会话初始化

source "$(dirname "$0")/utils.sh"

log "=== 新会话开始 ==="

# 加载项目上下文
if [ -f ".env" ]; then
  add_context "已加载 .env 环境变量"
fi

# 检查 Go 版本
GO_VERSION=$(go version 2>/dev/null)
if [ -n "$GO_VERSION" ]; then
  add_context "Go 环境: $GO_VERSION"
fi

exit 0
