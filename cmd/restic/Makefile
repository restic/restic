# try to get version from git
VERSION = $(shell ./version.sh)
VERSION ?= "unknown version"
LDFLAGS = -X main.version $(VERSION)
TAGS =

.PHONY: all both clean debug

# include config file if it exists
-include $(CURDIR)/config.mk

all: restic

both: restic restic.debug

debug:  restic.debug

restic: $(wildcard *.go) $(wildcard ../../*.go) $(wildcard ../../*/*.go)
	go build -tags "$(TAGS)" $(GOFLAGS) -ldflags "$(LDFLAGS)"

restic.debug: $(wildcard *.go) $(wildcard ../../*.go) $(wildcard ../../*/*.go)
	go build -o restic.debug -tags "debug" $(GOFLAGS) -ldflags "$(LDFLAGS)"

clean:
	go clean
	rm -f restic restic.debug
