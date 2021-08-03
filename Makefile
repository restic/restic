.PHONY: all clean test restic install

all: restic

restic:
	go run build.go

clean:
	rm -f restic

test:
	go test ./cmd/... ./internal/...

install:
	cp restic /usr/bin/
	

