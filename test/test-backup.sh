set -e

prepare
run khepri backup "${BASE}/fake-data"
run khepri restore "$(khepri list ref)" "${BASE}/fake-data-restore"
dirdiff "${BASE}/fake-data" "${BASE}/fake-data-restore"
cleanup
