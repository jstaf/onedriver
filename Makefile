.PHONY: all, test, srpm, rpm, changes, dsc, deb, clean, install

# autocalculate software/package versions
VERSION := $(shell grep Version onedriver.spec | sed 's/Version: *//g')
RELEASE := $(shell grep -oP "Release: *[0-9]+" onedriver.spec | sed 's/Release: *//g')
DIST := $(shell rpm --eval "%{?dist}" 2> /dev/null || echo 1)
RPM_FULL_VERSION = $(VERSION)-$(RELEASE)$(DIST)

# test-specific variables
TEST_UID := $(shell id -u)
TEST_GID := $(shell id -g)


all: onedriver onedriver-launcher


onedriver: $(shell find fs/ -type f) cmd/onedriver/main.go
	go build \
		-ldflags="-X github.com/jstaf/onedriver/cmd/common.commit=$(shell git rev-parse HEAD)" \
		./cmd/onedriver


onedriver-headless: $(shell find fs/ -type f) cmd/onedriver/main.go
	CGO_ENABLED=0 go build -o onedriver-headless \
		-ldflags="-X github.com/jstaf/onedriver/cmd/common.commit=$(shell git rev-parse HEAD)" \
		./cmd/onedriver


# -Wno-deprecated-declarations is for gotk3, which uses deprecated methods for older
# glib compatibility: https://github.com/gotk3/gotk3/issues/762#issuecomment-919035313
onedriver-launcher: $(shell find ui/ -type f) cmd/onedriver-launcher/main.go
	CGO_CFLAGS=-Wno-deprecated-declarations go build -v \
		-ldflags="-X github.com/jstaf/onedriver/cmd/common.commit=$(shell git rev-parse HEAD)" \
		./cmd/onedriver-launcher


install: onedriver onedriver-launcher
	cp onedriver /usr/bin/
	cp onedriver-launcher /usr/bin/
	mkdir /usr/share/icons/onedriver/
	cp resources/onedriver.svg /usr/share/icons/onedriver/
	cp resources/onedriver.png /usr/share/icons/onedriver/
	cp resources/onedriver.desktop /usr/share/applications/
	cp resources/onedriver@.service /etc/systemd/user/
	gzip -c resources/onedriver.1 > /usr/share/man/man1/onedriver.1.gz
	mandb


# used to create release tarball for rpmbuild
v$(VERSION).tar.gz: $(shell git ls-files)
	rm -rf onedriver-$(VERSION)
	mkdir -p onedriver-$(VERSION)
	git ls-files > filelist.txt
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
onedriver_$(VERSION).orig.tar.gz: v$(VERSION).tar.gz
	cp $< $@


# create the debian source package for the current version
changes: onedriver_$(VERSION)-$(RELEASE)_source.changes
onedriver_$(VERSION)-$(RELEASE)_source.changes: onedriver_$(VERSION).orig.tar.gz
	cd onedriver-$(VERSION) && debuild -S -sa -d


# just a helper target to use while building debs
dsc: onedriver_$(VERSION)-$(RELEASE).dsc
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
test: onedriver onedriver-launcher dmel.fa
	rm -f *.race* fusefs_tests.log
	CGO_ENABLED=0 gotest -v -parallel=8 -count=1 $(shell go list ./ui/... | grep -v offline)
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" \
		gotest -race -v -parallel=8 -count=1 ./cmd/...
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" \
		gotest -race -v -parallel=8 -count=1 ./fs/graph/...
	GORACE="log_path=fusefs_tests.race strip_path_prefix=1" \
		gotest -race -v -parallel=8 -count=1 ./fs
	go test -c ./fs/offline
	@echo "sudo is required to run tests of offline functionality:"
	sudo unshare -n -S $(TEST_UID) -G $(TEST_GID) ./offline.test -test.v -test.parallel=8 -test.count=1


# will literally purge everything: all built artifacts, all logs, all tests,
# all files tests depend on, all auth tokens... EVERYTHING
clean:
	fusermount -uz mount/ || true
	rm -f *.db *.rpm *.deb *.dsc *.changes *.build* *.upload *.xz filelist.txt .commit
	rm -f *.log *.fa *.gz *.test vgcore.* onedriver onedriver-headless onedriver-launcher .auth_tokens.json
	rm -rf util-linux-*/ onedriver-*/ vendor/ build/
