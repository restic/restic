.PHONY: all clean test restic

all: restic

restic:
	go run build.go -t debug

clean:
	rm -f restic

test:
	go test ./cmd/... ./internal/...

default: restic

