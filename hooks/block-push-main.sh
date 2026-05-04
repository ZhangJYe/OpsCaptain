#!/bin/bash
# 阻止直接推送到 main 分支

source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

if echo "$TOOL_INPUT" | grep -qE 'git\s+push.*\b(main|master)\b'; then
  block "不能直接推送到 main/master 分支，请创建 PR"
fi

allow
