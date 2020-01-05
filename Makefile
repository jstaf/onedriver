.PHONY = all, test, test_offline, test_no_race, rpm, clean
TEST_UID = $(shell id -u)
TEST_GID = $(shell id -g)

onedriver: graph/*.go graph/*.c graph/*.h logger/*.go cmd/onedriver/*.go
	go build ./cmd/onedriver

all: onedriver test test_offline onedriver.deb rpm

# kind of a yucky build using nfpm - will be replaced later with a real .deb
# build pipeline
onedriver.deb: onedriver
	nfpm pkg --target $@

rpm: onedriver.spec
	rm -f ~/rpmbuild/RPMS/x86_64/onedriver*.rpm
	mkdir -p ~/rpmbuild/SOURCES
	spectool -g -R $<
	# skip generation of debuginfo package
	rpmbuild -bb --define "debug_package %{nil}" $<
	cp ~/rpmbuild/RPMS/x86_64/onedriver*.rpm .

# a large text file for us to test upload sessions with. #science
dmel.fa:
	curl ftp://ftp.ensemblgenomes.org/pub/metazoa/release-42/fasta/drosophila_melanogaster/dna/Drosophila_melanogaster.BDGP6.22.dna.chromosome.X.fa.gz | zcat > $@

# cache disabled to always force rerun of all tests
# (some tests can fail due to race conditions (since all fuse ops are async))
test: onedriver dmel.fa
	rm -f *.race*
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" go test -race -v -parallel=8 -count=1 ./graph

test_no_race: onedriver dmel.fa
	go test -v -count=1 ./graph
	go test -i ./offline

# Install test dependencies and build test binary online, then disable network
# access and run tests as the current user. No way to get around using sudo to 
# change UIDs, otherwise we don't have permission to mount the fuse filesystem.
test_offline: onedriver
	go test -c ./offline
	sudo unshare -n -S $(TEST_UID) -G $(TEST_GID) ./offline.test -test.v

# for autocompletion by ide-clangd
compile_flags.txt:
	pkg-config --cflags gtk+-3.0 webkit2gtk-4.0 | sed 's/ /\n/g' > $@

# will literally purge everything: all built artifacts, all logs, all tests,
# all files tests depend on, all auth tokens... EVERYTHING
clean:
	fusermount -uz mount/ || true
	rm -f *.db *.rpm *.deb *.log *.fa *.gz onedriver auth_tokens.json
