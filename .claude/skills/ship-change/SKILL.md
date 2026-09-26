---
name: ship-change
description: Take an implementation request in lastfirst-api from requirement to a pull request with no actionable review-bot findings left — split it into one PR per change, implement each on its own branch from origin/main, pass the pre-PR checklist, /code-review and the OCR review, open the PR, then repeat review rounds until every CodeRabbit and Codex finding is fixed or declined with evidence. Use for any request here to implement, fix, harden or change behavior ("구현해", "고쳐", "PR 올려"), even when the request does not mention review, branches or PRs.
---

# Ship a change

In this repository a change is finished when no actionable CodeRabbit or Codex
finding remains on its PR and every other finding is declined with evidence,
not when it compiles. A PR that bundles several requirements draws review
rounds across all of them, because every fix in one area reopens review of the
rest; this skill keeps each change small enough to converge. Follow every
step.

## 1. Split the request into changes

List the behaviors the request names. Each one that could be reverted on its
own is its own change, its own branch and its own PR. Write the list down with
a branch name for each (`fix/…`, `feat/…`, `chore/…`) before editing anything.

A change whose bounds are derived from another change's bounds — a shutdown
deadline written over a write timeout, a grace period written over a shutdown
deadline — is *stacked*: branch it from the parent's branch and open its PR
with `--base <parent-branch>`. Every other change branches from `origin/main`.
Do not bundle coupled changes into one PR to avoid stacking.

Every PR in a stack has to be safe to merge without the ones above it: merged
alone, it must not break or worsen a production invariant `AGENTS.md` declares,
in **Scope** or **Production boundaries**. When a PR is unsafe without one
piece of a PR above it, move just that piece down into it rather than folding
the whole stack into one PR. This covers what the diff changes, not debt that
was already there.

`main` is checked out in another worktree, so `git switch -c <branch>
origin/main`; never `git checkout main`.

## 2. Read before writing

- The files the change touches, their callers, and the far side of every
  boundary it crosses (`deploy/`, `Dockerfile`, Caddy in front).
- `go doc` for each library call the change depends on. Write from the doc,
  not from memory.
- `.coderabbit.yaml` `path_instructions` and `AGENTS.md` **Production
  boundaries** for the paths involved, and **Test changes** in
  `.agents/code-review.md` when the change adds, changes or removes a test.
  They are the rules the bots review against, so they are authoring rules
  here.
- When the change edits a process document (skill, checklist, `AGENTS.md`,
  `CLAUDE.md`), every other document that prescribes the same step. Make them
  agree in the same PR.

## 3. Implement and verify

Make the smallest diff that makes the named behavior hold end to end. Add a
deterministic test for each invariant the change establishes when one is
practical, and confirm the test fails without the fix for the reason the bug
causes, not on a compile or fixture error. For a Go change, run:

```bash
test -z "$(gofmt -l .)" && git diff --check && go vet ./... && go build ./... && go test ./...
```

plus `go test -race ./...` for shared state, goroutines, cancellation or
shutdown. For any other change, run `git diff --check`.

## 4. Pre-PR gate

1. Commit. Every review below covers `git diff <base>...HEAD`.
2. Work through `.agents/pre-pr-checklist.md`, then run `/code-review` and the
   OCR review in `.agents/ocr-review.md`. Fix what they find, or note why not.
   If any fix changes the diff, take it back through section 3 (Implement and
   verify), then through this section again.
3. Record the review with `.claude/skills/ship-change/record-review.sh <base>`,
   run as a command of its own, where `<base>` is the `--base` the PR will be
   opened with. It writes the stamp the gate in
   `.claude/hooks/require-code-review.sh` checks, so `gh pr create` then runs
   on the first try.

## 5. Open the PR

First draw the change with the `pr-lens` skill (`.claude/skills/pr-lens`):
write `.pr-lens/graph.json` from `git diff --find-renames <base>...HEAD`,
validate it, and render it in the light theme. Run the CLI as
`npx @coldtea/pr-lens-cli@0.7.0` wherever the skill writes `@latest`, and
change that version only in the same PR that updates the vendored skill.
Attaching needs GitHub CLI 2.99 or later; check `gh --version`.

Write the body to a file named for this PR's branch in the session scratchpad,
not to a shared path such as `.pr-lens/body.md`, which other sessions in the
worktree overwrite. Write it in its own step, never in the command that runs
`gh pr create`: when the gate denies a command, none of it runs, so a file
written inside it is never written and the retry sends whatever the path held
before.

```bash
git push -u origin HEAD
gh pr create --base <main-or-parent> --title "…" --body-file <file> \
  --attach .pr-lens/<view>-light-<hash>.svg
```

Repeat `--attach` for each diagram the body references. When an upload fails,
`gh` still creates the PR, prints its URL and exits non-zero; run
`gh pr edit <pr> --body-file <file>` with the same `--attach` flags rather than
creating the PR again.

Then read the body back with `gh pr view <pr> --json body` and check that it is
this PR's text and that each diagram sits under **Diagram** as an uploaded
asset. `gh` appends an attachment the body does not reference to the end of the
body, so a diagram at the bottom means the body is not the one written for this
PR.

Write the title and body in Korean. The body has these sections, under the
English headings below:

- **Problem** — what fails today, as a concrete scenario.
- **Diagram** — the architecture view, then the data-flow view when the change
  has a sequence worth following, each as `![alt](.pr-lens/<file>.svg)` with
  the same path passed to `--attach`.
- **Approach** — what changed and why this shape, including every bound and
  the event it counts from.
- **Validation** — what section 3 did not cover: commands skipped or added,
  tests added and whether they fail without the fix, and what was not
  exercised. Do not list the section 3 commands or the section 4 reviews
  when they ran as written, and omit the section when nothing else is left.
- **Pre-deploy checks** — anything that depends on infrastructure this
  repository does not define, with the command that verifies it. Omit the
  section when there are none.
- **Out of scope** — related work this PR leaves to other PRs, naming them.
  Name only PRs this one builds on or overlaps with; list a whole series in its
  tracking issue, not in every PR. Omit the section when there is nothing to
  name.
- **Known limitations** — findings declined as accepted risk, each with the
  sequence it needs. Omit the section when there are none.

For a stacked PR, say which PR it builds on. CodeRabbit skips PRs whose base is
not `main`; comment `@coderabbitai review` on those.

## 6. Review rounds

Wait for both bots on the current head, without polling by hand:

```bash
until .claude/skills/ship-change/review-state.sh <pr> | grep -Eq '^verdict: (findings|blocked|unapproved|quiet)'; do sleep 60; done
.claude/skills/ship-change/review-state.sh <pr>
```

Run the loop in the background (Monitor) and move on to another PR meanwhile.
On `verdict: blocked`, post the comment the script names for the blocked bot,
then ask the user whether to wait for it or finish without it.

For each open finding, read the code it points at, then take the first of
these that applies:

1. **Not valid, or not fixable in this repository** — decline with repository
   evidence: file and line, doc quote, or a reproduction. A finding that needs
   infrastructure the repository does not define goes into **Pre-deploy
   checks** in the PR body instead of a guessed manifest.
2. **Needs new state, permissions or triggers** — the fix would add state
   carried across events, a new permission, or a new trigger. Ask the user
   before fixing it, however the finding would otherwise be classified.
3. **Accepted risk** — the failure needs a theoretical sequence, such as
   concurrent manual actions or limits no real change reaches, and what fails is
   an advisory control that other safeguards still cover, never a production
   invariant from `AGENTS.md`. Decline it with the sequence it needs and why the
   risk is accepted, and list it under **Known limitations** in the PR body.
4. **Valid** — fix it in a commit whose message names each defect it fixes,
   including uncommon interleavings that break a production invariant. Take
   the new diff through sections 3 and 4 again before pushing.

Stop and ask the user before fixing when every finding in a round targets a
mechanism added by the previous round's fix: present the simpler design that
does not need that mechanism. After the third round, ask the user whether to
continue before every further round.

Answer every finding in its own thread, naming the commit and what changed, or
the evidence for declining:

```bash
gh api repos/PrayAnalog/lastfirst-api/pulls/<pr>/comments/<comment-id>/replies -f body="…"
```

Findings posted only in a review body ("Outside diff range comments" or
"Outside the diff") get a PR comment instead. Thread replies are written in
Korean, matching CodeRabbit's configured language.

Push, update the PR body if the approach changed, and wait again. Stop when
the script prints `verdict: quiet`, or when the user decides to stop at one of
the questions above. `verdict: unapproved` means every
thread is answered but CodeRabbit has not approved; read its latest review
body, its summary comment and every thread marked `bot-replied` before
deciding whether anything is still open.

`quiet` counts replies, not resolutions. Before reporting a PR as done, re-read
each finding against the code its reply points to and check that its failure
scenario is gone, or that the reply gives repository evidence for declining it.
The script finds outside-diff findings by their "Outside diff range comments"
and "Outside the diff" headings, and CodeRabbit may word them otherwise,
so read every CodeRabbit review body on the PR yourself. A bot that paused,
skipped the PR or ran out of usage without reviewing the current head has not
reviewed it: report it to the user as not run, never as clean.

## 7. Close the loop

When a round surfaces a class of defect `.agents/pre-pr-checklist.md` does not
already name, add it there in a separate `chore/` PR, citing the PR it came
from. The next change should not draw the same finding.

Do not merge. Report each PR's URL and verdict.
