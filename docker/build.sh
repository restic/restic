#!/bin/sh
root="$(readlink -f "$0")"
root="$(dirname "$(dirname "${root}")")"

set -e

export DOCKER_BUILDKIT=${DOCKER_BUILDKIT-1}

echo "Build docker image restic/restic:latest"
docker build \
  --rm \
  --pull \
  --file "${root}"/docker/Dockerfile \
  --tag restic/restic:latest \
  "${root}" "$@"
