.PHONY: all clean test

all: restic

restic: $(SOURCE)
	go run build.go

clean:
	rm -rf restic

test: $(SOURCE)
	go test ./...
