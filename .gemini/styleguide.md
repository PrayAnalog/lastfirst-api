# Review guide

Backend for lastfirst.app: a Go HTTP service that reverses a YouTube playlist
and serves the built frontend as static files. Single binary, no database,
deployed to Kubernetes.

## Report

- Returned errors that are dropped, or values used before their error is
  checked.
- Request-scoped work that does not propagate the incoming `context.Context`.
- Shared state mutated without synchronization. `internal/ratelimit` is called
  concurrently from HTTP handlers.
- Handlers that can panic on malformed or missing input.
- Responses that expose internal error text, stack traces, or the YouTube API
  key to clients.
- Quota accounting mistakes: a reservation taken on a path that later fails
  without releasing it, or a YouTube API call made before the budget is
  reserved.
- In `deploy/`: missing readiness or liveness probes, missing resource
  requests and limits, changes that would drop connections during a rolling
  update, and secrets written as literal values instead of `secretKeyRef`.

## Do not report

- Missing comments, doc comments, or tests.
- Renaming or restructuring for style alone.
- Formatting that `gofmt` already settles.
- Anything in `go.sum`.
