# Code Review Guidance

Report a finding only when this diff introduces or worsens a concrete failure.
Name the trigger, the deployed configuration it runs under, the callers
involved, the impact, and the smallest correction. Do not report a deployment
assumption the repository does not confirm, in `deploy/`, `Dockerfile`,
`README.md`, `AGENTS.md` or elsewhere, as a defect. Do not block on taste, naming, comments, or formatter output.

In service code, `Dockerfile` and `deploy/`, trace quota attempts and the
period they belong to, bounded input, cancellation, listener and handler
shutdown, and the one-app-container boundary to the end. Check shared state
reached by concurrent handlers, resource cleanup, secrets and internal errors
in responses, and HTTP status and JSON shape. Check these edges: the metadata
call made before any page; the first call for an empty playlist; zero, one,
maximum and overflow pages; short pages and a playlist that grows mid-walk;
billing of a failed call; cancellation between calls; a release after the quota
period rolled over; a new client after the per-IP table is full; a signal
arriving together with a listener failure. Count call attempts at the external
boundary, not result sizes. Read a bounded body to EOF before expensive work.
Keep client responses to sanitized upstream failures stable while server logs
retain a non-secret category. Fit the shutdown budget inside the container's
`stop_grace_period`, and keep `deploy/` running exactly one app container,
stopping the old one before the new one starts.

In development tooling, put failures in normal use and real trust boundaries
first. Do not ask for state that guards against a rare skip of an advisory
reviewer, concurrent manual actions, or scale no real change reaches.

Separate existing debt, policy choices and speculation from required fixes; a
default the diff newly makes unsafe counts as introduced. Ask for a test only
when you can name the regression its absence lets through, and check that a
proposed test fails without the fix. Set P1 by real impact and reachability,
not by which rule the diff breaks. Mark optional polish as non-blocking.

Report, for example: a failed page call whose quota reservation is refunded
although YouTube may have billed it; `main` returning before `Shutdown` has
drained in-flight requests. Do not report, for example: tracking recently
closed sibling PRs so an advisory review never skips.

## Test changes

Apply the same concrete-failure threshold to tests, whether written by a human
or an LLM. Review added, changed and removed tests together with the behavior
they protect. Name an incorrect implementation the test would accept, a valid
change it would reject, or a reachable source of flaky failures. Do not turn
this into a coverage target, a required unit/integration/E2E ratio, or a demand
for tests on every function. Missing coverage alone is not a finding.

- Derive expected results from the public contract, a stated requirement or an
  independently checked example. Flag assertions that calculate their expected
  result using the operation under test, duplicate its faulty logic, or merely
  assert success while accepting a concrete wrong result. Existing behavior can
  be intentionally preserved by characterization tests; do not treat it as proof
  that the behavior satisfies the requirements.
- Check that the test reaches the production behavior it claims to protect.
  A mock returning the asserted value cannot establish that the replaced logic
  works. Prefer observable outputs and contractual side effects over private
  structure or incidental call order. Call counts and ordering remain meaningful
  when they enforce billing, cancellation or another real contract; do not flag
  interaction assertions just because they use mocks.
- Compare deleted tests, weakened assertions and updated snapshots with the
  intended behavior change. Flag lost protection for a named, still-required
  behavior, including suites regenerated to match a faulty implementation.
  Legitimate requirement changes can require new expectations. Request the
  smallest correction that restores protection, not blanket test immutability.
- For bug fixes, verify that the regression test fails on the unfixed behavior
  for the intended reason and passes with the fix. Compilation or fixture
  failures do not demonstrate bug detection. If execution is unavailable,
  distinguish reasoning from observed results. For high-risk assertions, a
  targeted temporary mutation can help verify a suspected blind spot; restore
  it afterwards. Do not require repository-wide mutation testing or infer
  correctness from coverage, passing tests or mutation scores alone.
- Check reachable nondeterminism: sleeps used to assume goroutine progress,
  shared state leaking between tests, uncontrolled clocks or live services in
  otherwise isolated tests. Name how normal execution can produce a false
  result. For concurrent invariants, check the asserted state transition;
  passing the race detector alone does not establish logical correctness.

Prefer the smallest test scope that exposes the named regression. Recommend a
broader test only when the failure crosses a boundary the smaller test replaces
or omits. Duplicate cases across layers, helper style and test count alone are
not blocking findings. Property checks need an adequate oracle too: for example,
a no-op implementation satisfies "reversing twice returns the original", so
that property alone does not establish reversal.

If a review request is technically unsound, respond with repository evidence
and trade-offs. Report valid findings; apply fixes and rerun checks only when the
user explicitly requests implementation.
