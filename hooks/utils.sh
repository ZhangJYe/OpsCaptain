#!/bin/bash
# 公共工具函数

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> .claude-audit.log
}

json_escape() {
  printf '%s' "$1" | perl -0777 -pe 's/\\/\\\\/g; s/"/\\"/g; s/\n/\\n/g; s/\r/\\r/g; s/\t/\\t/g'
}

block() {
  local reason="$1"
  echo "{\"decision\":\"block\",\"reason\":\"$(json_escape "$reason")\"}"
  exit 2
}

allow() {
  echo "{\"decision\":\"allow\"}"
  exit 0
}

warn() {
  local context="$1"
  echo "{\"decision\":\"allow\",\"context\":\"$(json_escape "$context")\"}"
  exit 0
}

add_context() {
  local context="$1"
  echo "{\"context\":\"$(json_escape "$context")\"}"
  exit 0
}
