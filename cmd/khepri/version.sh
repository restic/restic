#!/bin/sh

VERSION=$(git log --max-count=1 --pretty='%ad-%h' --date=short HEAD 2>/dev/null)

if [ -n "$VERSION" ]; then
    if ! sh -c "git diff -s --exit-code && git diff --cached -s --exit-code"; then
        VERSION+="+"
    fi
else
    VERSION="unknown version"
fi

echo $VERSION
