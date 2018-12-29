***********
Local Cache
***********

In order to speed up certain operations, restic manages a local cache of data.
This document describes the data structures for the local cache with version 1.

Versions
========

The cache directory is selected according to the `XDG base dir specification
<https://standards.freedesktop.org/basedir-spec/basedir-spec-latest.html>`__.
Each repository has its own cache sub-directory, consisting of the repository ID
which is chosen at ``init``. All cache directories for different repos are
independent of each other.

The cache dir for a repo contains a file named ``version``, which contains a
single ASCII integer line that stands for the current version of the cache. If
a lower version number is found the cache is recreated with the current
version. If a higher version number is found the cache is ignored and left as
is.

Snapshots, Data and Indexes
===========================

Snapshot, Data and Index files are cached in the sub-directories ``snapshots``,
``data`` and  ``index``, as read from the repository.

Expiry
======

Whenever a cache directory for a repo is used, that directory's modification
timestamp is updated to the current time. By looking at the modification
timestamps of the repo cache directories it is easy to decide which directories
are old and haven't been used in a long time. Those are probably stale and can
be removed.
