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
		go build -a -ldflags "-s" -o $@ ./$<

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

test-integration: .gopath
	cd $(BASEPATH) && go test $(GOTESTFLAGS) \
			./backend \
			-cover -covermode=count -coverprofile=integration-sftp.cov \
			-test.integration \
			-test.sftppath=$(SFTP_PATH)

	cd $(BASEPATH) && go test $(GOTESTFLAGS) \
			./cmd/restic \
			-cover -covermode=count -coverprofile=integration.cov \
			-test.integration \
			-test.datafile=$(PWD)/testsuite/fake-data.tar.gz

all.cov: .gopath $(SOURCE) test-integration
	cd $(BASEPATH) && \
		go list ./... | while read pkg; do \
			go test -covermode=count -coverprofile=$$(echo $$pkg | base64).cov $$pkg; \
		done

	echo "mode: count" > all.cov
	tail -q -n +2 *.cov >> all.cov

env:
	@echo export GOPATH=\"$(GOPATH)\"

goenv:
	go env

list: .gopath
	cd $(BASEPATH) && \
		go list ./...
