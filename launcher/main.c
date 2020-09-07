#include <gtk/gtk.h>
#include <stdio.h>
#include <stdlib.h>

#include "dir_chooser.h"
#include "onedriver.h"
#include "systemd.h"

static void mountpoint_cb(GtkWidget *widget, gpointer data) {
    char *unit_name, *mount, *escaped_mountpoint;

    mount = dir_chooser("Select a mountpoint");
    systemd_path_escape(mount, &escaped_mountpoint);
    systemd_template_unit("onedriver@.service", escaped_mountpoint, &unit_name);

    printf("unit name: %s\n", unit_name);

    if (systemd_unit_is_active(unit_name)) {
        g_print("active\n");
    } else {
        g_print("off\n");
    }

    if (systemd_unit_is_enabled(unit_name)) {
        g_print("enabled\n");
    } else {
        g_print("disabled\n");
    }

    free(mount);
    free(unit_name);
    free(escaped_mountpoint);
}

static void activate(GtkApplication *app, gpointer data) {
    GtkWidget *window = gtk_application_window_new(app);
    gtk_window_set_default_size(GTK_WINDOW(window), 550, 400);

    GtkWidget *header = gtk_header_bar_new();
    gtk_header_bar_set_show_close_button(GTK_HEADER_BAR(header), TRUE);
    gtk_header_bar_set_title(GTK_HEADER_BAR(header), "onedriver");
    gtk_window_set_titlebar(GTK_WINDOW(window), header);

    GtkWidget *mountpoint_btn =
        gtk_button_new_from_icon_name("list-add-symbolic", GTK_ICON_SIZE_BUTTON);
    g_signal_connect(mountpoint_btn, "clicked", G_CALLBACK(mountpoint_cb), NULL);
    gtk_header_bar_pack_start(GTK_HEADER_BAR(header), mountpoint_btn);

    GtkWidget *listbox = gtk_list_box_new();
    gtk_container_add(GTK_CONTAINER(window), listbox);
    gtk_list_box_set_activate_on_single_click(GTK_LIST_BOX(listbox), FALSE);
    gtk_list_box_drag_unhighlight_row(GTK_LIST_BOX(listbox));

    GtkWidget *row = gtk_list_box_row_new();
    gtk_list_box_row_set_selectable(GTK_LIST_BOX_ROW(row), FALSE);
    gtk_list_box_row_set_activatable(GTK_LIST_BOX_ROW(row),
                                     FALSE); // only for initial label
    GtkWidget *howto = gtk_label_new("Create a new mountpoint with \"+\".");
    gtk_container_add(GTK_CONTAINER(row), howto);
    gtk_list_box_insert(GTK_LIST_BOX(listbox), row, -1);

    gtk_widget_show_all(window);
}

int main(int argc, char **argv) {
    GtkApplication *app =
        gtk_application_new("com.github.jstaf.onedriver", G_APPLICATION_FLAGS_NONE);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
    return g_application_run(G_APPLICATION(app), argc, argv);
}
