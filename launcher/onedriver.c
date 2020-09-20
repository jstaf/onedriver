#include <dirent.h>
#include <glib.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>
#include <unistd.h>

#include "onedriver.h"

/**
 * Block until the fs is available, or a timeout is reached. If the timeout is
 * -1, will wait until a default of 120 seconds.
 */
void fs_poll_until_avail(const char *mountpoint, int timeout) {
    bool found = false;
    if (timeout == -1) {
        timeout = 120;
    }
    for (int i = 0; i < timeout * 10; i++) {
        DIR *dir = opendir(mountpoint);
        if (!dir) {
            return;
        }
        struct dirent *entry;
        while ((entry = readdir(dir)) != NULL) {
            if (strcmp(entry->d_name, XDG_VOLUME_INFO) == 0) {
                found = true;
                break;
            }
        }
        closedir(dir);

        if (found) {
            break;
        }
        usleep(100 * 1000); // 0.1 seconds
    }
}

/**
 * Grab the FS account name from .xdg-volume-info. Returned value should be freed by
 * caller.
 */
char *fs_account_name(const char *mount_name) {
    int mount_len = strlen(mount_name);
    char fname[mount_len + strlen(XDG_VOLUME_INFO) + 2];
    strcpy((char *)&fname, mount_name);
    strcat((char *)&fname, "/");
    strcat((char *)&fname, XDG_VOLUME_INFO);
    FILE *file = fopen(fname, "r");
    if (file == NULL) {
        g_error("Could not open file %s\n", fname);
        return NULL;
    }

    char *account_name = NULL;
    char *line = NULL;
    size_t len = 0;
    while (getline(&line, &len, file) != -1) {
        if (strncmp(line, "Name=", 5) == 0) {
            account_name = strdup(line + 5);
            account_name[strlen(account_name) - 1] = '\0'; // get rid of newline
            break;
        }
    }
    fclose(file);
    return account_name;
}

/**
 * Check that the mountpoint is actually valid: mounpoint exists and nothing is in it.
 */
bool fs_mountpoint_is_valid(const char *mountpoint) {
    if (!mountpoint || !strlen(mountpoint)) {
        return false;
    }

    bool valid = true;
    DIR *dir = opendir(mountpoint);
    if (!dir) {
        return false;
    }
    struct dirent *entry;
    while ((entry = readdir(dir)) != NULL) {
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0) {
            continue;
        }
        valid = false;
        break;
    }
    closedir(dir);

    return valid;
}
