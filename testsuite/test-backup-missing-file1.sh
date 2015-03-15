set -em

# setup restic
prepare
run restic init

# start backup, break between readdir and lstat
DEBUG_BREAK=pipe.walk1 DEBUG_BREAK_PIPE="fake-data/0/0/9" run restic.debug backup "${BASE}/fake-data" && debug "done"

# remove file
rm -f "${BASE}/fake-data/0/0/9/37"

# resume backup
fg

cleanup
