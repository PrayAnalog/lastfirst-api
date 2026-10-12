# Codex Review

A third local review that runs alongside `/code-review` and the OCR review,
so findings the Codex bot would post on the PR can be fixed before it opens.
It calls the Codex CLI, which reviews with an OpenAI model under the account
`codex login` signed in to, and reports findings without editing the working
tree.

Commit the change and leave the working tree clean before running it: Codex
diffs the merge base against the working tree, not against `HEAD`, so
uncommitted edits would be reviewed while the stamp records only `HEAD`. Run
it from the repository root in the foreground, with the Bash tool's longest
timeout, so its output lands in the same turn as `record-review.sh`:

```bash
.claude/skills/ship-change/codex-effort.sh review <base>
```

It runs `codex review --base origin/<base>` twice in parallel, at low and at
high reasoning effort, prints each final review under its effort, and logs
both runs under `codex-effort/` in the git common directory. It prints the
path of a run's transcript only when that run exits non-zero; read it then.

`<base>` is the base the PR will be opened against, as in the other two
reviews. The Codex plugin's `/codex:review --base <base>` runs one such
review, at the effort in `~/.codex/config.toml`, for a person typing it; it is
marked `disable-model-invocation`, so a session runs the script instead.

When `codex` is missing or not signed in, stop and tell the user: install it
with `npm install -g @openai/codex` and sign in with `codex login`. When the
timeout kills the review before it prints one, stop and tell the user too;
do not record the review.

Handle what it finds the same way as `/code-review` findings: fix each one, or
record why you are declining it. Then log each finding with the efforts that
reported it, `both` when the two reviews describe the same defect, and what
you did with it:

```bash
.claude/skills/ship-change/codex-effort.sh finding <low|high|both> <fixed|declined> "<one-line summary>"
```
