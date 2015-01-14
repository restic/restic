set -em

# setup restic
prepare
run restic init

# start backup, break before saving files
DEBUG_BREAK=Archiver.Snapshot run restic.debug backup "${BASE}/fake-data" && debug "done"

# remove file
rm -f "${BASE}/fake-data/0/0/9/37"

# resume backup
fg

cleanup
