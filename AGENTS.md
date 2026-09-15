# Repository Instructions

## Scope

This is the Go backend for `lastfirst.app`. It serves the frontend, accepts
playlist input over HTTP, calls the YouTube Data API, and runs on Kubernetes.
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

## Workflow

- Never commit or push directly to `main`. Use a dedicated branch; Codex branches
  use `codex/<topic>`.
- Preserve unrelated working-tree changes and keep each diff focused.
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
- Reserve quota before a billable call; commit or release the reservation on
  every exit path, including partial failure.
- Configure HTTP server and client timeouts deliberately. For lifecycle changes,
  inspect the app, probes, `Dockerfile`, and Kubernetes manifests together;
  drain in-flight requests within the termination grace period.
- Return stable client errors and keep diagnostic context in logs. Never expose
  secrets, stack traces, raw internal errors, API keys, or sensitive upstream
  responses.
- Trust forwarding headers only when the deployment topology enforces that trust
  boundary. Keep deployment secrets in secret references.

## Verification

- Test externally visible behavior and production invariants. Add a deterministic
  regression test for a bug when practical.
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
