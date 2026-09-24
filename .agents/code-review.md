# Code Review Guidance

Report a finding only when this diff introduces or worsens a concrete failure.
Name the trigger, the deployed configuration it runs under, the callers
involved, the impact, and the smallest correction. Do not report a deployment
assumption you have not confirmed in `deploy/`, `Dockerfile` or `AGENTS.md` as
a defect. Do not block on taste, naming, comments, or formatter output.

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

If a review request is technically unsound, respond with repository evidence
and trade-offs. Report valid findings; apply fixes and rerun checks only when the
user explicitly requests implementation.
