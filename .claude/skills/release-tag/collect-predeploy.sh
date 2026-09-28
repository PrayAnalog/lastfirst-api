#!/bin/bash
# Usage: collect-predeploy.sh
#
# Prints the pull requests a new tag would deploy (the latest v* tag..origin/main),
# the deploy/ files changed in that range, and the "Pre-deploy checks" section of
# each pull request that has one.
set -eo pipefail

git fetch origin --tags --quiet
prev=$(git tag --list 'v*' --sort=-v:refname | head -n 1)
range="$prev..origin/main"

echo "range: $range"
echo "deploy/ files changed:"
git diff --name-only "$range" -- deploy/ | sed 's/^/  /'

prs=$(git log --format=%s "$range" | { grep -oE '\(#[0-9]+\)$' || true; } | tr -d '(#)' | sort -n)
echo "pull requests: $(echo $prs)"

for pr in $prs; do
  section=$(gh pr view "$pr" --json body --jq .body | awk '/^## Pre-deploy checks/{f=1;next} /^## /{f=0} f')
  if [ -n "$(tr -d '[:space:]' <<<"$section")" ]; then
    printf '\n=== #%s\n%s\n' "$pr" "$section"
  fi
done
