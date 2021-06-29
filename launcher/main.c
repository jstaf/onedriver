#include <gtk/gtk.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>

#include "dir_chooser.h"
#include "onedriver.h"
#include "systemd.h"

// some useful icon constants (from gtk3-icon-browser)
#define PLUS_ICON "list-add-symbolic"
#define MINUS_ICON "user-trash-symbolic"
#define MOUNT_ICON "folder-remote-symbolic"
#define UNMOUNT_ICON "media-eject-symbolic"
#define ENABLED_ICON "object-select-symbolic"

#define MOUNT_MESSAGE "Mount or unmount selected OneDrive account"

static GHashTable *mounts;

/**
 * Enable or disable a mountpoint when button is clicked.
 */
static void enable_mountpoint_cb(GtkWidget *widget, char *unit_name) {
    gboolean active = gtk_toggle_button_get_active(GTK_TOGGLE_BUTTON(widget));
    if (systemd_unit_set_enabled(unit_name, (bool)active)) {
        // set the toggle state if systemd call was successful
        gtk_toggle_button_set_active(GTK_TOGGLE_BUTTON(widget), (gboolean)active);
    }
}

/**
 * Start or stop the mountpoint for an acccount.
 */
static void activate_mount_cb(GtkWidget *widget, gboolean state, char *unit_name) {
    if (!systemd_unit_set_active(unit_name, state)) {
        gtk_switch_set_active(GTK_SWITCH(widget), !state);
    }
}

/**
 * Delete the mountpoint after prompting for confirmation.
 */
static void delete_mount_cb(GtkWidget *widget, char *unit_name) {
    GtkWidget *window = gtk_widget_get_ancestor(widget, GTK_TYPE_WINDOW);
    GtkWidget *dialog = gtk_dialog_new_with_buttons(
        "Remove mountpoint?", GTK_WINDOW(window), GTK_DIALOG_MODAL, "Cancel",
        GTK_RESPONSE_REJECT, "Remove", GTK_RESPONSE_ACCEPT, NULL);

    if (gtk_dialog_run(GTK_DIALOG(dialog)) == GTK_RESPONSE_ACCEPT) {
        systemd_unit_set_enabled(unit_name, false);
        systemd_unit_set_active(unit_name, false);

        char *instance;
        char *path = malloc(512);
        const char *cachedir = g_get_user_cache_dir();

        systemd_untemplate_unit(unit_name, &instance);
        sprintf(path, "%s/onedriver/%s/auth_tokens.json", cachedir, instance);
        remove(path);
        sprintf(path, "%s/onedriver/%s/onedriver.db", cachedir, instance);
        remove(path);
        sprintf(path, "%s/onedriver/%s/", cachedir, instance);
        rmdir(path);

        free(instance);
        free(path);
        gtk_widget_destroy(gtk_widget_get_ancestor(widget, GTK_TYPE_LIST_BOX_ROW));
    }
    gtk_widget_destroy(dialog);
}

/**
 * Open the mountpoint when a user clicks on it.
 */
static void activate_row_cb(GtkListBox *box, GtkListBoxRow *row, gpointer user_data) {
    const char *mount = g_hash_table_lookup(mounts, row);

    // start the mount if it's not started already
    char *unit_name, *escaped;
    systemd_path_escape(mount, &escaped);
    systemd_template_unit(ONEDRIVER_SERVICE_TEMPLATE, escaped, &unit_name);
    if (!systemd_unit_is_active(unit_name)) {
        // TODO update the mountpoint button to reflect that it's been set active
        systemd_unit_set_active(unit_name, true);
        fs_poll_until_avail(mount, 10);
    }
    free(unit_name);
    free(escaped);

    char uri[512] = "file://";
    strncat(uri, mount, 504);
    g_app_info_launch_default_for_uri(uri, NULL, NULL);
}

/**
 * Creates a new listboxrow for use in the main gui listbox.
 */
static GtkWidget *new_mount_row(char *mount) {
    GtkWidget *row = gtk_list_box_row_new();
    gtk_list_box_row_set_selectable(GTK_LIST_BOX_ROW(row), TRUE);
    GtkWidget *box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 5);
    gtk_container_add(GTK_CONTAINER(row), box);

    char *escaped_path, *unit_name;
    systemd_path_escape(mount, &escaped_path);
    systemd_template_unit(ONEDRIVER_SERVICE_TEMPLATE, escaped_path, &unit_name);

    char *tilde_path = escape_home(mount);
    char *account_name = fs_account_name(escaped_path);
    GtkWidget *label;
    if (account_name) {
        label = gtk_label_new(NULL);
        char *markup = g_markup_printf_escaped(
            "\%s <span style=\"italic\" weight=\"light\">(\%s)</span>    ", account_name,
            tilde_path);
        gtk_label_set_markup(GTK_LABEL(label), markup);
        free(markup);
        free(account_name);
    } else {
        label = gtk_label_new(tilde_path);
    }
    free(tilde_path);
    free(escaped_path);
    // unit_name is not freed - it is used by callbacks when triggered at a later date
    gtk_box_pack_start(GTK_BOX(box), label, FALSE, FALSE, 5);

    GtkWidget *delete_mountpoint_btn =
        gtk_button_new_from_icon_name(MINUS_ICON, GTK_ICON_SIZE_BUTTON);
    gtk_widget_set_tooltip_text(delete_mountpoint_btn,
                                "Remove OneDrive account from local computer");
    g_signal_connect(delete_mountpoint_btn, "clicked", G_CALLBACK(delete_mount_cb),
                     unit_name);
    gtk_box_pack_end(GTK_BOX(box), delete_mountpoint_btn, FALSE, FALSE, 0);

    // add a button to enable the mountpoint
    GtkWidget *unit_enabled_btn = gtk_toggle_button_new();
    GtkWidget *enabled_img =
        gtk_image_new_from_icon_name(ENABLED_ICON, GTK_ICON_SIZE_BUTTON);
    gtk_button_set_image(GTK_BUTTON(unit_enabled_btn), enabled_img);
    gtk_widget_set_tooltip_text(unit_enabled_btn, "Start mountpoint on login");
    gtk_toggle_button_set_active(GTK_TOGGLE_BUTTON(unit_enabled_btn),
                                 (gboolean)systemd_unit_is_enabled(unit_name));
    g_signal_connect(unit_enabled_btn, "toggled", G_CALLBACK(enable_mountpoint_cb),
                     unit_name);
    gtk_box_pack_end(GTK_BOX(box), unit_enabled_btn, FALSE, FALSE, 0);

    // and a button to actually start/stop the mountpoint

    GtkWidget *mount_toggle = gtk_switch_new();
    gtk_switch_set_active(GTK_SWITCH(mount_toggle), systemd_unit_is_active(unit_name));
    gtk_widget_set_tooltip_text(mount_toggle, MOUNT_MESSAGE);
    g_signal_connect(mount_toggle, "state-set", G_CALLBACK(activate_mount_cb), unit_name);
    gtk_box_pack_end(GTK_BOX(box), mount_toggle, FALSE, FALSE, 0);

    g_hash_table_insert(mounts, row, strdup(mount));
    return row;
}

/**
 * Callback for creating a new mountpoint.
 */
static void new_mountpoint_cb(GtkWidget *widget, GtkListBox *box) {
    char *unit_name, *mount, *escaped_mountpoint;

    mount = dir_chooser("Select a mountpoint");
    if (!fs_mountpoint_is_valid(mount)) {
        g_print(
            "Mountpoint \"%s\" was not valid. Mountpoint must be an empty directory.\n",
            mount);
        free(mount);
        return;
    }

    systemd_path_escape(mount, &escaped_mountpoint);
    systemd_template_unit(ONEDRIVER_SERVICE_TEMPLATE, escaped_mountpoint, &unit_name);

    // start the mountpoint and open it
    systemd_unit_set_active(unit_name, true);
    fs_poll_until_avail(mount, -1);
    char uri[512] = "file://";
    strncat(uri, mount, 506);
    g_app_info_launch_default_for_uri(uri, NULL, NULL);

    // now create the row
    GtkWidget *row = new_mount_row(mount);
    gtk_list_box_insert(box, row, -1);
    gtk_widget_show_all(row);

    free(mount);
    free(unit_name);
    free(escaped_mountpoint);
}

/**
 * Actually creates the GUI window.
 */
static void activate(GtkApplication *app, gpointer data) {
    mounts = g_hash_table_new(g_direct_hash, g_direct_equal);

    GtkWidget *window = gtk_application_window_new(app);
    gtk_window_set_default_size(GTK_WINDOW(window), 550, 400);

    GtkWidget *header = gtk_header_bar_new();
    gtk_header_bar_set_show_close_button(GTK_HEADER_BAR(header), TRUE);
    gtk_header_bar_set_title(GTK_HEADER_BAR(header), "onedriver");
    gtk_window_set_titlebar(GTK_WINDOW(window), header);

    GtkWidget *listbox = gtk_list_box_new();
    gtk_container_add(GTK_CONTAINER(window), listbox);
    gtk_list_box_set_activate_on_single_click(GTK_LIST_BOX(listbox), TRUE);
    gtk_list_box_drag_unhighlight_row(GTK_LIST_BOX(listbox));
    g_signal_connect(GTK_LIST_BOX(listbox), "row-activated", G_CALLBACK(activate_row_cb),
                     NULL);

    GtkWidget *mountpoint_btn =
        gtk_button_new_from_icon_name(PLUS_ICON, GTK_ICON_SIZE_BUTTON);
    gtk_widget_set_tooltip_text(mountpoint_btn, "Add a new OneDrive account");
    g_signal_connect(mountpoint_btn, "clicked", G_CALLBACK(new_mountpoint_cb), listbox);
    gtk_header_bar_pack_start(GTK_HEADER_BAR(header), mountpoint_btn);

    char **existing_mounts = fs_known_mounts();
    for (char **found = existing_mounts; *found; found++) {
        GtkWidget *row = new_mount_row(*found);
        gtk_list_box_insert(GTK_LIST_BOX(listbox), row, -1);
        free(*found);
    }
    free(existing_mounts);

    gtk_list_box_unselect_all(GTK_LIST_BOX(listbox));
    gtk_widget_show_all(window);
}

int main(int argc, char **argv) {
    GtkApplication *app =
        gtk_application_new("com.github.jstaf.onedriver", G_APPLICATION_FLAGS_NONE);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
    return g_application_run(G_APPLICATION(app), argc, argv);
}
