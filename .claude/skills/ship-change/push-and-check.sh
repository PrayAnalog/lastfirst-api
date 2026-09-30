#!/bin/bash
# Usage: push-and-check.sh <pr-number>
#
# Pushes the checked-out branch, then prints review-state.sh's report for the
# PR. Run it from the worktree that made the commits, so that the report can
# tell which push its bot reactions follow.
set -euo pipefail

pr=${1:?usage: push-and-check.sh <pr-number>}

git push
"$(dirname "$0")/review-state.sh" "$pr"
