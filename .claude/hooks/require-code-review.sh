#!/bin/bash
input=$(</dev/stdin)

case "$input" in
  *"gh pr create"*) ;;
  *) exit 0 ;;
esac

if ! command -v jq >/dev/null 2>&1; then
  echo "require-code-review: jq not found, so the pre-PR review gate cannot run. Install jq or remove the hook from .claude/settings.json." >&2
  exit 1
fi

cmd=$(printf '%s' "$input" | jq -r '.tool_input.command // ""')

printf '%s' "$cmd" | grep -Eq '(^|[;&|(])[[:space:]]*gh[[:space:]]+pr[[:space:]]+create([[:space:]]|$)' || exit 0

sha=$(git rev-parse HEAD 2>/dev/null) || exit 0
gitdir=$(git rev-parse --git-dir 2>/dev/null) || exit 0
stamp="$gitdir/claude-code-review-$sha"

[ -f "$stamp" ] && exit 0
: > "$stamp"

jq -n '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "deny",
    permissionDecisionReason: "Run /code-review on this branch'"'"'s diff before opening the PR. Apply anything it finds (or say why you are rejecting it), then run this same command again — it will not be blocked a second time for this commit."
  }
}'
