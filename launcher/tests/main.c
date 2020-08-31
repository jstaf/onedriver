#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "../systemd.h"
#include "minunit.h"

MU_TEST(systemd_test_path_escape) {
    char *escaped;
    systemd_path_escape("/home/test/yesYes", &escaped);
    mu_check(strcmp(escaped, "home-test-yesYes") == 0);
    free(escaped);

    systemd_path_escape("words@ test", &escaped);
    mu_check(strcmp(escaped, "words\\x40\\x20test") == 0);
    free(escaped);
}

MU_TEST_SUITE(systemd_tests) { MU_RUN_TEST(systemd_test_path_escape); }

int main(int argc, char **argv) {
    MU_RUN_SUITE(systemd_tests);
    MU_REPORT();
    return MU_EXIT_CODE;
}
