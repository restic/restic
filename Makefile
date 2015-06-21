.PHONY: all clean env test bench gox test-integration

TMPGOPATH=$(PWD)/.gopath
VENDORPATH=$(PWD)/Godeps/_workspace
BASE=github.com/restic/restic
BASEPATH=$(TMPGOPATH)/src/$(BASE)

GOPATH=$(TMPGOPATH):$(VENDORPATH)

GOTESTFLAGS ?= -v
GOX_OS ?= linux darwin openbsd freebsd
SFTP_PATH ?= /usr/lib/ssh/sftp-server

CMDS=$(patsubst cmd/%,%,$(wildcard cmd/*))
CMDS_DEBUG=$(patsubst %,%.debug,$(CMDS))

SOURCE=$(wildcard *.go) $(wildcard */*.go) $(wildcard */*/*.go)

export GOPATH GOX_OS

all: restic

.gopath:
	mkdir -p .gopath/src/github.com/restic
	ln -snf ../../../.. .gopath/src/github.com/restic/restic

%: cmd/% .gopath $(SOURCE)
	cd $(BASEPATH) && \
		go build -a -tags release -ldflags "-s" -o $@ ./$<

%.debug: cmd/% .gopath $(SOURCE)
	cd $(BASEPATH) && \
		go build -a -tags debug -ldflags "-s" -o $@ ./$<

clean:
	rm -rf .gopath $(CMDS) $(CMDS_DEBUG) *.cov restic_*
	go clean ./...

test: .gopath
	cd $(BASEPATH) && \
		go test $(GOTESTFLAGS) ./...

bench: .gopath
	cd $(BASEPATH) && \
		go test $(GOTESTFLAGS) -bench ./...

gox: .gopath $(SOURCE)
	cd $(BASEPATH) && \
		gox -verbose -os "$(GOX_OS)" ./cmd/restic

all.cov: .gopath $(SOURCE)
	cd $(BASEPATH) && go run run_tests.go all.cov

env:
	@echo export GOPATH=\"$(GOPATH)\"

goenv:
	go env

list: .gopath
	cd $(BASEPATH) && \
		go list ./...
