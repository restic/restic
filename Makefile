.PHONY: all clean test restic

all: restic

restic:
	go run -mod=vendor build.go || go run build.go

clean:
	rm -f restic

test:
	go test ./cmd/... ./internal/...

