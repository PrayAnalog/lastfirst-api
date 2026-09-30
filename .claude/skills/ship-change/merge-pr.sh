#!/bin/bash
# Usage: merge-pr.sh <pr-number> <head-sha>
#
# Squash-merges the PR once review-state.sh reports `verdict: quiet`, every
# check has passed and GitHub reports the merge state CLEAN, then prints the
# PR's state and merge commit. The merge fails unless the PR's head is
# <head-sha>. Otherwise it prints what stopped it and exits non-zero without
# merging. Run it from the worktree that pushed the head, as review-state.sh
# requires.
set -euo pipefail

pr=${1:?usage: merge-pr.sh <pr-number> <head-sha>}
sha=$(git rev-parse --verify "${2:?usage: merge-pr.sh <pr-number> <head-sha>}^{commit}")

report=$("$(dirname "$0")/review-state.sh" "$pr")
if [ "$(tail -n 1 <<<"$report")" != "verdict: quiet" ]; then
  printf '%s\n' "$report"
  exit 1
fi

checks=$(gh pr checks "$pr") || {
  printf '%s\n' "$checks"
  exit 1
}

merge_state=$(gh pr view "$pr" --json mergeStateStatus --jq .mergeStateStatus)
if [ "$merge_state" != CLEAN ]; then
  echo "merge-pr: merge state of #$pr is $merge_state, not CLEAN" >&2
  exit 1
fi

gh pr merge "$pr" --squash --match-head-commit "$sha"
gh pr view "$pr" --json state,mergeCommit --jq '"\(.state) \(.mergeCommit.oid)"'
