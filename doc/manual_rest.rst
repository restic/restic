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
      diff          Show differences between two snapshots
      dump          Print a backed-up file to stdout
      find          Find a file or directory
      forget        Remove snapshots from the repository
      generate      Generate manual pages and auto-completion files (bash, zsh)
      help          Help about any command
      init          Initialize a new repository
      key           Manage keys (passwords)
      list          List objects in the repository
      ls            List files in a snapshot
      migrate       Apply migrations
      mount         Mount the repository
      prune         Remove unneeded data from the repository
      rebuild-index Build a new index file
      restore       Extract the data from a snapshot
      snapshots     List all snapshots
      tag           Modify tags on snapshots
      unlock        Remove locks other processes created
      version       Print version information

    Flags:
          --cacert file              file to load root certificates from (default: use system certificates)
          --cache-dir string         set the cache directory
          --cleanup-cache            auto remove old cache directories
      -h, --help                     help for restic
          --json                     set output mode to JSON for commands that support it
          --limit-download int       limits downloads to a maximum rate in KiB/s. (default: unlimited)
          --limit-upload int         limits uploads to a maximum rate in KiB/s. (default: unlimited)
          --no-cache                 do not use a local cache
          --no-lock                  do not lock the repo, this allows some operations on read-only repos
      -o, --option key=value         set extended option (key=value, can be specified multiple times)
      -p, --password-file string     read the repository password from a file (default: $RESTIC_PASSWORD_FILE)
      -q, --quiet                    do not output comprehensive progress report
      -r, --repo string              repository to backup to or restore from (default: $RESTIC_REPOSITORY)
          --tls-client-cert string   path to a file containing PEM encoded TLS client certificate and private key
      -v, --verbose n[=-1]           be verbose (specify --verbose multiple times or level n)

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

    Usage:
      restic backup [flags] FILE/DIR [FILE/DIR] ...

    Flags:
      -e, --exclude pattern                  exclude a pattern (can be specified multiple times)
          --exclude-caches                   excludes cache directories that are marked with a CACHEDIR.TAG file
          --exclude-file file                read exclude patterns from a file (can be specified multiple times)
          --exclude-if-present stringArray   takes filename[:header], exclude contents of directories containing filename (except filename itself) if header of that file is as provided (can be specified multiple times)
          --files-from string                read the files to backup from file (can be combined with file args)
      -f, --force                            force re-reading the target files/directories (overrides the "parent" flag)
      -h, --help                             help for backup
          --hostname hostname                set the hostname for the snapshot manually. To prevent an expensive rescan use the "parent" flag
      -x, --one-file-system                  exclude other file systems
          --parent string                    use this parent snapshot (default: last snapshot in the repo that has the same target files/directories)
          --stdin                            read backup from stdin
          --stdin-filename string            file name to use when reading from stdin (default "stdin")
          --tag tag                          add a tag for the new snapshot (can be specified multiple times)
          --time string                      time of the backup (ex. '2012-11-01 22:08:41') (default: now)
          --with-atime                       store the atime for all files and directories

    Global Flags:
          --cacert file              file to load root certificates from (default: use system certificates)
          --cache-dir string         set the cache directory
          --cleanup-cache            auto remove old cache directories
          --json                     set output mode to JSON for commands that support it
          --limit-download int       limits downloads to a maximum rate in KiB/s. (default: unlimited)
          --limit-upload int         limits uploads to a maximum rate in KiB/s. (default: unlimited)
          --no-cache                 do not use a local cache
          --no-lock                  do not lock the repo, this allows some operations on read-only repos
      -o, --option key=value         set extended option (key=value, can be specified multiple times)
      -p, --password-file string     read the repository password from a file (default: $RESTIC_PASSWORD_FILE)
      -q, --quiet                    do not output comprehensive progress report
      -r, --repo string              repository to backup to or restore from (default: $RESTIC_REPOSITORY)
          --tls-client-cert string   path to a file containing PEM encoded TLS client certificate and private key
      -v, --verbose n[=-1]           be verbose (specify --verbose multiple times or level n)

Subcommand that support showing progress information such as ``backup``,
``check`` and ``prune`` will do so unless the quiet flag ``-q`` or
``--quiet`` is set. When running from a non-interactive console progress
reporting will be limited to once every 10 seconds to not fill your
logs. Use ``backup`` with the quiet flag ``-q`` or ``--quiet`` to skip
the initial scan of the source directory, this may shorten the backup
time needed for large directories.

Additionally on Unix systems if ``restic`` receives a SIGUSR1 signal the
current progress will be written to the standard output so you can check up
on the status at will.

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

Note the snapshot ID has changed, so between each change we need to look
up the new ID of the snapshot. But there is an even better way, the
``tag`` command accepts ``--tag`` for a filter, so we can filter
snapshots based on the tag we just added.

So we can add and remove tags incrementally like this:

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

Under the hood
--------------

Browse repository objects
~~~~~~~~~~~~~~~~~~~~~~~~~

Internally, a repository stores data of several different types
described in the `design
documentation <https://github.com/restic/restic/blob/master/doc/Design.rst>`__.
You can ``list`` objects such as blobs, packs, index, snapshots, keys or
locks with the following command:

.. code-block:: console

    $ restic -r /srv/restic-repo list snapshots
    d369ccc7d126594950bf74f0a348d5d98d9e99f3215082eb69bf02dc9b3e464c

The ``find`` command searches for a given
`pattern <http://golang.org/pkg/path/filepath/#Match>`__ in the
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
Sparse files are not handled in a special way yet, and aren't restored.

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



Caching
-------

Restic keeps a cache with some files from the repository on the local machine.
This allows faster operations, since meta data does not need to be loaded from
a remote repository. The cache is automatically created, usually in an
OS-specific cache folder:

 * Linux/other: ``~/.cache/restic`` (or ``$XDG_CACHE_HOME/restic``)
 * macOS: ``~/Library/Caches/restic``
 * Windows: ``%LOCALAPPDATA%/restic``

The command line parameter ``--cache-dir`` can each be used to override the
default cache location. The parameter ``--no-cache`` disables the cache
entirely. In this case, all data is loaded from the repo.

The cache is ephemeral: When a file cannot be read from the cache, it is loaded
from the repository.

Within the cache directory, there's a sub directory for each repository the
cache was used with. Restic updates the timestamps of a repo directory each
time it is used, so by looking at the timestamps of the sub directories of the
cache directory it can decide which sub directories are old and probably not
needed any more. You can either remove these directories manually, or run a
restic command with the ``--cleanup-cache`` flag.
