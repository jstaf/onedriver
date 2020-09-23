#pragma once

#include <stdbool.h>

#define ONEDRIVER_NAME "onedriver"
#define ONEDRIVER_SERVICE_TEMPLATE "onedriver@.service"
#define XDG_VOLUME_INFO ".xdg-volume-info"

// represents a mountpoint
typedef struct {
    char account_name[1024];
    char mountpoint[1024];
    char systemd_unit[1024];
} fsmount;

void fs_poll_until_avail(const char *mountpoint, int timeout);
char *fs_account_name(const char *mountpoint);
bool fs_mountpoint_is_valid(const char *mountpoint);
char **fs_known_mounts();
