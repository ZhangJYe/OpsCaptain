#!/bin/bash

source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

if echo "$TOOL_INPUT" | grep -qE 'git\s+push.*\b(main|master)\b'; then
  add_context "⚠️ 检测到推送到 main/master 分支，请确认这是预期操作"
fi

allow
