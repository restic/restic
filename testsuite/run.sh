#!/bin/bash

set -e

export dir=$(dirname "$0")
export fake_data_file="${dir}/fake-data.tar.gz"

prepare() {
    export BASE="$(mktemp --tmpdir --directory restic-testsuite-XXXXXX)"
    export RESTIC_REPOSITORY="${BASE}/restic-backup"
    export RESTIC_PASSWORD="foobar"
    export DATADIR="${BASE}/fake-data"
    debug "repository is at ${RESTIC_REPOSITORY}"

    mkdir -p "$DATADIR"
    (cd "$DATADIR"; tar xz) < "$fake_data_file"
    debug "extracted fake data to ${DATADIR}"
}

cleanup() {
    if [ "$DEBUG" = "1" ]; then
        debug "leaving dir ${BASE}"
        return
    fi

    rm -rf "${BASE}"
    debug "removed dir ${BASE}"
    unset BASE
    unset RESTIC_REPOSITORY
}

msg() {
    printf "%s\n" "$*"
}

pass() {
    printf "\e[32m%s\e[39m\n" "$*"
}

err() {
    printf "\e[31m%s\e[39m\n" "$*"
}

debug() {
    if [ "$DEBUG" = "1" ]; then
        printf "\e[33m%s\e[39m\n" "$*"
    fi
}

fail() {
    err "$@"
    exit 1
}

run() {
    if [ "$DEBUG" = "1" ]; then
        "$@"
    else
        "$@" > /dev/null
    fi
}

export -f prepare cleanup msg debug pass err fail run

if [ -z "$BASEDIR" ]; then
    echo "BASEDIR not set" >&2
    exit 2
fi

which restic > /dev/null || fail "restic binary not found!"
which restic.debug > /dev/null || fail "restic.debug binary not found!"
which dirdiff > /dev/null || fail "dirdiff binary not found!"

debug "restic path: $(which restic)"
debug "restic.debug path: $(which restic.debug)"
debug "dirdiff path: $(which dirdiff)"
debug "path: $PATH"

debug "restic versions:"
run restic version
run restic.debug version

if [ "$#" -gt 0 ]; then
    testfiles="$1"
else
    testfiles=(${dir}/test-*.sh)
fi

echo "testfiles: ${testfiles[@]}"

failed=""
for testfile in "${testfiles[@]}"; do
    msg "================================================================================"
    msg "run test $testfile"
    msg ""

    current=$(basename "${testfile}" .sh)

    if [ "$DEBUG" = "1" ]; then
        OPTS="-v"
    fi

    if bash $OPTS "${testfile}"; then
        pass "${current} pass"
    else
        err "${current} failed!"
        failed+=" ${current}"
    fi
done

if [ -n "$failed" ]; then
    err "failed tests: ${failed}"
    msg "restic versions:"
    run restic version
    run restic.debug version
    exit 1
fi
