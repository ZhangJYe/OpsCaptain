#!/bin/bash
# 保护敏感文件

source "$(dirname "$0")/utils.sh"

TOOL_INPUT="$1"

# 敏感文件列表
SENSITIVE_PATTERNS=(
  "config\.yaml"
  "\.env"
  "\.env\."
  "secret"
  "password"
  "credentials"
  "\.pem"
  "\.key"
  "id_rsa"
)

for pattern in "${SENSITIVE_PATTERNS[@]}"; do
  if echo "$TOOL_INPUT" | grep -qiE "$pattern"; then
    block "检测到敏感文件操作: $pattern，需要人工确认"
  fi
done

allow
