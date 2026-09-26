#!/bin/bash
# record-review.sh [<base>] records that the diff from <base> (the default
# branch when omitted) to HEAD was reviewed, writing the stamp the gate in
# .claude/hooks/require-code-review.sh checks before `gh pr create --base <base>`.
. "$(dirname "$0")/../../hooks/review-stamp.sh"

sha=$(git rev-parse --verify HEAD) || exit 1
gitdir=$(git rev-parse --absolute-git-dir) || exit 1
stamp=$(review_stamp "$sha" "$gitdir" "$1") || {
  echo "record-review: $stamp" >&2
  exit 1
}
touch "$stamp"
