# CLAUDE.md

- Before opening a PR, and before every push that answers review feedback, work
  through `.agents/pre-pr-checklist.md` against the branch diff, then run
  `/code-review` against the base branch. The gate in
  `.claude/hooks/require-code-review.sh` denies `gh pr create` until both have
  happened.
