# Repository Instructions

## Purpose and scope

This repository is the Go backend for `lastfirst.app`. It serves the built
frontend, accepts playlist input over HTTP, calls the YouTube Data API, and is
deployed to Kubernetes. Treat public API behavior, YouTube quota use, and clean
connection draining as production concerns.

Keep this file limited to durable rules that change how work is done. Put
specialized instructions in a nearer `AGENTS.md` only when a subtree develops
materially different commands or constraints.

## Sources of truth

- `cmd/server` owns process wiring and HTTP server lifecycle.
- `internal/httpapi` owns HTTP contracts, request handling, and quota decisions.
- `internal/youtube` is the external YouTube API boundary.
- `internal/ratelimit` contains state shared by concurrent handlers.
- `deploy` and `Dockerfile` define production lifecycle and packaging.
- `README.md` documents public behavior, configuration, and developer commands.

Read the implementation, its callers, and relevant deployment configuration
before changing behavior. Do not assume the named file is the whole system
boundary.

## Change workflow

- Never commit or push directly to `main`.
- Work on a dedicated branch. Codex branches use `codex/<topic>`; other clients
  may use the repository's `feat/…`, `fix/…`, or `chore/…` convention.
- Preserve unrelated working-tree changes. Do not rewrite or discard work that
  is not part of the request.
- For a behavioral change, trace the entry point through downstream calls and
  cleanup or deployment behavior. Identify the invariants and relevant failure
  paths before editing.
- Keep each diff focused on one outcome. Avoid drive-by refactors, speculative
  abstractions, and new dependencies that are not needed for that outcome.
- A tightly coupled safety fix is in scope when the requested change creates or
  exposes the relevant production boundary. Explain any non-obvious expansion.
- When a convenience API is replaced with an explicit server, client, or config
  value, inventory the defaults the convenience API supplied and decide each
  newly exposed zero value deliberately.
- Commit the completed change, push the branch, and open a PR with `gh pr create`.
  Wait for review and do not merge unless the user explicitly asks.
- Apart from the required branch push and PR creation, do not run release,
  image-push, deployment, or other externally mutating commands unless the user
  explicitly authorizes that action.

## Implementation principles

- Prefer the smallest design that fixes the root cause and matches existing Go
  patterns. Do not hide lifecycle, ownership, or failure behavior behind an
  abstraction unless it removes real duplication or enforces an invariant.
- Preserve public behavior unless the request changes it. Treat HTTP status,
  JSON shape, configuration names/defaults, and deployment assumptions as
  compatibility surfaces.
- Check both success and failure paths. Every acquired resource, goroutine,
  reservation, and background operation must have a clear owner and termination
  path.
- Handle returned errors before using dependent values. Add useful context while
  preserving error identity when callers rely on `errors.Is` or `errors.As`.
- Propagate request `context.Context` through request-scoped work and external
  calls. Use a separate bounded context for cleanup that must continue after a
  request or signal context is canceled.
- Guard all mutable state reached by concurrent HTTP handlers. Make goroutine
  lifetime and shutdown behavior explicit; do not leak goroutines or block them
  indefinitely on channels or I/O.
- Preserve quota and rate-limit invariants on every exit path. Reserve before
  making a billable call, and deliberately commit or release reservations after
  partial failure.
- Never commit credentials or log/return secrets, stack traces, raw internal
  errors, or upstream responses that may contain sensitive data.

## Boundary checklist

Apply this checklist when a change touches a network, process, storage, or
deployment boundary. Skip irrelevant items rather than adding ceremonial code.

- Validate untrusted input for type, syntax, semantics, size, and multiplicity
  before expensive work. Reject malformed input without panics.
- Bound resource consumption with appropriate body-size, time, concurrency,
  quota, and state-retention limits. Avoid unbounded maps, queues, retries, and
  reads.
- For HTTP servers and clients, review the full lifecycle rather than accepting
  zero-value defaults blindly: connection setup, header/body reads, handler or
  upstream work, writes, idle reuse, cancellation, and shutdown. An intentionally
  unbounded phase, such as streaming, should be evident from the design.
- Fail safely: send stable client-facing errors, retain diagnostic context in
  server logs, and avoid exposing implementation details.
- Treat proxy-derived identity such as forwarding headers as trusted only when
  the deployment topology enforces that trust boundary.
- When process lifecycle changes, inspect the application, health checks,
  `Dockerfile`, and Kubernetes manifests together. Stop accepting traffic before
  teardown, allow in-flight work to drain, and keep application shutdown budgets
  within the deployment's termination grace period; Kubernetes hooks consume the
  same budget.
- Keep readiness about traffic eligibility and liveness about whether restart is
  required. Do not make slow or fragile external dependencies cause restart loops
  without a deliberate reason.
- Keep secrets in Kubernetes secret references or an equivalent secret store.
  When deployment manifests change, verify probes, resource requests/limits,
  and rolling-update availability rather than assuming platform defaults are
  suitable.

## Tests and verification

- Test externally visible behavior and invariants, not private implementation
  details. A bug fix should include a regression test when the behavior can be
  exercised deterministically.
- Cover the happy path plus relevant malformed input, boundary values, partial
  failure, cancellation, timeout, and concurrent access. Do not add tests merely
  to increase a metric.
- Prefer deterministic tests with bounded waits. Avoid real network services,
  wall-clock sleeps, and order dependence when a fake, local server, or explicit
  synchronization can express the behavior.
- Run `gofmt` on every changed Go file.
- Run the smallest relevant checks while iterating, then run the repository-wide
  checks before handing off:

```bash
git diff --check
go test ./...
go vet ./...
go build ./...
```

- Run `go test -race ./...` for changes involving shared state, goroutines,
  cancellation, or shutdown.
- Do not manually edit generated dependency metadata. If imports change, use Go
  tooling and inspect both `go.mod` and `go.sum`.
- If a relevant check cannot run, report exactly what was skipped and why. Never
  claim a check passed without running it.

## Documentation and handoff

- Update `README.md` when public API behavior, configuration, local commands, or
  deployment steps change.
- Comments should explain non-obvious intent, invariants, and operational
  trade-offs. Do not restate the code or add comments solely to satisfy a review.
- Before committing, inspect the final diff and working tree. The handoff or PR
  should state the problem, the chosen approach, validation performed, assumptions,
  and any remaining risk.

## Code Review Rules

- Review the complete affected behavior across callers, configuration, tests,
  and deployment files, not only the edited lines.
- Report actionable correctness, security, reliability, operability, or
  compatibility problems. Describe the concrete failure scenario and a safe
  correction; do not block on taste, naming preference, or formatter output.
- Give priority to unsafe defaults newly made explicit by a change, even when the
  latent risk predates the diff. Clearly distinguish a regression introduced by
  the patch from existing debt that the patch merely exposes.
- Check error paths, cancellation, timeouts, resource cleanup, concurrency,
  input/resource bounds, quota accounting, sensitive-data exposure, and public
  contract changes whenever they are relevant.
- Verify that tests would fail for the defect they claim to prevent. Do not demand
  tests or comments without identifying the behavior or invariant they protect.
- Prefer a focused change that improves overall code health over unrelated
  perfection. Mark optional polish as non-blocking.
- If a review request is technically unsound, respond with repository evidence
  and trade-offs instead of implementing it mechanically. Apply valid findings
  and re-run the affected checks.

## Maintaining these instructions

- Keep rules concise, repository-specific, and verifiable. Remove stale facts and
  duplicate material instead of accumulating advice.
- Prefer executable enforcement in tests, CI, linters, permissions, or deployment
  policy for deterministic rules; this file should explain the constraint and
  the safe path.
- When the same valid review issue recurs, first add the smallest regression test
  or automated check that can prevent it. Update this file only when a durable
  repository decision is missing.
