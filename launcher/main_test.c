#include <stdlib.h>
#include <string.h>

#include "minunit.h"
#include "systemd.h"

MU_TEST(test_systemd_path_escape) {
    char *escaped;
    systemd_path_escape("/home/test/yesYes", &escaped);
    mu_check(strcmp(escaped, "home-test-yesYes") == 0);
    free(escaped);

    systemd_path_escape("words@ test", &escaped);
    mu_check(strcmp(escaped, "words\\x40\\x20test") == 0);
    free(escaped);
}

MU_TEST(test_systemd_template_unit) {
    char *escaped;
    systemd_template_unit("onedriver@.service", "this-is-a-test", &escaped);
    mu_check(strcmp(escaped, "onedriver@this-is-a-test.service") == 0);
    free(escaped);
}

MU_TEST_SUITE(systemd_tests) {
    MU_RUN_TEST(test_systemd_path_escape);
    MU_RUN_TEST(test_systemd_template_unit);
}

int main(int argc, char **argv) {
    MU_RUN_SUITE(systemd_tests);
    MU_REPORT();
    return MU_EXIT_CODE;
}
