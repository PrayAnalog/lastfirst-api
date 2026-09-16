# Pre-PR Checklist

Work through this against the branch diff before `gh pr create`, and again
before every push that answers review feedback. Each item stands for a defect
that reached review in this repository at least once.

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

## 3. Derive coupled limits from one source and order them

List every bound the change touches — handler deadline, `ReadHeaderTimeout`,
`ReadTimeout`, `WriteTimeout`, `IdleTimeout`, shutdown deadline,
`terminationGracePeriodSeconds`, `preStop`, probe timings — next to the event
each one starts counting from. Bounds that start at different events are not
comparable without the gap between those events as headroom. Write each wider
bound as an expression over the narrower one so the ordering cannot drift when
one value is edited.

## 4. Settle every resource claim on every exit path

- Reserve before the billable call, never after. A call made in order to size
  the reservation has already spent the resource.
- Size the settlement by *attempts made*, not by results returned: a request
  that failed may still have been billed, and a successful page may return
  fewer items than its maximum.
- Account for calls that happen even when the predicted count is zero.
- Tie the claim to the accounting period it was taken in and ignore a
  settlement whose period has already rolled over.

## 5. Check the far side of every boundary the change touches

An application-side lifecycle change is unfinished until the manifests agree:
closing the listener on `SIGTERM` still returns errors to clients while the
pod is in the Service endpoint list, so the drain needs a `preStop` delay and
a grace period that covers it. The same applies to trusted headers (who sets
it), probes (what they exercise), and image entrypoints (what receives the
signal).

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

## 8. Answer each review round in the thread

Fix the valid findings in a commit, then reply to every thread with the commit
that addressed it and what changed, or with the reason it is being declined.
Re-run this checklist over the new diff before pushing.
