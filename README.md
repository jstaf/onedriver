[![Build Status](https://travis-ci.org/jstaf/onedriver.svg?branch=master)](https://travis-ci.org/jstaf/onedriver)
[![Coverage Status](https://coveralls.io/repos/github/jstaf/onedriver/badge.svg?branch=master)](https://coveralls.io/github/jstaf/onedriver?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/jstaf/onedriver)](https://goreportcard.com/report/github.com/jstaf/onedriver)

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

* No configuration- it just works. There's nothing to setup. There's no special
  interface beyond your normal file browser.
* Files are opened and downloaded on-demand, with aggressive caching of file 
  contents and metadata locally. onedriver does not waste disk space on files
  that are supposed to be stored in the cloud.
* Can be used offline. Files you've opened previously will be available even if 
  your computer has no access to the internet.
* Stateless. Unlike a few other OneDrive clients, there's nothing to break 
  locally. You never have to worry about somehow messing up your local copy and 
  having to figure out how to fix things before you can access your files again.
* All filesystem operations are asynchronous and thread-safe, allowing you to 
  perform as many tasks as you want simultaneously.
* Free and open-source.

## Building onedriver

In addition to the traditional Go tooling, you will need a C compiler and
development headers for `webkit2gtk-4.0`. On Fedora, these can be obtained with 
`dnf install golang gcc pkg-config webkit2gtk3-devel`. 
On Ubuntu, these dependencies can be installed with
`apt install golang gcc pkg-config libwebkit2gtk-4.0-dev`.

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

A headless, Go-only binary can be built with `CGO_ENABLED=0 go build ./cmd/onedriver`. Note
that this build will not have any kind of GUI for authentication (follow the
text instructions in the terminal). Though it's not officially supported, 
the headless build should work on macOS, BSD, and even Windows as long as you 
have a variant of FUSE installed.

### Running the tests

There are two test suites - one for online use and one for offline use. Note 
that the offline tests require `sudo` to remove network access to simulate no 
access to the network. A newer version of `unshare` is compiled before running
tests to support running on older distributions like Ubuntu 18.04 where the
default version of `unshare` is too old to use.

```bash
# note - the tests will write and delete files/folders on your onedrive account
# at the path /onedriver_tests
make test
```

### Installation

onedriver has multiple installation methods depending on your needs.

```bash
# create an RPM for system-wide installation on RHEL/CentOS/Fedora
make rpm

# create a .deb for system-wide installation on Ubuntu/Debian
make onedriver.deb

# install directly from source
make
sudo make install

# install for current user only
make localinstall
```

To start onedriver automatically and ensure you always have access to your files,
you can start onedriver as a systemd user service. In this example, `$MOUNTPOINT`
refers to where we want OneDrive to be mounted at relative to our home directory.
For instance, to mount OneDrive at the path `~/Documents/OneDrive`, 
`$MOUNTPOINT` would be `Documents/OneDrive`

```bash
# create the mountpoint and determine the service name
mkdir -p "$MOUNTPOINT"
systemctl --user daemon-reload
export SERVICE_NAME=$(systemd-escape --template onedriver@.service "$MOUNTPOINT")

# mount onedrive
systemctl --user start $SERVICE_NAME

# mount onedrive on login
systemctl --user enable $SERVICE_NAME

# check onedriver's logs
journalctl --user -u $SERVICE_NAME
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
fusermount -uz mount
killall make  # if running tests via make
```

## Known issues & disclaimer

Many file browsers (like GNOME's Nautilus) will attempt to automatically 
download all files within a directory in order to create thumbnail images.
This is somewhat annoying, but only needs to happen once - after the initial
thumbnail images have been created, thumbnails will persist between
filesystem restarts.

This project is still in active development and key features may still be
missing. To see current progress, check out the 
[projects page](https://github.com/jstaf/onedriver/projects/1). 
I don't recommend using it until the initial release is complete (though
testing is always welcome!).
