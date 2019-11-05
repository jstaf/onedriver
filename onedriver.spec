Name:          onedriver
Version:       0.5
Release:       1%{?dist}
Summary:       A native FUSE filesystem for Microsoft Onedrive

License:       GPLv3
URL:           https://github.com/jstaf/onedriver
Source0:       https://github.com/jstaf/onedriver/archive/v%{version}.tar.gz

BuildRequires: rpmdevtools
BuildRequires: golang >= 1.12.0
BuildRequires: gcc
BuildRequires: pkg-config
BuildRequires: webkit2gtk3-devel
Requires:      webkit2gtk3

%description
Onedriver is a native Linux filesystem for Microsoft Onedrive. Unlike other 
Onedrive clients, Onedriver is not a file sync client - files and metadata are 
downloaded on-demand with the goal of having no local state to break.

%prep
%autosetup

%build
GOOS=linux go build

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p %{buildroot}/%{_bindir}
cp %{name} %{buildroot}/%{_bindir}

%files
%defattr(-,root,root,-)
%attr(755, root, root) %{_bindir}/%{name}

%changelog
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
