#!/bin/sh
set -u

cd "$(dirname "$0")/.." || exit 1
base=${TMPDIR:-/tmp}
work=$(mktemp -d "$base/cs2m-check.XXXXXX") || exit 1
trap 'rm -rf "$work"' 0 HUP INT TERM
GOCACHE=$work/cache
GOTMPDIR=$work/tmp
mkdir "$GOCACHE" "$GOTMPDIR" || exit 1
export GOCACHE GOTMPDIR
failed=0
blocked=0

run() {
    label=$1
    shift
    if "$@"; then
        printf 'PASS %s\n' "$label"
    else
        code=$?
        printf 'FAIL %s (exit %s)\n' "$label" "$code"
        failed=1
    fi
}

format_check() {
    files=$(gofmt -l cmd internal) || return 1
    if [ -n "$files" ]; then
        printf '%s\n' "$files"
        return 1
    fi
}

run gofmt format_check
run vet go vet ./...
run tests go test -count=1 ./...
run race go test -race -count=1 ./...
run coverage go test -coverprofile="$work/coverage.out" -count=1 ./...
if [ -f "$work/coverage.out" ]; then
    go tool cover -func="$work/coverage.out" | tail -1
fi

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    run build go build ./...
    run git-diff-check git diff --check
else
    printf 'BLOCKED build: no usable Git repository; Go VCS stamping cannot complete\n'
    printf 'BLOCKED git-diff-check: no usable Git repository\n'
    blocked=1
    run build-without-vcs-stamping go build -buildvcs=false ./...
fi

if [ "$failed" -ne 0 ] || [ "$blocked" -ne 0 ]; then
    exit 1
fi
