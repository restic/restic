[![Stories in Ready](https://badge.waffle.io/restic/restic.png?label=ready&title=Ready)](https://waffle.io/restic/restic)
[![wercker status](https://app.wercker.com/status/e78e51f3e5af7fff50962332615ce9a3/s/master "wercker status")](https://app.wercker.com/project/bykey/e78e51f3e5af7fff50962332615ce9a3)
[![sourcegraph status](https://sourcegraph.com/api/repos/github.com/restic/restic/.badges/status.png)](https://sourcegraph.com/github.com/restic/restic)

WARNING
=======

WARNING: At the moment, consider restic as alpha quality software, it is not
yet finished. Do not use it for real data!

Restic
======

Restic is a program that does backups right. The design goals are:

 * Easy: Doing backups should be a frictionless process, otherwise you are
   tempted to skip it.  Restic should be easy to configure and use, so that in
   the unlikely event of a data loss you can just restore it. Likewise,
   restoring data should not be complicated.

 * Fast: Backing up your data with restic should only be limited by your
   network or harddisk bandwidth so that you can backup your files every day.
   Nobody does backups if it takes too much time. Restoring backups should only
   transfer data that is needed for the files that are to be restored, so that
   this process is also fast.

 * Verifiable: Much more important than backup is restore, so restic enables
   you to easily verify that all data can be restored.

 * Secure: Restic uses cryptography to guarantee confidentiality and integrity
   of your data. The location the backup data is stored is assumed not to be a
   trusted environment (e.g. a shared space where others like system
   administrators are able to access your backups). Restic is built to secure
   your data against such attackers.

 * Efficient: With the growth of data, additional snapshots should only take
   the storage of the actual increment. Even more, duplicate data should be
   de-duplicated before it is actually written to the storage backend to save
   precious backup space.


Building
========

Install Go (at least 1.3), then run:

    export GOPATH=~/src/go
    go get github.com/restic/restic/cmd/restic
    $GOPATH/bin/restic --help


Contribute
==========

Contributions are welcome! Please make sure that all code submitted in
pull-requests is properly formatted with `gofmt`. Installing the script
`fmt-check` from https://github.com/edsrzf/gofmt-git-hook locally as a
pre-commit hook checks formatting before commiting, just copy this script to
`.git/hooks/pre-commit`.

If you are unsure what to do, please have a look at the github issues,
especially those tagged
[minor complexity](https://github.com/restic/restic/labels/minor%20complexity).

License
=======

Restic is licensed under "BSD 2-Clause License". You can find the complete text
in the file `LICENSE`.
