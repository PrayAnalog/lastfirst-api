#!/bin/bash
# Usage: push-and-check.sh <pr-number>
#
# Pushes the checked-out branch, then prints review-state.sh's report for the
# PR once GitHub reports the pushed commit as the PR's head; until then it
# exits non-zero without a report. Run it from the worktree that made the
# commits, so that the report can tell which push its bot reactions follow.
set -euo pipefail

pr=${1:?usage: push-and-check.sh <pr-number>}

git push
head=$(gh pr view "$pr" --json headRefOid --jq .headRefOid)
if [ "$head" != "$(git rev-parse HEAD)" ]; then
  echo "push-and-check: #$pr head is ${head:0:7}, not the pushed $(git rev-parse --short HEAD); run review-state.sh $pr once it updates" >&2
  exit 1
fi
"$(dirname "$0")/review-state.sh" "$pr"
