#!/usr/bin/env bash
# cgo cannot conditionally use different packages based on which system packages
# are installed so this script is here to autodetect which webkit2gtk c headers
# we have access to

if [ -n "$CGO_ENABLED" ] && [ "$CGO_ENABLED" -eq 0 ]; then
    exit 0
fi

if pkg-config webkit2gtk-4.0; then
    sed -i 's/webkit2gtk-4.1/webkit2gtk-4.0/g' fs/graph/oauth2_gtk.go
elif ! pkg-config webkit2gtk-4.1; then
    echo "webkit2gtk development headers must be installed"
    exit 1
fi
