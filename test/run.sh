#!/bin/bash

set -e

export khepri="${1:-khepri}"; shift
export dirdiff="${1:-dirdiff}"; shift
export dir=$(dirname "$0")
export fake_data_file="${dir}/fake-data.tar.gz"

prepare() {
    export BASE="$(mktemp --tmpdir --directory khepri-testsuite-XXXXXX)"
    export KHEPRI_REPOSITORY="${BASE}/khepri-backup"
    export DATADIR="${BASE}/fake-data"
    export KHEPRI_PASSWORD="foobar"
    debug "repository is at ${KHEPRI_REPOSITORY}"

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
    unset KHEPRI_REPOSITORY
}

khepri() {
    "${khepri}" "$@"
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

export -f khepri dirdiff prepare cleanup msg debug pass err fail run

if [ ! -x "$khepri" ]; then
    fail khepri binary not found!
fi

if [ "$#" -gt 0 ]; then
    testfiles="$1"
else
    testfiles=(${dir}/test-*.sh)
fi

echo "testfiles: $testfiles"

for testfile in "$testfiles"; do
    current=$(basename "${testfile}" .sh)

    bash "${testfile}" && pass "${current} pass" || err "${current} failed!"
done
