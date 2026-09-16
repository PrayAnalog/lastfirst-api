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
gitdir=$(git rev-parse --absolute-git-dir 2>/dev/null) || exit 0
stamp="$gitdir/claude-code-review-$sha"

[ -f "$stamp" ] && exit 0

jq -n --arg stamp "$stamp" '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "deny",
    permissionDecisionReason: (
      "Work through .agents/pre-pr-checklist.md against this branch'"'"'s diff and run /code-review on it before opening the PR. Apply what they find, or record why you are declining it. Then record the review by running `touch " + ($stamp | @sh) + "` and retry this command. Applying fixes moves HEAD, so the next commit needs its own review."
    )
  }
}'
