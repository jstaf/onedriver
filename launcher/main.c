#include <gtk/gtk.h>
#include <stdio.h>
#include <stdlib.h>

#include "dir_chooser.h"
#include "systemd.h"

static void mountpoint_cb(GtkWidget *widget, gpointer data) {
    char *unit_name, *mount, *escaped_mountpoint;

    mount = dir_chooser("Select a mountpoint");
    systemd_path_escape(mount, &escaped_mountpoint);
    systemd_template_unit("onedriver@.service", escaped_mountpoint, &unit_name);

    printf("unit name: %s\n", unit_name);

    switch (systemd_unit_status(unit_name)) {
    case SYSTEMD_UNIT_ACTIVE:
        g_print("active\n");
        break;
    default:
        g_print("off\n");
        break;
    }

    free(mount);
    free(unit_name);
    free(escaped_mountpoint);
}

static void activate(GtkApplication *app, gpointer data) {
    GtkWidget *window = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(window), "onedriver");

    GtkWidget *listbox = gtk_list_box_new();
    gtk_container_add(GTK_CONTAINER(window), listbox);

    GtkWidget *row = gtk_list_box_row_new();
    gtk_list_box_row_set_selectable(GTK_LIST_BOX_ROW(row), FALSE);
    GtkWidget *button = gtk_button_new_with_label("New mountpoint");
    g_signal_connect(button, "clicked", G_CALLBACK(mountpoint_cb), NULL);
    gtk_container_add(GTK_CONTAINER(row), button);
    gtk_list_box_insert(GTK_LIST_BOX(listbox), row, -1);

    gtk_widget_show_all(window);
}

int main(int argc, char **argv) {
    GtkApplication *app =
        gtk_application_new("com.github.jstaf.onedriver", G_APPLICATION_FLAGS_NONE);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
    return g_application_run(G_APPLICATION(app), argc, argv);
}
