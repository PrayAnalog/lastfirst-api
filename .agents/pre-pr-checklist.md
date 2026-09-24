# Pre-PR Checklist

Work through this against the branch diff before `gh pr create`, and again
before every push that answers review feedback. Each item stands for a defect
that reached review in this repository at least once.

## 0. The diff is one change

Describe the diff in one sentence. If the sentence needs "and" to cover two
behaviors that could be reverted separately, split the branch: #20 carried
seven such behaviors and every fix to one of them reopened review of all the
others. A change whose bounds are written over another change's bounds is
stacked on that change's branch rather than folded into it.

## 1. Walk the named behavior as an event sequence

Write down the sequence the request names — signal arrives, listener closes,
handler returns, process exits — and point at the line that performs each step.
A call that *starts* the behavior is not the behavior: `Shutdown` invoked from a
goroutine nobody waits on drains nothing.

## 2. Account for every error the diff can produce

For each goroutine, channel, and call the diff adds, name the line that receives
its error on *every* path through the function, including paths that return
early or take a different `select` branch. A buffered channel written by a
goroutine and read on only one path loses real failures silently.

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
`ReadTimeout`, `WriteTimeout`, `IdleTimeout`, shutdown deadline,
`terminationGracePeriodSeconds`, `preStop`, probe timings, the ingress proxy
timeouts in front of the handler — next to the event each one starts counting
from. Bounds that start at different events are not comparable without the gap
between those events as headroom. Write each wider bound as an expression over
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

An application-side lifecycle change is unfinished until the manifests agree:
closing the listener on `SIGTERM` still returns errors to clients while the
pod is in the Service endpoint list, so the drain needs a `preStop` delay and
a grace period that covers it. The same applies to trusted headers (who sets
it), probes (what they exercise), and image entrypoints (what receives the
signal).

Two far sides are easy to miss. A limit the application enforces — body size,
request duration — is enforced first by the ingress in front of it when the
ingress limit is tighter, and clients then get the ingress's answer instead of
the application's. And state kept in process memory — the daily budget, the
per-IP limiter — is multiplied by every pod alive at once, which includes the
old pod still draining during a rollout or eviction.

A strategy that sequences revision updates may not govern replacement after an
ordinary pod deletion. Verify the workload controller's behavior for rollouts,
evictions, and deletions. If the workload kind or object identity changes,
remember that `kubectl apply` does not prune the old controller: document a
scale-to-zero, wait-for-deletion, and removal sequence before creating the new
one.

## 6. Re-read the authoring rules for the paths touched

Re-read `path_instructions` in `.coderabbit.yaml` and **Production boundaries**
in `AGENTS.md` for each directory in the diff, and apply them as authoring
rules, not just review rules.

## 7. Separate what this repository can fix

A finding that depends on infrastructure this repository does not define —
load balancer source-IP preservation, ingress controller labels — is not fixed
by guessing at a manifest. Record it in the PR body as a pre-deploy check or an
explicit non-goal, with the reason it cannot be settled here, and answer the
review thread with that same repository evidence.

## 8. Decide every default the diff makes explicit

Replacing an implicit construct with an explicit one — `http.ListenAndServe`
with an `http.Server`, a default client with a configured one — puts its zero
values in the diff, and review treats them as decisions this change made. Set
each field the construct exposes deliberately, or say in the PR body why the
zero value is correct.

## 9. Commit a regression test that fails on the base

A bug fix ships with a deterministic test that fails against the base branch
and passes on the fix. #63 closed a spoofable rate-limit key, checked the fix
with a table test, and deleted the test before committing; review sent it back
because nothing would stop a later edit from restoring the bypass. Run the new
test against the base implementation before pushing and name the cases that
failed there in the PR body. When a deterministic test is genuinely
impractical, say why in the PR body instead.

## 10. Answer each review round in the thread

Sort the findings as `ship-change` step 6 does: ask before a fix that needs new
state, permissions or triggers, decline accepted risks with the sequence they
need, and fix the other valid findings in a commit. Then reply to every thread
with the commit that addressed it and what changed, or with the reason it is
being declined.
Re-run this checklist over the new diff before pushing.
