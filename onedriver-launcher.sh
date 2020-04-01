#!/bin/bash
# This is a minimal launcher to facilitate launching onedriver via a .desktop entry.
set -eo pipefail

if [ -z "$1" ]; then
    echo "Usage: onedriver-launcher.sh <mountpoint>"
    exit 1
fi
MOUNT=$(realpath "$1")

# Is onedriver running on that mountpoint? If not, mount it.
SERVICE_NAME=$(systemd-escape --template onedriver@.service $MOUNT)
if ! $(mount | grep onedriver | grep -q $MOUNT); then
    mkdir -p $MOUNT
    systemctl --user daemon-reload
    systemctl start --user $SERVICE_NAME
fi

xdg-open $MOUNT
