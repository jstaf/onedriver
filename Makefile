.PHONY: all, test, srpm, rpm, dsc, deb, clean, expire_now, install, localinstall

TEST_UID = $(shell id -u)
TEST_GID = $(shell id -g)
RPM_VERSION = $(shell grep Version onedriver.spec | sed 's/Version: *//g')
RPM_RELEASE = $(shell grep -oP "Release: *[0-9]+" onedriver.spec | sed 's/Release: *//g')
RPM_DIST = $(shell rpm --eval "%{?dist}")
UNSHARE_VERSION = 2.34
ifeq ($(shell unshare --help | grep setuid | wc -l), 1)
	UNSHARE = unshare
else
	UNSHARE = ./unshare
	EXTRA_TEST_DEPS = unshare
endif


onedriver: graph/*.go graph/*.c graph/*.h logger/*.go cmd/onedriver/*.go
	go build -ldflags="-X main.commit=$(shell git rev-parse HEAD)" ./cmd/onedriver


all: onedriver test


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


# used to create release tarball for rpmbuild
onedriver-$(RPM_VERSION).tar.gz: $(shell git ls-files)
	mkdir -p onedriver-$(RPM_VERSION)
	git ls-files > filelist.txt
	# no git repo while making rpm, so need to add the git commit info as a file
	git rev-parse HEAD > .commit
	echo .commit >> filelist.txt
	rsync -a --files-from=filelist.txt . onedriver-$(RPM_VERSION)
	tar -czvf $@ onedriver-$(RPM_VERSION)/
	rm -rf onedriver-$(RPM_VERSION)


srpm: onedriver-$(RPM_VERSION)-$(RPM_RELEASE)$(RPM_DIST).src.rpm 
onedriver-$(RPM_VERSION)-$(RPM_RELEASE)$(RPM_DIST).src.rpm: onedriver-$(RPM_VERSION).tar.gz onedriver.spec
	mkdir -p ~/rpmbuild/SOURCES
	cp $< ~/rpmbuild/SOURCES
	rpmbuild -bs onedriver.spec
	cp ~/rpmbuild/SRPMS/$@ .


# build the rpm for the current version defined in the specfile
rpm: onedriver-$(RPM_VERSION)-$(RPM_RELEASE)$(RPM_DIST).x86_64.rpm
onedriver-$(RPM_VERSION)-$(RPM_RELEASE)$(RPM_DIST).x86_64.rpm: onedriver-$(RPM_VERSION).tar.gz onedriver.spec
	mkdir -p ~/rpmbuild/SOURCES
	cp $< ~/rpmbuild/SOURCES
	rpmbuild -bb onedriver.spec
	cp ~/rpmbuild/RPMS/x86_64/$@ .


# create the deb for the current version
dsc: onedriver_$(RPM_VERSION)-$(RPM_RELEASE).dsc
onedriver_$(RPM_VERSION)-$(RPM_RELEASE).dsc: onedriver-$(RPM_VERSION).tar.gz
	rm -rf build/
	mkdir -p build/
	cp $< build/onedriver_$(RPM_VERSION).orig.tar.gz
	cd build && tar -xzf onedriver_$(RPM_VERSION).orig.tar.gz && dpkg-source --build onedriver-0.7.2
	cp build/$@ .


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


# force auth renewal the next time onedriver starts
expire_now:
	sed -i 's/"expires_at":[0-9]\+/"expires_at":0/g' ~/.cache/onedriver/auth_tokens.json


# for autocompletion by ide-clangd
compile_flags.txt:
	pkg-config --cflags gtk+-3.0 webkit2gtk-4.0 | sed 's/ /\n/g' > $@


# will literally purge everything: all built artifacts, all logs, all tests,
# all files tests depend on, all auth tokens... EVERYTHING
clean:
	fusermount -uz mount/ || true
	rm -f *.db *.rpm *.deb *.log *.fa *.gz *.test onedriver unshare auth_tokens.json filelist.txt
	rm -rf util-linux-*
