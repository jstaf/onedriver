Name:          onedriver
Version:       0.10.0
Release:       1%{?dist}
Summary:       A native Linux filesystem for Microsoft Onedrive

License:       GPLv3
URL:           https://github.com/jstaf/onedriver
Source0:       https://github.com/jstaf/onedriver/archive/onedriver-%{version}.tar.gz

BuildRequires: golang >= 1.12.0
BuildRequires: git
BuildRequires: gcc
BuildRequires: pkg-config
BuildRequires: webkit2gtk3-devel
Requires:      fuse
Requires:      webkit2gtk3
Suggests:      systemd

%description
Onedriver is a native Linux filesystem for Microsoft Onedrive. Files and
metadata are downloaded on-demand with the goal of having no local state to
break.

%prep
%autosetup

%build
GOOS=linux go build -mod=vendor -ldflags="-X main.commit=$(cat .commit)"

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p %{buildroot}/%{_bindir}
mkdir -p %{buildroot}/usr/share/icons/%{name}
mkdir -p %{buildroot}/usr/share/applications
mkdir -p %{buildroot}/usr/lib/systemd/user
cp %{name} %{buildroot}/%{_bindir}
cp resources/%{name}-launcher.sh %{buildroot}/%{_bindir}
cp resources/%{name}.png %{buildroot}/usr/share/icons/%{name}
cp resources/%{name}.svg %{buildroot}/usr/share/icons/%{name}
cp resources/%{name}.desktop %{buildroot}/usr/share/applications
cp resources/%{name}@.service %{buildroot}/usr/lib/systemd/user

# fix for el8 build in mock
%define _empty_manifest_terminate_build 0
%files
%defattr(-,root,root,-)
%attr(755, root, root) %{_bindir}/%{name}
%attr(755, root, root) %{_bindir}/%{name}-launcher.sh
%attr(644, root, root) /usr/share/icons/%{name}/%{name}.png
%attr(644, root, root) /usr/share/icons/%{name}/%{name}.svg
%attr(644, root, root) /usr/share/applications/%{name}.desktop
%attr(644, root, root) /usr/lib/systemd/user/%{name}@.service

%changelog
* Sun May 16 2021 Jeff Stafford <jeff.stafford@protonmail.com> - 0.10.0
- Adds AUR installation method for Arch-based distros - thanks fmoledina!
- Fix a rare crash while syncing server-side changes missing checksums.
- Fix a race-condition that caused uploaded files to occasionally be replaced by a 0-byte 
  copy (most commonly caused by the way LibreOffice saves files).
- Cap number of uploads that can be in-progress at any time to 5. This makes uploading 
  uploading directories with lots of files appear to go a bit faster.

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
