#!/bin/sh -e

# This must be tested against busybox sh, since there are quirks in its
# implementation of tooling.  Busybox rejects `ionice -c0 -n<something>` for example.
set -- /usr/bin/restic "$@"
if [ -n "${IONICE_CLASS}" ]; then
	set -- ionice -c "${IONICE_CLASS}" -n "${IONICE_PRIORITY:-4}" "$@"
fi

exec nice -n "${NICE:-0}"  "$@"
