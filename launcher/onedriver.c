#define _DEFAULT_SOURCE

#include <dirent.h>
#include <glib.h>
#include <json-glib/json-glib.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#include "onedriver.h"
#include "systemd.h"

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
 * Grab the FS account name from auth_tokens.json. Returned value should be freed by
 * caller.
 */
char *fs_account_name(const char *instance) {
    const char *cachedir = g_get_user_cache_dir();
    char *fname = malloc(512);
    sprintf(fname, "%s/onedriver/%s/auth_tokens.json", cachedir, instance);

    char *account_name = NULL;
    GError *error = NULL;
    JsonParser *parser = json_parser_new();
    json_parser_load_from_file(parser, fname, &error);
    if (error) {
        g_error("%s", error->message);
        g_error_free(error);
        g_object_unref(parser);
        free(fname);
        return account_name;
    }

    JsonReader *reader = json_reader_new(json_parser_get_root(parser));
    if (json_reader_read_member(reader, "account")) {
        account_name = strdup(json_reader_get_string_value(reader));
    }
    json_reader_end_member(reader);

    g_object_unref(reader);
    g_object_unref(parser);
    free(fname);
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

/**
 * Get a null-terminated array of strings, each corresponding to the path of a mountpoint.
 * These are detected from the folder names of onedriver's cache dir.;
 */
char **fs_known_mounts() {
    char *cachedir = malloc(strlen(g_get_user_cache_dir()) + strlen(ONEDRIVER_NAME) + 2);
    strcat(strcat(strcpy(cachedir, g_get_user_cache_dir()), "/"), ONEDRIVER_NAME);
    DIR *cache = opendir(cachedir);
    if (!cache) {
        char **r = malloc(sizeof(char *));
        *r = NULL;
        return r;
    }
    free(cachedir);

    int idx = 0;
    int size = 10;
    char **r = calloc(sizeof(char *), size);
    struct dirent *entry;
    while ((entry = readdir(cache)) != NULL) {
        if (entry->d_type & DT_DIR && entry->d_name[0] != '.') {
            // unescape the systemd unit name of all folders in cache directory
            char *path = systemd_unescape((const char *)&(entry->d_name));
            char *fullpath = malloc(strlen(path) + 2);
            memcpy(fullpath, "/\0", 2);
            strcat(fullpath, path);
            free(path);

            // do the mountpoints they point to actually exist?
            struct stat st;
            if (stat(fullpath, &st) == 0 && st.st_mode & S_IFDIR) {
                // yep, add em
                r[idx++] = fullpath;
                if (idx > size) {
                    size *= 2;
                    r = realloc(r, size * sizeof(char *));
                }
            }
        }
    }
    r[idx] = NULL;
    return r;
}

/**
 * Strip the /home/username part from a path and replace it with "~". Result should be
 * freed by caller.
 */
char *escape_home(const char *path) {
    const char *homedir = g_get_home_dir();
    int len = strlen(homedir);
    if (strncmp(path, homedir, len) == 0) {
        char *replaced = strdup(path + len - 1);
        replaced[0] = '~';
        return replaced;
    }
    return strdup(path);
}

/**
 * Replace the tilde in a path with the absolute path
 */
char *unescape_home(const char *path) {
    if (path[0] == '/') {
        return strdup(path);
    }
    const char *homedir = g_get_home_dir();
    int len = strlen(homedir);
    char *new_path = malloc(strlen(path) - 1 + len);
    return strcat(strcpy(new_path, homedir), path + 1);
}
