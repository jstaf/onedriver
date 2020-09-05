#include <dirent.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <unistd.h>

#include "minunit.h"
#include "systemd.h"

#define ONEDRIVER_SERVICE_NAME "onedriver@.service"

// does systemd path escaping work correctly?
MU_TEST(test_systemd_path_escape) {
    char *escaped;
    systemd_path_escape("/home/test/yesYes", &escaped);
    mu_check(strcmp(escaped, "home-test-yesYes") == 0);
    free(escaped);

    systemd_path_escape("words@ test", &escaped);
    mu_check(strcmp(escaped, "words\\x40\\x20test") == 0);
    free(escaped);
}

// does systemd unit name templating work correctly?
MU_TEST(test_systemd_template_unit) {
    char *escaped;
    systemd_template_unit(ONEDRIVER_SERVICE_NAME, "this-is-a-test", &escaped);
    mu_check(strcmp(escaped, "onedriver@this-is-a-test.service") == 0);
    free(escaped);
}

// can we enable and disable systemd units? (and correctly check if the units are
// enabled/disabled?)
MU_TEST(test_systemd_unit_enabled) {
    char cwd[1024];
    getcwd(cwd, 1024);
    strcat(cwd, "/mount");

    char *cwd_escaped, *unit_name;
    systemd_path_escape(cwd, &cwd_escaped);
    systemd_template_unit(ONEDRIVER_SERVICE_NAME, cwd_escaped, &unit_name);
    free(cwd_escaped);

    // make sure things are disabled before test start
    mu_check(systemd_unit_set_enabled(unit_name, false));
    mu_check(!systemd_unit_is_enabled(unit_name));

    // actual test content
    mu_assert(systemd_unit_set_enabled(unit_name, true), "Could not enable unit.");
    mu_assert(systemd_unit_is_enabled(unit_name),
              "Unit could not detect unit as enabled.");
    mu_assert(systemd_unit_set_enabled(unit_name, false), "Could not disable unit.");
    mu_assert(!systemd_unit_is_enabled(unit_name),
              "Unit was still enabled after disabling.");

    free(unit_name);
}

// can we enable and disable the onedriver service (and correctly check if the unit is
// active/stopped?)
MU_TEST(test_systemd_unit_active) {
    char cwd[1024];
    getcwd(cwd, 1024);
    strcat(cwd, "/mount");

    char *cwd_escaped, *unit_name;
    systemd_path_escape(cwd, &cwd_escaped);
    systemd_template_unit(ONEDRIVER_SERVICE_NAME, cwd_escaped, &unit_name);
    free(cwd_escaped);

    // make sure things are off before we start
    mu_check(systemd_unit_set_active(unit_name, false));
    mu_check(!systemd_unit_is_active(unit_name));

    mu_assert(systemd_unit_set_active(unit_name, true), "Could not start unit.");
    mu_assert(systemd_unit_is_active(unit_name), "Did not detect unit as active");

    // is the actual service started? we should be able to find .xdg-volume-info if so...
    DIR *dir = opendir(cwd);
    struct dirent *entry;
    bool found = false;
    while ((entry = readdir(dir)) != NULL) {
        if (strcmp(entry->d_name, ".xdg-volume-info") == 0) {
            found = true;
            break;
        }
    }
    closedir(dir);
    mu_assert(found, "Could not find .xdg-volume-info in mounted directory");

    mu_assert(systemd_unit_set_active(unit_name, false), "Could not stop unit.");
    mu_assert(systemd_unit_is_active(unit_name), "Did not detect unit as stopped");

    free(unit_name);
}

MU_TEST_SUITE(systemd_tests) {
    MU_RUN_TEST(test_systemd_path_escape);
    MU_RUN_TEST(test_systemd_template_unit);
    MU_RUN_TEST(test_systemd_unit_enabled);
    MU_RUN_TEST(test_systemd_unit_active);
}

int main(int argc, char **argv) {
    MU_RUN_SUITE(systemd_tests);
    MU_REPORT();
    return MU_EXIT_CODE;
}
