# review_stamp <sha> <gitdir> [<base>] prints the path of the stamp that
# records a review of the diff from <base> to <sha>. When there is no such
# diff it prints why and returns 1. Both require-code-review.sh and
# record-review.sh source it, so the stamp they check and write is the same.
review_stamp() {
  local sha=$1 gitdir=$2 base_ref=$3 base_sha merge_base base_key

  if [ -z "$base_ref" ]; then
    base_ref=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null)
    [ -n "$base_ref" ] || base_ref=main
  fi
  base_ref=${base_ref#origin/}

  base_sha=$(git rev-parse --verify "origin/$base_ref^{commit}" 2>/dev/null) ||
    base_sha=$(git rev-parse --verify "$base_ref^{commit}" 2>/dev/null) || {
      echo "This gate reviews the diff against $base_ref, which does not resolve to a commit here. Fetch it, or name a base this checkout has."
      return 1
    }

  merge_base=$(git merge-base "$sha" "$base_sha" 2>/dev/null) || {
    echo "$sha and $base_ref share no history, so there is no diff this gate can attest."
    return 1
  }

  # What gets reviewed is the three-dot diff, merge_base..HEAD, so both of those
  # are in the key: a new commit or a rebase moves one end or the other and the
  # stamp will not exist yet. The base tip itself is deliberately not in the key,
  # because main advancing over commits this branch does not touch leaves that
  # diff untouched too, and re-reviewing it would find nothing. The base *ref* is
  # in the key, hashed because it can contain a slash, so that targeting a
  # different branch needs its own review even when the two share a merge base.
  base_key=$(printf '%s' "$base_ref" | git hash-object --stdin 2>/dev/null | cut -c1-12)
  [ -n "$base_key" ] || {
    echo "Could not derive a stamp key for base $base_ref."
    return 1
  }
  echo "$gitdir/claude-code-review-$sha-$base_key-$merge_base"
}
