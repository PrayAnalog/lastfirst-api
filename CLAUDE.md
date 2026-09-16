# CLAUDE.md

- For any request to implement, fix or change behavior, follow
  `.claude/skills/ship-change/SKILL.md` end to end without waiting to be told
  the steps: one branch and PR per change, then review rounds until
  `review-state.sh` reports `verdict: quiet`.
- Before opening a PR, and before every push that answers review feedback, work
  through `.agents/pre-pr-checklist.md` against the branch diff, then run
  `/code-review` against the base branch. The gate in
  `.claude/hooks/require-code-review.sh` denies `gh pr create` until both have
  happened.
