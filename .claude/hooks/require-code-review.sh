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

sha=$(git rev-parse --verify HEAD 2>/dev/null) || exit 0
gitdir=$(git rev-parse --absolute-git-dir 2>/dev/null) || exit 0

. "$(dirname "$0")/review-stamp.sh"

deny() {
  jq -n --arg reason "$1" '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: $reason
    }
  }'
  exit 0
}

# gh pr create can open a PR for a branch and a base other than the ones that
# were reviewed here, so read both off the command instead of assuming them.
flag_value() {
  printf '%s\n' "$cmd" | awk -v f="$1" '
    {
      for (i = 1; i <= NF; i++) {
        if ($i == f && i < NF) { print $(i + 1); exit }
        if (index($i, f "=") == 1) { print substr($i, length(f) + 2); exit }
      }
    }'
}

head_ref=$(flag_value --head)
if [ -n "$head_ref" ] && [ "$(git rev-parse --verify "$head_ref^{commit}" 2>/dev/null)" != "$sha" ]; then
  deny "This gate can only vouch for the branch that is checked out, and --head $head_ref is not it. Check that branch out and review it there, or drop --head."
fi

base_ref=$(flag_value --base)
stamp=$(review_stamp "$sha" "$gitdir" "$base_ref") || deny "$stamp"

[ -f "$stamp" ] && exit 0

deny "Work through .agents/pre-pr-checklist.md against this branch's diff and run /code-review and the OCR review in .agents/ocr-review.md on it before opening the PR. Apply what they find, or record why you are declining it. Then record the review by running \`.claude/skills/ship-change/record-review.sh${base_ref:+ $(printf '%q' "$base_ref")}\` as a command of its own and retry this command. Applying fixes moves HEAD, so the next commit needs its own review."
