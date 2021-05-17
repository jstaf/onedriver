#include <gtk/gtk.h>
#include <stdio.h>
#include <string.h>
#include <webkit2/webkit2.h>

/**
 * Exit the main loop when the window is destroyed.
 */
static void destroy_window(GtkWidget *widget, gpointer data) { gtk_main_quit(); }

/**
 * Catch redirects once authentication completes.
 */
static void web_view_load_changed(WebKitWebView *web_view, WebKitLoadEvent load_event,
                                  char *auth_redirect_url_ptr) {
    static const char *auth_complete_url = "https://login.live.com/oauth20_desktop.srf";
    const char *url = webkit_web_view_get_uri(web_view);

    if (load_event == WEBKIT_LOAD_REDIRECTED &&
        strncmp(auth_complete_url, url, strlen(auth_complete_url)) == 0) {
        // catch redirects to the oauth2 redirect only and destroy the window
        strncpy(auth_redirect_url_ptr, url, 2047);
        GtkWidget *parent = gtk_widget_get_parent(GTK_WIDGET(web_view));
        gtk_widget_destroy(parent);
    }
}

/**
 * Open a popup GTK auth window and return the final redirect location.
 */
char *webkit_auth_window(char *auth_url, char *account_name) {
    gtk_init(NULL, NULL);
    GtkWidget *auth_window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    if (account_name && strlen(account_name) > 0) {
        char title[512];
        snprintf(title, 511, "onedriver (%s)", account_name);
        gtk_window_set_title(GTK_WINDOW(auth_window), title);
        gtk_window_set_default_size(GTK_WINDOW(auth_window), 525, 600);
    } else {
        gtk_window_set_title(GTK_WINDOW(auth_window), "onedriver");
        gtk_window_set_default_size(GTK_WINDOW(auth_window), 450, 600);
    }

    // create browser and add to gtk window
    WebKitWebView *web_view = WEBKIT_WEB_VIEW(webkit_web_view_new());
    gtk_container_add(GTK_CONTAINER(auth_window), GTK_WIDGET(web_view));
    webkit_web_view_load_uri(web_view, auth_url);

    char auth_redirect_value[2048];
    auth_redirect_value[0] = '\0';
    g_signal_connect(web_view, "load-changed", G_CALLBACK(web_view_load_changed),
                     &auth_redirect_value);
    g_signal_connect(auth_window, "destroy", G_CALLBACK(destroy_window), web_view);

    // show and grab focus
    gtk_widget_grab_focus(GTK_WIDGET(web_view));
    gtk_widget_show_all(auth_window);
    gtk_main();

    return strdup(auth_redirect_value);
}
