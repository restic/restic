#!/bin/sh

set -e

# figure out the directory this script sits in
dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)


echo "Build docker image restic/restic:latest"
docker build --rm -t restic/restic:latest -f "$dir/Dockerfile" "$dir/.."
