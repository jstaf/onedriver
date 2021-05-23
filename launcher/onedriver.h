#pragma once

#include <stdbool.h>

#define ONEDRIVER_NAME "onedriver"
#define ONEDRIVER_SERVICE_TEMPLATE "onedriver@.service"
#define XDG_VOLUME_INFO ".xdg-volume-info"

void fs_poll_until_avail(const char *mountpoint, int timeout);
char *fs_account_name(const char *mountpoint);
bool fs_mountpoint_is_valid(const char *mountpoint);
char **fs_known_mounts();
char *escape_home(const char *path);
char *unescape_home(const char *path);
