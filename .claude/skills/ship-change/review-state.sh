#!/bin/bash
# Usage: review-state.sh <pr-number>
#
# Reports where the review bots stand on the PR's current head commit and
# ends with one verdict line:
#   verdict: waiting     a bot has not reported on this head yet
#   verdict: findings    a review thread, or a CodeRabbit outside-diff
#                        finding, has no reply from you yet
#
# Run it from the worktree that pushed the head: Codex answers a clean review
# with a reaction on the PR rather than on a commit, so only the local push
# time says which head that reaction is about.
#   verdict: unapproved  every thread is answered but CodeRabbit has not
#                        approved this head; read its latest review body
#                        and any thread marked bot-replied
#   verdict: quiet       both bots are done with this head and nothing is open
set -euo pipefail

pr=${1:?usage: review-state.sh <pr-number>}
repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
me=$(gh api user --jq .login)
head=$(gh pr view "$pr" --json headRefOid --jq .headRefOid)
branch=$(gh pr view "$pr" --json headRefName --jq .headRefName)

pushed=$(TZ=UTC git reflog show --date=format-local:%Y-%m-%dT%H:%M:%SZ --format='%H %gd' \
  "refs/remotes/origin/$branch" 2>/dev/null | awk -v h="$head" '$1 == h' | tail -n 1 |
  sed -n 's/.*@{\(.*\)}$/\1/p' || true)

echo "head: ${head:0:7} (pushed ${pushed:-at an unknown time from this checkout})"
reported=1

# CodeRabbit does not always leave a review object on the head it covered: a
# pass with nothing to add only updates its summary comment, whose
# final_review_risk_coverage marker names the commit it reached.
rabbit_head_review=$(gh api --paginate "repos/$repo/pulls/$pr/reviews" \
  --jq ".[] | select(.user.login == \"coderabbitai[bot]\" and .commit_id == \"$head\") | .id")
rabbit_verdict=$(gh api --paginate "repos/$repo/pulls/$pr/reviews" \
  --jq ".[] | select(.user.login == \"coderabbitai[bot]\" and (.state == \"APPROVED\" or .state == \"CHANGES_REQUESTED\")) | .state" | tail -n 1)
rabbit_summary=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
  --jq '.[] | select(.user.login == "coderabbitai[bot]" and (.body | contains("summarize by coderabbit.ai"))) | .body')
if [ -n "$rabbit_head_review" ] || grep -q "\"coveredCommitId\":\"$head\"" <<<"$rabbit_summary"; then
  echo "coderabbit: covered this head, latest decision on the PR: ${rabbit_verdict:-none}"
elif grep -qE 'Reviews paused|Review skipped' <<<"$rabbit_summary"; then
  echo "coderabbit: paused or skipped; comment '@coderabbitai review' on the PR"
  reported=0
else
  echo "coderabbit: pending"
  reported=0
fi

# Findings outside the diff exist only in a review body, never as a thread, so
# they stay open until a PR comment from you follows the review.
outside_at=$(gh api --paginate "repos/$repo/pulls/$pr/reviews" \
  --jq ".[] | select(.user.login == \"coderabbitai[bot]\" and .commit_id == \"$head\" and (.body | contains(\"Outside diff range comments\"))) | .submitted_at" | tail -n 1)
outside_open=0
if [ -n "$outside_at" ]; then
  replied=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
    --jq ".[] | select(.user.login == \"$me\" and .created_at > \"$outside_at\" and (.body | test(\"^\\\\s*@(codex|coderabbitai) \\\\w+\\\\s*$\") | not)) | .id")
  if [ -z "$replied" ]; then
    echo "coderabbit: outside-diff findings in its review body of $outside_at are unanswered"
    outside_open=1
  fi
fi

codex_review=$(gh api --paginate "repos/$repo/pulls/$pr/reviews" \
  --jq ".[] | select(.user.login == \"chatgpt-codex-connector[bot]\" and .commit_id == \"$head\") | .id")
codex_thumb=""
codex_limit=""
if [ -n "$pushed" ]; then
  codex_thumb=$(gh api --paginate "repos/$repo/issues/$pr/reactions" \
    --jq ".[] | select(.user.login == \"chatgpt-codex-connector[bot]\" and .content == \"+1\" and .created_at >= \"$pushed\") | .id")
  codex_limit=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
    --jq ".[] | select(.user.login == \"chatgpt-codex-connector[bot]\" and .created_at >= \"$pushed\" and (.body | test(\"usage limits\"))) | .id")
fi
if [ -n "$codex_review" ]; then
  echo "codex: posted findings on this head"
elif [ -z "$pushed" ]; then
  echo "codex: cannot attribute its reactions without this head's push time; run from the worktree that pushed it"
  reported=0
elif [ -n "$codex_thumb" ]; then
  echo "codex: no suggestions for this head"
elif [ -n "$codex_limit" ]; then
  echo "codex: out of usage; comment '@codex review' once the limit resets"
  reported=0
else
  echo "codex: pending"
  reported=0
fi

threads=$(gh api graphql -F owner="${repo%/*}" -F name="${repo#*/}" -F pr="$pr" -f query='
query($owner: String!, $name: String!, $pr: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100) {
        nodes {
          isResolved
          root: comments(first: 1) { nodes { databaseId author { login } path line originalLine body } }
          all: comments(first: 100) { nodes { author { login } } }
        }
      }
    }
  }
}' --jq ".data.repository.pullRequest.reviewThreads.nodes[]
  | select(.isResolved | not)
  | .root.nodes[0] as \$c
  | [.all.nodes[].author.login] as \$who
  | (if (\$who | index(\"$me\")) == null then \"UNANSWERED \"
     elif \$who[-1] == \"$me\" then \"answered   \"
     else \"bot-replied\" end)
    + \"  \(\$c.databaseId)  \(\$c.author.login)  \(\$c.path):\(\$c.line // \$c.originalLine)  \"
    + ((\$c.body | capture(\"\\\\*\\\\*(?<t>[^*]+)\\\\*\\\\*\").t // (\$c.body | split(\"\\n\")[0]))
       | sub(\"<sub>.*</sub>\\\\s*\"; \"\"))")

if [ -n "$threads" ]; then
  echo "open threads:"
  printf '%s\n' "$threads" | sed 's/^/  /'
fi

if grep -q '^UNANSWERED' <<<"$threads" || [ "$outside_open" = 1 ]; then
  echo "verdict: findings"
elif [ "$reported" = 0 ]; then
  echo "verdict: waiting"
elif [ "$rabbit_verdict" != APPROVED ]; then
  echo "verdict: unapproved"
else
  echo "verdict: quiet"
fi
