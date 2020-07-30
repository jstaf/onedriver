#include <gtk/gtk.h>

static void activate(GtkApplication *app, gpointer data) {
    GtkWidget *window = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(window), "onedriver");
    gtk_container_set_border_width(GTK_CONTAINER(window), 10);

    GtkWidget *listbox = gtk_list_box_new();
    gtk_container_add(GTK_CONTAINER(window), listbox);

    gtk_widget_show_all(window);
}

int main(int argc, char **argv) {
    GtkApplication *app =
        gtk_application_new("com.github.jstaf.onedriver", G_APPLICATION_FLAGS_NONE);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
    return g_application_run(G_APPLICATION(app), argc, argv);
}
