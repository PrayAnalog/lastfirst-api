#!/bin/bash
# Usage: review-bodies.sh <pr-number>
#
# Prints what section 6 of SKILL.md says to read yourself once review-state.sh
# stops at `verdict: unapproved` or `verdict: quiet`, in one call: every
# CodeRabbit review body on the PR, CodeRabbit's summary comment, and every
# review thread with all of its comments, resolved ones included, because
# CodeRabbit resolves a thread itself once it accepts the reply. HTML
# comments are dropped everywhere. Review bodies keep their <details> blocks,
# which is where CodeRabbit puts findings outside the diff; the summary and
# thread comments drop them, since there they hold the walkthrough, the
# analysis chain and the proposed patch.
set -euo pipefail

pr=${1:?usage: review-bodies.sh <pr-number>}
repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
no_comments='gsub("<!--[\\s\\S]*?-->"; "")'
no_details='gsub("<details>[\\s\\S]*?</details>"; "")'
squeeze='gsub("\\n\\s*\\n(\\s*\\n)+"; "\n\n")'

echo "## CodeRabbit review bodies"
gh api --paginate "repos/$repo/pulls/$pr/reviews" \
  --jq ".[] | select(.user.login == \"coderabbitai[bot]\")
    | \"### \(.state) on \(.commit_id[0:7]) at \(.submitted_at)\n\" + (.body | $no_comments | $squeeze)"

echo
echo "## CodeRabbit summary comment"
gh api --paginate "repos/$repo/issues/$pr/comments" \
  --jq ".[] | select(.user.login == \"coderabbitai[bot]\" and (.body | contains(\"summarize by coderabbit.ai\")))
    | .body | $no_comments | $no_details | $squeeze"

echo
echo "## Review threads"
cursor=""
while :; do
  page=$(gh api graphql -F owner="${repo%/*}" -F name="${repo#*/}" -F pr="$pr" ${cursor:+-F cursor="$cursor"} -f query='
query($owner: String!, $name: String!, $pr: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          isResolved
          comments(first: 100) { nodes { fullDatabaseId author { login } path line originalLine body } }
        }
      }
    }
  }
}')
  jq -r ".data.repository.pullRequest.reviewThreads.nodes[]
    | .comments.nodes as \$c
    | \"### \(\$c[0].fullDatabaseId)  \(\$c[0].path):\(\$c[0].line // \$c[0].originalLine)\(if .isResolved then \"  (resolved)\" else \"\" end)\n\"
      + ([\$c[] | \"[\(.author.login)]\n\" + (.body | $no_comments | $no_details | $squeeze)] | join(\"\n\"))" <<<"$page"
  [ "$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage' <<<"$page")" = true ] || break
  cursor=$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor' <<<"$page")
done
