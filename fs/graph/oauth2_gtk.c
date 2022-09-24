#include <gtk/gtk.h>
#include <stdio.h>
#include <string.h>
#include <webkit2/webkit2.h>

/**
 * Get the host from a URI
 */
char *uri_get_host(char *uri) {
    if (!uri || strlen(uri) == 1) {
        return NULL;
    }

    int start = 0;
    for (int i = 1; i < strlen(uri); i++) {
        if (uri[i] != '/') {
            // only care about "/"
            continue;
        }

        if (uri[i - 1] == '/') {
            // we're at the the "//" in "https://"
            start = i + 1;
        } else if (start > 0) {
            int len = i - start;
            char *host = malloc(len);
            strncpy(host, uri + start, len);
            host[len] = '\0';
            return host;
        }
    }

    if (start > 0) {
        return strdup(uri + start);
    }
    return NULL;
}

/**
 * Exit the main loop when the window is destroyed.
 */
static void destroy_window(GtkWidget *widget, gpointer data) { gtk_main_quit(); }

/**
 * Handle TLS errors with the microsoft side of things.
 */
static gboolean web_view_load_failed_tls(WebKitWebView *web_view, char *failing_uri,
                                         GTlsCertificate *certificate,
                                         GTlsCertificateFlags errors,
                                         gpointer user_data) {
    char *reason;
    switch (errors) {
    case 0:
        reason = "No error - There was no error verifying the certificate.";
        break;
    case G_TLS_CERTIFICATE_UNKNOWN_CA:
        reason = "G_TLS_CERTIFICATE_UNKNOWN_CA - The signing certificate authority is "
                 "not known.";
        break;
    case G_TLS_CERTIFICATE_BAD_IDENTITY:
        reason = "G_TLS_CERTIFICATE_BAD_IDENTITY - The certificate does not match the "
                 "expected identity of the site that it was retrieved from.";
        break;
    case G_TLS_CERTIFICATE_NOT_ACTIVATED:
        reason = "G_TLS_CERTIFICATE_NOT_ACTIVATED - The certificate's activation time is "
                 "still in the future.";
        break;
    case G_TLS_CERTIFICATE_EXPIRED:
        reason = "G_TLS_CERTIFICATE_EXPIRED - The certificate has expired.";
        break;
    case G_TLS_CERTIFICATE_REVOKED:
        reason = "G_TLS_CERTIFICATE_REVOKED - The certificate has been revoked according "
                 "to the GTlsConnection's certificate revocation list.";
        break;
    case G_TLS_CERTIFICATE_INSECURE:
        reason = "G_TLS_CERTIFICATE_INSECURE - The certificate's algorithm is considered "
                 "insecure.";
        break;
    case G_TLS_CERTIFICATE_GENERIC_ERROR:
        reason = "G_TLS_CERTIFICATE_GENERIC_ERROR - Some other error occurred validating "
                 "the certificate.";
        break;
    default:
        snprintf(reason, 256,
                 "Multiple failures (%d) - There were multiple errors during certificate "
                 "verification.",
                 errors);
        break;
    }

    g_print("Webkit load failed with TLS errors for %s : %s\n", failing_uri, reason);

    // something is up with Fedora 35's verification of this particular cert,
    // so we specifically only allow G_TLS_CERTIFICATE_GENERIC_ERROR for only this cert.
    char *host = uri_get_host(failing_uri);
    if (errors & G_TLS_CERTIFICATE_GENERIC_ERROR &&
        strncmp("account.live.com", host, 17) == 0) {
        WebKitWebContext *context = webkit_web_view_get_context(web_view);
        // allow these failing domains from the webpage and reload
        webkit_web_context_allow_tls_certificate_for_host(context, certificate,
                                                          "account.live.com");
        webkit_web_context_allow_tls_certificate_for_host(context, certificate,
                                                          "acctcdn.msauth.net");
        webkit_web_context_allow_tls_certificate_for_host(context, certificate,
                                                          "acctcdn.msftauth.net");
        g_print("Ignoring G_TLS_CERTIFICATE_GENERIC_ERROR for this certificate as a "
                "workaround for https://bugzilla.redhat.com/show_bug.cgi?id=2024296 - "
                "reloading page.\n");
        webkit_web_view_reload(web_view);
        return true;
    }
    return false;
}

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
    g_signal_connect(web_view, "load-failed-with-tls-errors",
                     G_CALLBACK(web_view_load_failed_tls), NULL);
    g_signal_connect(auth_window, "destroy", G_CALLBACK(destroy_window), web_view);

    // show and grab focus
    gtk_widget_grab_focus(GTK_WIDGET(web_view));
    gtk_widget_show_all(auth_window);
    gtk_main();

    return strdup(auth_redirect_value);
}
