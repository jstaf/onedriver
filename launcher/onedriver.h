#pragma once

#define ONEDRIVER_SERVICE_TEMPLATE "onedriver@.service"

/*
 * Block until the filesystem becomes available.
 */
void poll_fs_availability(const char *mountpoint);
