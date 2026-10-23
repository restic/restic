***********
Local Cache
***********

In order to speed up certain operations, restic manages a local cache of data.
The location of the cache directory depends on the operating system and the
environment; see :ref:`caching`.

Each repository has its own cache sub-directory, consisting of the repository ID
which is chosen at ``init``. All cache directories for different repositories are
independent of each other.

Snapshots, data and indexes
===========================

Snapshot, Data and Index files are cached in the sub-directories ``snapshots``,
``data`` and ``index``, as read from the repository.

Expiry
======

Whenever a cache directory for a repository is used, that directory's modification
timestamp is updated to the current time. By looking at the modification
timestamps of the repository cache directories it is easy to decide which directories
are old and haven't been used in a long time. Those are probably stale and can
be removed.

The ``cache`` command lists the local cache directories and can remove stale
ones:

.. code-block:: console

    $ restic cache
    Repo ID     Last Used    Old    Size
    ------------------------------------
    604bdfd4    2 days ago   no     1.5 GiB
    ------------------------------------
    1 cache dirs in /home/user/.cache/restic

Cache directories that have not been used for 30 days are considered old; the
threshold can be changed with ``--max-age`` (in days). Old cache directories
can be removed using ``restic cache --cleanup``, which honors ``--max-age``
too. Pass ``--no-size`` to skip computing the size of each cache directory.

restic also prints a warning at startup when it finds old cache directories,
suggesting ``restic cache --cleanup``. To remove old cache directories
automatically on every restic run, set the global flag ``--cleanup-cache``.
