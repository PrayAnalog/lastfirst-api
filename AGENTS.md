# Repository Instructions

## Scope

This is the Go backend for `lastfirst.app`. It serves the frontend, accepts
playlist input over HTTP, calls the YouTube Data API, and runs with Docker
Compose on a single DigitalOcean droplet behind Caddy.
Public HTTP behavior, YouTube quota accounting, concurrent handler state, and
clean connection draining are production invariants.

## Ownership

- `cmd/server`: process wiring and HTTP server lifecycle.
- `internal/httpapi`: HTTP contracts, handlers, and quota decisions.
- `internal/youtube`: YouTube API boundary.
- `internal/ratelimit`: state shared by concurrent handlers.
- `deploy` and `Dockerfile`: production lifecycle and packaging.
- `README.md`: public behavior, configuration, and developer commands.

Trace behavioral changes through callers and relevant deployment configuration.
Preserve HTTP status and JSON shape, configuration names and defaults, quota
semantics, and deployment assumptions unless the request changes them.

Before changing a production flow, state its invariants and trace every state
transition across the owning packages and deployment boundary. Include races
between independently triggered events, not only the intended success path.

## Workflow

- Never commit or push directly to `main`. Use a dedicated branch named for the
  change type, such as `feat/<topic>`, `fix/<topic>`, or `chore/<topic>`.
- Preserve unrelated working-tree changes and keep each diff focused.
- Before the first push or PR for an implementation, and before every push
  that answers review feedback, work through `.agents/pre-pr-checklist.md`
  against the branch diff, then run a dedicated code review against the base
  branch using `.agents/code-review.md`.
- Verify and fix every actionable correctness, security, reliability, and
  compatibility finding, then rerun required checks.
- If review fixes materially change behavior, review the updated diff again.
  Push and create the PR only when no actionable findings remain.
- For implementation requests, commit completed changes, push the branch, and
  open a PR with `gh pr create`. Wait for review; merge only when the user asks.
- Do not release, push images, or deploy without explicit authorization.
- For code-review requests, read `.agents/code-review.md`.

## Production boundaries

- Validate untrusted input and bound request bodies, time, concurrency, quota,
  and retained state before expensive work.
- Propagate request `context.Context` to external calls. Use a separate bounded
  context only for cleanup that must outlive request or signal cancellation.
- Guard mutable state reached by concurrent handlers and give every goroutine or
  acquired resource an explicit owner and termination path.
- Reserve quota before each billable call. Count attempted calls at the API
  boundary instead of inferring them from returned data; settle unused capacity
  on every exit path, and never release an old-period reservation into a new
  quota period.
- Configure HTTP server and client timeouts deliberately. For lifecycle changes,
  inspect the app, `Dockerfile`, `deploy/docker-compose.yml`, and
  `deploy/caddy/Caddyfile` together; wait for both the listener and in-flight
  handlers, and fit shutdown inside the container's stop grace period.
- Fully consume and validate bounded request bodies before starting expensive
  or billable work; a successful first decode alone does not prove the body is
  within its limit or contains only one value.
- Return stable client errors and keep diagnostic context in logs. Never expose
  secrets, stack traces, raw internal errors, API keys, or sensitive upstream
  responses.
- Trust forwarding headers only when the deployment topology enforces that trust
  boundary. Keep deployment secrets in secret references.
- Treat process-local quota, rate-limit, and concurrency state as a deployment
  constraint: run exactly one app container, let a redeploy stop the old
  container before the new one starts, and do not scale horizontally unless
  those controls move to shared storage with atomic operations.

## Review guidelines

- Write all code-review findings, summaries, and other reviewer-facing prose in
  Korean. Keep code identifiers, file paths, commands, and quoted source text
  in their original form.
- Production boundaries and the race tracing under **Ownership** apply to the
  service code, `Dockerfile` and `deploy/`. They are not the bar for
  `.github/` workflows or other development tooling.
- In `.github/` and development tooling, flag failures reachable in normal use:
  workflow permissions wider than needed, secrets exposed to untrusted code,
  and fork pull requests crossing the trust boundary. Do not flag races that
  need concurrent manual actions, or limits no real change reaches.
- The `Code Review` workflow is one of three reviewers. Skipping it in a rare
  sequence is an accepted risk, not a defect, once CodeRabbit and Codex have
  both reviewed the same head. Neither is guaranteed to: each has paused,
  skipped a PR, or run out of usage before.

## Verification

- Test externally visible behavior and production invariants. Add a deterministic
  regression test for a bug when practical.
- Exercise adversarial interleavings relevant to the change, such as signal vs.
  listener failure, quota reset vs. reservation release, partial upstream
  failure, and upstream data growing or shrinking between calls.
- Run `gofmt` on changed Go files and `git diff --check` for every change.
- For Go changes, run targeted package tests while iterating, then before handoff:
  `go test ./...`, `go vet ./...`, and `go build ./...`.
- Also run `go test -race ./...` for shared-state, goroutine, cancellation, or
  shutdown changes.
- Do not repeat passed checks unless code changed, a failure needs diagnosis, or
  an unresolved risk justifies it. Report any skipped required check and why.
- Use Go tooling for dependency metadata and inspect changes to both `go.mod` and
  `go.sum`.

Update `README.md` when public behavior, configuration, commands, or deployment
steps change. Before committing, inspect the final diff and working tree.
