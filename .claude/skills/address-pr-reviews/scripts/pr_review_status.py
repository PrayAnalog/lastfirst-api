#!/usr/bin/env python3
"""Summarize a GitHub PR's review threads so an agent can spot what's new.

Fetches inline review comments and top-level reviews via `gh api`, groups
inline comments into threads, and flags each thread with whether the
authenticated gh user (or --author) had the last word. That flag is a
heuristic, not a verdict: a bot can speak last just to acknowledge a
rebuttal ("agreed, no action needed"), so read the thread before deciding
whether it actually needs a reply.

Review bodies are printed separately (not threaded) because tools like
CodeRabbit sometimes post findings only in the review body ("outside diff
range comments") with no comment id to reply to -- those need a top-level
`gh pr comment` reply instead of a threaded one.

Usage:
    pr_review_status.py <owner>/<repo> <pr_number> [--author LOGIN]
"""
import argparse
import json
import subprocess
import sys


def gh_api(path):
    """Fetch every page of a `gh api` list endpoint."""
    items = []
    page = 1
    while True:
        sep = "&" if "?" in path else "?"
        out = subprocess.run(
            ["gh", "api", f"{path}{sep}per_page=100&page={page}"],
            capture_output=True, text=True, check=True,
        ).stdout
        batch = json.loads(out)
        if not batch:
            break
        items.extend(batch)
        if len(batch) < 100:
            break
        page += 1
    return items


def truncate(text, n=200):
    text = (text or "").strip().replace("\n", " ")
    return text if len(text) <= n else text[: n - 1] + "…"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repo", help="owner/repo")
    ap.add_argument("pr", type=int)
    ap.add_argument("--author", help="gh login whose reply closes a thread; "
                                      "defaults to the authenticated user")
    args = ap.parse_args()

    author = args.author
    if not author:
        author = subprocess.run(
            ["gh", "api", "user", "--jq", ".login"],
            capture_output=True, text=True, check=True,
        ).stdout.strip()

    comments = gh_api(f"repos/{args.repo}/pulls/{args.pr}/comments")
    reviews = gh_api(f"repos/{args.repo}/pulls/{args.pr}/reviews")

    threads = {}
    for c in comments:
        root = c.get("in_reply_to_id") or c["id"]
        threads.setdefault(root, []).append(c)

    print(f"=== PR #{args.pr} review status ({args.repo}) ===")
    print(f"(threads where the last message isn't from '{author}' are flagged"
          f" NEEDS REVIEW -- verify, don't assume)\n")

    if not threads:
        print("No inline review comments.\n")

    for root_id, msgs in threads.items():
        msgs.sort(key=lambda c: c["created_at"])
        head = msgs[0]
        print(f"Thread @ {head['path']}:{head.get('line') or head.get('original_line')} "
              f"(root id {root_id})")
        for m in msgs:
            print(f"  [{m['id']}] {m['user']['login']} ({m['created_at']}): "
                  f"{truncate(m['body'])}")
        last_speaker = msgs[-1]["user"]["login"]
        status = "ok (you spoke last)" if last_speaker == author else "NEEDS REVIEW"
        print(f"  -> last speaker: {last_speaker} -- {status}\n")

    print("--- Review bodies (check for findings with no comment id, e.g. "
          "CodeRabbit's 'outside diff range' section) ---\n")
    for r in reviews:
        if r["user"]["login"] == author:
            continue
        body = (r.get("body") or "").strip()
        if not body:
            continue
        print(f"[{r['state']}] {r['user']['login']} on commit {r['commit_id'][:10]} "
              f"({r['submitted_at']}):")
        print(f"  {truncate(body, 500)}\n")


if __name__ == "__main__":
    main()
