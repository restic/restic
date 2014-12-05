.PHONY: clean all test

FLAGS=
#FLAGS+=-race

test:
	for dir in cmd/* ; do \
		(cd "$$dir"; go build $(FLAGS)) \
	done
	test/run.sh cmd/restic/restic cmd/dirdiff/dirdiff

clean:
	go clean
	for dir in cmd/* ; do \
		(cd "$$dir"; go clean) \
	done
