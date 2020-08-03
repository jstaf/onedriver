#pragma once

#define DIGITS "0123456789"
#define LOWERCASE_LETTERS "abcdefghijklmnopqrstuvwxyz"
#define UPPERCASE_LETTERS "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
#define LETTERS LOWERCASE_LETTERS UPPERCASE_LETTERS
#define VALID_CHARS DIGITS LETTERS ":-_.\\"

char systemd_hexchar(int x);
char *systemd_escape(const char *str);
int systemd_path_escape(const char *path, char **ret);
int systemd_template_unit(const char *template, const char *instance, char **ret);
int systemd_unit_status(const char *unit_name);
