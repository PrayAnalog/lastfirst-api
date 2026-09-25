# OCR Delegation Review

A second local review that runs alongside `/code-review` (or
`.agents/code-review.md`). Open Code Review (OCR) selects the reviewable files
and resolves a rule checklist for each; you review each file against its
checklist with your own model. OCR makes no LLM call and needs no API key.

Run OCR through the pinned package, from the repository root:

```bash
OCR_NO_UPDATE=1 npx -y -p @alibaba-group/open-code-review@1.12.9 ocr <args>
```

`OCR_NO_UPDATE=1` stops the CLI's background self-update from replacing the
pinned version.

1. **Preview.** `ocr delegate preview --format json --from origin/<base> --to HEAD`.
   Its `reviewable_files` are the coverage checklist. Files it excludes are
   out of scope for this review; `/code-review` still covers them.
2. **Rules.** `ocr delegate rule --format json <reviewable paths...>`. Each
   file's rule group is its review checklist.
3. **Diffs.** `git diff <merge_base>..HEAD -- <path>`, using `merge_base` from
   the preview.
4. **Review.** For every reviewable file, check the diff against its rule
   group, reading callers and context as needed. Mark each file reviewed, or
   skipped with a concrete reason. Do not stop at the first finding.
5. **Report.** For each finding, give the path, the line range in the new
   file, a severity (critical, high, medium or low), and a concrete failure
   scenario. Always report critical and high findings. Report medium findings
   with their context. Report low findings only when they are clearly
   valuable. Drop likely false positives.

Handle what it finds the same way as `/code-review` findings: fix each one, or
record why you are declining it.
