#!/usr/bin/env bash
set -euo pipefail

version="${1#v}"

awk -v version="$version" '
BEGIN {
	header = "## [" version "]"
	found = 0
	started = 0
}
index($0, header) == 1 {
	found = 1
	next
}
found && /^## \[/ {
	exit
}
found {
	if (!started && $0 == "") {
		next
	}
	started = 1
	print
}
END {
	if (!found) {
		print "release notes for " version " not found in CHANGELOG.md" > "/dev/stderr"
		exit 1
	}
}
' CHANGELOG.md
