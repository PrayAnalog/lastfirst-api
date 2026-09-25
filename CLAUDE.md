# CLAUDE.md

- For any request to implement, fix or change behavior, follow
  `.claude/skills/ship-change/SKILL.md` end to end without waiting to be told
  the steps: one branch and PR per change, then review rounds until
  `review-state.sh` reports `verdict: quiet` or the user stops them at one of
  the skill's questions.
- Before opening a PR, and before every push that answers review feedback, work
  through `.agents/pre-pr-checklist.md` against the branch diff, then run
  `/code-review` and the OCR review in `.agents/ocr-review.md` against the base
  branch. The gate in `.claude/hooks/require-code-review.sh` denies
  `gh pr create` until all of these have happened.
