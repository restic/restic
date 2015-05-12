#!/bin/bash

# tempdir for binaries
export BASEDIR="$(mktemp --tmpdir --directory restic-testsuite-XXXXXX)"
export DEBUG_LOG="${BASEDIR}/restic.log"

export TZ=UTC

echo "restic testsuite basedir ${BASEDIR}"

# run tests
testsuite/run.sh "$@"
