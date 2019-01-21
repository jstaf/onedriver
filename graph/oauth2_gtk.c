#include <stdio.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

static WebKitWebView *web_view_static = NULL;
static char *auth_redirect_value = NULL;


/**
 * Called when user closes the window, grabs the auth URL for us to use.
 */
static void destroy_window_cb(GtkWidget* widget, GtkWidget* window) {
    auth_redirect_value = strdup(webkit_web_view_get_uri(web_view_static));
    gtk_main_quit();
}


static gboolean close_web_view_cb(WebKitWebView* web_view, GtkWidget* window) {
    gtk_widget_destroy(window);
    return TRUE;
}

/**
 * Open a popup GTK auth window and return the final redirect location.
 */
char *auth_window(char *auth_url) {
    gtk_init(NULL, NULL);
    GtkWidget *auth_window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_default_size(GTK_WINDOW(auth_window), 400, 600);
    g_signal_connect(auth_window, "destroy", G_CALLBACK(destroy_window_cb), NULL);

    // create browser and add to gtk window
    WebKitWebView *web_view = WEBKIT_WEB_VIEW(webkit_web_view_new());
    gtk_container_add(GTK_CONTAINER(auth_window), GTK_WIDGET(web_view));
    web_view_static = web_view;  // very hacky, i know...
    webkit_web_view_load_uri(web_view, auth_url);
    g_signal_connect(web_view, "close", G_CALLBACK(close_web_view_cb), auth_window);

    // show and grab focus
    gtk_widget_grab_focus(GTK_WIDGET(web_view));
    gtk_widget_show_all(auth_window);
    gtk_main();

    // go will explode if you return a null pointer
    if (!auth_redirect_value) {
        auth_redirect_value = "";
    }
    return auth_redirect_value;
}
