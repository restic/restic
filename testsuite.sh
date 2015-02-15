#!/bin/bash

# tempdir for binaries
export BASEDIR="$(mktemp --tmpdir --directory restic-testsuite-XXXXXX)"
export BINDIR="${BASEDIR}/bin"
export PATH="${BINDIR}:$PATH"
export DEBUG_LOG="${BASEDIR}/restic.log"

echo "restic testsuite basedir ${BASEDIR}"

# build binaries
go build -a -o "${BINDIR}/restic" ./cmd/restic
go build -a -tags debug -o "${BINDIR}/restic.debug" ./cmd/restic
go build -a -o "${BINDIR}/dirdiff" ./cmd/dirdiff

# run tests
testsuite/run.sh "$@"
