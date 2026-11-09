#!/bin/sh
# Mutation score per package: the share of small deliberate code changes that some test catches.
# Coverage says a line ran. A mutation score says a test would fail if the line were wrong.
#
#   scripts/mutation.sh [pkg...]           print the score (default: every tested package)
#   scripts/mutation.sh --update [pkg...]  record it as the baseline in mutation.txt
#   scripts/mutation.sh --check [pkg...]   fail if a package fell below its baseline
#
# It needs gremlins (go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0).
# gremlins is a developer tool, so it stays out of go.mod and the agentlog binary.
# A full run takes several minutes, so CI does not run it.
set -eu
cd "$(dirname "$0")/.."
mode=print
case "${1:-}" in --update | --check) mode=${1#--}; shift ;; esac
gremlins=$(command -v gremlins || echo "$(go env GOPATH)/bin/gremlins")
[ -x "$gremlins" ] || { echo "gremlins is not installed: go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0" >&2; exit 2; }
[ $# -gt 0 ] || set -- $(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.Dir}}{{end}}' ./... | sed "s|^$PWD/|./|")

table=$(for pkg in "$@"; do
	# A test that reads the machine's real logs can fail by itself. A mutant counts as killed only when the unmutated tests pass.
	go test -count=1 "$pkg" >/dev/null 2>&1 || { echo "tests fail before mutation: $pkg" >&2; exit 1; }
	# gremlins sets each mutant's timeout from one unmutated run, so a cached result or a fast suite would time out every mutant untested.
	score=$(GOFLAGS=-count=1 "$gremlins" unleash --timeout-coefficient 30 "$pkg" 2>&1 | sed -n 's/^Test efficacy: \([0-9.]*\)%.*/\1/p')
	# A package whose own tests run none of its code, such as cmd/agentlog, has no score.
	[ -n "$score" ] && score=$(printf '%.1f%%' "$score") || score=-
	printf '%-24s %6s\n' "${pkg#./}" "$score"
done)

case $mode in
update)
	printf '%s\n' "$table" > mutation.txt
	printf '%s\n' "$table"
	;;
check)
	printf '%s\n' "$table" | awk 'NR == FNR { base[$1] = $2 + 0; next }
	($1 in base) && $2 + 0 < base[$1] - 0.05 {
		printf "mutation score fell: %s %s, baseline %.1f%%\n", $1, $2, base[$1]; bad = 1
	}
	END { exit bad }' mutation.txt -
	;;
*)
	printf '%s\n' "$table"
	;;
esac
