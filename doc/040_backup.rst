..
  Normally, there are no heading levels assigned to certain characters as the structure is
  determined from the succession of headings. However, this convention is used in Python’s
  Style Guide for documenting which you may follow:

  # with overline, for parts
  * for chapters
  = for sections
  - for subsections
  ^ for subsubsections
  " for paragraphs

##########
Backing up
##########

Now we're ready to backup some data. The contents of a directory at a
specific point in time is called a "snapshot" in restic. Run the
following command and enter the repository password you chose above
again:

.. code-block:: console

    $ restic -r /tmp/backup backup ~/work
    enter password for repository:
    scan [/home/user/work]
    scanned 764 directories, 1816 files in 0:00
    [0:29] 100.00%  54.732 MiB/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
    duration: 0:29, 54.47MiB/s
    snapshot 40dc1520 saved

As you can see, restic created a backup of the directory and was pretty
fast! The specific snapshot just created is identified by a sequence of
hexadecimal characters, ``40dc1520`` in this case.

If you run the command again, restic will create another snapshot of
your data, but this time it's even faster. This is de-duplication at
work!

.. code-block:: console

    $ restic -r /tmp/backup backup ~/work
    enter password for repository:
    using parent snapshot 40dc1520aa6a07b7b3ae561786770a01951245d2367241e71e9485f18ae8228c
    scan [/home/user/work]
    scanned 764 directories, 1816 files in 0:00
    [0:00] 100.00%  0B/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
    duration: 0:00, 6572.38MiB/s
    snapshot 79766175 saved

You can even backup individual files in the same repository.

.. code-block:: console

    $ restic -r /tmp/backup backup ~/work.txt
    scan [/home/user/work.txt]
    scanned 0 directories, 1 files in 0:00
    [0:00] 100.00%  0B/s  220B / 220B  1 / 1 items  0 errors  ETA 0:00
    duration: 0:00, 0.03MiB/s
    snapshot 31f7bd63 saved

In fact several hosts may use the same repository to backup directories
and files leading to a greater de-duplication.

Please be aware that when you backup different directories (or the
directories to be saved have a variable name component like a
time/date), restic always needs to read all files and only afterwards
can compute which parts of the files need to be saved. When you backup
the same directory again (maybe with new or changed files) restic will
find the old snapshot in the repo and by default only reads those files
that are new or have been modified since the last snapshot. This is
decided based on the modify date of the file in the file system.

Now is a good time to run ``restic check`` to verify that all data
is properly stored in the repository. You should run this command regularly
to make sure the internal structure of the repository is free of errors.

You can exclude folders and files by specifying exclude-patterns. Either
specify them with multiple ``--exclude``'s or one ``--exclude-file``

.. code-block:: console

    $ cat exclude
    # exclude go-files
    *.go
    # exclude foo/x/y/z/bar foo/x/bar foo/bar
    foo/**/bar
    $ restic -r /tmp/backup backup ~/work --exclude=*.c --exclude-file=exclude

Patterns use `filepath.Glob <https://golang.org/pkg/path/filepath/#Glob>`__ internally,
see `filepath.Match <https://golang.org/pkg/path/filepath/#Match>`__ for syntax.
Additionally ``**`` excludes arbitrary subdirectories.
Environment-variables in exclude-files are expanded with
`os.ExpandEnv <https://golang.org/pkg/os/#ExpandEnv>`__.

By specifying the option ``--one-file-system`` you can instruct restic
to only backup files from the file systems the initially specified files
or directories reside on. For example, calling restic like this won't
backup ``/sys`` or ``/dev`` on a Linux system:

.. code-block:: console

    $ restic -r /tmp/backup backup --one-file-system /

By using the ``--files-from`` option you can read the files you want to
backup from a file. This is especially useful if a lot of files have to
be backed up that are not in the same folder or are maybe pre-filtered
by other software.

or example maybe you want to backup files that have a certain filename
in them:

.. code-block:: console

    $ find /tmp/somefiles | grep 'PATTERN' > /tmp/files_to_backup

You can then use restic to backup the filtered files:

.. code-block:: console

    $ restic -r /tmp/backup backup --files-from /tmp/files_to_backup

Incidentally you can also combine ``--files-from`` with the normal files
args:

.. code-block:: console

    $ restic -r /tmp/backup backup --files-from /tmp/files_to_backup /tmp/some_additional_file

Comparing Snapshots
*******************

Restic has a `diff` command which shows the difference between two snapshots
and displays a small statistic, just pass the command two snapshot IDs:

.. code-block:: console

    $ restic -r /tmp/backup diff 5845b002 2ab627a6
    password is correct
    comparing snapshot ea657ce5 to 2ab627a6:

     C   /restic/cmd_diff.go
    +    /restic/foo
     C   /restic/restic

    Files:           0 new,     0 removed,     2 changed
    Dirs:            1 new,     0 removed
    Others:          0 new,     0 removed
    Data Blobs:     14 new,    15 removed
    Tree Blobs:      2 new,     1 removed
      Added:   16.403 MiB
      Removed: 16.402 MiB


Backing up special items and metadata
*************************************

**Symlinks** are archived as symlinks, ``restic`` does not follow them.
When you restore, you get the same symlink again, with the same link target
and the same timestamps.

If there is a **bind-mount** below a directory that is to be saved, restic descends into it.

**Device files** are saved and restored as device files. This means that e.g. ``/dev/sda`` is
archived as a block device file and restored as such. This also means that the content of the
corresponding disk is not read, at least not from the device file.

By default, restic does not save the access time (atime) for any files or other
items, since it is not possible to reliably disable updating the access time by
restic itself. This means that for each new backup a lot of metadata is
written, and the next backup needs to write new metadata again. If you really
want to save the access time for files and directories, you can pass the
``--with-atime`` option to the ``backup`` command.

Reading data from stdin
***********************

Sometimes it can be nice to directly save the output of a program, e.g.
``mysqldump`` so that the SQL can later be restored. Restic supports
this mode of operation, just supply the option ``--stdin`` to the
``backup`` command like this:

.. code-block:: console

    $ mysqldump [...] | restic -r /tmp/backup backup --stdin

This creates a new snapshot of the output of ``mysqldump``. You can then
use e.g. the fuse mounting option (see below) to mount the repository
and read the file.

By default, the file name ``stdin`` is used, a different name can be
specified with ``--stdin-filename``, e.g. like this:

.. code-block:: console

    $ mysqldump [...] | restic -r /tmp/backup backup --stdin --stdin-filename production.sql

Tags for backup
***************

Snapshots can have one or more tags, short strings which add identifying
information. Just specify the tags for a snapshot one by one with ``--tag``:

.. code-block:: console

    $ restic -r /tmp/backup backup --tag projectX --tag foo --tag bar ~/work
    [...]

The tags can later be used to keep (or forget) snapshots with the ``forget``
command. The command ``tag`` can be used to modify tags on an existing
snapshot.
