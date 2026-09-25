---
name: ship-change
description: Take an implementation request in lastfirst-api from requirement to a pull request with no actionable review-bot findings left — split it into one PR per change, implement each on its own branch from origin/main, pass the pre-PR checklist and /code-review, open the PR, then repeat review rounds until every CodeRabbit and Codex finding is fixed or declined with evidence. Use for any request here to implement, fix, harden or change behavior ("구현해", "고쳐", "PR 올려"), even when the request does not mention review, branches or PRs.
---

# Ship a change

In this repository a change is finished when no actionable CodeRabbit or Codex
finding remains on its PR and every other finding is declined with evidence,
not when it compiles. PRs #7, #17 and #20 each "worked"
and each went through rounds of findings; #20 bundled seven requirements and
drew 24 comments, because every fix in one area started a new round across all
of them. This skill is the process that avoids that. Follow every step; do not
wait to be told the details.

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
  boundaries** for the paths involved. They are the rules the bots review
  against, so they are authoring rules here.
- When the change edits a process document (skill, checklist, `AGENTS.md`,
  `CLAUDE.md`), every other document that prescribes the same step. Make them
  agree in the same PR.

## 3. Implement and verify

Make the smallest diff that makes the named behavior hold end to end. Add a
deterministic test for each invariant the change establishes when one is
practical, and confirm the test fails without the fix. For a Go change, run:

```bash
test -z "$(gofmt -l .)" && git diff --check && go vet ./... && go build ./... && go test ./...
```

plus `go test -race ./...` for shared state, goroutines, cancellation or
shutdown. For any other change, run `git diff --check`.

## 4. Pre-PR gate

1. Work through `.agents/pre-pr-checklist.md` against `git diff <base>...HEAD`.
2. Run `/code-review` on the branch. Fix what it finds, or note why not.
3. Commit, then record the review with the `touch` command the gate in
   `.claude/hooks/require-code-review.sh` prints when it denies `gh pr create`.

## 5. Open the PR

```bash
git push -u origin HEAD
gh pr create --base <main-or-parent> --title "…" --body-file <file>
```

The body has these sections, in English:

- **Problem** — what fails today, as a concrete scenario.
- **Approach** — what changed and why this shape, including every bound and
  the event it counts from.
- **Validation** — the commands run and what was not exercised.
- **Pre-deploy checks** — anything that depends on infrastructure this
  repository does not define, with the command that verifies it.
- **Out of scope** — related work left to other PRs, naming them.
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
   including uncommon interleavings that break a production invariant. Re-run
   step 3 and the checklist over the new diff before pushing.

Stop and ask the user before fixing when every finding in a round targets a
mechanism added by the previous round's fix: present the simpler design that
does not need that mechanism. After the third round, ask the user whether to
continue before every further round. PR #64 took nine rounds; seven of its
findings came from one mechanism added mid-review.

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
and "Outside the diff" (#32) headings, and CodeRabbit may word them otherwise,
so read every CodeRabbit review body on the PR yourself. A bot that paused,
skipped the PR or ran out of usage without reviewing the current head has not
reviewed it: report it to the user as not run, never as clean.

## 7. Close the loop

When a round surfaces a class of defect `.agents/pre-pr-checklist.md` does not
already name, add it there in a separate `chore/` PR, citing the PR it came
from. The next change should not draw the same finding.

Do not merge. Report each PR's URL and verdict.
