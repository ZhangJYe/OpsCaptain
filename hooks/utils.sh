#!/bin/bash
# 公共工具函数

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> .claude-audit.log
}

block() {
  local reason="$1"
  echo "{\"decision\":\"block\",\"reason\":\"$reason\"}"
  exit 2
}

allow() {
  echo "{\"decision\":\"allow\"}"
  exit 0
}

add_context() {
  local context="$1"
  echo "{\"context\":\"$context\"}"
  exit 0
}
