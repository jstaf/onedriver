#!/usr/bin/env bash
# TODO: Remove compose, separate into docker-run.sh and docker-compose-run.sh?

MOUNTPOINT=$MOUNTPOINT docker compose up --detach
docker compose exec onedriver /build/onedriver --no-browser /mount/
# If exits, clean up container
MOUNTPOINT=$MOUNTPOINT docker compose down
