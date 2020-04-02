Name:          onedriver
Version:       0.7.2
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
GOOS=linux go build -mod=vendor -ldflags="-X main.commit=$(cat .commit)" ./cmd/onedriver

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p %{buildroot}/%{_bindir}
mkdir -p %{buildroot}/usr/share/icons
mkdir -p %{buildroot}/usr/share/applications
mkdir -p %{buildroot}/usr/lib/systemd/user
cp onedriver %{buildroot}/%{_bindir}
cp resources/onedriver-launcher.sh %{buildroot}/%{_bindir}
cp resources/onedriver.png %{buildroot}/usr/share/icons
cp resources/onedriver.svg %{buildroot}/usr/share/icons
cp resources/onedriver.desktop %{buildroot}/usr/share/applications
cp resources/onedriver@.service %{buildroot}/usr/lib/systemd/user

# fix for el8 build in mock
%define _empty_manifest_terminate_build 0
%files
%defattr(-,root,root,-)
%attr(755, root, root) %{_bindir}/onedriver
%attr(755, root, root) %{_bindir}/onedriver-launcher.sh
%attr(644, root, root) /usr/share/icons/onedriver.png
%attr(644, root, root) /usr/share/icons/onedriver.svg
%attr(644, root, root) /usr/share/applications/onedriver.desktop
%attr(644, root, root) /usr/lib/systemd/user/onedriver@.service

%changelog
* Wed Apr 1 2020 Jeff Stafford <jeff.stafford@protonmail.com> - 0.8.0
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
