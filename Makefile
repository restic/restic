.PHONY: all clean test restic

all: restic

restic:
	go run build.go

clean:
	rm -f restic

test:
	go test ./cmd/... ./internal/...

