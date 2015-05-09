#!/bin/bash

TARGETFILE="$1"

go list ./... | while read pkg; do
    go test -covermode=count -coverprofile=$(base64 <<< $pkg).cov $pkg
done

echo "mode: count" > $TARGETFILE
tail -q -n +2 *.cov */*.cov */*/*.cov >> $TARGETFILE
