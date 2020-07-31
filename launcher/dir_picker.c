#include <gtk/gtk.h>

/**
 * Grab selected directory and exit GTK loop.
 */
static void response_cb(GtkNativeDialog *self, gint response_id, char *path) {
    if (response_id == GTK_RESPONSE_ACCEPT) {
        strncpy(path, gtk_file_chooser_get_filename(GTK_FILE_CHOOSER(self)), 511);
    }
    gtk_main_quit();
}

/**
 * Creates a popup folder chooser via GTK.
 */
char *dir_chooser(char *title) {
    gtk_init(NULL, NULL);
    GtkFileChooserNative *chooser = gtk_file_chooser_native_new(
        title, NULL, GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER, "Select", NULL);
    gtk_file_chooser_set_current_folder(GTK_FILE_CHOOSER(chooser), g_get_home_dir());
    GtkNativeDialog *dialog = GTK_NATIVE_DIALOG(chooser);

    char path[512] = "";
    g_signal_connect(dialog, "response", G_CALLBACK(response_cb), &path);

    gtk_native_dialog_show(dialog);
    gtk_main();

    g_object_unref(chooser);
    return strdup(path);
}
