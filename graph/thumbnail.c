#include <libgnomeui/gnome-thumbnail.h>
#include <stdio.h>
#include <time.h>

/*
 * Create a failed thumbnail for the given URI.
 */
void fail_thumbnail(const char *uri, time_t mtime) {
  GnomeThumbnailFactory *factory =
      gnome_thumbnail_factory_new(GNOME_THUMBNAIL_SIZE_NORMAL);
  GnomeThumbnailFactory *factory_large =
      gnome_thumbnail_factory_new(GNOME_THUMBNAIL_SIZE_LARGE);
  gnome_thumbnail_factory_create_failed_thumbnail(factory, uri, mtime);
  gnome_thumbnail_factory_create_failed_thumbnail(factory_large, uri, mtime);
  g_object_unref(factory);
  g_object_unref(factory_large);
}
