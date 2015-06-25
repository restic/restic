.PHONY: all clean test

SOURCE=$(wildcard *.go) $(wildcard */*.go) $(wildcard */*/*.go)

export GOPATH GOX_OS

all: restic

restic: $(SOURCE)
	go run build.go

restic.debug: $(SOURCE)
	go run build.go -tags debug

clean:
	rm -rf restic restic.debug

test: $(SOURCE)
	go run run_tests.go /dev/null

all.cov: $(SOURCE)
	go run run_tests.go all.cov
