#include <dirent.h>
#include <stdbool.h>
#include <string.h>
#include <sys/types.h>
#include <unistd.h>

#include "onedriver.h"

void poll_fs_availability(const char *mountpoint) {
    bool found = false;
    for (int i = 0; i < 100; i++) {
        DIR *dir = opendir(mountpoint);
        struct dirent *entry;
        while ((entry = readdir(dir)) != NULL) {
            if (strcmp(entry->d_name, ".xdg-volume-info") == 0) {
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
