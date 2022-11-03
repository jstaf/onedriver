Name:          onedriver
Version:       0.13.0
Release:       2%{?dist}
Summary:       A native Linux filesystem for Microsoft Onedrive

License:       GPLv3
URL:           https://github.com/jstaf/onedriver
Source0:       https://github.com/jstaf/onedriver/archive/refs/tags/v%{version}.tar.gz

%if 0%{?suse_version}
BuildRequires: go >= 1.17
%else
BuildRequires: golang >= 1.17.0
%endif
BuildRequires: git
BuildRequires: gcc
BuildRequires: pkg-config
BuildRequires: webkit2gtk3-devel

%if 0%{?suse_version}
%if 0%{?suse_version} > 1500
# tumbleweed
Requires: libwebkit2gtk-4_1-0
%else
# leap 15.3
Requires: libwebkit2gtk-4_0-37
%endif
# other EL distros
%else
Requires: webkit2gtk3
%endif
Requires: fuse
Suggests: systemd

%description
Onedriver is a native Linux filesystem for Microsoft Onedrive. Files and
metadata are downloaded on-demand instead of syncing the entire drive to
your local computer.

%prep
%autosetup

%build
%if 0%{?suse_version} > 1500
# done via sed because #cgo flags appear to ignore #ifdef
sed -i 's/webkit2gtk-4.0/webkit2gtk-4.1/g' fs/graph/oauth2_gtk.go
%endif
if rpm -q pango | grep -q 1.42; then
  BUILD_TAGS=-tags=pango_1_42,gtk_3_22
fi
go build -v -mod=vendor $BUILD_TAGS \
  -ldflags="-X github.com/jstaf/onedriver/cmd/common.commit=$(cat .commit)" \
  ./cmd/onedriver
go build -v -mod=vendor $BUILD_TAGS \
  -ldflags="-X github.com/jstaf/onedriver/cmd/common.commit=$(cat .commit)" \
  ./cmd/onedriver-launcher
gzip resources/onedriver.1

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p %{buildroot}/%{_bindir}
mkdir -p %{buildroot}/usr/share/icons/%{name}
mkdir -p %{buildroot}/usr/share/applications
mkdir -p %{buildroot}/usr/lib/systemd/user
mkdir -p %{buildroot}/usr/share/man/man1
cp %{name} %{buildroot}/%{_bindir}
cp %{name}-launcher %{buildroot}/%{_bindir}
cp resources/%{name}.png %{buildroot}/usr/share/icons/%{name}
cp resources/%{name}-128.png %{buildroot}/usr/share/icons/%{name}
cp resources/%{name}.svg %{buildroot}/usr/share/icons/%{name}
cp resources/%{name}.desktop %{buildroot}/usr/share/applications
cp resources/%{name}@.service %{buildroot}/usr/lib/systemd/user
cp resources/%{name}.1.gz %{buildroot}/usr/share/man/man1

# fix for el8 build in mock
%define _empty_manifest_terminate_build 0
%files
%defattr(-,root,root,-)
%attr(755, root, root) %{_bindir}/%{name}
%attr(755, root, root) %{_bindir}/%{name}-launcher
%attr(644, root, root) /usr/share/icons/%{name}/%{name}.png
%attr(644, root, root) /usr/share/icons/%{name}/%{name}-128.png
%attr(644, root, root) /usr/share/icons/%{name}/%{name}.svg
%attr(644, root, root) /usr/share/applications/%{name}.desktop
%attr(644, root, root) /usr/lib/systemd/user/%{name}@.service
%doc
%attr(644, root, root) /usr/share/man/man1/%{name}.1.gz

%changelog
* Tue Nov 1 2022 Jeff Stafford <jeff.stafford@protonmail.com> - 0.13.0
- The GUI has been rewritten in golang for ease of maintenance and code sharing with 
  the rest of the onedriver application.
- onedriver can now be configured with a config file at "~/.config/onedriver/config.yml".
- There is now a configuration menu in the GUI. You can now set a couple configuration
  options that were previously only possible with "systemctl edit".
- The onedriver CLI now stores its cache in the same path that the GUI expects,
  meaning that invoking the onedriver filesystem directly and via the GUI will share the
  cache as long as the mountpoint is the same.
- onedriver now prefers multipart downloads for files >10MB instead of a single massive 
  GET request. This should significantly improve reliability when working with large files.

* Tue Nov 2 2021 Jeff Stafford <jeff.stafford@protonmail.com> - 0.12.0
- Major internal rewrite - onedriver now talks directly to the kernel instead of using
  go-fuse/fs as an intermediary. This makes metadata operations a bit faster.
- onedriver better handles completion of multipart uploads and does not repeatedly
  upload files on success. This significantly improves upload speed.
- Fixes a crash when writes begin at an offset beyond maximum file length. This fixes a
  bug where running ld inside the filesystem would cause it to crash.
- Switch to using zerolog instead of logrus for logging. Though zerolog is supposedly 
  faster, the real reason to switch is that it's much easier for me (and hopefully you)
  to read! Also, pretty colors!
- onedriver now gives you the option to choose to authenticate via the terminal when 
  authenticating via the new --no-browser option (this is the functionality from the 
  old "headless" build).
- Add a workaround for the TLS cert authentication issue from
  https://bugzilla.redhat.com/show_bug.cgi?id=2024296

* Tue Aug 17 2021 Jeff Stafford <jeff.stafford@protonmail.com> - 0.11.2
- onedriver now disallows rmdir on nonempty directories.
- The filesystem now detects if it is offline more reliably.

* Sun Jul 11 2021 Jeff Stafford <jeff.stafford@protonmail.com> - 0.11.1
- Fix startup crash in onedriver-launcher when onedriver has not been launched before.

* Sat Jul 3 2021 Jeff Stafford <jeff.stafford@protonmail.com> - 0.11.0
- Now includes a snazzy GUI for managing your mountpoints. No terminal skills are required
  to use onedriver now.
- The upload logic has been rewritten to no longer use 0-byte files as placeholders in 
  any scenario. This fixes a race condition where software like LibreOffice, KeepassXC, or 
  Krita could generate a 0-byte file instead of the intended file when the file was 4MB or
  larger.
- onedriver now uses etags AND modification times when syncing server-side changes back to
  the client. This reduces the number of times that files must be redownloaded because of
  bad timestamp data from the Microsoft API.

* Mon May 17 2021 Jeff Stafford <jeff.stafford@protonmail.com> - 0.10.1
- Fix the onedriver .desktop launcher so it uses the new systemd unit name.

* Mon May 17 2021 Jeff Stafford <jeff.stafford@protonmail.com> - 0.10.0
- Add AUR installation method for Arch-based distros - thanks fmoledina!
- Add manpage for onedriver - thanks GenericGuy!
- The onedriver systemd service now restarts itself in the event of a crash -
  thanks dipunm!
- Fix a rare crash while syncing server-side changes missing checksums.
- Fix a race-condition that caused uploaded files to occasionally be replaced by a 0-byte 
  copy (most commonly caused by the way LibreOffice saves files).
- Cap number of uploads that can be in-progress at any time to 5. This makes uploading 
  uploading directories with lots of files appear to go a bit faster.
- The account name is now displayed in the title bar if you need to reauthenticate to
  OneDrive (makes it easier to know which credentials to use when prompted).

* Tue Sep 29 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.9.2
- Adds fix for server-side update to Microsoft's authentication APIs.
- Fix a crash on auth renewal after computer suspend or other network interruption.

* Sat Jun 6 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.9.1
- Filenames are now sanitized when uploading new files.
- onedriver now only syncs metadata changes for a file from server to client if its
  contents have changed as well. This means that programs like LibreOffice will no longer
  complain about their lockfiles being updated while saving.

* Wed Jun 3 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.9.0
- Multiple OneDrive drives can now be mounted simultaneously via systemd.
- Uploads are now retried, with failed uploads retried automatically.
- In-progress uploads are now cached on disk and resumed the next time onedriver starts
  if the upload is terminated prematurely (for instance, if a user shuts down their computer)
- All uploads are now verified against checksums of their local content.

* Thu Apr 2 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.8.0
- Add a desktop launcher for single drive scenarios (better multi-drive support coming soon!).
- Fix for directories containing more than 200 items.
- Miscellaneous fixes and tests for OneDrive for Business
- Compatibility with Go 1.14

* Mon Feb 17 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.7.2
- Allow use of disk cache after filesystem transitions from offline to online.

* Mon Feb 17 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.7.1
- Fix for filesystem coming up blank after user systemd session start.

* Wed Feb 12 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.7.0
- Now has drive username in Nautilus sidebar and small OneDrive logo on mountpoint.
- No longer requires manually closing the authentication window.
- Add systemd user service for automount on boot.
- Now transitions gracefully from online to offline (or vice-versa) depending on network availability.

* Thu Jan 16 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.6
- Filesystem metadata is now serialized to disk at regular intervals.
- Using on-disk metadata, onedriver can now be used in read-only mode while offline.
- onedriver now stores its on-disk cache and auth tokens under the normal user cache directory.

* Mon Nov 4 2019 Jeff Stafford <jeff.stafford@protonmail.com> - 0.5
- Add a dedicated thread responsible for syncing remote changes to local cache every 30s.
- Add a dedicated thread to monitor, deduplicate, and retry uploads.
- Now all HTTP requests will retry server-side 5xx errors a single time by default.
- Print HTTP status code with Graph API errors where they occur.
- Purge file contents from memory on flush() and store them on disk.
- onedriver now validates on-disk file contents using checksums before using them.

* Sun Sep 15 2019 Jeff Stafford <jeff.stafford@protonmail.com> - 0.4
- Port to go-fuse version 2 and the new nodefs API for improved performance.

* Sat Sep 7 2019 Jeff Stafford <jeff.stafford@protonmail.com> - 0.3
- Initial .spec file
