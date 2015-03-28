set -em

# setup restic
prepare
run restic init

# start backup with non existing dir
run timeout 10s restic.debug backup "${BASE}/fake-data/0/0/"{0,1,foobar,5} && debug "done" || false

cleanup
