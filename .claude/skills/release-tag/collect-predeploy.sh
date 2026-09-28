#!/bin/bash
# Usage: collect-predeploy.sh
#
# Prints the pull requests a new tag would deploy (the tag of the latest GitHub
# Release, which exists only after a successful deploy, up to the commit
# origin/main is at, printed in the range line), the deploy/ files changed in
# that range, and the "Pre-deploy checks" section of each pull request that has one.
set -eo pipefail

git fetch origin --tags --quiet
prev=$(gh release list --limit 10000 --exclude-drafts --exclude-pre-releases --json tagName --jq '.[].tagName' |
  sed 's/-/~/' | sort -V | sed 's/~/-/' | tail -n 1)
head=$(git rev-parse origin/main)
range="$prev..$head"

echo "range: $range"
echo "commits: $(git rev-list --count "$range")"
echo "deploy/ files changed:"
git diff --name-status "$range" -- deploy/ | sed 's/^/  /'

shas=$(gh api --paginate "repos/{owner}/{repo}/compare/$prev...$head?per_page=100" --jq '.commits[].sha')
prs=
for sha in $shas; do
  prs+=$'\n'$(gh api "repos/{owner}/{repo}/commits/$sha/pulls" --jq '.[] | select(.merged_at != null and .base.ref == "main") | .number')
done
prs=$(sort -un <<<"$prs")
echo "pull requests: $(echo $prs)"

for pr in $prs; do
  section=$(gh pr view "$pr" --json body --jq .body | awk '/^## Pre-deploy checks/{f=1;next} /^## /{f=0} f')
  if [ -n "$(tr -d '[:space:]' <<<"$section")" ]; then
    printf '\n=== #%s\n%s\n' "$pr" "$section"
  fi
done
