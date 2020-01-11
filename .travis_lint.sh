#!/bin/bash
# checks formatting and other linting errors during tests.
set -eu

GOFMT_FAILURES="$($HOME/gopath/bin/goimports -l .)"
if [ -n "$GOFMT_FAILURES" ]; then
    echo "The following files failed formatting checks (check against goimports):"
    echo "$GOFMT_FAILURES"
    exit 1
fi
