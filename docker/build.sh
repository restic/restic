#!/bin/sh

set -e

echo "Build binary using golang docker image"
docker run --rm -ti \
    -v "`pwd`":/go/src/github.com/restic/restic \
    -w /go/src/github.com/restic/restic golang:1.8.3-alpine go run build.go

echo "Build docker image restic/restic:latest"
docker build --rm -t restic/restic:latest -f docker/Dockerfile .
