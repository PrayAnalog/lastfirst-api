# Codex Review

A third local review that runs alongside `/code-review` and the OCR review,
so findings the Codex bot would post on the PR can be fixed before it opens.
It calls the Codex CLI, which reviews with an OpenAI model under the account
`codex login` signed in to, and reports findings without editing the working
tree.

Commit the change before running it: `--base` reviews the branch diff, so
uncommitted edits are not reviewed. Run it from the repository root, in the
background, since a review can take several minutes:

```bash
codex review --base origin/<base>
```

`<base>` is the base the PR will be opened against, as in the other two
reviews. The Codex plugin's `/codex:review --base <base>` runs the same review
for a person typing it; it is marked `disable-model-invocation`, so a session
runs the CLI instead.

When `codex` is missing or not signed in, stop and tell the user: install it
with `npm install -g @openai/codex` and sign in with `codex login`.

Handle what it finds the same way as `/code-review` findings: fix each one, or
record why you are declining it.
