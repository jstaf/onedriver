.PHONY = all, test, test_no_race, rpm, clean

onedriver: graph/*.go graph/*.c graph/*.h logger/*.go cmd/onedriver/*.go
	go build ./cmd/onedriver

all: onedriver test onedriver.deb rpm

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
	rm -f fusefs_tests.race*
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" go test -race -v -parallel=8 -count=1 ./graph
	unshare -nr go test -race -v -parallel=8 -count=1 ./offline

test_no_race: onedriver dmel.fa
	go test -v -count=1 ./graph

# for autocompletion by ide-clangd
compile_flags.txt:
	pkg-config --cflags gtk+-3.0 webkit2gtk-4.0 | sed 's/ /\n/g' > $@

# will literally purge everything: all built artifacts, all logs, all tests,
# all files tests depend on, all auth tokens... EVERYTHING
clean:
	fusermount -uz mount/ || true
	rm -f *.db *.rpm *.deb *.log *.fa *.gz onedriver auth_tokens.json
