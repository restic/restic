set -e

prepare
run restic init

# create testfile
echo "testfile" > ${BASE}/fake-data/file

# run first backup
run restic backup "${BASE}/fake-data"

# remember snapshot id
SNAPSHOT=$(run restic list snapshots)

# add data to testfile
date >> ${BASE}/fake-data/file

# run backup again
run restic backup "${BASE}/fake-data"

# add data to testfile
date >> ${BASE}/fake-data/file

# run incremental backup
run restic backup -p "$SNAPSHOT" "${BASE}/fake-data"

run restic fsck -o --check-data
cleanup
