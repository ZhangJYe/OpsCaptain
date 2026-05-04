#!/bin/bash
# 审计日志

TOOL_NAME="$1"
TOOL_INPUT="$2"
TOOL_OUTPUT="$3"

sanitize() {
  printf '%s' "$1" | perl -pe '
    s/\b\d{1,3}(?:\.\d{1,3}){3}\b/[private-ip]/g;
    s/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/[uuid]/g;
    s/(bearer\s+)[A-Za-z0-9._~+\/=-]+/${1}[redacted]/ig;
    s/((?:api[_-]?key|token|secret|password|credential|access[_-]?key|private[_-]?key)["'\''\s]*[:=]\s*)("[^"]*"|'\''[^'\'']*'\''|[^\s,}]+)/${1}[redacted]/ig;
  '
}

truncate() {
  local value="$1"
  local max_len="$2"
  if [ ${#value} -gt "$max_len" ]; then
    value="${value:0:$max_len}...(截断)"
  fi
  printf '%s' "$value"
}

# 审计日志只保留脱敏摘要
MAX_LEN=500
SAFE_INPUT=$(truncate "$(sanitize "$TOOL_INPUT")" "$MAX_LEN")
SAFE_OUTPUT=$(truncate "$(sanitize "$TOOL_OUTPUT")" "$MAX_LEN")

echo "[$(date '+%Y-%m-%d %H:%M:%S')] $TOOL_NAME" >> .claude-audit.log
echo "  Input: $SAFE_INPUT" >> .claude-audit.log
echo "  Output: $SAFE_OUTPUT" >> .claude-audit.log
echo "---" >> .claude-audit.log

exit 0
