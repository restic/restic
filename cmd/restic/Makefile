.PHONY: all clean debug

# include config file if it exists
-include $(CURDIR)/config.mk

all: restic

debug: restic.debug

restic: $(wildcard *.go) $(wildcard ../../*.go) $(wildcard ../../*/*.go)
	go build -a

restic.debug: $(wildcard *.go) $(wildcard ../../*.go) $(wildcard ../../*/*.go)
	go build -a -tags debug -o restic.debug

clean:
	go clean
	rm -f restic restic.debug
