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
codex review --base origin/<base> 2>"<scratchpad>/codex-review.log"
```

stdout carries only the final review; stderr carries the agent's whole
transcript, so read the log only when the command exits non-zero.

`<base>` is the base the PR will be opened against, as in the other two
reviews. The Codex plugin's `/codex:review --base <base>` runs the same review
for a person typing it; it is marked `disable-model-invocation`, so a session
runs the CLI instead.

When `codex` is missing or not signed in, stop and tell the user: install it
with `npm install -g @openai/codex` and sign in with `codex login`.

Handle what it finds the same way as `/code-review` findings: fix each one, or
record why you are declining it.
