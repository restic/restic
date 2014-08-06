.PHONY: clean all test

test:
	go test
	for dir in cmd/* ; do \
		(cd "$$dir"; go build) \
	done
	test/run.sh cmd/khepri/khepri cmd/dirdiff/dirdiff

clean:
	go clean
	for dir in cmd/* ; do \
		(cd "$$dir"; go clean) \
	done
