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

# run restic restore "$(basename "$RESTIC_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore"
# dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore/fake-data"

# SNAPSHOT=$(run restic list snapshots)
# run restic backup "${BASE}/fake-data" $SNAPSHOT
# run restic restore "$(basename "$RESTIC_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore-incremental"
# dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore-incremental/fake-data"

# run restic fsck -o --check-data
cleanup
