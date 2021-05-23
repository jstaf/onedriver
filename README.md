[![Run tests](https://github.com/jstaf/onedriver/workflows/Run%20tests/badge.svg)](https://github.com/jstaf/onedriver/actions?query=workflow%3A%22Run+tests%22)
[![Coverage Status](https://coveralls.io/repos/github/jstaf/onedriver/badge.svg?branch=master)](https://coveralls.io/github/jstaf/onedriver?branch=master)
[![Copr build status](https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/package/onedriver/status_image/last_build.png)](https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/package/onedriver/)

onedriver
======================================

**onedriver is a native Linux filesystem for Microsoft OneDrive.**

The overwhelming majority of OneDrive clients are actually sync tools, and will
actually download the entire contents of your OneDrive to disk. No one wants
this. Why are you paying for cloud storage if it has to stay on your local
computer?

onedriver is not a sync client. It is a network filesystem that exposes the
contents of your OneDrive to the kernel directly. To your computer, there is
no difference between working with files on OneDrive and the files on your
local hard disk. Just mount onedriver to a directory, and get started with
your files on OneDrive!

**Getting started with onedriver is as simple as running `onedriver /path/to/mount/onedrive/at`**

### Features

* **No configuration** - it just works. There's nothing to setup. There's no special
  interface beyond your normal file browser.
* **Files are opened and downloaded on-demand**, with aggressive caching of file 
  contents and metadata locally. onedriver only downloads the files you access,
  and only redownloads files changed by another computer when you access them.
* **Can be used offline.** Files you've opened previously will be available even if 
  your computer has no access to the internet. The filesystem becomes read-only
  if you lose internet access, and automatically enables write access again when you 
  reconnect to the internet.
* **Fast.** onedriver is a parallel network filesystem. You can perform as many 
  simultaneous operations as you want and there are multiple levels of caching to 
  ensure that accessing your files is as snappy and quick as it can be.
* **Has a user interface.** You don't need to be a command-line expert to set up
  OneDrive on Linux.
* **Free and open-source.**

## Quick start

Users on Fedora/CentOS/RHEL systems are recommended to install onedriver from 
[COPR](https://copr.fedorainfracloud.org/coprs/jstaf/onedriver/).
This will install the latest version of onedriver through your package manager 
and ensure it stays up-to-date with bugfixes and new features.

```bash
sudo dnf copr enable jstaf/onedriver
sudo dnf install onedriver
```

Ubuntu/Pop!_OS/Debian users can install onedriver from this
[PPA](https://launchpad.net/~jstaf/+archive/ubuntu/onedriver).
Like the COPR install, this will enable you to install onedriver through your
package manager and install updates as they become available.

```bash
sudo add-apt-repository ppa:jstaf/onedriver
sudo apt update
sudo apt install onedriver
```

Arch/Manjaro/EndeavourOS users can install onedriver from the 
[AUR](https://aur.archlinux.org/packages/onedriver/).

Other installation options are available below if you would prefer to manually
install things or build the latest version from source.

Post-installation, you can start onedriver either via the `onedriver-launcher` 
desktop app, or via the command line: `onedriver /path/to/mount/onedrive/at/`.

### Multiple drives and starting OneDrive on login

**Note:** You can also set this up through the GUI via the `onedriver-launcher`
desktop app installed via rpm/deb/`make install`.

To start onedriver automatically and ensure you always have access to your files,
you can start onedriver as a systemd user service. In this example, `$MOUNTPOINT`
refers to where we want OneDrive to be mounted at (for instance, `~/OneDrive`).
Mounting OneDrive via systemd allows multiple drives to be mounted at the same 
time (as long as they use different mountpoints).

```bash
# create the mountpoint and determine the service name
mkdir -p $MOUNTPOINT
export SERVICE_NAME=$(systemd-escape --template onedriver@.service --path $MOUNTPOINT)

# mount onedrive
systemctl --user daemon-reload
systemctl --user start $SERVICE_NAME

# mount onedrive on login
systemctl --user enable $SERVICE_NAME

# check onedriver's logs for the current day
journalctl --user -u $SERVICE_NAME --since today
```

## Building onedriver yourself

In addition to the traditional [Go tooling](https://golang.org/dl/), 
you will need a C compiler and development headers for `webkit2gtk-4.0`
and `json-glib`. On Fedora, these can be obtained with 
`dnf install golang gcc pkg-config webkit2gtk3-devel json-glib-devel`. 
On Ubuntu, these dependencies can be installed with
`apt install golang gcc pkg-config libwebkit2gtk-4.0-dev libjson-glib-dev`.

```bash
# to build and run the binary
make
mkdir mount
./onedriver mount/

# in new window, check out the mounted filesystem
ls -l mount

# unmount the filesystem
fusermount -u mount
```

A headless binary (no GUI) can be built with `make onedriver-headless`.
If you don't know which target to build, this isn't the one for you (run
`make` instead). When using the headless build, follow the text instructions
in the terminal to perform first-time authentication to the Microsoft Graph
API. Though it's not officially supported, the headless build should work on
macOS, BSD, and even Windows as long as you have a variant of FUSE installed
(for instance, OSXFUSE on macOS or libfuse on BSD).

### Running the tests


The tests will write and delete files/folders on your onedrive account at the
path `/onedriver_tests`. Note that the offline test suite requires `sudo` to
remove network access to simulate being offline. 

```bash
make test
```

### Installation

onedriver has multiple installation methods depending on your needs.

```bash
# install directly from source
make
sudo make install

# create an RPM for system-wide installation on RHEL/CentOS/Fedora using mock
sudo dnf install golang gcc webkit2gtk3-devel json-glib-devel pkg-config git \
    rsync rpmdevtools rpm-build mock
sudo usermod -aG mock $USER
make rpm

# create a .deb for system-wide installation on Ubuntu/Debian using pbuilder
sudo apt update
sudo apt install golang gcc libwebkit2gtk-4.0-dev libjson-glib-dev pkg-config git \
    rsync devscripts debhelper build-essential pbuilder
sudo pbuilder create  # may need to add "--distribution focal" on ubuntu
make deb
```

## Troubleshooting

Most errors can be solved by simply restarting the program. onedriver is
designed to recover cleanly from errors with no extra effort.

It's possible that there may be a deadlock or segfault that I haven't caught in 
my tests. If this happens, the onedriver filesystem and subsequent ops may hang
indefinitely (ops will hang while the kernel waits for the dead onedriver 
process to respond). When this happens, you can cleanly unmount the filesystem 
with the following:

```bash
# in new terminal window
fusermount -uz $MOUNTPOINT
killall make  # if running tests via make
```

onedriver can be completely reset (delete all cached local data) with 
`onedriver -w`.

## Known issues & disclaimer

Many file browsers (like GNOME's Nautilus) will attempt to automatically 
download all files within a directory in order to create thumbnail images.
This is somewhat annoying, but only needs to happen once - after the initial
thumbnail images have been created, thumbnails will persist between
filesystem restarts.

Microsoft does not support symbolic links (or anything remotely like them) on
OneDrive. Attempting to create symbolic links within the filesystem returns
ENOSYS (function not implemented) because the functionality hasn't been
implemented... by Microsoft. Similarly, Microsoft does not expose the OneDrive
Recycle Bin APIs - if you want to empty or restore the OneDrive Recycle Bin, you
must do so through the OneDrive web UI (onedriver uses the native system
trash/restore functionality independently of the OneDrive Recycle Bin).

OneDrive is not a good place to backup files to. Use a tool like
[restic](https://restic.net/) or [borg](https://www.borgbackup.org/) if you're
looking for a reliable encrypted backup tool.

This project is still in active development and is provided AS IS. There are no
guarantees. It might kill your cat.
