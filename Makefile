.PHONY = test

onedriver: graph/*.go graph/*.c graph/*.h
	go build

test: onedriver
	./$< -a
	ln -s -f ../auth_tokens.json graph/
	go test ./graph

# for autocompletion by ide-clangd
compile_flags.txt:
	pkg-config --cflags gtk+-3.0 webkit2gtk-4.0 | sed 's/ /\n/g' > $@
