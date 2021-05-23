// for strdupa
#define _GNU_SOURCE

#include <assert.h>
#include <errno.h>
#include <gio/gio.h>
#include <glib.h>
#include <glib/gi18n.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "systemd.h"

char systemd_hexchar(int x) {
    static const char table[16] = "0123456789abcdef";
    return table[x & 15];
}

int systemd_unhexchar(char c) {
    if (c >= '0' && c <= '9') {
        return c - '0';
    }
    if (c >= 'a' && c <= 'f') {
        return c - 'a' + 10;
    }
    if (c >= 'A' && c <= 'F') {
        return c - 'A' + 10;
    }
    return -EINVAL;
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
 * The returned string should be freed.
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
    *r = '\0';
    return repl;
}

char *systemd_unescape(const char *f) {
    char *r = strdup(f);
    if (!r) {
        return NULL;
    }

    char *t;
    for (t = r; *f; f++) {
        if (*f == '-') {
            *(t++) = '/';
        } else if (*f == '\\') {
            if (f[1] != 'x') {
                free(r);
                return NULL;
            }

            int a = systemd_unhexchar(f[2]);
            if (a < 0) {
                free(r);
                return NULL;
            }

            int b = systemd_unhexchar(f[3]);
            if (b < 0) {
                free(r);
                return NULL;
            }

            *(t++) = (char)(((uint8_t)a << 4U) | (uint8_t)b);
            f += 3;
        } else {
            *(t++) = *f;
        }
    }
    *t = 0;
    return r;
}

/**
 * Escape a systemd unit path. Most logic is borrowed from:
 * https://github.com/systemd/systemd/blob/master/src/basic/unit-name.c
 * ret should be freed by the caller.
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
 * Unescape a systemd unit path.
 */
int systemd_path_unescape(const char *instance, char **ret) {
    char *unescaped = systemd_unescape(instance);
    if (!unescaped) {
        return -1;
    }
    *ret = malloc(strlen(unescaped) + 1);
    (*ret)[0] = '/';
    strcpy(*ret + 1, unescaped);
    free(unescaped);
    return 0;
}

/**
 * Perform the function of the CLI utility systemd-escape.
 * Logic based on unit_name_replace_instance fromq
 * https://github.com/systemd/systemd/blob/master/src/basic/unit-name.c
 * ret should be freed by the caller.
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
 * Untemplate a systemd unit name.
 */
int systemd_untemplate_unit(const char *unit_name, char **ret) {
    int start = -1;
    int end = strlen(unit_name);
    for (int i = 0; i < strlen(unit_name); i++) {
        if (unit_name[i] == '@') {
            start = i + 1;
        } else if (unit_name[i] == '.') {
            end = i;
        }
    }

    if (start == -1) {
        // no match
        return -1;
    }

    int len = end - start;
    char *instance = malloc(len);
    strncpy(instance, unit_name + start, len);
    instance[len] = '\0';
    *ret = instance;
    return 0;
}

/**
 * Connect to DBus and create a new proxy object.
 * The resulting GDBusProxy object should be freed via g_object_unref.
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
    GVariant *response = g_dbus_proxy_call_sync(
        proxy, "org.freedesktop.systemd1.Manager.GetUnit",
        g_variant_new("(s)", unit_name), G_DBUS_CALL_FLAGS_NONE, -1, NULL, &err);
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
    r = strcmp(g_variant_get_string(state_var, NULL), "active") == 0;

    g_object_unref(unit_proxy);
    g_variant_unref(state_var);
    return r;
}

/**
 * Turn a systemd unit off or on. Returns true on success.
 */
bool systemd_unit_set_active(const char *unit_name, bool active) {
    GError *err = NULL;
    GDBusProxy *proxy =
        dbus_proxy_new(G_BUS_TYPE_SESSION, SYSTEMD_BUS_NAME, SYSTEMD_OBJECT_PATH,
                       "org.freedesktop.systemd1.Manager", &err);
    if (err) {
        g_error("Could not create systemd dbus proxy: %s\n", err->message);
        g_error_free(err);
        return false;
    }

    char *method_name;
    if (active) {
        method_name = "org.freedesktop.systemd1.Manager.StartUnit";
    } else {
        method_name = "org.freedesktop.systemd1.Manager.StopUnit";
    }
    // call params ref: https://www.freedesktop.org/wiki/Software/systemd/dbus/
    GVariant *response = g_dbus_proxy_call_sync(
        proxy, method_name, g_variant_new("(ss)", unit_name, "replace"),
        G_DBUS_CALL_FLAGS_NONE, -1, NULL, &err);
    g_object_unref(proxy);
    if (err) {
        g_error("Failed while changing unit state to %d: %s\n", (int)active,
                err->message);
        g_error_free(err);
        return false;
    }
    g_variant_unref(response);
    return true;
}

/**
 * systemd_unit_is_enabled returns if a systemd unit is enabled
 */
bool systemd_unit_is_enabled(const char *unit_name) {
    GError *err = NULL;
    GDBusProxy *proxy =
        dbus_proxy_new(G_BUS_TYPE_SESSION, SYSTEMD_BUS_NAME, SYSTEMD_OBJECT_PATH,
                       "org.freedesktop.systemd1.Manager", &err);
    if (err) {
        g_error("Could not create systemd dbus proxy: %s\n", err->message);
        g_error_free(err);
        return false;
    }

    GVariant *response = g_dbus_proxy_call_sync(
        proxy, "org.freedesktop.systemd1.Manager.GetUnitFileState",
        g_variant_new("(s)", unit_name), G_DBUS_CALL_FLAGS_NONE, -1, NULL, &err);
    g_object_unref(proxy);
    if (err) {
        g_error("Could not determine unit file state: %s\n", err->message);
        g_error_free(err);
        return false;
    }

    const gchar *enabled_state;
    g_variant_get(response, "(s)", &enabled_state);
    bool r = strcmp(enabled_state, "enabled") == 0;

    g_variant_unref(response);
    return r;
}

/**
 * Enable or disable a user systemd unit. Returns true on success.
 */
bool systemd_unit_set_enabled(const char *unit_name, bool enabled) {
    GError *err = NULL;
    GDBusProxy *proxy =
        dbus_proxy_new(G_BUS_TYPE_SESSION, SYSTEMD_BUS_NAME, SYSTEMD_OBJECT_PATH,
                       "org.freedesktop.systemd1.Manager", &err);
    if (err) {
        g_error("Could not create systemd dbus proxy: %s\n", err->message);
        g_error_free(err);
        return false;
    }

    GVariantBuilder *unit_name_builder = g_variant_builder_new(G_VARIANT_TYPE("as"));
    g_variant_builder_add(unit_name_builder, "s", unit_name);
    GVariant *response;
    if (enabled) { // different method calls depending on enable/disable
        // ref: https://www.freedesktop.org/wiki/Software/systemd/dbus/
        // call_params: unit files, persistent (/etc vs /run), replace links
        response = g_dbus_proxy_call_sync(
            proxy, "org.freedesktop.systemd1.Manager.EnableUnitFiles",
            g_variant_new("(asbb)", unit_name_builder, false, true),
            G_DBUS_CALL_FLAGS_NONE, -1, NULL, &err);
    } else {
        // call_params: unit files, persistent
        response = g_dbus_proxy_call_sync(
            proxy, "org.freedesktop.systemd1.Manager.DisableUnitFiles",
            g_variant_new("(asb)", unit_name_builder, false), G_DBUS_CALL_FLAGS_NONE, -1,
            NULL, &err);
    }
    g_variant_builder_unref(unit_name_builder);
    g_object_unref(proxy);
    if (err) {
        g_error("Could not change systemd unit enabled state to %d: %s\n", (int)enabled,
                err->message);
        g_error_free(err);
        return false;
    }
    g_variant_unref(response);
    return true;
}
