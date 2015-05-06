set -e

prepare
run restic init
run restic backup "${BASE}/fake-data"
run restic restore "$(basename "$RESTIC_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore/fake-data"

SNAPSHOT=$(restic list snapshots)
run restic backup -p "$SNAPSHOT" "${BASE}/fake-data"
run restic restore "$(basename "$RESTIC_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore-incremental"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore-incremental/fake-data"

echo "snapshot id is $SNAPSHOT"
restic ls "$SNAPSHOT" fake-data/0/0/1 | head -n 10

run restic fsck -o --check-data
cleanup
