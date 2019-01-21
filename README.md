onedriver
======================================

Onedriver is a native Linux client for Microsoft Onedrive. 

## Building / running

```bash
# to build and run the binary
go build -o main/onedriver ./main
mkdir mount
main/onedriver mount/

# in new window, check out the mounted filesystem
ls -l mount 

# unmount the filesystem
fusermount -u mount
```

## Running tests

```bash
# generate the initial auth tokens and symlink to test directory
go run main/authenticate.go
ln -s ../auth_tokens.json onedriver/  # yes, this is a hack

# run tests
go test ./onedriver
```
