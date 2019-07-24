[![Build Status](https://travis-ci.org/jstaf/onedriver.svg?branch=master)](https://travis-ci.org/jstaf/onedriver)
[![Coverage Status](https://coveralls.io/repos/github/jstaf/onedriver/badge.svg?branch=master)](https://coveralls.io/github/jstaf/onedriver?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/jstaf/onedriver)](https://goreportcard.com/report/github.com/jstaf/onedriver)

onedriver
======================================

Onedriver is a native Linux client for Microsoft Onedrive.

## Why onedriver?

"There's a few Onedrive clients available now, why did you write another one?"

The overwhelming majority of clients are "dumb sync" clients, and will actually 
download the entire contents of your Onedrive to disk (abraunegg/onedrive, 
rclone, Insync). No one wants this. Why are you paying for cloud storage if it
has to stay on your local computer?

Some sync clients require a sysadmin-level skills to use or are missing GUIs 
(rclone, abraunegg/onedrive). Ideally, anyone should be able to just open the 
client and have it work.

Some Onedrive clients cost money and are not open-source (Insync, odrive). This 
makes these products non-viable for a lot of users and organizations.

But perhaps most importantly, I kind of just enjoy writing this stuff and there
weren't any good ways to access the files I had on Onedrive. Now there are :)

### Onedriver goals

* Files are opened and downloaded on-demand, with aggressive caching of file 
  contents and metadata locally. Onedriver does not waste disk space on files
  that are supposed to be stored in the cloud.
* No configuration- it just works. There's nothing to setup. There's no special
  interface beyond your normal file browser.
* Stateless. Unlike a few other Onedrive clients, there's nothing to 
  break locally. You never have to worry about somehow messing up your local 
  copy and having to figure out how to fix things before you can access your 
  files again. The server *always* has the definitive copy.
* Free and open-source.

## Disclaimer

This project is still in active development and key features may still be 
missing. To see current progress, check out the 
[projects page](https://github.com/jstaf/onedriver/projects/1). 
I don't recommend using it until the initial release is complete (though 
testing is always welcome!). 

## Building / running

In addition to the traditional Go tooling, you will need a C
compiler and development headers for `webkit2gtk-4.0`. On Fedora, these can be
obtained with `dnf install gcc pkg-config webkit2gtk3-devel`. On Ubuntu, these
dependencies can be installed with
`apt install gcc pkg-config libwebkit2gtk-4.0-dev`.

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

## Running tests

```bash
# note - the tests will write and delete files/folders on your onedrive account
# at the path /onedriver_tests
make test
```
