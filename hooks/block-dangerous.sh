#!/bin/bash
# 阻止危险命令

source "$(dirname "$0")/utils.sh"

TOOL_NAME="$1"
TOOL_INPUT="$2"

# 阻止 rm -rf 关键目录
if echo "$TOOL_INPUT" | grep -qE 'rm\s+(-rf?|--recursive)\s+'; then
  if echo "$TOOL_INPUT" | grep -qE '\.(git|env|docker|ssh|config)'; then
    block "检测到删除关键目录操作，已阻止"
  fi
fi

# 阻止格式化磁盘
if echo "$TOOL_INPUT" | grep -qE '(mkfs|fdisk|dd\s+if=)'; then
  block "检测到磁盘操作命令，已阻止"
fi

# 阻止修改系统文件
if echo "$TOOL_INPUT" | grep -qE '(\/etc\/|\/usr\/|\/var\/)'; then
  block "检测到系统目录修改，已阻止"
fi

allow
