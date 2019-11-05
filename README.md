[![Build Status](https://travis-ci.org/jstaf/onedriver.svg?branch=master)](https://travis-ci.org/jstaf/onedriver)
[![Coverage Status](https://coveralls.io/repos/github/jstaf/onedriver/badge.svg?branch=master)](https://coveralls.io/github/jstaf/onedriver?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/jstaf/onedriver)](https://goreportcard.com/report/github.com/jstaf/onedriver)

onedriver
======================================

onedriver is a native Linux filesystem for Microsoft OneDrive.

The overwhelming majority of OneDrive clients are sync tools, and will
actually download the entire contents of your OneDrive to disk. No one wants
this. Why are you paying for cloud storage if it has to stay on your local
computer?

onedriver is not a sync client. It is a network filesystem that exposes the
contents of your OneDrive to the kernel directly. To your computer, there is
no difference between working with files on OneDrive and the files on your
local hard disk. Just mount onedriver to a directory, and get started with
your files on OneDrive!

Using onedriver is as simple as:

```bash
onedriver /path/to/mount/onedrive/at
```

### Features

* Files are opened and downloaded on-demand, with aggressive caching of file 
  contents and metadata locally. onedriver does not waste disk space on files
  that are supposed to be stored in the cloud.
* No configuration- it just works. There's nothing to setup. There's no special
  interface beyond your normal file browser.
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
go build
mkdir mount
./onedriver mount/

# in new window, check out the mounted filesystem
ls -l mount

# unmount the filesystem
fusermount -u mount
```

A headless, Go-only binary can be built with `CGO_ENABLED=0 go build`. Note
that this build will not have any kind of GUI for authentication (follow the
text instructions in the terminal). Though it's not officially supported, 
the headless build should work on macOS, BSD, and even Windows as long as you 
have a variant of FUSE installed.

### Running tests

```bash
# note - the tests will write and delete files/folders on your onedrive account
# at the path /onedriver_tests
make test
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
