# This Dockerfiles configures a container that is similar to the Travis CI
# environment and can be used to run tests locally.
#
# build the image:
#   docker build -t restic/test .
#
# run tests:
#   docker run --rm -v $PWD:/home/travis/gopath/src/github.com/restic/restic restic/test go run run_integration_tests.go
#
# run interactively with:
#   docker run --interactive --tty --rm -v $PWD:/home/travis/gopath/src/github.com/restic/restic restic/test /bin/bash
#
# run a tests:
#   docker run --rm -v $PWD:/home/travis/gopath/src/github.com/restic/restic restic/test go test -v ./backend

FROM ubuntu:14.04

ARG GOVERSION=1.5.2
ARG GOARCH=amd64

# install dependencies
RUN apt-get update
RUN apt-get install -y --no-install-recommends ca-certificates wget git build-essential openssh-server

# add and configure user
ENV HOME /home/travis
RUN useradd -m -d $HOME -s /bin/bash travis

# run everything below as user travis
USER travis
WORKDIR $HOME

# download and install Go
RUN wget -q -O /tmp/go.tar.gz https://storage.googleapis.com/golang/go${GOVERSION}.linux-${GOARCH}.tar.gz
RUN tar xf /tmp/go.tar.gz && rm -f /tmp/go.tar.gz
ENV GOROOT $HOME/go
ENV PATH $PATH:$GOROOT/bin

ENV GOPATH $HOME/gopath
ENV PATH $PATH:$GOPATH/bin

RUN mkdir -p $GOPATH/src/github.com/restic/restic

# install tools
RUN go get golang.org/x/tools/cmd/cover
RUN go get github.com/mattn/goveralls
RUN go get github.com/mitchellh/gox
RUN GO15VENDOREXPERIMENT=1 go get github.com/minio/minio

# set TRAVIS_BUILD_DIR for integration script
ENV TRAVIS_BUILD_DIR $GOPATH/src/github.com/restic/restic
ENV GOPATH $GOPATH:${TRAVIS_BUILD_DIR}/Godeps/_workspace

WORKDIR $TRAVIS_BUILD_DIR
