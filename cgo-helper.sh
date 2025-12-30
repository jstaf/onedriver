#!/usr/bin/env bash
# cgo cannot conditionally use different packages based on which system packages
# are installed so this script is here to autodetect which webkit2gtk c headers
# we have access to.
# 0.14.2: webkit2gtk-4.0 deprecated on all major distros, so we default to 4.1 now.
# If you need 4.0 support, use onedriver 0.14.1 or earlier.

if [ -n "$CGO_ENABLED" ] && [ "$CGO_ENABLED" -eq 0 ]; then
    exit 0
fi

# 0.14.1: Check for webkit2gtk-4.0 first for backward compatibility
# We'll keep this code here for a while, but now we default to 4.1
#if pkg-config webkit2gtk-4.0; then
#    sed -i 's/webkit2gtk-4.1/webkit2gtk-4.0/g' fs/graph/oauth2_gtk.go
#elif ! pkg-config webkit2gtk-4.1; then
#    echo "webkit2gtk development headers must be installed"
#    exit 1
#fi

# 0.14.2: Check for webkit2gtk-4.1
if ! pkg-config webkit2gtk-4.1; then
    echo "webkit2gtk-4.1 development headers must be installed"
    exit 1
fi
