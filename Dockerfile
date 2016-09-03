# This Dockerfiles configures a container that is similar to the Travis CI
# environment and can be used to run tests locally.
#
# build the image:
#   docker build -t restic/test .
#
# run all tests and cross-compile restic:
#   docker run --rm -v $PWD:/home/travis/restic restic/test go run run_integration_tests.go -minio minio
#
# run interactively:
#   docker run --interactive --tty --rm -v $PWD:/home/travis/restic restic/test /bin/bash
#
# run a subset of tests:
#   docker run --rm -v $PWD:/home/travis/restic restic/test gb test -v ./backend
#
# build the image for an older version of Go:
#   docker build --build-arg GOVERSION=1.3.3 -t restic/test:go1.3.3 .

FROM ubuntu:14.04

ARG GOVERSION=1.7
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
ENV GOPATH $HOME/gopath
ENV PATH $PATH:$GOROOT/bin:$GOPATH/bin:$HOME/bin

RUN mkdir -p $HOME/restic

# pre-install tools, this speeds up running the tests itself
RUN go get github.com/constabulary/gb/...
RUN go get golang.org/x/tools/cmd/cover
RUN go get github.com/mitchellh/gox
RUN go get github.com/pierrre/gotestcover
RUN mkdir $HOME/bin \
    && wget -q -O $HOME/bin/minio https://dl.minio.io/server/minio/release/linux-${GOARCH}/minio \
    && chmod +x $HOME/bin/minio

# set TRAVIS_BUILD_DIR for integration script
ENV TRAVIS_BUILD_DIR $HOME/restic

WORKDIR $HOME/restic
