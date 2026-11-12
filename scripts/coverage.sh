#!/bin/sh
# Statement coverage per package, counted across the whole test suite: a
# package's code run by another package's tests counts as covered.
# The numbers come only from the fixtures.
# go test ./... still runs the tests that read this machine's logs.
#
#   scripts/coverage.sh           print coverage
#   scripts/coverage.sh --update  record it as the baseline in coverage.txt
#   scripts/coverage.sh --check   fail if the total or any package fell below
#                                 the baseline
#
# The baseline only moves up by hand, with --update, after the new numbers
# have been looked at.
set -eu
cd "$(dirname "$0")/.."
profile=$(mktemp "${TMPDIR:-/tmp}/agentlog-cover.XXXXXX")
base=${TMPDIR:-/tmp}
home=$(mktemp -d "${base%/}/agentlog-cover-home.XXXXXX")
tmp=$(mktemp -d "${base%/}/agentlog-cover-tmp.XXXXXX")
trap 'rm -f "$profile"; rm -rf "$home" "$tmp"' EXIT
# Only packages with tests are run; the rest are still measured through
# -coverpkg. (Running a package without tests under -coverpkg needs the
# covdata tool, which some Go installs leave out.)
tested=$(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
# shellcheck disable=SC2086
# Under -short, tests that read this machine's logs skip so coverage measures fixtures alone.
# A clean environment excludes the caller's harness IDs, log folders, and other runs' worker claims.
# Separate home and temp folders without // keep paths consistent for TestScreens and TestWhereRunsGo.
env -i PATH="$PATH" HOME="$home" TMPDIR="$tmp" \
	GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" GOPATH="$(go env GOPATH)" \
	GOTOOLCHAIN="$(go env GOTOOLCHAIN)" GOFLAGS="$(go env GOFLAGS)" \
	go test -short -count=1 -coverpkg=./... -coverprofile="$profile" $tested >"$tmp/test.log" 2>&1 || { tail -40 "$tmp/test.log" >&2; exit 1; }

# A block is covered if any test binary ran it.
table=$(awk -F'[: ]' 'NR > 1 {
	key = $1 ":" $2
	stmts[key] = $3
	if ($4 > 0) hit[key] = 1
	pkg = $1; sub(/\/[^\/]*$/, "", pkg); sub(/^github.com\/Ryheff24\/agentlog\//, "", pkg)
	owner[key] = pkg
}
END {
	for (k in stmts) {
		p = owner[k]; total[p] += stmts[k]; all += stmts[k]
		if (k in hit) { covered[p] += stmts[k]; allc += stmts[k] }
	}
	for (p in total) printf "%-22s %5.1f%%  %5d statements\n", p, 100 * covered[p] / total[p], total[p]
	printf "%-22s %5.1f%%  %5d statements\n", "TOTAL", 100 * allc / all, all
}' "$profile" | LC_ALL=C sort)

case "${1:-}" in
--update)
	printf '%s\n' "$table" > coverage.txt
	printf '%s\n' "$table"
	;;
--check)
	printf '%s\n' "$table" | awk 'NR == FNR { base[$1] = $2 + 0; next }
	($1 in base) && $2 + 0 < base[$1] - 0.05 {
		printf "coverage fell: %s %s, baseline %.1f%%\n", $1, $2, base[$1]; bad = 1
	}
	END { exit bad }' coverage.txt -
	;;
*)
	printf '%s\n' "$table"
	;;
esac
