.PHONY: clean all debug test

all:
	for dir in ./cmd/* ; do \
		(echo "$$dir"; cd "$$dir"; go build) \
	done

debug:
	(cd cmd/restic; go build -a -tags debug)

test:
	./testsuite.sh

clean:
	go clean ./...

fmt:
	gofmt -w=true **/*.go
