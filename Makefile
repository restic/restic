.PHONY: clean all test release debug

GOFLAGS=
#GOFLAGS+=-race

all: release test

release:
	for dir in cmd/* ; do \
		test -f "$$dir/Makefile" && \
		(GOFLAGS="$(GOFLAGS)" make -C "$$dir") \
	done

debug:
	for dir in cmd/* ; do \
		test -f "$$dir/Makefile" && \
		(GOFLAGS="$(GOFLAGS)" make -C "$$dir" debug) \
	done

test: release debug
	go test -v ./...
	test/run.sh cmd/restic:cmd/dirdiff

test-%: test/test-%.sh
	echo $*
	test/run.sh cmd/restic:cmd/dirdiff "test/$@.sh"

clean:
	go clean
	for dir in cmd/* ; do \
		test -f "$$dir/Makefile" && \
		(make -C "$$dir" clean) \
	done
