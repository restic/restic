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

export -f prepare cleanup msg debug pass err fail run

# first argument is restic path
export PATH="$1:$PATH"; shift

which restic || fail "restic binary not found!"
which dirdiff || fail "dirdiff binary not found!"

debug "restic path: $(which restic)"
debug "dirdiff path: $(which dirdiff)"

if [ "$#" -gt 0 ]; then
    testfiles="$1"
else
    testfiles=(${dir}/test-*.sh)
fi

echo "testfiles: ${testfiles[@]}"

failed=""
for testfile in "${testfiles[@]}"; do
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
    exit 1
fi
