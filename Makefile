.PHONY: all clean env test bench gox test-integration

TMPGOPATH=$(PWD)/.gopath
VENDORPATH=$(PWD)/Godeps/_workspace
BASE=github.com/restic/restic
BASEPATH=$(TMPGOPATH)/src/$(BASE)

GOPATH=$(TMPGOPATH):$(VENDORPATH)

GOTESTFLAGS ?= -v
GOX_OS ?= linux darwin openbsd freebsd
SFTP_PATH ?= /usr/lib/ssh/sftp-server

export GOPATH GOX_OS

all: restic

.gopath:
	mkdir -p .gopath/src/github.com/restic
	ln -sf ../../../.. .gopath/src/github.com/restic/restic

restic: .gopath
	cd $(BASEPATH) && \
		go build -a -ldflags "-s" -o restic ./cmd/restic

restic.debug: .gopath
	cd $(BASEPATH) && \
		go build -a -tags debug -o restic ./cmd/restic

clean:
	rm -rf .gopath restic *.cov restic_*
	go clean ./...

test: .gopath
	cd $(BASEPATH) && \
		go test $(GOTESTFLAGS) ./...

bench: .gopath
	cd $(BASEPATH) && \
		go test GOTESTFLAGS) bench ./...

gox: .gopath
	cd $(BASEPATH) && \
		gox -verbose -os "$(GOX_OS)" ./cmd/restic

test-integration: .gopath
	cd $(BASEPATH)/backend && \
		go test $(GOTESTFLAGS) -test.sftppath $(SFTP_PATH) ./...

all.cov: .gopath
	cd $(BASEPATH) && \
		go list ./... | \
			while read pkg; do \
				go test -covermode=count -coverprofile=$$(base64 <<< $$pkg).cov $$pkg; \
			done
	echo "mode: count" > all.cov; tail -q -n +2 *.cov >> all.cov

env:
	@echo export GOPATH=\"$(GOPATH)\"
