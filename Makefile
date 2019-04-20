.PHONY = test

# development copy with race detection - for a normal copy, use "go build"
onedriver: graph/*.go graph/*.c graph/*.h logger/*.go main.go
	go build -race

# a large text file for us to test upload sessions with. #science
dmel.fa:
	curl ftp://ftp.ensemblgenomes.org/pub/metazoa/release-42/fasta/drosophila_melanogaster/dna/Drosophila_melanogaster.BDGP6.22.dna.chromosome.X.fa.gz | zcat > $@

# cache disabled to always force rerun of all tests
# (some tests can fail due to race conditions (since all fuse ops are async))
test: onedriver dmel.fa
	go test -race -count=1 ./logger ./graph

# for autocompletion by ide-clangd
compile_flags.txt:
	pkg-config --cflags gtk+-3.0 webkit2gtk-4.0 | sed 's/ /\n/g' > $@
