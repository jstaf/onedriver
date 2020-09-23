#pragma once

#include <stdbool.h>

#define DIGITS "0123456789"
#define LOWERCASE_LETTERS "abcdefghijklmnopqrstuvwxyz"
#define UPPERCASE_LETTERS "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
#define LETTERS LOWERCASE_LETTERS UPPERCASE_LETTERS
#define VALID_CHARS DIGITS LETTERS ":-_.\\"

#define SYSTEMD_BUS_NAME "org.freedesktop.systemd1"
#define SYSTEMD_OBJECT_PATH "/org/freedesktop/systemd1"

char systemd_hexchar(int x);
int systemd_unhexchar(char c);
char *systemd_escape(const char *str);
char *systemd_unescape(const char *str);
int systemd_path_escape(const char *path, char **ret);
int systemd_template_unit(const char *template, const char *instance, char **ret);
bool systemd_unit_is_active(const char *unit_name);
bool systemd_unit_set_active(const char *unit_name, bool active);
bool systemd_unit_is_enabled(const char *unit_name);
bool systemd_unit_set_enabled(const char *unit_name, bool enabled);
