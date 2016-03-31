.PHONY: all clean test restic

all: restic

restic:
	go run build.go

clean:
	rm -rf restic

test:
	go test ./...
