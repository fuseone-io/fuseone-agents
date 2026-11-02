#!/usr/bin/env bash
# Refuses to let `make release` tag anything but a clean checkout of origin/main.
#
# The tag must name a commit that is on main. Releases go through a pull
# request, and a squash merge rewrites the release branch's commits, so a tag
# made on the branch names a commit main never contains.
set -euo pipefail

# --porcelain lists staged, unstaged and untracked changes alike; `git diff
# --quiet` sees only the unstaged ones.
dirty=$(git status --porcelain)
if [ -n "$dirty" ]; then
	echo "the tree is dirty; a tag must name a commit somebody can check out:" >&2
	echo "$dirty" >&2
	exit 1
fi

git fetch --quiet origin main
head=$(git rev-parse HEAD)
main=$(git rev-parse refs/remotes/origin/main)
if [ "$head" != "$main" ]; then
	echo "HEAD is $head but origin/main is $main." >&2
	echo "Run make release on main, at the merged commit, after the changelog pull request has merged." >&2
	exit 1
fi
