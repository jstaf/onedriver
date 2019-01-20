onedriver
======================================

Onedriver is a native Linux client for Microsoft Onedrive. 

## Building / running

```bash
# to run
go run main/onedriver.go

# to build the binary
go build -o main/onedriver ./main
```

## Running tests

```bash
# generate the initial auth tokens and symlink to test directory
go run main/onedriver.go
ln -s ../auth_tokens.json onedriver/  # yes, this is a hack

# run tests
go test ./onedriver
```
