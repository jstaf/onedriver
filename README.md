[![Build Status](https://travis-ci.org/jstaf/onedriver.svg?branch=master)](https://travis-ci.org/jstaf/onedriver)

onedriver
======================================

Onedriver is a native Linux client for Microsoft Onedrive. 

## Building / running

Note that in addition to the traditional Go tooling, you will need a C
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
# generate the initial auth tokens and symlink to test directory
go build
./onedriver -a
ln -s ../auth_tokens.json graph/  # yes, this is a hack

# run tests
go test ./graph
```
