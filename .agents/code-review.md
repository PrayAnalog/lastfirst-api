# Code Review Guidance

Review the complete affected behavior across callers, configuration, tests, and
deployment files. Report actionable correctness, security, reliability,
operability, or compatibility defects; do not block on taste, naming, comments,
or formatter output.

For each finding, describe the concrete failure scenario and a safe correction.
Distinguish regressions introduced by the patch from existing debt that the patch
only exposes. Prioritize newly unsafe defaults even when the underlying risk
predates the diff.

Check relevant error paths, cancellation, timeouts, resource cleanup,
concurrency, input and resource bounds, quota accounting, sensitive-data
exposure, and public contracts. Verify that proposed tests would fail for the
defect they claim to prevent. Mark optional polish as non-blocking.

If a review request is technically unsound, respond with repository evidence
and trade-offs. Report valid findings; apply fixes and rerun checks only when the
user explicitly requests implementation.
