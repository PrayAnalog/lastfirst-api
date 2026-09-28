#!/bin/bash
# Usage: needs-diagram.sh <base>
#
# Prints `draw` when the diff from <base> to HEAD needs a PR Lens diagram
# under section 5 of SKILL.md, and `skip` when it does not: tests, docs and
# process files are set aside, and what is left needs a diagram when it spans
# more than one directory or touches Dockerfile, deploy/ or .github/workflows/.
set -eo pipefail

base=${1:?usage: needs-diagram.sh <base>}
base=${base#origin/}
base_sha=$(git rev-parse --verify --quiet "origin/$base^{commit}" || git rev-parse --verify "$base^{commit}")

files=$(git diff --name-only --no-renames "$base_sha...HEAD")
left=$(grep -Ev '(_test\.go|\.md)$|^\.claude/|^\.agents/' <<<"$files" || true)

if [ -z "$left" ]; then
  echo skip
  exit 0
fi
dirs=$(sed -E 's#^([^/]*)$#./\1#; s#/[^/]+$##' <<<"$left" | sort -u | wc -l)

if grep -Eq '^(Dockerfile$|deploy/|\.github/workflows/)' <<<"$left"; then
  echo draw
elif [ "$dirs" -gt 1 ]; then
  echo draw
else
  echo skip
fi
