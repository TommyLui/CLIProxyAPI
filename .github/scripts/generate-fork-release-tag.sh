#!/usr/bin/env bash

set -euo pipefail

upstream_repository="${UPSTREAM_REPOSITORY:-router-for-me/CLIProxyAPI}"
release_sha="${RELEASE_SHA:-HEAD}"
upstream_tag_namespace="refs/release-upstream-tags"

release_sha="$(git rev-parse "${release_sha}^{commit}")"

while IFS= read -r ref; do
  git update-ref -d "$ref"
done < <(git for-each-ref --format='%(refname)' "$upstream_tag_namespace")

git fetch --quiet --force --no-tags \
  "https://github.com/${upstream_repository}.git" \
  "+refs/tags/v*:refs/release-upstream-tags/v*"

base_tag=""
base_commit=""
while IFS= read -r ref; do
  tag="${ref#${upstream_tag_namespace}/}"
  if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    continue
  fi

  commit="$(git rev-parse "${ref}^{commit}")"
  if git merge-base --is-ancestor "$commit" "$release_sha"; then
    base_tag="$tag"
    base_commit="$commit"
    break
  fi
done < <(git for-each-ref --sort=-version:refname --format='%(refname)' "$upstream_tag_namespace")

if [[ -z "$base_tag" ]]; then
  echo "No upstream release tag is an ancestor of ${release_sha}" >&2
  exit 1
fi

commit_count="$(git rev-list --count "${base_commit}..${release_sha}")"
short_sha="$(git rev-parse --short=8 "$release_sha")"

printf '%s-fork-%s-g%s\n' "$base_tag" "$commit_count" "$short_sha"
