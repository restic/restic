# try to get version from git
VERSION = $(shell ./version.sh)
VERSION ?= "unknown version"
LDFLAGS = -X main.version $(VERSION)
TAGS =

.PHONY: all clean debug

all: khepri

khepri: *.go
	go build $(TAGS) -ldflags "$(LDFLAGS)"

debug: TAGS=-tags debug
debug: khepri

clean:
	go clean
