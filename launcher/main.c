#include <gtk/gtk.h>
#include <stdio.h>
#include <stdlib.h>

#include "dir_chooser.h"
#include "onedriver.h"
#include "systemd.h"

static void mountpoint_cb(GtkWidget *widget, GtkListBox *box) {
    static bool has_mount = false;
    char *unit_name, *mount, *escaped_mountpoint;

    mount = dir_chooser("Select a mountpoint");
    if (!strlen(mount)) { // user cancelled selection
        free(mount);
        return;
    }

    systemd_path_escape(mount, &escaped_mountpoint);
    systemd_template_unit(ONEDRIVER_SERVICE_TEMPLATE, escaped_mountpoint, &unit_name);

    GtkWidget *row = gtk_list_box_row_new();
    gtk_list_box_row_set_selectable(GTK_LIST_BOX_ROW(row), FALSE);
    GtkWidget *name = gtk_label_new(unit_name);
    gtk_container_add(GTK_CONTAINER(row), name);
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
        gtk_button_new_from_icon_name("list-add-symbolic", GTK_ICON_SIZE_BUTTON);
    g_signal_connect(mountpoint_btn, "clicked", G_CALLBACK(mountpoint_cb), listbox);
    gtk_header_bar_pack_start(GTK_HEADER_BAR(header), mountpoint_btn);

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
