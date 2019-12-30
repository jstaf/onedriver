#!/bin/bash
# checks formatting and other linting errors during tests.

GOFMT_FAILURES="$(goimports -l .)"
if [ -n "$GOFMT_FAILURES" ]; then
    echo "The following files failed formatting checks (check against goimports):"
    echo "$GOFMT_FAILURES"
    exit 1
fi
