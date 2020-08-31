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

    // from systemd's do_escape
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
            // '/' becomes '-'
            *(r++) = '-';
        } else if (*s == '-' || *s == '\\' || !strchr(VALID_CHARS, *s)) {
            // escape symbols
            r = escape_char(*s, r);
        } else {
            // leave characters in VALID_CHARS untouched
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

/**
 * Connect to DBus and create a new proxy object.
 */
static GDBusProxy *dbus_proxy_new(GBusType type, const char *bus_name,
                                  const char *object_path, const char *interface,
                                  GError **err) {
    GDBusProxy *proxy = NULL;
    GDBusConnection *bus = g_bus_get_sync(type, NULL, err);
    if (!*err) {
        proxy = g_dbus_proxy_new_sync(bus, G_DBUS_PROXY_FLAGS_NONE, NULL, bus_name,
                                      object_path, interface, NULL, err);
    }
    g_object_unref(bus);
    return proxy;
}

/**
 * systemd_unit_is_active will return true if a systemd unit is currently running
 */
bool systemd_unit_is_active(const char *unit_name) {
    bool r = false;
    GError *err = NULL;

    // get the service unit path from systemd
    GDBusProxy *proxy =
        dbus_proxy_new(G_BUS_TYPE_SESSION, SYSTEMD_BUS_NAME, SYSTEMD_OBJECT_PATH,
                       "org.freedesktop.systemd1.Manager", &err);
    if (err) {
        g_error("Could not create dbus proxy: %s\n", err->message);
        g_error_free(err);
        return r;
    }
    GVariant *call_params = g_variant_new("(s)", unit_name);
    GVariant *response =
        g_dbus_proxy_call_sync(proxy, "org.freedesktop.systemd1.Manager.GetUnit",
                               call_params, G_DBUS_CALL_FLAGS_NONE, -1, NULL, &err);
    g_object_unref(proxy);
    if (err) {
        if (strstr(err->message, "org.freedesktop.systemd1.NoSuchUnit")) {
            return r;
        }
        g_error("dbus error: %s\n", err->message);
        g_error_free(err);
        return r;
    }

    // get systemd unit's ActiveState property
    const gchar *unit_path;
    g_variant_get(response, "(o)", &unit_path);
    GDBusProxy *unit_proxy =
        dbus_proxy_new(G_BUS_TYPE_SESSION, SYSTEMD_BUS_NAME, unit_path,
                       "org.freedesktop.systemd1.Unit", &err);
    g_variant_unref(response);
    if (err) {
        g_error("Could not create systemd dbus proxy: %s\n", err->message);
        g_error_free(err);
        return r;
    }
    GVariant *state_var = g_dbus_proxy_get_cached_property(unit_proxy, "ActiveState");
    const gchar *state = g_variant_get_string(state_var, NULL);
    r = strcmp(state, "active") == 0;

    g_object_unref(unit_proxy);
    g_variant_unref(state_var);
    return r;
}

/**
 * systemd_unit_is_enabled returns if a systemd unit is enabled
 */
bool systemd_unit_is_enabled(const char *unit_name) {
    bool r = false;
    GError *err = NULL;

    GDBusProxy *proxy =
        dbus_proxy_new(G_BUS_TYPE_SESSION, SYSTEMD_BUS_NAME, SYSTEMD_OBJECT_PATH,
                       "org.freedesktop.systemd1.Manager", &err);
    if (err) {
        g_error("Could not create systemd dbus proxy: %s\n", err->message);
        g_error_free(err);
        return r;
    }

    GVariant *call_params = g_variant_new("(s)", unit_name);
    GVariant *response =
        g_dbus_proxy_call_sync(proxy, "org.freedesktop.systemd1.Manager.GetUnitFileState",
                               call_params, G_DBUS_CALL_FLAGS_NONE, -1, NULL, &err);
    if (err) {
        g_error("Could not determine unit file state: %s\n", err->message);
        g_error_free(err);
        return r;
    }

    const gchar *enabled_state;
    g_variant_get(response, "(s)", &enabled_state);
    r = strcmp(enabled_state, "enabled") == 0;

    g_variant_unref(response);
    g_object_unref(proxy);
    return r;
}