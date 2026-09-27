#!/bin/bash
# Usage: open-pr.sh <base> <title> <body-file> [<diagram>...]
#
# Pushes HEAD, opens its PR against <base> with each diagram attached, and
# checks that the body GitHub stored is <body-file> with every diagram
# reference pointing at an uploaded asset. Like the gate in
# .claude/hooks/require-code-review.sh, it refuses to push until
# record-review.sh has recorded a review of this diff against <base>.
set -eo pipefail
. "$(dirname "$0")/../../hooks/review-stamp.sh"

base=${1:?usage: open-pr.sh <base> <title> <body-file> [<diagram>...]}
title=${2:?usage: open-pr.sh <base> <title> <body-file> [<diagram>...]}
body_file=${3:?usage: open-pr.sh <base> <title> <body-file> [<diagram>...]}
shift 3

sha=$(git rev-parse --verify HEAD)
gitdir=$(git rev-parse --absolute-git-dir)
stamp=$(review_stamp "$sha" "$gitdir" "$base") || {
  echo "open-pr: $stamp" >&2
  exit 1
}
if [ ! -f "$stamp" ]; then
  echo "open-pr: no review of this diff against $base is recorded. Work through .agents/pre-pr-checklist.md, run /code-review and the OCR review in .agents/ocr-review.md, then run .claude/skills/ship-change/record-review.sh $(printf '%q' "$base") as a command of its own." >&2
  exit 1
fi

attach=()
for f in "$@"; do attach+=(--attach "$f"); done

git push -u origin HEAD

# When an upload fails, gh still creates the PR and exits non-zero; editing
# it with the same flags retries the uploads instead of opening a second PR.
if ! gh pr create --base "$base" --title "$title" --body-file "$body_file" ${attach[@]+"${attach[@]}"}; then
  gh pr view --json number >/dev/null || exit 1
  gh pr edit --body-file "$body_file" ${attach[@]+"${attach[@]}"}
fi

url=$(gh pr view --json url --jq .url)
stored=$(gh pr view --json body --jq .body | tr -d '\r')
images='!\[[^]]*\]\([^)]*\)'
strip() { sed -E 's/(!\[[^]]*\])\([^)]*\)/\1()/g'; }

# CodeRabbit appends its summary to the body, so the file only has to be a
# prefix. gh appends an attachment the body does not reference, which shows
# up as one image more than the file has.
want=$(strip <"$body_file")
have=$(strip <<<"$stored")
count() { { grep -oE "$1" || true; } | wc -l | tr -d ' '; }
wanted=$(count "$images" <"$body_file")
total=$(count "$images" <<<"$stored")
uploaded=$(count '!\[[^]]*\]\(https://github\.com/user-attachments/[^)]*\)' <<<"$stored")

echo "$url"
if [ "${have#"$want"}" = "$have" ]; then
  echo "open-pr: the stored body is not $body_file" >&2
  exit 1
fi
if [ "$total" != "$wanted" ] || [ "$uploaded" != "$wanted" ]; then
  echo "open-pr: $body_file references $wanted diagrams, the stored body has $total images of which $uploaded are uploaded assets" >&2
  exit 1
fi
echo "body: $body_file, $uploaded diagrams uploaded"
