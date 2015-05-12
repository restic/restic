[![Stories in Ready](https://badge.waffle.io/restic/restic.png?label=ready&title=Ready)](https://waffle.io/restic/restic)
[![Build Status](https://travis-ci.org/restic/restic.svg?branch=master)](https://travis-ci.org/restic/restic)
[![sourcegraph status](https://sourcegraph.com/api/repos/github.com/restic/restic/.badges/status.png)](https://sourcegraph.com/github.com/restic/restic)
[![Coverage Status](https://coveralls.io/repos/restic/restic/badge.svg)](https://coveralls.io/r/restic/restic)

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

Install Go/Golang (at least version 1.3), then run `make`, afterwards you'll
find the binary in the current directory:

   $ make
   [...]

   $ ./restic --help
   Usage:
     restic [OPTIONS] <command>

   Application Options:
     -r, --repo= Repository directory to backup to/restore from

   Help Options:
     -h, --help  Show this help message

   Available commands:
     backup     save file/directory
     cache      manage cache
     cat        dump something
     find       find a file/directory
     fsck       check the repository
     init       create repository
     key        manage keys
     list       lists data
     ls         list files
     restore    restore a snapshot
     snapshots  show snapshots
     version    display version


Contribute and Documentation
============================

Contributions are welcome! More information can be found in
[`CONTRIBUTING.md`](CONTRIBUTING.md). A document describing the design of
restic and the data structures stored on disc is contained in
[`doc/Design.md`](doc/Design.md).

Contact
=======

If you discover a bug or find something surprising, please feel free to [open a
github issue](https://github.com/restic/restic/issues/new). If you would like
to chat about restic, there is also the IRC channel #restic on
irc.freenode.net. Or just write me an email :)

License
=======

Restic is licensed under "BSD 2-Clause License". You can find the complete text
in the file `LICENSE`.
