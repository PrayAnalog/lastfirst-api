#!/bin/bash
# codex-effort.sh review <base>
# codex-effort.sh finding <low|high|both> <fixed|declined> <summary>
# codex-effort.sh report
#
# Runs the local Codex review at low and high reasoning effort and keeps a
# log under the git common directory, shared by every worktree of this clone,
# so the two efforts can be compared once enough pull requests are merged.
set -uo pipefail

dir="$(git rev-parse --path-format=absolute --git-common-dir)/codex-effort" || exit 1
mkdir -p "$dir" || exit 1
efforts=(low high)
target=15

case "${1:-}" in
review)
  base=$2
  head=$(git rev-parse --verify HEAD) || exit 1
  branch=$(git rev-parse --abbrev-ref HEAD) || exit 1
  files=$(git diff --name-only --no-renames "origin/$base...HEAD") || exit 1
  code=false
  [ -n "$(printf '%s\n' "$files" | grep -Ev '(\.md$|^\.claude/|^\.agents/)')" ] && code=true

  for e in "${efforts[@]}"; do
    (
      start=$(date +%s)
      codex review -c model_reasoning_effort="$e" --base "origin/$base" \
        >"$dir/$head-$e.out" 2>"$dir/$head-$e.log"
      echo "$? $(( $(date +%s) - start ))" >"$dir/$head-$e.meta"
    ) &
  done
  wait

  fail=0
  for e in "${efforts[@]}"; do
    read -r rc secs <"$dir/$head-$e.meta"
    jq -nc --arg time "$(date -u +%FT%TZ)" --arg branch "$branch" --arg head "$head" \
      --arg base "$base" --arg effort "$e" --argjson seconds "$secs" --argjson exit "$rc" \
      --argjson code "$code" --rawfile output "$dir/$head-$e.out" \
      '{time: $time, branch: $branch, head: $head, base: $base, effort: $effort,
        seconds: $seconds, exit: $exit, code: $code, output: $output}' \
      >>"$dir/runs.jsonl" || exit 1
    echo "=== codex review, $e effort, ${secs}s, exit $rc"
    cat "$dir/$head-$e.out"
    if [ "$rc" -ne 0 ]; then
      echo "transcript: $dir/$head-$e.log"
      fail=1
    fi
  done
  exit "$fail"
  ;;
finding)
  head=$(git rev-parse --verify HEAD) || exit 1
  branch=$(git rev-parse --abbrev-ref HEAD) || exit 1
  jq -nc --arg time "$(date -u +%FT%TZ)" --arg branch "$branch" --arg head "$head" \
    --arg raised "$2" --arg outcome "$3" --arg summary "$4" \
    '{time: $time, branch: $branch, head: $head, raised: $raised,
      outcome: $outcome, summary: $summary}' >>"$dir/findings.jsonl"
  ;;
report)
  if [ ! -f "$dir/runs.jsonl" ]; then
    echo "merged code PRs reviewed at both efforts: 0/$target"
    exit 0
  fi
  reviewed=$(jq -r 'select(.code) | .branch' "$dir/runs.jsonl" | LC_ALL=C sort -u) || exit 1
  merged=$(gh pr list --state merged --limit 1000 --json headRefName --jq '.[].headRefName' | LC_ALL=C sort -u) || exit 1
  selected=$(LC_ALL=C comm -12 <(printf '%s\n' "$reviewed") <(printf '%s\n' "$merged") | grep .)
  n=$(printf '%s' "$selected" | grep -c .)
  echo "merged code PRs reviewed at both efforts: $n/$target"
  [ "$n" -ge "$target" ] || exit 0
  branches=$(printf '%s\n' "$selected" | jq -Rsc 'split("\n") | map(select(. != ""))') || exit 1
  jq -s --argjson b "$branches" 'map(select(.code and (.branch | IN($b[])))) | group_by(.effort) | map({effort: .[0].effort, runs: length,
    failed: map(select(.exit != 0)) | length,
    mean_seconds: (map(.seconds) | add / length)})' "$dir/runs.jsonl" || exit 1
  if [ -f "$dir/findings.jsonl" ]; then
    jq -s --argjson b "$branches" 'map(select(.branch | IN($b[]))) | group_by([.raised, .outcome]) | map({raised: .[0].raised,
      outcome: .[0].outcome, count: length})' "$dir/findings.jsonl" || exit 1
  else
    echo "no findings recorded"
  fi
  ;;
*)
  echo "usage: codex-effort.sh review <base> | finding <low|high|both> <fixed|declined> <summary> | report" >&2
  exit 2
  ;;
esac
