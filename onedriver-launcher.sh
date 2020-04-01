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
if ! systemctl is-active --quiet --user $SERVICE_NAME; then
    echo "Mounting filesystem..."
    mkdir -p $MOUNT
    systemctl --user daemon-reload
    systemctl start --user $SERVICE_NAME
    sleep 2
else
    echo "Filesystem already mounted."
fi
echo "Further logs can be checked via \"journalctl --user -u $SERVICE_NAME\""

xdg-open $MOUNT
