#!/usr/bin/env bash
# Build and run in Docker. Run with privileges for FUSE and interactively for login.
# Mount at $MOUNTPOINT on host, defaults to ~/Onedrive

set -e
if [ -z "$(docker images -q onedriver 2> /dev/null)" ]; then
  docker build -t onedriver .
fi
MOUNT="${MOUNTPOINT:-"$HOME/Onedrive"}"
mkdir -p $MOUNT
docker run -it \
           -v $MOUNT:/mount:rw,rshared \
           --device /dev/fuse \
           --cap-add SYS_ADMIN \
           --security-opt apparmor:unconfined \
           --restart unless-stopped \
           onedriver