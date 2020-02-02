.PHONY: all, test, rpm, clean, install, localinstall

TEST_UID = $(shell id -u)
TEST_GID = $(shell id -g)
UNSHARE_VERSION = 2.34
ifeq ($(shell unshare --help | grep setuid | wc -l), 1)
	UNSHARE = unshare
else
	UNSHARE = ./unshare
	EXTRA_TEST_DEPS = unshare
endif


onedriver: graph/*.go graph/*.c graph/*.h logger/*.go cmd/onedriver/*.go
	go build ./cmd/onedriver


all: onedriver test onedriver.deb rpm


install: onedriver
	cp $< /usr/bin/$<
	cp onedriver@.service /etc/systemd/user/
	systemctl daemon-reload


localinstall: onedriver
	mkdir -p ~/.config/systemd/user ~/.local/bin
	cp $< ~/.local/bin/$<
	cp onedriver@.service ~/.config/systemd/user/
	sed -i 's/\/usr\/bin/%h\/.local\/bin/g' ~/.config/systemd/user/onedriver@.service
	systemctl --user daemon-reload


# kind of a yucky build using nfpm - will be replaced later with a real .deb
# build pipeline
onedriver.deb: onedriver
	nfpm pkg --target $@


rpm: onedriver.spec
	rm -f ~/rpmbuild/RPMS/x86_64/onedriver*.rpm
	mkdir -p ~/rpmbuild/SOURCES
	#spectool -g -R $<
	# skip generation of debuginfo package
	rpmbuild -bb --define "debug_package %{nil}" $<
	cp ~/rpmbuild/RPMS/x86_64/onedriver*.rpm .


# a large text file for us to test upload sessions with. #science
dmel.fa:
	curl ftp://ftp.ensemblgenomes.org/pub/metazoa/release-42/fasta/drosophila_melanogaster/dna/Drosophila_melanogaster.BDGP6.22.dna.chromosome.X.fa.gz | zcat > $@


# For offline tests, the test binary is built online, then network access is
# disabled and tests are run. sudo is required - otherwise we don't have
# permission to mount the fuse filesystem.
test: onedriver dmel.fa $(EXTRA_TEST_DEPS)
	rm -f *.race*
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" go test -race -v -parallel=8 -count=1 ./graph || true
	go test -c ./offline
	@echo "sudo is required to run tests of offline functionality:"
	sudo $(UNSHARE) -n -S $(TEST_UID) -G $(TEST_GID) ./offline.test -test.v -test.parallel=8 -test.count=1


# used by travis CI since the version of unshare is too old on ubuntu 18.04
unshare:
	rm -rf util-linux-$(UNSHARE_VERSION)*
	wget https://mirrors.edge.kernel.org/pub/linux/utils/util-linux/v$(UNSHARE_VERSION)/util-linux-$(UNSHARE_VERSION).tar.gz
	tar -xzf util-linux-$(UNSHARE_VERSION).tar.gz
	cd util-linux-$(UNSHARE_VERSION) && ./configure --disable-dependency-tracking
	make -C util-linux-$(UNSHARE_VERSION) unshare
	cp util-linux-$(UNSHARE_VERSION)/unshare .


# for autocompletion by ide-clangd
compile_flags.txt:
	pkg-config --cflags gtk+-3.0 webkit2gtk-4.0 | sed 's/ /\n/g' > $@


# will literally purge everything: all built artifacts, all logs, all tests,
# all files tests depend on, all auth tokens... EVERYTHING
clean:
	fusermount -uz mount/ || true
	rm -f *.db *.rpm *.deb *.log *.fa *.gz *.test onedriver unshare auth_tokens.json
	rm -rf util-linux-*
