***********
Local Cache
***********

In order to speed up certain operations, restic manages a local cache of data.
The location of the cache directory depends on the operating system and the
environment; see :ref:`caching`.

Each repository has its own cache sub-directory, consisting of the repository ID
which is chosen at ``init``. All cache directories for different repositories are
independent of each other.

Snapshots, Data and Indexes
===========================

Snapshot, Data and Index files are cached in the sub-directories ``snapshots``,
``data`` and  ``index``, as read from the repository.

Expiry
======

Whenever a cache directory for a repository is used, that directory's modification
timestamp is updated to the current time. By looking at the modification
timestamps of the repository cache directories it is easy to decide which directories
are old and haven't been used in a long time. Those are probably stale and can
be removed.
