#!/bin/sh

set -o nounset
set -o errexit
set -o pipefail
set -e

restic="/usr/bin/restic"

# if there are command arguments passed to the container execute them and exit.
if [ $# -ne 0 ]; then
  ${restic} $@
  exit 0
fi

# check if the repo exists and initialize it if not
${restic} cat config > /dev/null || ${restic} init

# backup data
${restic} backup ${RESTIC_DATA:-"/data"}

# apply retention policy if it is set
if [ -n "${RESTIC_FORGET:-}" ]; then 
  ${restic} forget ${RESTIC_FORGET} --prune
fi

exit 0