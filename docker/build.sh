#!/bin/sh

set -e


echo "Build docker image restic/restic:latest"
docker build --rm -t restic/restic:latest -f docker/Dockerfile .
