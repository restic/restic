#!/bin/sh

set -e

export DOCKER_BUILDKIT=${DOCKER_BUILDKIT-1}

echo "Build docker image restic/restic:latest"
docker build \
  --rm \
  --pull \
  --file docker/Dockerfile \
  --tag restic/restic:latest \
  .
