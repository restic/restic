#!/bin/bash

set -e

export restic="${1:-restic}"; shift
export dirdiff="${1:-dirdiff}"; shift
export dir=$(dirname "$0")
export fake_data_file="${dir}/fake-data.tar.gz"

prepare() {
    export BASE="$(mktemp --tmpdir --directory restic-testsuite-XXXXXX)"
    export RESTIC_REPOSITORY="${BASE}/restic-backup"
    export DATADIR="${BASE}/fake-data"
    export RESTIC_PASSWORD="foobar"
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

restic() {
    "${restic}" "$@"
}

dirdiff() {
    "${dirdiff}" "$@"
}

msg() {
    printf "%s: %s\n" "$(basename "$0" .sh)" "$*"
}

pass() {
    printf "\e[32m%s: %s\e[39m\n" "$(basename "$0" .sh)" "$*"
}

err() {
    printf "\e[31m%s: %s\e[39m\n" "$(basename "$0" .sh)" "$*"
}

debug() {
    if [ "$DEBUG" = "1" ]; then
        printf "\e[33m%s: %s\e[39m\n" "$(basename "$0" .sh)" "$*"
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

export -f restic dirdiff prepare cleanup msg debug pass err fail run

if [ ! -x "$restic" ]; then
    fail restic binary not found!
fi

if [ "$#" -gt 0 ]; then
    testfiles="$1"
else
    testfiles=(${dir}/test-*.sh)
fi

echo "testfiles: ${testfiles[@]}"

failed=""
for testfile in "${testfiles[@]}"; do
    current=$(basename "${testfile}" .sh)

    if bash "${testfile}"; then
        pass "${current} pass"
    else
        err "${current} failed!"
        failed+=" ${current}"
    fi
done

if [ -n "$failed" ]; then
    err "failed tests: ${failed}"
    exit 1
fi
