[![Run tests](https://github.com/jstaf/onedriver/workflows/Run%20tests/badge.svg)](https://github.com/jstaf/onedriver/actions?query=workflow%3A%22Run+tests%22)
[![Coverage Status](https://coveralls.io/repos/github/jstaf/onedriver/badge.svg?branch=master)](https://coveralls.io/github/jstaf/onedriver?branch=master)
[![Copr build status](https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/package/onedriver/status_image/last_build.png)](https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/package/onedriver/)

# onedriver

**onedriver is a native Linux filesystem for Microsoft OneDrive.**

onedriver is a network filesystem that gives your computer direct access to your
files on Microsoft OneDrive. This is not a sync client. Instead of syncing
files, onedriver performs an on-demand download of files when your computer
attempts to use them. onedriver allows you to use files on OneDrive as if they
were files on your local computer.

onedriver is extremely straightforwards to use:

- Install onedriver using your favorite installation method.
- Click the "+" button in the app to setup one or more OneDrive accounts.
  (There's a command-line workflow for those who prefer doing things that way
  too!)
- Just start using your files on OneDrive as if they were normal files.

I've spent a lot of time trying to make onedriver fast, convenient, and easy to
use. Though you can use it on servers, the goal here is to make it easy to work
with OneDrive files on your Linux desktop. This allows you to easily sync files
between any number of Windows, Mac, and Linux computers. You can setup your
phone to auto-upload photos to OneDrive and edit and view them on your Linux
computer. You can switch between LibreOffice on your local computer and the
Microsoft 365 online apps as needed when working. Want to migrate from Windows
to Linux? Just throw all your Windows files into OneDrive, add your OneDrive
account to Linux with onedriver, and call it a day.

**Microsoft OneDrive works on Linux.**

Getting started with your files on OneDrive is as easy as running:
`onedriver /path/to/mount/onedrive/at` (there's also a helpful GUI!). To get a
list of all the arguments onedriver can be run with you can read the manual page
by typing `man onedriver` or get a quick summary with `onedriver --help`.

## Key features

onedriver has several nice features that make it significantly more useful than
other OneDrive clients:

- **Files are only downloaded when you use them.** onedriver will only download
  a file if you (or a program on your computer) uses that file. You don't need
  to wait hours for a sync client to sync your entire OneDrive account to your
  local computer or try to guess which files and folders you might need later
  while setting up a "selective sync". onedriver gives you instant access to
  _all_ of your files and only downloads the ones you use.

- **Bidirectional sync.** Although onedriver doesn't actually "sync" any files,
  any changes that occur on OneDrive will be automatically reflected on your
  local machine. onedriver will only redownload a file when you access a file
  that has been changed remotely on OneDrive. If you somehow simultaneously
  modify a file both locally on your computer and also remotely on OneDrive,
  your local copy will always take priority (to avoid you losing any local
  work).

- **Can be used offline.** Files you've opened previously will be available even
  if your computer has no access to the internet. The filesystem becomes
  read-only if you lose internet access, and automatically enables write access
  again when you reconnect to the internet.

- **Fast.** Great care has been taken to ensure that onedriver never makes a
  network request unless it actually needs to. onedriver caches both filesystem
  metadata and file contents both in memory and on-disk. Accessing your OneDrive
  files will be fast and snappy even if you're engaged in a fight to the death
  for the last power outlet at a coffeeshop with bad wifi. (This has definitely
  never happened to me before, why do you ask?)

- **Has a user interface.** You can add and remove your OneDrive accounts
  without ever using the command-line. Once you've added your OneDrive accounts,
  there's no special interface beyond your normal file browser.

- **Free and open-source.** They're your files. Why should you have to pay to
  access them? onedriver is licensed under the GPLv3, which means you will
  _always_ have access to use onedriver to access your files on OneDrive.

## Quick start

### Fedora/CentOS/RHEL

Users on Fedora/CentOS/RHEL systems are recommended to install onedriver from
[COPR](https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/). This will
install the latest version of onedriver through your package manager and ensure
it stays up-to-date with bugfixes and new features.

```bash
sudo dnf copr enable jstaf/onedriver
sudo dnf install onedriver
```

### OpenSUSE

OpenSUSE users need to add the COPR repo either for Leap or Tumbleweed

```bash
# Leap 15.4
sudo zypper addrepo -g -r https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/repo/opensuse-leap-15.4/jstaf-onedriver-opensuse-leap-15.4.repo onedriver
sudo zypper --gpg-auto-import-keys refresh
sudo zypper install onedriver

# Tumbleweed
sudo zypper addrepo -g -r https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/repo/opensuse-tumbleweed/jstaf-onedriver-opensuse-tumbleweed.repo onedriver
sudo zypper --gpg-auto-import-keys refresh
sudo zypper install onedriver
```

### Ubuntu/Pop!\_OS/Debian

Ubuntu/Pop!\_OS/Debian users can install onedriver from the
[OpenSUSE Build Service](https://software.opensuse.org/download.html?project=home%3Ajstaf&package=onedriver)
(despite the name, OBS also does a nice job of building packages for Debian).
Like the COPR install, this will enable you to install onedriver through your
package manager and install updates as they become available. If you previously
installed onedriver via PPA, you can purge the old PPA from your system via:
`sudo add-apt-repository --remove ppa:jstaf/onedriver`

### Arch/Manjaro/EndeavourOS

Arch/Manjaro/EndeavourOS users can install onedriver from the
[AUR](https://aur.archlinux.org/packages/onedriver/).

Post-installation, you can start onedriver either via the `onedriver-launcher`
desktop app, or via the command line: `onedriver /path/to/mount/onedrive/at/`.

### Gentoo

Gentoo users can install onedriver from
[this ebuild overlay](https://github.com/foopsss/gentoo-overlay) provided by a user. If
you don't want to add user-hosted overlays to your system you may copy the
ebuild for the latest version to a local overlay, which can be created by
following the instructions available in the
[Gentoo Wiki](https://wiki.gentoo.org/wiki/Creating_an_ebuild_repository).

Make sure to carefully review the ebuild for the package before installing it

### NixOS/NixPkgs

NixOS and Nix users can install onedriver from
[the unstable channel](https://search.nixos.org/packages?channel=unstable&query=onedriver)
either by adding the package to their system's configuration (if they are using
NixOS) or by installing it manually via `nix-env -iA unstable.onedriver`.



## Multiple drives and starting OneDrive on login via systemd

**Note:** You can also set this up through the GUI via the `onedriver-launcher`
desktop app installed via rpm/deb/`make install`. You can skip this section if
you're using the GUI. It's honestly easier.

To start onedriver automatically and ensure you always have access to your
files, you can start onedriver as a systemd user service. In this example,
`$MOUNTPOINT` refers to where we want OneDrive to be mounted at (for instance,
`~/OneDrive`).

```bash
# create the mountpoint and determine the service name
mkdir -p $MOUNTPOINT
export SERVICE_NAME=$(systemd-escape --template onedriver@.service --path $MOUNTPOINT)

# mount onedrive
systemctl --user daemon-reload
systemctl --user start $SERVICE_NAME

# automatically mount onedrive when you login
systemctl --user enable $SERVICE_NAME

# check onedriver's logs for the current day
journalctl --user -u $SERVICE_NAME --since today
```

## Building onedriver yourself

In addition to the traditional [Go tooling](https://golang.org/dl/), you will
need a C compiler and development headers for `webkit2gtk-4.0` and `json-glib`.
On Fedora, these can be obtained with
`dnf install golang gcc pkg-config webkit2gtk3-devel json-glib-devel`. On
Ubuntu, these dependencies can be installed with
`apt install golang gcc pkg-config libwebkit2gtk-4.0-dev libjson-glib-dev`.

```bash
# to build and run the binary
make
mkdir mount
./onedriver mount/

# in new window, check out the mounted filesystem
ls -l mount

# unmount the filesystem
fusermount3 -uz mount
# you can also just "ctrl-c" onedriver to unmount it
```

### Running the tests

The tests will write and delete files/folders on your onedrive account at the
path `/onedriver_tests`. Note that the offline test suite requires `sudo` to
remove network access to simulate being offline.

```bash
# setup test tooling for first time run
make test-init

# actually run tests
make test
```

### Installation from source

onedriver has multiple installation methods depending on your needs.

```bash
# install directly from source
make
sudo make install

# create an RPM for system-wide installation on RHEL/CentOS/Fedora using mock
sudo dnf install golang gcc webkit2gtk3-devel json-glib-devel pkg-config git \
    rsync rpmdevtools rpm-build mock
sudo usermod -aG mock $USER
newgrp mock
make rpm

# create a .deb for system-wide installation on Ubuntu/Debian using pbuilder
sudo apt update
sudo apt install golang gcc libwebkit2gtk-4.0-dev libjson-glib-dev pkg-config git \
    rsync devscripts debhelper build-essential pbuilder
sudo pbuilder create  # may need to add "--distribution focal" on ubuntu
make deb
```

## Troubleshooting

During your OneDrive travels, you might hit a bug that I haven't squashed yet.
Don't panic! In most cases, the filesystem will report what happened to whatever
program you're using. (As an example, an error mentioning a "read-only
filesystem" indicates that your computer is currently offline.)

If the filesystem appears to hang or "freeze" indefinitely, its possible the
fileystem has crashed. To resolve this, just restart the program by unmounting
and remounting things via the GUI or by running `fusermount3 -uz $MOUNTPOINT` on
the command-line.

If you really want to go back to a clean slate, onedriver can be completely
reset (delete all cached local data) by deleting mounts in the GUI or running
`onedriver -w`.

If you encounter a bug or have a feature request, open an issue in the "Issues"
tab here on GitHub. The two most informative things you can put in a bug report
are the logs from the bug/just before encountering the bug (get logs via
`journalctl --user -u $SERVICE_NAME --since today` ... see docs for correct
value of `$SERVICE_NAME`) and/or instructions on how to reproduce the issue.
Otherwise I have to guess what the problem is :disappointed:

## Known issues & disclaimer

Many file browsers (like
[GNOME's Nautilus](https://gitlab.gnome.org/GNOME/nautilus/-/issues/1209)) will
attempt to automatically download all files within a directory in order to
create thumbnail images. This is somewhat annoying, but only needs to happen
once - after the initial thumbnail images have been created, thumbnails will
persist between filesystem restarts.

Microsoft does not support symbolic links (or anything remotely like them) on
OneDrive. Attempting to create symbolic links within the filesystem returns
ENOSYS (function not implemented) because the functionality hasn't been
implemented... by Microsoft. Similarly, Microsoft does not expose the OneDrive
Recycle Bin APIs - if you want to empty or restore the OneDrive Recycle Bin, you
must do so through the OneDrive web UI (onedriver uses the native system
trash/restore functionality independently of the OneDrive Recycle Bin).

onedriver loads files into memory when you access them. This makes things very
fast, but obviously doesn't work very well if you have very large files. Use a
sync client like [rclone](https://rclone.org/) if you need the ability to copy
multi-gigabyte files to OneDrive.

OneDrive is not a good place to backup files to. Use a tool like
[restic](https://restic.net/) or [borg](https://www.borgbackup.org/) if you're
looking for a reliable encrypted backup tool. I know some of you want to "back
up your files to OneDrive". Don't do it. Restic and Borg are better in every
possible way than any OneDrive client ever will be when it comes to creating
backups you can count on.

Finally, this project is still in active development and is provided AS IS.
There are no guarantees. It might kill your cat.
