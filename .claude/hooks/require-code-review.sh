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
if [ -z "$base_ref" ]; then
  base_ref=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null)
  [ -n "$base_ref" ] || base_ref=main
fi
base_ref=${base_ref#origin/}

base_sha=$(git rev-parse --verify "origin/$base_ref^{commit}" 2>/dev/null) ||
  base_sha=$(git rev-parse --verify "$base_ref^{commit}" 2>/dev/null) ||
  deny "This gate reviews the diff against $base_ref, which does not resolve to a commit here. Fetch it, or name a base this checkout has."

merge_base=$(git merge-base "$sha" "$base_sha" 2>/dev/null) ||
  deny "$sha and $base_ref share no history, so there is no diff this gate can attest."

# What gets reviewed is the three-dot diff, merge_base..HEAD, so both of those
# are in the key: a new commit or a rebase moves one end or the other and the
# stamp will not exist yet. The base tip itself is deliberately not in the key,
# because main advancing over commits this branch does not touch leaves that
# diff untouched too, and re-reviewing it would find nothing. The base *ref* is
# in the key, hashed because it can contain a slash, so that targeting a
# different branch needs its own review even when the two share a merge base.
base_key=$(printf '%s' "$base_ref" | git hash-object --stdin 2>/dev/null | cut -c1-12)
[ -n "$base_key" ] || deny "Could not derive a stamp key for base $base_ref."
stamp="$gitdir/claude-code-review-$sha-$base_key-$merge_base"

[ -f "$stamp" ] && exit 0

deny "Work through .agents/pre-pr-checklist.md against this branch's diff and run /code-review and the OCR review in .agents/ocr-review.md on it before opening the PR. Apply what they find, or record why you are declining it. Then record the review by running \`touch $(printf '%q' "$stamp")\` and retry this command. Applying fixes moves HEAD, so the next commit needs its own review."
