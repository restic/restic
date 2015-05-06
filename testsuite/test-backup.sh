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

# first line contains snapshot id
# second line contains timestamp of directory creation
# so compare from line three on
echo "snapshot id is $SNAPSHOT"
restic ls "$SNAPSHOT" | tail -n +3 > "${BASE}/test-ls-output"
diff -au "${dir}/test-ls-expected" "${BASE}/test-ls-output"

run restic fsck -o --check-data
cleanup
