#include <gtk/gtk.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>

#include "dir_chooser.h"
#include "onedriver.h"
#include "systemd.h"

// some useful icon constants (from gtk3-icon-browser)
#define PLUS_ICON "list-add-symbolic"
#define MINUS_ICON "list-remove-symbolic"
#define MOUNT_ICON "folder-remote-symbolic"
#define UNMOUNT_ICON "media-eject-symbolic"
#define ENABLED_ICON "system-reboot-symbolic"

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

static void activate_mount_cb(GtkWidget *widget, char *unit_name) {
    // TODO
    // * start/stop mountpoint
    // * set busy signal + make icon unclickable
    // * (if active) poll for filesystem availability
    // * set correct icon + make icon clickable
}

static void delete_mount_cb(GtkWidget *widget, char *unit_name) {
    // TODO
    // * present user with "are you sure? dialog
    // * disable mount
    // * stop mount
    // * rmtree cache folder
    gtk_widget_destroy(gtk_widget_get_ancestor(widget, GTK_TYPE_LIST_BOX_ROW));
}

static GtkWidget *new_mount_row(char *mount) {
    GtkWidget *row = gtk_list_box_row_new();
    gtk_list_box_row_set_selectable(GTK_LIST_BOX_ROW(row), FALSE);
    GtkWidget *box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 5);
    gtk_container_add(GTK_CONTAINER(row), box);

    GtkWidget *name = gtk_label_new(mount);
    gtk_box_pack_start(GTK_BOX(box), name, FALSE, FALSE, 5);

    char *escaped_path, *unit_name;
    systemd_path_escape(mount, &escaped_path);
    systemd_template_unit(ONEDRIVER_SERVICE_TEMPLATE, escaped_path, &unit_name);
    free(escaped_path);
    // unit name is not freed - it is used by callbacks when triggered at a later date

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
    char *icon_name = systemd_unit_is_active(unit_name) ? UNMOUNT_ICON : MOUNT_ICON;
    GtkWidget *mount_toggle =
        gtk_button_new_from_icon_name(icon_name, GTK_ICON_SIZE_BUTTON);
    gtk_widget_set_tooltip_text(mount_toggle, "Mount selected OneDrive account");
    g_signal_connect(mount_toggle, "clicked", G_CALLBACK(activate_mount_cb), unit_name);
    gtk_box_pack_end(GTK_BOX(box), mount_toggle, FALSE, FALSE, 0);

    return row;
}

static void mountpoint_cb(GtkWidget *widget, GtkListBox *box) {
    static bool has_mount = false;
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

    GtkWidget *row = new_mount_row(mount);
    gtk_list_box_insert(box, row, -1);
    gtk_widget_show_all(row);

    free(mount);
    free(unit_name);
    free(escaped_mountpoint);

    if (!has_mount) {
        // remove how-to message now that the user has a mount
        gtk_widget_destroy(GTK_WIDGET(gtk_list_box_get_row_at_index(box, 0)));
        has_mount = true;
    }
}

static void activate(GtkApplication *app, gpointer data) {
    GtkWidget *window = gtk_application_window_new(app);
    gtk_window_set_default_size(GTK_WINDOW(window), 550, 400);

    GtkWidget *header = gtk_header_bar_new();
    gtk_header_bar_set_show_close_button(GTK_HEADER_BAR(header), TRUE);
    gtk_header_bar_set_title(GTK_HEADER_BAR(header), "onedriver");
    gtk_window_set_titlebar(GTK_WINDOW(window), header);

    GtkWidget *listbox = gtk_list_box_new();
    gtk_container_add(GTK_CONTAINER(window), listbox);
    gtk_list_box_set_activate_on_single_click(GTK_LIST_BOX(listbox), FALSE);
    gtk_list_box_drag_unhighlight_row(GTK_LIST_BOX(listbox));

    GtkWidget *mountpoint_btn =
        gtk_button_new_from_icon_name(PLUS_ICON, GTK_ICON_SIZE_BUTTON);
    gtk_widget_set_tooltip_text(mountpoint_btn, "Add a new OneDrive account");
    g_signal_connect(mountpoint_btn, "clicked", G_CALLBACK(mountpoint_cb), listbox);
    gtk_header_bar_pack_start(GTK_HEADER_BAR(header), mountpoint_btn);

    char **existing_mounts = fs_known_mounts();
    for (char **mounts = existing_mounts; *mounts; mounts++) {
        GtkWidget *row = new_mount_row(*mounts);
        gtk_list_box_insert(GTK_LIST_BOX(listbox), row, -1);
        free(*mounts);
    }
    free(existing_mounts);

    gtk_widget_show_all(window);
}

int main(int argc, char **argv) {
    GtkApplication *app =
        gtk_application_new("com.github.jstaf.onedriver", G_APPLICATION_FLAGS_NONE);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
    return g_application_run(G_APPLICATION(app), argc, argv);
}
