.PHONY: clean all test

test:
	go test -race ./...
	for dir in cmd/* ; do \
		(cd "$$dir"; go build -race) \
	done
	test/run.sh cmd/khepri/khepri cmd/dirdiff/dirdiff

clean:
	go clean
	for dir in cmd/* ; do \
		(cd "$$dir"; go clean) \
	done
