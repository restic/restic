set -e

prepare
run restic init

# first backup without dedup
run restic backup "${BASE}/fake-data"
size=$(du -sm "$RESTIC_REPOSITORY" | cut -f1)
debug "size before: $size"

# second backup with dedup
run restic backup "${BASE}/fake-data"
size2=$(du -sm "$RESTIC_REPOSITORY" | cut -f1)
debug "size after: $size2"

# check if the repository hasn't grown more than 5%
threshhold=$(($size+$size/20))
debug "threshhold is $threshhold"
if [[ "$size2" -gt "$threshhold" ]]; then
    fail "dedup failed, repo grown more than 5%, before ${size}MiB after ${size2}MiB threshhold ${threshhold}MiB"
fi

cleanup
