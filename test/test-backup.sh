set -e

prepare
run khepri init
run khepri backup "${BASE}/fake-data"
run khepri restore "$(basename "$KHEPRI_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore/fake-data"

SNAPSHOT=$(run khepri list snapshots)
run khepri backup "${BASE}/fake-data" $SNAPSHOT
run khepri restore "$(basename "$KHEPRI_REPOSITORY"/snapshots/*)" "${BASE}/fake-data-restore-incremental"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore-incremental/fake-data"

run khepri fsck all
cleanup
