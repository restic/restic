#!/bin/bash

set -e

if [[ -z "$VERSION" ]]; then
    echo '$VERSION unset'
    exit 1
fi

dir=$(mktemp -d --tmpdir restic-release-XXXXXX)
echo "path is ${dir}"

for R in       \
    darwin/386     \
    darwin/amd64   \
    freebsd/386    \
    freebsd/amd64  \
    freebsd/arm    \
    linux/386      \
    linux/amd64    \
    linux/arm      \
    linux/arm64    \
    openbsd/386   \
    openbsd/amd64 \
    windows/386    \
    windows/amd64  \
    ; do \

    OS=$(dirname $R)
    ARCH=$(basename $R)
    filename=restic_${VERSION}_${OS}_${ARCH}

    if [[ "$OS" == "windows" ]]; then
        filename="${filename}.exe"
    fi

    echo $filename

    go run build.go --goos $OS --goarch $ARCH --output ${filename}
    if [[ "$OS" == "windows" ]]; then
        zip ${filename%.exe}.zip ${filename}
        rm ${filename}
        mv ${filename%.exe}.zip ${dir}
    else
        bzip2 ${filename}
        mv ${filename}.bz2 ${dir}
    fi
done

echo "packing sources"
git archive --format=tar --prefix=restic-$VERSION/ v$VERSION | gzip -n > restic-$VERSION.tar.gz
mv restic-$VERSION.tar.gz ${dir}

echo "creating checksums"
pushd ${dir}
sha256sum restic_*.{zip,bz2} restic-$VERSION.tar.gz > SHA256SUMS
gpg --armor --detach-sign SHA256SUMS
popd

echo "creating source signature file"
gpg --armor --detach-sign ${dir}/restic-$VERSION.tar.gz

echo
echo "done, path is ${dir}"
