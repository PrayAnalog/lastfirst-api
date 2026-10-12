#!/bin/bash
# Usage: review-state.sh <pr-number>
#
# Reports where the review bots stand on the PR's current head commit and
# ends with one verdict line:
#   verdict: waiting     a bot has not reported on this head yet
#   verdict: blocked     a bot will not report until it is asked again or its
#                        usage resets; the line above names what to post
#   verdict: findings    a review thread, or a CodeRabbit outside-diff
#                        finding, has no reply from you yet
#
# Run it from the worktree that pushed the head: Codex usually answers a clean
# review with a reaction on the PR rather than on a commit, so only the local
# push time says which head that reaction is about. A clean-review comment that
# names its commit needs no push time.
#   verdict: quiet       both bots are done with this head and every open
#                        thread has a reply from you somewhere, even if a bot
#                        replied after it; whether a reply resolved its
#                        finding is not checked
#
# The bots that must report are cubic and Codex, and Greptile once per PR
# that touches cmd/, internal/, deploy/, Dockerfile or go.mod. CodeRabbit no
# longer reviews this repository on its own, so it is not waited for; its
# threads and outside-diff findings still count when it does review.
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
blocked=0

# cubic reports each review as a check run on the commit it reviewed: success
# once the review is posted, neutral when it did not run, such as at a usage
# limit, which it explains in a PR comment. That run stays the latest until a
# requested review starts a new one, so a request posted after it finished
# means the bot is working, not blocked.
cubic=$(gh api "repos/$repo/commits/$head/check-runs?check_name=cubic%20%C2%B7%20AI%20code%20reviewer" \
  --jq '[.check_runs[] | select(.app.slug == "cubic-dev-ai")][0] // empty | "\(.status) \(.conclusion) \(.completed_at)"')
cubic_done_at=${cubic##* }
cubic=${cubic% *}
cubic_asked=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
  --jq ".[] | select(.user.login == \"$me\" and .created_at > \"$cubic_done_at\" and (.body | test(\"^\\\\s*@cubic-dev-ai review\"))) | .id")
case "$cubic" in
  "completed success")
    echo "cubic: reviewed this head"
    ;;
  completed*)
    if [ -n "$cubic_asked" ]; then
      echo "cubic: review requested after it did not run"
      reported=0
    else
      echo "cubic: did not review this head (${cubic#completed }); read its latest PR comment, then comment '@cubic-dev-ai review' once it can run"
      blocked=1
    fi
    ;;
  "")
    echo "cubic: pending; a PR opened before cubic was installed needs '@cubic-dev-ai review this PR'"
    reported=0
    ;;
  *)
    echo "cubic: pending"
    reported=0
    ;;
esac

# Greptile reviews only when asked, and its free plan covers about one review
# per production PR a month, so it is asked once per PR, not per push: the
# latest Greptile Review check run on any commit of the PR counts, and a
# request posted after that run finished means it is working again. A rebase
# drops the reviewed commit from the PR; Greptile's summary comment stays, so
# with no run left it stands for a successful one that finished when the
# comment was last updated.
changed=$(gh pr diff "$pr" --name-only)
if grep -Eq '^(cmd/|internal/|deploy/|Dockerfile$|go\.mod$)' <<<"$changed"; then
  shas=$(gh api --paginate "repos/$repo/pulls/$pr/commits" --jq '.[].sha')
  greptile=""
  for sha in $shas; do
    run=$(gh api "repos/$repo/commits/$sha/check-runs?check_name=Greptile%20Review" \
      --jq '[.check_runs[] | select(.app.slug == "greptile-apps")][0] // empty | "\(.completed_at) \(.status) \(.conclusion)"')
    if [[ "$run" > "$greptile" ]]; then
      greptile=$run
    fi
  done
  if [ -z "$greptile" ]; then
    greptile=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
      --jq '.[] | select(.user.login == "greptile-apps[bot]" and (.body | contains("Confidence Score"))) | "\(.updated_at) completed success"' | tail -n 1)
  fi
  greptile_done_at=${greptile%% *}
  greptile=${greptile#* }
  greptile_asked=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
    --jq ".[] | select(.user.login == \"$me\" and .created_at > \"$greptile_done_at\" and (.body | test(\"^\\\\s*@greptileai(\\\\s+review)?\\\\s*$\"))) | .id")
  if [ -n "$greptile_asked" ]; then
    echo "greptile: pending"
    reported=0
  else
    case "$greptile" in
      "completed success")
        echo "greptile: reviewed this PR"
        ;;
      completed*)
        echo "greptile: did not review this PR (${greptile#completed }); read its latest PR comment, then comment '@greptileai' once it can run"
        blocked=1
        ;;
      "")
        echo "greptile: not asked; comment '@greptileai' on the PR once"
        blocked=1
        ;;
      *)
        echo "greptile: pending"
        reported=0
        ;;
    esac
  fi
fi

# Findings outside the diff exist only in a review body, never as a thread, so
# they stay open until a PR comment from you follows the review, whichever
# commit that review was on.
outside_at=$(gh api --paginate "repos/$repo/pulls/$pr/reviews" \
  --jq ".[] | select(.user.login == \"coderabbitai[bot]\" and (.body | test(\"Outside diff range comments|Outside the diff\"))) | .submitted_at" | tail -n 1)
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
# A clean pass is sometimes a comment naming the commit it reviewed instead of
# a reaction, which ties it to this head without needing the push time.
codex_clean=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
  --jq ".[] | select(.user.login == \"chatgpt-codex-connector[bot]\" and (.body | contains(\"Didn't find any major issues\")))
    | (.body | capture(\"Reviewed commit:\\\\*\\\\* \`(?<sha>[0-9a-f]{7,40})\`\").sha) as \$sha
    | select(\"$head\" | startswith(\$sha)) | .id")
codex_thumb=""
codex_limit=""
if [ -n "$pushed" ]; then
  codex_thumb=$(gh api --paginate "repos/$repo/issues/$pr/reactions" \
    --jq ".[] | select(.user.login == \"chatgpt-codex-connector[bot]\" and .content == \"+1\" and .created_at >= \"$pushed\") | .id")
  # Out of usage until a review is requested after the latest such notice.
  codex_limit=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
    --jq "[.[] | select(.created_at >= \"$pushed\")
      | select((.user.login == \"chatgpt-codex-connector[bot]\" and (.body | test(\"usage limits\")))
          or (.user.login == \"$me\" and (.body | test(\"^\\\\s*@codex review\\\\s*$\"))))]
      | last | select(. != null and .user.login != \"$me\") | .id")
fi
if [ -n "$codex_review" ]; then
  echo "codex: posted findings on this head"
elif [ -n "$codex_clean" ]; then
  echo "codex: no suggestions for this head"
elif [ -z "$pushed" ]; then
  echo "codex: cannot attribute its reactions without this head's push time; run from the worktree that pushed it"
  reported=0
elif [ -n "$codex_thumb" ]; then
  echo "codex: no suggestions for this head"
elif [ -n "$codex_limit" ]; then
  echo "codex: out of usage; comment '@codex review' once the limit resets"
  blocked=1
else
  echo "codex: pending"
  reported=0
fi

threads=""
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
          root: comments(first: 1) { nodes { fullDatabaseId author { login } path line originalLine body } }
          recent: comments(last: 100) { totalCount nodes { fullDatabaseId author { login } } }
        }
      }
    }
  }
}')
  # Only the latest 100 comments of a thread are read. Past that, a reply from
  # you further back is not seen and the thread reads as unanswered, which errs
  # toward reading it again rather than toward calling the PR quiet.
  threads+=$(jq -r --arg me "$me" '.data.repository.pullRequest.reviewThreads.nodes[]
    | select(.isResolved | not)
    | .root.nodes[0] as $c
    | ($c.body | gsub("<details>[\\s\\S]*?</details>"; "")) as $text
    | [.recent.nodes[] | select(.fullDatabaseId != $c.fullDatabaseId) | .author.login] as $who
    | (if ($who | index($me)) == null then "UNANSWERED "
       elif $who[-1] == $me then "answered   "
       else "bot-replied" end)
      + "  \($c.fullDatabaseId)  \($c.author.login)  \($c.path):\($c.line // $c.originalLine)  "
      + (($text | capture("\\*\\*(?<t>[^*]+)\\*\\*").t // ($text | ltrimstr("\n") | split("\n")[0]))
         | sub("<sub>.*</sub>\\s*"; ""))' <<<"$page")
  threads+=$'\n'
  [ "$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage' <<<"$page")" = true ] || break
  cursor=$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor' <<<"$page")
done
threads=$(sed '/^$/d' <<<"$threads")

if [ -n "$threads" ]; then
  echo "open threads:"
  printf '%s\n' "$threads" | sed 's/^/  /'
fi

if grep -q '^UNANSWERED' <<<"$threads" || [ "$outside_open" = 1 ]; then
  echo "verdict: findings"
elif [ "$blocked" = 1 ]; then
  echo "verdict: blocked"
elif [ "$reported" = 0 ]; then
  echo "verdict: waiting"
else
  echo "verdict: quiet"
fi
