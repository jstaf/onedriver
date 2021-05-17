.PHONY: all, test, srpm, rpm, changes, dsc, deb, clean, auth_expire_now, auth_invalidate, install, localinstall

# autocalculate software/package versions
VERSION = $(shell grep Version onedriver.spec | sed 's/Version: *//g')
RELEASE = $(shell grep -oP "Release: *[0-9]+" onedriver.spec | sed 's/Release: *//g')
DIST = $(shell rpm --eval "%{?dist}" 2> /dev/null || echo 1)
RPM_FULL_VERSION = $(VERSION)-$(RELEASE)$(DIST)

# test-specific variables
TEST_UID = $(shell id -u)
TEST_GID = $(shell id -g)
UNSHARE_VERSION = 2.34
ifeq ($(shell unshare --help | grep setuid | wc -l), 1)
	UNSHARE = unshare
else
	UNSHARE = ./unshare
	EXTRA_TEST_DEPS = unshare
endif


onedriver: $(shell find fs/ -type f) logger/*.go main.go
	go build -ldflags="-X main.commit=$(shell git rev-parse HEAD)"


onedriver-headless: $(shell find fs/ -type f) logger/*.go main.go
	CGO_ENABLED=0 go build -o onedriver-headless -ldflags="-X main.commit=$(shell git rev-parse HEAD)"


# run all tests, build all artifacts, compute checksums for release
all: test checksums.txt
checksums.txt: onedriver-headless onedriver-$(VERSION).tar.gz onedriver-$(RPM_FULL_VERSION).x86_64.rpm onedriver_$(VERSION)-$(RELEASE)_amd64.deb
	sha256sum $^ > checksums.txt


install: onedriver
	cp $< /usr/bin/$<
	cp resources/onedriver@.service /etc/systemd/user/
	gzip -c resources/onedriver.1 > /usr/share/man/man1/onedriver.1.gz
	mandb
	systemctl daemon-reload


localinstall: onedriver
	mkdir -p ~/.config/systemd/user ~/.local/bin
	cp $< ~/.local/bin/$<
	cp resources/onedriver@.service ~/.config/systemd/user/
	sed -i 's/\/usr\/bin/%h\/.local\/bin/g' ~/.config/systemd/user/onedriver@.service
	systemctl --user daemon-reload


# used to create release tarball for rpmbuild
v$(VERSION).tar.gz: $(shell git ls-files)
	rm -rf onedriver-$(VERSION)
	mkdir -p onedriver-$(VERSION)
	git ls-files > filelist.txt
	# needed for debian build
	git rev-parse HEAD > .commit
	echo .commit >> filelist.txt
	rsync -a --files-from=filelist.txt . onedriver-$(VERSION)
	go mod vendor
	cp -R vendor/ onedriver-$(VERSION)
	tar -czf $@ onedriver-$(VERSION)


# build srpm package used for rpm build with mock
srpm: onedriver-$(RPM_FULL_VERSION).src.rpm 
onedriver-$(RPM_FULL_VERSION).src.rpm: v$(VERSION).tar.gz
	rpmbuild -ts $<
	cp $$(rpm --eval '%{_topdir}')/SRPMS/$@ .


# build the rpm for the default mock target
MOCK_CONFIG=$(shell readlink -f /etc/mock/default.cfg | grep -oP '[a-z0-9-]+x86_64')
rpm: onedriver-$(RPM_FULL_VERSION).x86_64.rpm
onedriver-$(RPM_FULL_VERSION).x86_64.rpm: onedriver-$(RPM_FULL_VERSION).src.rpm
	mock -r /etc/mock/$(MOCK_CONFIG).cfg $<
	cp /var/lib/mock/$(MOCK_CONFIG)/result/$@ .


# create a release tarball for debian builds
onedriver_$(VERSION).orig.tar.gz: onedriver-$(VERSION).tar.gz
	cp $< $@


# create the debian source package for the current version
changes: onedriver_$(VERSION)-$(RELEASE)_source.changes
onedriver_$(VERSION)-$(RELEASE)_source.changes: onedriver_$(VERSION).orig.tar.gz
	cd onedriver-$(VERSION) && debuild -S -sa -d


# just a helper target to use while building debs
onedriver_$(VERSION)-$(RELEASE).dsc: onedriver_$(VERSION).orig.tar.gz
	dpkg-source --build onedriver-$(VERSION)


# create the debian package in a chroot via pbuilder
deb: onedriver_$(VERSION)-$(RELEASE)_amd64.deb
onedriver_$(VERSION)-$(RELEASE)_amd64.deb: onedriver_$(VERSION)-$(RELEASE).dsc
	sudo mkdir -p /var/cache/pbuilder/aptcache
	sudo pbuilder --build $<
	cp /var/cache/pbuilder/result/$@ .


# a large text file for us to test upload sessions with. #science
dmel.fa:
	curl ftp://ftp.ensemblgenomes.org/pub/metazoa/release-42/fasta/drosophila_melanogaster/dna/Drosophila_melanogaster.BDGP6.22.dna.chromosome.X.fa.gz | zcat > $@


# For offline tests, the test binary is built online, then network access is
# disabled and tests are run. sudo is required - otherwise we don't have
# permission to mount the fuse filesystem.
test: onedriver dmel.fa $(EXTRA_TEST_DEPS)
	rm -f *.race* fusefs_tests.log
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" gotest -race -v -parallel=8 -count=1 ./fs/graph
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" gotest -race -v -parallel=8 -count=1 ./fs
	go test -c ./fs/offline
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
auth_expire_now:
	sed -i 's/"expires_at":[0-9]\+/"expires_at":0/g' ~/.cache/onedriver/auth_tokens.json


auth_invalidate:
	sed -i 's/"access_token":.\{5\}/"access_token":"/g' ~/.cache/onedriver/auth_tokens.json


# for autocompletion by ide-clangd
compile_flags.txt:
	pkg-config --cflags gtk+-3.0 webkit2gtk-4.0 | sed 's/ /\n/g' > $@


# will literally purge everything: all built artifacts, all logs, all tests,
# all files tests depend on, all auth tokens... EVERYTHING
clean:
	fusermount -uz mount/ || true
	rm -f *.db *.rpm *.deb *.dsc *.changes *.build* *.upload *.xz filelist.txt .commit
	rm -f *.log *.fa *.gz *.test onedriver onedriver-headless unshare .auth_tokens.json
	rm -rf util-linux-*/ onedriver-*/ vendor/
