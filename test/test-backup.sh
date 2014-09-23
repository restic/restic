set -e

prepare
run khepri init
run khepri backup "${BASE}/fake-data"
run khepri restore "$(khepri snapshots)" "${BASE}/fake-data-restore"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore/fake-data"
cleanup
