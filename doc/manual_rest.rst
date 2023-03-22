Manual
======

Usage help
----------

Usage help is available:

.. code-block:: console

    $ ./restic --help

    restic is a backup program which allows saving multiple revisions of files and
    directories in an encrypted repository stored on different backends.

    Usage:
      restic [command]

    Available Commands:
      backup        Create a new backup of files and/or directories
      cache         Operate on local cache directories
      cat           Print internal objects to stdout
      check         Check the repository for errors
      copy          Copy snapshots from one repository to another
      diff          Show differences between two snapshots
      dump          Print a backed-up file to stdout
      find          Find a file, a directory or restic IDs
      forget        Remove snapshots from the repository
      generate      Generate manual pages and auto-completion files (bash, fish, zsh)
      help          Help about any command
      init          Initialize a new repository
      key           Manage keys (passwords)
      list          List objects in the repository
      ls            List files in a snapshot
      migrate       Apply migrations
      mount         Mount the repository
      prune         Remove unneeded data from the repository
      rebuild-index Build a new index
      recover       Recover data from the repository not referenced by snapshots
      restore       Extract the data from a snapshot
      rewrite       Rewrite snapshots to exclude unwanted files
      self-update   Update the restic binary
      snapshots     List all snapshots
      stats         Scan the repository and show basic statistics
      tag           Modify tags on snapshots
      unlock        Remove locks other processes created
      version       Print version information

    Flags:
          --cacert file                file to load root certificates from (default: use system certificates)
          --cache-dir directory        set the cache directory. (default: use system default cache directory)
          --cleanup-cache              auto remove old cache directories
          --compression mode           compression mode (only available for repository format version 2), one of (auto|off|max) (default auto)
      -h, --help                       help for restic
          --insecure-tls               skip TLS certificate verification when connecting to the repository (insecure)
          --json                       set output mode to JSON for commands that support it
          --key-hint key               key ID of key to try decrypting first (default: $RESTIC_KEY_HINT)
          --limit-download rate        limits downloads to a maximum rate in KiB/s. (default: unlimited)
          --limit-upload rate          limits uploads to a maximum rate in KiB/s. (default: unlimited)
          --no-cache                   do not use a local cache
          --no-lock                    do not lock the repository, this allows some operations on read-only repositories
      -o, --option key=value           set extended option (key=value, can be specified multiple times)
          --pack-size size             set target pack size in MiB, created pack files may be larger (default: $RESTIC_PACK_SIZE)
          --password-command command   shell command to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)
      -p, --password-file file         file to read the repository password from (default: $RESTIC_PASSWORD_FILE)
      -q, --quiet                      do not output comprehensive progress report
      -r, --repo repository            repository to backup to or restore from (default: $RESTIC_REPOSITORY)
          --repository-file file       file to read the repository location from (default: $RESTIC_REPOSITORY_FILE)
          --tls-client-cert file       path to a file containing PEM encoded TLS client certificate and private key
      -v, --verbose                    be verbose (specify multiple times or a level using --verbose=n, max level/times is 2)

    Use "restic [command] --help" for more information about a command.

Similar to programs such as ``git``, restic has a number of
sub-commands. You can see these commands in the listing above. Each
sub-command may have own command-line options, and there is a help
option for each command which lists them, e.g. for the ``backup``
command:

.. code-block:: console

    $ ./restic backup --help

    The "backup" command creates a new snapshot and saves the files and directories
    given as the arguments.

    EXIT STATUS
    ===========

    Exit status is 0 if the command was successful.
    Exit status is 1 if there was a fatal error (no snapshot created).
    Exit status is 3 if some source data could not be read (incomplete snapshot created).

    Usage:
      restic backup [flags] [FILE/DIR] ...

    Flags:
      -n, --dry-run                                do not upload or write any data, just show what would be done
      -e, --exclude pattern                        exclude a pattern (can be specified multiple times)
          --exclude-caches                         excludes cache directories that are marked with a CACHEDIR.TAG file. See https://bford.info/cachedir/ for the Cache Directory Tagging Standard
          --exclude-file file                      read exclude patterns from a file (can be specified multiple times)
          --exclude-if-present filename[:header]   takes filename[:header], exclude contents of directories containing filename (except filename itself) if header of that file is as provided (can be specified multiple times)
          --exclude-larger-than size               max size of the files to be backed up (allowed suffixes: k/K, m/M, g/G, t/T)
          --files-from file                        read the files to backup from file (can be combined with file args; can be specified multiple times)
          --files-from-raw file                    read the files to backup from file (can be combined with file args; can be specified multiple times)
          --files-from-verbatim file               read the files to backup from file (can be combined with file args; can be specified multiple times)
      -f, --force                                  force re-reading the target files/directories (overrides the "parent" flag)
      -h, --help                                   help for backup
      -H, --host hostname                          set the hostname for the snapshot manually. To prevent an expensive rescan use the "parent" flag
          --iexclude pattern                       same as --exclude pattern but ignores the casing of filenames
          --iexclude-file file                     same as --exclude-file but ignores casing of filenames in patterns
          --ignore-ctime                           ignore ctime changes when checking for modified files
          --ignore-inode                           ignore inode number changes when checking for modified files
          --no-scan                                do not run scanner to estimate size of backup
      -x, --one-file-system                        exclude other file systems, don't cross filesystem boundaries and subvolumes
          --parent snapshot                        use this parent snapshot (default: last snapshot in the repository that has the same target files/directories, and is not newer than the snapshot time)
          --read-concurrency n                     read n file concurrently (default: $RESTIC_READ_CONCURRENCY or 2)
          --stdin                                  read backup from stdin
          --stdin-filename filename                filename to use when reading from stdin (default "stdin")
          --tag tags                               add tags for the new snapshot in the format `tag[,tag,...]` (can be specified multiple times) (default [])
          --time time                              time of the backup (ex. '2012-11-01 22:08:41') (default: now)
          --use-fs-snapshot                        use filesystem snapshot where possible (currently only Windows VSS)
          --with-atime                             store the atime for all files and directories

    Global Flags:
          --cacert file                file to load root certificates from (default: use system certificates)
          --cache-dir directory        set the cache directory. (default: use system default cache directory)
          --cleanup-cache              auto remove old cache directories
          --compression mode           compression mode (only available for repository format version 2), one of (auto|off|max) (default auto)
          --insecure-tls               skip TLS certificate verification when connecting to the repository (insecure)
          --json                       set output mode to JSON for commands that support it
          --key-hint key               key ID of key to try decrypting first (default: $RESTIC_KEY_HINT)
          --limit-download rate        limits downloads to a maximum rate in KiB/s. (default: unlimited)
          --limit-upload rate          limits uploads to a maximum rate in KiB/s. (default: unlimited)
          --no-cache                   do not use a local cache
          --no-lock                    do not lock the repository, this allows some operations on read-only repositories
      -o, --option key=value           set extended option (key=value, can be specified multiple times)
          --pack-size size             set target pack size in MiB, created pack files may be larger (default: $RESTIC_PACK_SIZE)
          --password-command command   shell command to obtain the repository password from (default: $RESTIC_PASSWORD_COMMAND)
      -p, --password-file file         file to read the repository password from (default: $RESTIC_PASSWORD_FILE)
      -q, --quiet                      do not output comprehensive progress report
      -r, --repo repository            repository to backup to or restore from (default: $RESTIC_REPOSITORY)
          --repository-file file       file to read the repository location from (default: $RESTIC_REPOSITORY_FILE)
          --tls-client-cert file       path to a file containing PEM encoded TLS client certificate and private key
      -v, --verbose                    be verbose (specify multiple times or a level using --verbose=n, max level/times is 2)

Subcommands that support showing progress information such as ``backup``,
``check`` and ``prune`` will do so unless the quiet flag ``-q`` or
``--quiet`` is set. When running from a non-interactive console progress
reporting is disabled by default to not fill your logs. For interactive
and non-interactive consoles the environment variable ``RESTIC_PROGRESS_FPS``
can be used to control the frequency of progress reporting. Use for example
``0.016666`` to only update the progress once per minute.

Additionally, on Unix systems if ``restic`` receives a SIGUSR1 signal the
current progress will be written to the standard output so you can check up
on the status at will.

Setting the `RESTIC_PROGRESS_FPS` environment variable or sending a `SIGUSR1`
signal prints a status report even when `--quiet` was specified.

Manage tags
-----------

Managing tags on snapshots is done with the ``tag`` command. The
existing set of tags can be replaced completely, tags can be added or
removed. The result is directly visible in the ``snapshots`` command.

Let's say we want to tag snapshot ``590c8fc8`` with the tags ``NL`` and
``CH`` and remove all other tags that may be present, the following
command does that:

.. code-block:: console

    $ restic -r /srv/restic-repo tag --set NL --set CH 590c8fc8
    create exclusive lock for repository
    modified tags on 1 snapshots

Note the snapshot ID has changed, so between each change we need to look up the
new ID of the snapshot. But there is an even better way - the ``tag`` command
accepts a filter using the ``--tag`` option, so we can filter snapshots based
on the tag we just added. This way we can add and remove tags incrementally:

.. code-block:: console

    $ restic -r /srv/restic-repo tag --tag NL --remove CH
    create exclusive lock for repository
    modified tags on 1 snapshots

    $ restic -r /srv/restic-repo tag --tag NL --add UK
    create exclusive lock for repository
    modified tags on 1 snapshots

    $ restic -r /srv/restic-repo tag --tag NL --remove NL
    create exclusive lock for repository
    modified tags on 1 snapshots

    $ restic -r /srv/restic-repo tag --tag NL --add SOMETHING
    no snapshots were modified

To operate on untagged snapshots only, specify the empty string ``''`` as the
filter value to ``--tag``. The following command will add the tag ``OTHER``
to all untagged snapshots:

.. code-block:: console

    $ restic -r /srv/restic-repo tag --tag '' --add OTHER

Under the hood
--------------

Browse repository objects
~~~~~~~~~~~~~~~~~~~~~~~~~

Internally, a repository stores data of several different types
described in the `design
documentation <https://github.com/restic/restic/blob/master/doc/design.rst>`__.
You can ``list`` objects such as blobs, packs, index, snapshots, keys or
locks with the following command:

.. code-block:: console

    $ restic -r /srv/restic-repo list snapshots
    d369ccc7d126594950bf74f0a348d5d98d9e99f3215082eb69bf02dc9b3e464c

The ``find`` command searches for a given
`pattern <https://pkg.go.dev/path/filepath#Match>`__ in the
repository.

.. code-block:: console

    $ restic -r backup find test.txt
    debug log file restic.log
    debug enabled
    enter password for repository:
    found 1 matching entries in snapshot 196bc5760c909a7681647949e80e5448e276521489558525680acf1bd428af36
      -rw-r--r--   501    20      5 2015-08-26 14:09:57 +0200 CEST path/to/test.txt

The ``cat`` command allows you to display the JSON representation of the
objects or their raw content.

.. code-block:: console

    $ restic -r /srv/restic-repo cat snapshot d369ccc7d126594950bf74f0a348d5d98d9e99f3215082eb69bf02dc9b3e464c
    enter password for repository:
    {
      "time": "2015-08-12T12:52:44.091448856+02:00",
      "tree": "05cec17e8d3349f402576d02576a2971fc0d9f9776ce2f441c7010849c4ff5af",
      "paths": [
        "/home/user/work"
      ],
      "hostname": "kasimir",
      "username": "username",
      "uid": 501,
      "gid": 20
    }

Metadata handling
~~~~~~~~~~~~~~~~~

Restic saves and restores most default attributes, including extended attributes like ACLs.
Information about holes in a sparse file is not stored explicitly, that is during a backup
the zero bytes in a hole are deduplicated and compressed like any other data backed up.
Instead, the restore command optionally creates holes in files by detecting and replacing
long runs of zeros, in filesystems that support sparse files.

The following metadata is handled by restic:

- Name
- Type
- Mode
- ModTime
- AccessTime
- ChangeTime
- UID
- GID
- User
- Group
- Inode
- Size
- Links
- LinkTarget
- Device
- Content
- Subtree
- ExtendedAttributes


Getting information about repository data
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Use the ``stats`` command to count up stats about the data in the repository.
There are different counting modes available using the ``--mode`` flag,
depending on what you want to calculate. The default is the restore size, or
the size required to restore the files:

-  ``restore-size`` (default) counts the size of the restored files.
-  ``files-by-contents`` counts the total size of unique files as given by their
   contents. This can be useful since a file is considered unique only if it has
   unique contents. Keep in mind that a small change to a large file (even when the
   file name/path hasn't changed) will cause them to look like different files, thus
   essentially causing the whole size of the file to be counted twice.
-  ``raw-data`` counts the size of the blobs in the repository, regardless of how many
   files reference them. This tells you how much restic has reduced all your original
   data down to (either for a single snapshot or across all your backups), and compared
   to the size given by the restore-size mode, can tell you how much deduplication is
   helping you.
-  ``blobs-per-file`` is kind of a mix between files-by-contents and raw-data modes;
   it is useful for knowing how much value your backup is providing you in terms of unique
   data stored by file. Like files-by-contents, it is resilient to file renames/moves.
   Unlike files-by-contents, it does not balloon to high values when large files have
   small edits, as long as the file path stayed the same. Unlike raw-data, this mode
   DOES consider how many files point to each blob such that the more files a blob is
   referenced by, the more it counts toward the size.

For example, to calculate how much space would be
required to restore the latest snapshot (from any host that made it):

.. code-block:: console

    $ restic stats latest
    password is correct
    Total File Count:   10538
          Total Size:   37.824 GiB

If multiple hosts are backing up to the repository, the latest snapshot may not
be the one you want. You can specify the latest snapshot from only a specific
host by using the ``--host`` flag:

.. code-block:: console

    $ restic stats --host myserver latest
    password is correct
    Total File Count:   21766
          Total Size:   481.783 GiB

There we see that it would take 482 GiB of disk space to restore the latest
snapshot from "myserver".

In case you have multiple backups running from the same host so can also use
``--tag`` and ``--path`` to be more specific about which snapshots you
are looking for.

But how much space does that snapshot take on disk? In other words, how much
has restic's deduplication helped? We can check:

.. code-block:: console

    $ restic stats --host myserver --mode raw-data latest
    password is correct
    Total Blob Count:   340847
          Total Size:   458.663 GiB

Comparing this size to the previous command, we see that restic has saved
about 23 GiB of space with deduplication.

Which mode you use depends on your exact use case. Some modes are more useful
across all snapshots, while others make more sense on just a single snapshot,
depending on what you're trying to calculate.


Scripting
---------

Restic supports the output of some commands in JSON format, the JSON
data can then be processed by other programs (e.g.
`jq <https://stedolan.github.io/jq/>`__). The following example
lists all snapshots as JSON and uses ``jq`` to pretty-print the result:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots --json | jq .
    [
      {
        "time": "2017-03-11T09:57:43.26630619+01:00",
        "tree": "bf25241679533df554fc0fd0ae6dbb9dcf1859a13f2bc9dd4543c354eff6c464",
        "paths": [
          "/home/work/doc"
        ],
        "hostname": "kasimir",
        "username": "fd0",
        "uid": 1000,
        "gid": 100,
        "id": "bbeed6d28159aa384d1ccc6fa0b540644b1b9599b162d2972acda86b1b80f89e"
      },
      {
        "time": "2017-03-11T09:58:57.541446938+01:00",
        "tree": "7f8c95d3420baaac28dc51609796ae0e0ecfb4862b609a9f38ffaf7ae2d758da",
        "paths": [
          "/home/user/shared"
        ],
        "hostname": "kasimir",
        "username": "fd0",
        "uid": 1000,
        "gid": 100,
        "id": "b157d91c16f0ba56801ece3a708dfc53791fe2a97e827090d6ed9a69a6ebdca0"
      }
    ]

.. _temporary_files:

Temporary files
---------------

During some operations (e.g. ``backup`` and ``prune``) restic uses
temporary files to store data. These files will, by default, be saved to
the system's temporary directory, on Linux this is usually located in
``/tmp/``. The environment variable ``TMPDIR`` can be used to specify a
different directory, e.g. to use the directory ``/var/tmp/restic-tmp``
instead of the default, set the environment variable like this:

.. code-block:: console

    $ export TMPDIR=/var/tmp/restic-tmp
    $ restic -r /srv/restic-repo backup ~/work



.. _caching:

Caching
-------

Restic keeps a cache with some files from the repository on the local machine.
This allows faster operations, since meta data does not need to be loaded from
a remote repository. The cache is automatically created, usually in an
OS-specific cache folder:

 * Linux/other: ``$XDG_CACHE_HOME/restic``, or ``~/.cache/restic`` if
   ``XDG_CACHE_HOME`` is not set
 * macOS: ``~/Library/Caches/restic``
 * Windows: ``%LOCALAPPDATA%/restic``

If the relevant environment variables are not set, restic exits with an error
message.

The command line parameter ``--cache-dir`` or the environment variable
``$RESTIC_CACHE_DIR`` can be used to override the default cache location.  The
parameter ``--no-cache`` disables the cache entirely. In this case, all data
is loaded from the repository.

The cache is ephemeral: When a file cannot be read from the cache, it is loaded
from the repository.

Within the cache directory, there's a sub directory for each repository the
cache was used with. Restic updates the timestamps of a repository directory each
time it is used, so by looking at the timestamps of the sub directories of the
cache directory it can decide which sub directories are old and probably not
needed any more. You can either remove these directories manually, or run a
restic command with the ``--cleanup-cache`` flag.

