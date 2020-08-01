// for strdupa
#define _GNU_SOURCE

#include <assert.h>
#include <errno.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "systemd.h"

char hexchar(int x) {
    static const char table[16] = "0123456789abcdef";
    return table[x & 15];
}

static char *escape_char(char c, char *t) {
    assert(t);
    *(t++) = '\\';
    *(t++) = 'x';
    *(t++) = hexchar(c >> 4);
    *(t++) = hexchar(c);
    return t;
}

/**
 * This is the function that actually does the escaping. Based on:
 * https://github.com/systemd/systemd/blob/master/src/basic/unit-name.c
 */
char *unit_name_escape(const char *str) {
    assert(str);

    char *repl = malloc(strlen(str) * 4 + 1);
    if (!repl) {
        return NULL;
    }
    // we use new pointers here to avoid modifying the address of the original ones
    char *r = repl;
    char *s = str;

    // do_escape
    // strip leading '.' character if present
    if (*s == '.') {
        repl = escape_char(*s, repl);
        s++;
    }

    for (; *s; s++) {
        if (*s == '/') {
            *(repl++) = '-';
        } else if (*s == '-' || *s == '\\' || !strchr(VALID_CHARS, *s)) {
            repl = escape_char(*s, repl);
        } else {
            *(repl++) = *s;
        }
    }
    return r;
}

/**
 * Escape a systemd unit path. Most logic is borrowed from:
 * https://github.com/systemd/systemd/blob/master/src/basic/unit-name.c
 */
int unit_name_path_escape(const char *path, char **ret) {
    assert(path);
    assert(ret);

    char *p = strdupa(path);
    if (!p) {
        return -ENOMEM;
    }

    char *replaced;
    size_t path_len = strlen(p);
    if (!path_len || strcmp(p, "/") == 0) {
        replaced = strdup("-");
    } else {
        // strip leading and trailing slashes
        if (p[path_len - 1] == '/') {
            p[path_len - 1] = '\0';
        }
        if (p[0] == '/') {
            p++;
        }
        replaced = unit_name_escape(p);
    }
    if (!replaced) {
        return -ENOMEM;
    }

    *ret = replaced;
    return 0;
}

/**
 * Perform the function of the CLI utility systemd-escape.
 * Logic based on unit_name_replace_instance fromq
 * https://github.com/systemd/systemd/blob/master/src/basic/unit-name.c
 */
int unit_name_replace_instance(const char *template, const char *instance, char **ret) {
    assert(template);
    assert(instance);
    assert(ret);

    const char *at_pos = strchr(template, '@');
    const char *dot_pos = strrchr(template, '.');

    size_t a = at_pos - template;
    size_t b = strlen(instance);

    char *replaced = malloc(a + 1 + b + strlen(dot_pos) + 1);
    if (!replaced) {
        return -ENOMEM;
    }

    strcpy(mempcpy(mempcpy(replaced, template, a + 1), instance, b), dot_pos);
    *ret = replaced;
    return 0;
}
