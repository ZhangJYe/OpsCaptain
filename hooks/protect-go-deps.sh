#!/bin/bash

source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

if echo "$TOOL_INPUT" | grep -qE 'Makefile'; then
  block "Makefile 修改需要人工确认"
fi

if echo "$TOOL_INPUT" | grep -qE 'Dockerfile'; then
  block "Dockerfile 修改需要人工确认"
fi

allow
