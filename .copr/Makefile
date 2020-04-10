srpm:
	dnf -y install golang git rsync
	$(MAKE) -f Makefile srpm
	cp *.src.rpm $(outdir)
