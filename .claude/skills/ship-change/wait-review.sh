#!/bin/bash
# Usage: wait-review.sh <pr-number>
#
# Runs review-state.sh every 60 seconds until its verdict is anything but
# `waiting`, then prints that last report. Run it from the worktree that
# pushed the head, in the background (Monitor): it exits once, when there is
# something to act on.
pr=${1:?usage: wait-review.sh <pr-number>}

until report=$("$(dirname "$0")/review-state.sh" "$pr") &&
  grep -Eq '^verdict: (findings|blocked|unapproved|quiet)' <<<"$report"; do
  sleep 60
done
printf '%s\n' "$report"
