// for strdupa
#define _GNU_SOURCE

#include <assert.h>
#include <errno.h>
#include <gio/gio.h>
#include <glib.h>
#include <glib/gi18n.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>

#include "systemd.h"

char systemd_hexchar(int x) {
    static const char table[16] = "0123456789abcdef";
    return table[x & 15];
}

static char *escape_char(char c, char *t) {
    assert(t);
    *(t++) = '\\';
    *(t++) = 'x';
    *(t++) = systemd_hexchar(c >> 4);
    *(t++) = systemd_hexchar(c);
    return t;
}

/**
 * This is the function that actually does the escaping. Based on:
 * https://github.com/systemd/systemd/blob/master/src/basic/unit-name.c
 */
char *systemd_escape(const char *str) {
    assert(str);

    char *repl = malloc(strlen(str) * 4 + 1);
    if (!repl) {
        return NULL;
    }

    // do_escape
    // we use new pointers here to avoid modifying the address of the original ones
    char *r = repl;
    const char *s = str;

    // escape leading '.'
    if (*s == '.') {
        r = escape_char(*s, r);
        s++;
    }

    for (; *s; s++) {
        if (*s == '/') {
            *(r++) = '-';
        } else if (*s == '-' || *s == '\\' || !strchr(VALID_CHARS, *s)) {
            r = escape_char(*s, r);
        } else {
            *(r++) = *s;
        }
    }
    return repl;
}

/**
 * Escape a systemd unit path. Most logic is borrowed from:
 * https://github.com/systemd/systemd/blob/master/src/basic/unit-name.c
 */
int systemd_path_escape(const char *path, char **ret) {
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
        replaced = systemd_escape(p);
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
int systemd_template_unit(const char *template, const char *instance, char **ret) {
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

int systemd_unit_status(const char *unit_name) {
    GDBusConnection *bus = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, NULL);
    GDBusProxy *proxy = g_dbus_proxy_new_sync(
        bus, G_DBUS_PROXY_FLAGS_NONE, NULL, "org.freedesktop.systemd1",
        "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager", NULL, NULL);

    GVariant *call_params = g_variant_new_string(unit_name);
    GVariant *response =
        g_dbus_proxy_call_sync(proxy, "org.freedesktop.systemd1.Manager.GetUnit",
                               call_params, G_DBUS_CALL_FLAGS_NONE, -1, NULL, NULL);

    g_print("variant: %s\n", g_variant_get_string(response, NULL));

    g_variant_unref(call_params);
    g_variant_unref(response);
    g_object_unref(proxy);
    g_object_unref(bus);
    return 0;
}
