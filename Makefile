.PHONY: clean all debug test

all:
	for dir in ./cmd/* ; do \
		(echo "$$dir"; cd "$$dir"; godep go build) \
	done

debug:
	(cd cmd/restic; godep go build -a -tags debug)

test:
	./testsuite.sh

clean:
	godep go clean ./...
