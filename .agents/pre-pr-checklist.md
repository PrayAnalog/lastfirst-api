# Pre-PR Checklist

Work through this against the branch diff before `gh pr create`, and again
before every push that answers review feedback. Each item stands for a defect
that reached review in this repository at least once.

## 0. The diff is one change

Describe the diff in one sentence. If the sentence needs "and" to cover two
behaviors that could be reverted separately, split the branch: with several
such behaviors in one PR, every fix to one of them reopens review of all the
others. A change whose bounds are written over another change's bounds is
stacked on that change's branch rather than folded into it. Merged without the
PRs stacked above it, this diff must not break or worsen a production invariant
`AGENTS.md` declares; if it needs one piece of a PR above it, move just that
piece down into this one.

## 1. Walk the named behavior as an event sequence

Write down the sequence the request names — signal arrives, listener closes,
handler returns, process exits — and point at the line that performs each step.
A call that *starts* the behavior is not the behavior: `Shutdown` invoked from a
goroutine nobody waits on drains nothing.

## 2. Account for every error the diff can produce

For each goroutine, channel, and call the diff adds, name where its error goes
on *every* path through the function, including paths that return early or
take a different `select` branch, and say so when the rule below lets it be
ignored. A buffered channel written by a goroutine and read on only one path
loses real failures silently.

Handle an error when ignoring it changes state, the response, or resource
cleanup, or hides a failure someone could act on. A test that ignores a `RoundTrip` error can pass
while the request failed, so it counts; a health handler's `w.Write` error to
a client that has gone away does not, because nothing can still reach it.

Sanitizing an upstream error for clients must not erase the server-side failure
category. Keep client responses stable and secret-free while retaining a safe
HTTP status, DNS, TLS, timeout, or network category in logs; never copy raw
provider bodies or credential-bearing URLs to recover diagnostic detail.

## 3. Derive coupled limits from one source and order them

List every bound the change touches — handler deadline, `ReadHeaderTimeout`,
`ReadTimeout`, `WriteTimeout`, `IdleTimeout`, shutdown deadline, the
container's `stop_grace_period`, and the Caddy timeouts in front of the
handler — next to the event each one starts counting from. Bounds that start
at different events are not comparable without the gap between those events
as headroom. Write each wider bound as an expression over
the narrower one so the ordering cannot drift when one value is edited.

## 4. Claim a metered resource per call, not per prediction

Reserve before the call, never after: a call made in order to size the
reservation has already spent the resource.

Then ask what sizes the reservation. A count predicted up front has been wrong
here in every direction — short pages, a playlist that grew mid-request, a
request billed and then failed, a walk that outlived the daily reset — and each
was found separately, as its own review round, because each was a different
way for one guess to be off. Claiming a single unit immediately before each
call deletes the prediction and the reconciliation that keeps being subtly
wrong along with it.

Where a claim genuinely has to be made in bulk, size the settlement by attempts
made rather than results returned, tie it to the accounting period it was taken
in, and count the calls that still happen when the prediction is zero.

Do not claim for a call that provably cannot happen: a context already done
means the request never reaches the far side, so it cannot be billed. Do not
refund on a failure that cannot be told apart from a success that was billed —
over-charging stops the service early, under-charging spends what the meter is
there to protect.

## 5. Check the far side of every boundary the change touches

An application-side lifecycle change is unfinished until `deploy/` agrees:
`docker compose up -d app` sends `SIGTERM` to the old container and kills it
once its `stop_grace_period` (10s when unset) runs out, so the drain has to fit
inside that, and Caddy retries a request whose upstream refuses the connection
only for its `lb_try_duration`. The same applies to trusted headers (who sets
it), health checks (what they exercise), and image entrypoints (what receives
the signal).

Two far sides are easy to miss. A limit the application enforces — body size,
request duration — is enforced first by Caddy when Caddy's limit is tighter,
and clients then get Caddy's answer instead of the application's. And state
kept in process memory — the daily budget, the per-IP limiter — is multiplied
by every app container alive at once; keep the one-container rule in
`AGENTS.md` **Production boundaries** true for every change to `deploy/`.

## 6. Re-read the authoring rules for the paths touched

Re-read `path_instructions` in `.coderabbit.yaml` and **Production boundaries**
in `AGENTS.md` for each directory in the diff, and apply them as authoring
rules, not just review rules.

## 7. Separate what this repository can fix

A finding that depends on infrastructure this repository does not define —
the droplet's firewall, DNS, or its `.env` — is not fixed by guessing at a
manifest. Record it in the PR body as a pre-deploy check or an explicit
non-goal, with the reason it cannot be settled here, and answer the review
thread with that same repository evidence.

## 8. Decide every default the diff makes explicit

Replacing an implicit construct with an explicit one — `http.ListenAndServe`
with an `http.Server`, a default client with a configured one — puts its zero
values in the diff, and review treats them as decisions this change made. Set
each field the construct exposes deliberately, or say in the PR body why the
zero value is correct.

## 9. Commit a regression test that fails on the base

A bug fix ships with a deterministic test that fails against the base branch
and passes on the fix, and the test stays in the commit: a fix checked with a
test that is then deleted leaves nothing to stop a later edit from restoring
the bug. Run the new test against the base implementation before pushing and
name the cases that failed there in the PR body. The failure has to be the
bug itself: the assertion that names it, or the panic or race report the bug
directly causes. A test that does not compile, or whose fixture breaks, on the
base proves nothing. When the code under test does not exist on the base, add
a minimal temporary scaffold that lets the test reach its assertion and
returns something other than the expected result, and watch the assertion fail
against it. When a deterministic test is genuinely impractical, say why in the
PR body instead.

Check every test the diff adds, changes or removes against **Test changes** in
`.agents/code-review.md`; Codex reviews tests against it.

## 10. Answer each review round in the thread

Sort the findings as `ship-change` step 6 does: ask before a fix that needs new
state, permissions or triggers, decline accepted risks with the sequence they
need, and fix the other valid findings in a commit. Then reply to every thread
with the commit that addressed it and what changed, or with the reason it is
being declined.
Re-run this checklist over the new diff, then a code review against the base
branch, before pushing. Fix what the review finds, or record why not.
