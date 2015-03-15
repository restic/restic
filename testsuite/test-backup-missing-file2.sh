set -em

# setup restic
prepare
run restic init

# start backup, break between walk and save
DEBUG_BREAK=pipe.walk2 DEBUG_BREAK_PIPE="fake-data/0/0/9/37" run restic.debug backup "${BASE}/fake-data" && debug "done"

# remove file
rm -f "${BASE}/fake-data/0/0/9/37"

# resume backup
fg

cleanup
