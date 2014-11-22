# try to get version from git
VERSION = $(shell ./version.sh)
VERSION ?= "unknown version"
LDFLAGS = -X main.version $(VERSION)
TAGS =

.PHONY: all clean debug

# include config file if it exists
-include $(CURDIR)/config.mk

all: khepri

khepri: *.go $(wildcard ../../*.go) $(wildcard ../../*/*.go)
	go build $(TAGS) -ldflags "$(LDFLAGS)"

debug: TAGS=-tags debug_cmd
debug: khepri

clean:
	go clean
