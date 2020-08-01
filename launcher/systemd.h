#pragma once

#define DIGITS "0123456789"
#define LOWERCASE_LETTERS "abcdefghijklmnopqrstuvwxyz"
#define UPPERCASE_LETTERS "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
#define LETTERS LOWERCASE_LETTERS UPPERCASE_LETTERS
#define VALID_CHARS DIGITS LETTERS ":-_.\\"

char hexchar(int x);
char *unit_name_escape(const char *str);
int unit_name_path_escape(const char *path, char **ret);
int unit_name_replace_instance(const char *template, const char *instance, char **ret);
