set -e

prepare
run restic init
run restic backup "${BASE}/fake-data"
run restic restore "$(basename "$RESTIC_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore/fake-data"

SNAPSHOT=$(run restic list snapshots)
run restic backup "${BASE}/fake-data" $SNAPSHOT
run restic restore "$(basename "$RESTIC_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore-incremental"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore-incremental/fake-data"

run restic fsck all
cleanup
