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

    $ restic -r /srv/restic-repo --verbose backup ~/work
    open repository
    enter password for repository:
    repository a14e5863 opened (version 2, compression level auto)
    load index files
    start scan on [/home/user/work]
    start backup on [/home/user/work]
    scan finished in 1.837s: 5307 files, 1.720 GiB
    
    Files:        5307 new,     0 changed,     0 unmodified
    Dirs:         1867 new,     0 changed,     0 unmodified
    Added to the repository: 1.200 GiB (1.103 GiB stored)
    
    processed 5307 files, 1.720 GiB in 0:12
    snapshot 40dc1520 saved

As you can see, restic created a backup of the directory and was pretty
fast! The specific snapshot just created is identified by a sequence of
hexadecimal characters, ``40dc1520`` in this case.

You can see that restic tells us it processed 1.720 GiB of data, this is the
size of the files and directories in ``~/work`` on the local file system. It
also tells us that only 1.200 GiB was added to the repository. This means that
some of the data was duplicate and restic was able to efficiently reduce it.
The data compression also managed to compress the data down to 1.103 GiB.

If you don't pass the ``--verbose`` option, restic will print less data. You'll
still get a nice live status display. Be aware that the live status shows the
processed files and not the transferred data. Transferred volume might be lower
(due to de-duplication) or higher.

On Windows, the ``--use-fs-snapshot`` option will use Windows' Volume Shadow Copy
Service (VSS) when creating backups. Restic will transparently create a VSS
snapshot for each volume that contains files to backup. Files are read from the
VSS snapshot instead of the regular filesystem. This allows to backup files that are
exclusively locked by another process during the backup.

You can use the following extended options to change the VSS behavior:

 * ``-o vss.timeout`` specifies timeout for VSS snapshot creation, default value being 120 seconds
 * ``-o vss.exclude-all-mount-points`` disable auto snapshotting of all volume mount points
 * ``-o vss.exclude-volumes`` allows excluding specific volumes or volume mount points from snapshotting
 * ``-o vss.provider`` specifies VSS provider used for snapshotting

For example a 2.5 minutes timeout with snapshotting of mount points disabled can be specified as:

.. code-block:: console

    -o vss.timeout=2m30s -o vss.exclude-all-mount-points=true

and excluding drive ``d:\``, mount point ``c:\mnt`` and volume ``\\?\Volume{04ce0545-3391-11e0-ba2f-806e6f6e6963}\`` as:

.. code-block:: console

    -o vss.exclude-volumes="d:;c:\mnt\;\\?\volume{04ce0545-3391-11e0-ba2f-806e6f6e6963}"

VSS provider can be specified by GUID:

.. code-block:: console

    -o vss.provider={3f900f90-00e9-440e-873a-96ca5eb079e5}

or by name:

.. code-block:: console

    -o vss.provider="Hyper-V IC Software Shadow Copy Provider"

Also, ``MS`` can be used as alias for ``Microsoft Software Shadow Copy provider 1.0``.

By default VSS ignores Outlook OST files. This is not a restriction of restic
but the default Windows VSS configuration. The files not to snapshot are
configured in the Windows registry under the following key:

.. code-block:: console

    HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Control\BackupRestore\FilesNotToSnapshot

For more details refer the official Windows documentation e.g. the article
``Registry Keys and Values for Backup and Restore``.

If you run the backup command again, restic will create another snapshot of
your data, but this time it's even faster and no new data was added to the
repository (since all data is already there). This is de-duplication at work!

.. code-block:: console

    $ restic -r /srv/restic-repo --verbose backup ~/work
    open repository
    enter password for repository:
    repository a14e5863 opened (version 2, compression level auto)
    load index files
    using parent snapshot 40dc1520
    start scan on [/home/user/work]
    start backup on [/home/user/work]
    scan finished in 1.881s: 5307 files, 1.720 GiB
    
    Files:           0 new,     0 changed,  5307 unmodified
    Dirs:            0 new,     0 changed,  1867 unmodified
    Added to the repository: 0 B   (0 B   stored)

    processed 5307 files, 1.720 GiB in 0:03
    snapshot 79766175 saved

You can even backup individual files in the same repository (not passing
``--verbose`` means less output):

.. code-block:: console

    $ restic -r /srv/restic-repo backup ~/work.txt
    enter password for repository:
    snapshot 249d0210 saved

If you're interested in what restic does, pass ``--verbose`` twice (or
``--verbose=2``) to display detailed information about each file and directory
restic encounters:

.. code-block:: console

    $ echo 'more data foo bar' >> ~/work.txt

    $ restic -r /srv/restic-repo --verbose --verbose backup ~/work.txt
    open repository
    enter password for repository:
    lock repository
    load index files
    using parent snapshot f3f8d56b
    start scan
    start backup
    scan finished in 2.115s
    modified  /home/user/work.txt, saved in 0.007s (22 B added)
    modified  /home/user/, saved in 0.008s (0 B added, 378 B metadata)
    modified  /home/, saved in 0.009s (0 B added, 375 B metadata)
    processed 22 B in 0:02
    Files:           0 new,     1 changed,     0 unmodified
    Dirs:            0 new,     2 changed,     0 unmodified
    Data Blobs:      1 new
    Tree Blobs:      3 new
    Added:      1.116 KiB
    snapshot 8dc503fc saved

In fact several hosts may use the same repository to backup directories
and files leading to a greater de-duplication.

Now is a good time to run ``restic check`` to verify that all data
is properly stored in the repository. You should run this command regularly
to make sure the internal structure of the repository is free of errors.

File change detection
*********************

When restic encounters a file that has already been backed up, whether in the
current backup or a previous one, it makes sure the file's content is only
stored once in the repository. To do so, it normally has to scan the entire
content of the file. Because this can be very expensive, restic also uses a
change detection rule based on file metadata to determine whether a file is
likely unchanged since a previous backup. If it is, the file is not scanned
again.

The previous backup snapshot, called "parent" snapshot in restic terminology,
is determined as follows. By default restic groups snapshots by hostname and
backup paths, and then selects the latest snapshot in the group that matches
the current backup. You can change the selection criteria using the
``--group-by`` option, which defaults to ``host,paths``. To select the latest
snapshot with the same paths independent of the hostname, use ``paths``. Or,
to only consider the hostname and tags, use ``host,tags``. Alternatively, it
is possible to manually specify a specific parent snapshot using the
``--parent`` option. Finally, note that one would normally set the
``--group-by`` option for the ``forget`` command to the same value.

Change detection is only performed for regular files (not special files,
symlinks or directories) that have the exact same path as they did in a
previous backup of the same location.  If a file or one of its containing
directories was renamed, it is considered a different file and its entire
contents will be scanned again.

Metadata changes (permissions, ownership, etc.) are always included in the
backup, even if file contents are considered unchanged.

On **Unix** (including Linux and Mac), given that a file lives at the same
location as a file in a previous backup, the following file metadata
attributes have to match for its contents to be presumed unchanged:

* Modification timestamp (mtime).
* Metadata change timestamp (ctime).
* File size.
* Inode number (internal number used to reference a file in a filesystem).

The reason for requiring both mtime and ctime to match is that Unix programs
can freely change mtime (and some do). In such cases, a ctime change may be
the only hint that a file did change.

The following ``restic backup`` command line flags modify the change detection
rules:

* ``--force``: turn off change detection and rescan all files.
* ``--ignore-ctime``: require mtime to match, but allow ctime to differ.
* ``--ignore-inode``: require mtime to match, but allow inode number
   and ctime to differ.

The option ``--ignore-inode`` exists to support FUSE-based filesystems and
pCloud, which do not assign stable inodes to files.

Note that the device id of the containing mount point is never taken into
account. Device numbers are not stable for removable devices and ZFS snapshots.
If you want to force a re-scan in such a case, you can change the mountpoint.

On **Windows**, a file is considered unchanged when its path, size
and modification time match, and only ``--force`` has any effect.
The other options are recognized but ignored.

Skip creating snapshots if unchanged
************************************

By default, restic always creates a new snapshot even if nothing has changed
compared to the parent snapshot. To omit the creation of a new snapshot in this
case, specify the ``--skip-if-unchanged`` option.

Note that when using absolute paths to specify the backup source, then also
changes to the parent folders result in a changed snapshot. For example, a backup
of ``/home/user/work`` will create a new snapshot if the metadata of either
``/``, ``/home`` or ``/home/user`` change. To avoid this problem run restic from
the corresponding folder and use relative paths.

.. code-block:: console

    $ cd /home/user/work && restic -r /srv/restic-repo backup . --skip-if-unchanged

    open repository
    enter password for repository:
    repository a14e5863 opened (version 2, compression level auto)
    load index files
    using parent snapshot 40dc1520
    start scan on [.]
    start backup on [.]
    scan finished in 1.814s: 5307 files, 1.720 GiB
    
    Files:           0 new,     0 changed,  5307 unmodified
    Dirs:            0 new,     0 changed,  1867 unmodified
    Added to the repository: 0 B   (0 B   stored)

    processed 5307 files, 1.720 GiB in 0:03
    skipped creating snapshot


Dry Runs
********

You can perform a backup in dry run mode to see what would happen without
modifying the repository.

-  ``--dry-run``/``-n`` Report what would be done, without writing to the repository

Combined with ``--verbose``, you can see a list of changes:

.. code-block:: console

    $ restic -r /srv/restic-repo backup ~/work --dry-run -vv | grep "added"
    modified  /plan.txt, saved in 0.000s (9.110 KiB added)
    modified  /archive.tar.gz, saved in 0.140s (25.542 MiB added)
    Would be added to the repository: 25.551 MiB

.. _backup-excluding-files:

Excluding Files
***************

You can exclude folders and files by specifying exclude patterns, currently
the exclude options are:

-  ``--exclude`` Specified one or more times to exclude one or more items
-  ``--iexclude`` Same as ``--exclude`` but ignores the case of paths
-  ``--exclude-caches`` Specified once to exclude a folder's content if it contains `the special CACHEDIR.TAG file <https://bford.info/cachedir/>`__, but keep ``CACHEDIR.TAG``.
-  ``--exclude-file`` Specified one or more times to exclude items listed in a given file
-  ``--iexclude-file`` Same as ``exclude-file`` but ignores cases like in ``--iexclude``
-  ``--exclude-if-present foo`` Specified one or more times to exclude a folder's content if it contains a file called ``foo`` (optionally having a given header, no wildcards for the file name supported)
-  ``--exclude-larger-than size`` Specified once to exclude files larger than the given size
-  ``--exclude-cloud-files`` Specified once to exclude online-only cloud files (such as OneDrive Files On-Demand), currently only supported on Windows

Please see ``restic help backup`` for more specific information about each exclude option.

Let's say we have a file called ``excludes.txt`` with the following content:

::

    # exclude go-files
    *.go
    # exclude foo/x/y/z/bar foo/x/bar foo/bar
    foo/**/bar

It can be used like this:

.. code-block:: console

    $ restic -r /srv/restic-repo backup ~/work --exclude="*.c" --exclude-file=excludes.txt

This instructs restic to exclude files matching the following criteria:

* All files matching ``*.c`` (parameter ``--exclude``)
* All files matching ``*.go`` (second line in ``excludes.txt``)
* All files and sub-directories named ``bar`` which reside somewhere below a directory called ``foo`` (fourth line in ``excludes.txt``)

Patterns use the syntax of the Go function
`filepath.Match <https://pkg.go.dev/path/filepath#Match>`__
and are tested against the full path of a file/dir to be saved,
even if restic is passed a relative path to save. Empty lines and lines
starting with a ``#`` are ignored.

Environment variables in exclude files are expanded with `os.ExpandEnv
<https://pkg.go.dev/os#ExpandEnv>`__, so ``/home/$USER/foo`` will be
expanded to ``/home/bob/foo`` for the user ``bob``. To get a literal dollar
sign, write ``$$`` to the file - this has to be done even when there's no
matching environment variable for the word following a single ``$``. Note
that tilde (``~``) is not expanded, instead use the ``$HOME`` or equivalent
environment variable (depending on your operating system).

Patterns need to match on complete path components. For example, the pattern ``foo``:

* matches ``/dir1/foo/dir2/file`` and ``/dir/foo``
* does not match ``/dir/foobar`` or ``barfoo``

A trailing ``/`` is ignored, a leading ``/`` anchors the pattern at the root directory.
This means, ``/bin`` matches ``/bin/bash`` but does not match ``/usr/bin/restic``.

Regular wildcards cannot be used to match over the directory separator ``/``,
e.g. ``b*ash`` matches ``/bin/bash`` but does not match ``/bin/ash``. To match
across an arbitrary number of subdirectories, use the special ``**`` wildcard.
The ``**`` must be positioned between path separators. The pattern 
``foo/**/bar`` matches:

* ``/dir1/foo/dir2/bar/file``
* ``/foo/bar/file``
* ``/tmp/foo/bar``

Spaces in patterns listed in an exclude file can be specified verbatim. That is,
in order to exclude a file named ``foo bar star.txt``, put that just as it reads
on one line in the exclude file. Please note that beginning and trailing spaces
are trimmed - in order to match these, use e.g. a ``*`` at the beginning or end
of the filename.

Spaces in patterns listed in the other exclude options (e.g. ``--exclude`` on the
command line) are specified in different ways depending on the operating system
and/or shell. Restic itself does not need any escaping, but your shell may need
some escaping in order to pass the name/pattern as a single argument to restic.

On most Unixy shells, you can either quote or use backslashes. For example:

* ``--exclude='foo bar star/foo.txt'``
* ``--exclude="foo bar star/foo.txt"``
* ``--exclude=foo\ bar\ star/foo.txt``

If a pattern starts with exclamation mark and matches a file that
was previously matched by a regular pattern, the match is cancelled.
It works similarly to ``gitignore``, with the same limitation: once a
directory is excluded, it is not possible to include files inside the
directory. Here is a complete example to backup a selection of
directories inside the home directory. It works by excluding any
directory, then selectively add back some of them.

::

    $HOME/*
    !$HOME/Documents
    !$HOME/code
    !$HOME/.emacs.d
    !$HOME/games
    # [...]
    node_modules
    *~
    *.o
    *.lo
    *.pyc

By specifying the option ``--one-file-system`` you can instruct restic
to only backup files from the file systems the initially specified files
or directories reside on. In other words, it will prevent restic from crossing
filesystem boundaries and subvolumes when performing a backup.

For example, if you backup ``/`` with this option and you have external
media mounted under ``/media/usb`` then restic will not back up ``/media/usb``
at all because this is a different filesystem than ``/``. Virtual filesystems
such as ``/proc`` are also considered different and thereby excluded when
using ``--one-file-system``:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --one-file-system /

Please note that this does not prevent you from specifying multiple filesystems
on the command line, e.g:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --one-file-system / /media/usb

will back up both the ``/`` and ``/media/usb`` filesystems, but will not
include other filesystems like ``/sys`` and ``/proc``.

.. note:: ``--one-file-system`` is currently unsupported on Windows, and will
    cause the backup to immediately fail with an error.

Files larger than a given size can be excluded using the `--exclude-larger-than`
option:

.. code-block:: console

    $ restic -r /srv/restic-repo backup ~/work --exclude-larger-than 1M

This excludes files in ``~/work`` which are larger than 1 MiB from the backup.

The default unit for the size value is bytes, so e.g. ``--exclude-larger-than 2048``
would exclude files larger than 2048 bytes (2 KiB). To specify other units,
suffix the size value with one of ``k``/``K`` for KiB (1024 bytes), ``m``/``M`` for MiB (1024^2 bytes),
``g``/``G`` for GiB (1024^3 bytes) and ``t``/``T`` for TiB (1024^4 bytes), e.g. ``1k``, ``10K``, ``20m``,
``20M``,  ``30g``, ``30G``, ``2t`` or ``2T``).

Including Files
***************

The options ``--files-from``, ``--files-from-verbatim`` and ``--files-from-raw``
allow you to give restic a file containing lists of file patterns or paths to
be backed up. This is useful e.g. when you want to back up files from many
different locations, or when you use some other software to generate the list
of files to back up.

The argument passed to ``--files-from`` must be the name of a text file that
contains one *pattern* per line. The file must be encoded as UTF-8, or UTF-16
with a byte-order mark. Leading and trailing whitespace is removed from the
patterns. Empty lines and lines starting with a ``#`` are ignored and each
pattern is expanded when read, such that special characters in it are expanded
according to the syntax described in the documentation of the Go function
`filepath.Match <https://pkg.go.dev/path/filepath#Match>`__.

The argument passed to ``--files-from-verbatim`` must be the name of a text file
that contains one *path* per line, e.g. as generated by GNU ``find`` with the
``-print`` flag. Unlike ``--files-from``, ``--files-from-verbatim`` does not
expand any special characters in the list of paths, does not strip off any
whitespace and does not ignore lines starting with a ``#``. This option simply
reads and uses each line as-is, although empty lines are still ignored. Use this
option when you want to backup a list of filenames containing the special
characters that would otherwise be expanded when using ``--files-from``.

The ``--files-from-raw`` option is a variant of ``--files-from-verbatim`` that
requires each line in the file to be terminated by an ASCII NUL character (the
``\0`` zero byte) instead of a newline, so that it can even handle file paths
containing newlines in their name or are not encoded as UTF-8 (except on
Windows, where the listed filenames must still be encoded in UTF-8. This option
is the safest choice when generating the list of filenames from a script (e.g.
GNU ``find`` with the ``-print0`` flag).

All three options interpret the argument ``-`` as standard input and will read
the list of files/patterns from there instead of a text file.

In all cases, paths may be absolute or relative to ``restic backup``'s working
directory.

For example, maybe you want to backup files which have a name that matches a
certain regular expression pattern (uses GNU ``find``):

.. code-block:: console

    $ find /tmp/some_folder -regex PATTERN -print0 > /tmp/files_to_backup

You can then use restic to backup the filtered files:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --files-from-raw /tmp/files_to_backup

You can combine all three options with each other and with the normal file arguments:

.. code-block:: console

    $ restic backup --files-from /tmp/files_to_backup /tmp/some_additional_file
    $ restic backup --files-from /tmp/glob-pattern --files-from-raw /tmp/generated-list /tmp/some_additional_file

Comparing Snapshots
*******************

Restic has a ``diff`` command which shows the difference between two snapshots
and displays a small statistic, just pass the command two snapshot IDs:

.. code-block:: console

    $ restic -r /srv/restic-repo diff 5845b002 2ab627a6
    comparing snapshot ea657ce5 to 2ab627a6:

    M    /restic/cmd_diff.go
    +    /restic/foo
    M    /restic/restic

    Files:           0 new,     0 removed,     2 changed
    Dirs:            1 new,     0 removed
    Others:          0 new,     0 removed
    Data Blobs:     14 new,    15 removed
    Tree Blobs:      2 new,     1 removed
      Added:   16.403 MiB
      Removed: 16.402 MiB

To only compare files in specific subfolders, you can use the ``<snapshot>:<subfolder>``
syntax, where ``snapshot`` is the ID of a snapshot (or the string ``latest``) and ``subfolder``
is a path within the snapshot. For example, to only compare files in the ``/restic``
folder, you could use the following command:

.. code-block:: console

    $ restic -r /srv/restic-repo diff 5845b002:/restic 2ab627a6:/restic

By default, the ``diff`` command only lists differences in file contents.
The flag ``--metadata`` shows changes to file metadata, too.

The characters left of the file path show what has changed for this file:

+-------+-----------------------+
| ``+`` | added                 |
+-------+-----------------------+
| ``-`` | removed               |
+-------+-----------------------+
| ``T`` | entry type changed    |
+-------+-----------------------+
| ``M`` | file content changed  |
+-------+-----------------------+
| ``U`` | metadata changed      |
+-------+-----------------------+
| ``?`` | bitrot detected       |
+-------+-----------------------+

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

Backing up full security descriptors on Windows is only possible when the user
has ``SeBackupPrivilege`` privilege or is running as admin. This is a restriction
of Windows not restic.
If either of these conditions are not met, only the owner, group and DACL will
be backed up.

Note that ``restic`` does not back up some metadata associated with files. Of
particular note are:

* File creation date on Unix platforms
* Inode flags on Unix platforms

Reading data from a command
***************************

Sometimes, it can be useful to directly save the output of a program, for example,
``mysqldump`` so that the SQL can later be restored. Restic supports this mode
of operation; just supply the option ``--stdin-from-command`` when using the
``backup`` action, and write the command in place of the files/directories. To prevent
restic from interpreting the arguments for the command, make sure to add ``--`` before
the command starts:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --stdin-from-command -- mysqldump --host example mydb [...]

This command creates a new snapshot based on the standard output of ``mysqldump``.
By default, the command's standard output is saved in a file named ``stdin``.
A different name can be specified with ``--stdin-filename``:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --stdin-filename production.sql --stdin-from-command -- mysqldump --host example mydb [...]

Restic uses the command exit code to determine whether the command succeeded. A
non-zero exit code from the command causes restic to cancel the backup. This causes
restic to fail with exit code 1. No snapshot will be created in this case.

Reading data from stdin
***********************

.. warning::

    Restic cannot detect if data read from stdin is complete or not. As explained
    below, this can cause incomplete backup unless additional checks (outside of
    restic) are configured. If possible, use ``--stdin-from-command`` instead.

Alternatively, restic supports reading arbitrary data directly from the standard
input. Use the option ``--stdin`` of the ``backup`` command as  follows:

.. code-block:: console

    # Will not notice failures, see the warning below
    $ gzip bigfile.dat | restic -r /srv/restic-repo backup --stdin

This creates a new snapshot of the content of ``bigfile.dat``.
As for ``--stdin-from-command``, the default file name is ``stdin``; a
different name can be specified with ``--stdin-filename``.

**Important**: while it is possible to pipe a command output to restic using
``--stdin``, doing so is discouraged as it will mask errors from the
command, leading to corrupted backups. For example, in the following code
block, if ``mysqldump`` fails to connect to the MySQL database, the restic
backup will nevertheless succeed in creating an _empty_ backup:

.. code-block:: console

    # Will not notice failures, read the warning above
    $ mysqldump [...] | restic -r /srv/restic-repo backup --stdin

A simple solution is to use ``--stdin-from-command`` (see above). If you
still need to use the ``--stdin`` flag, you must use the shell option ``set -o pipefail``
(so that a non-zero exit code from one of the programs in the pipe makes the
whole chain return a non-zero exit code) and you must check the exit code of
the pipe and act accordingly (e.g., remove the last backup). Refer to the
`Use the Unofficial Bash Strict Mode <http://redsymbol.net/articles/unofficial-bash-strict-mode/>`__
for more details on this.

Description of snapshots
************************

It is possible to add arbitrary text to a snapshot using ``--description``
or ``--description-from-file``. This description can be abritrary text.

.. code-block:: console

    $ restic -r /srv/restic-repo backup --description "finished important article" ~/work
    [...]

The first line of the description for each snapshot can be viewed with
the ``snapshot`` command. To view the complete multi-line description of
a snapshot the ``description`` command can be used.

Tags for backup
***************

Snapshots can have one or more tags, short strings which add identifying
information. Just specify the tags for a snapshot one by one with ``--tag``:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --tag projectX --tag foo --tag bar ~/work
    [...]

The tags can later be used to keep (or forget) snapshots with the ``forget``
command. The command ``tag`` can be used to modify tags on an existing
snapshot.

Scheduling backups
******************

Restic does not have a built-in way of scheduling backups, as it's a tool
that runs when executed rather than a daemon. There are plenty of different
ways to schedule backup runs on various different platforms, e.g. systemd
and cron on Linux/BSD and Task Scheduler in Windows, depending on one's
needs and requirements. If you don't want to implement your own scheduling,
you can use `resticprofile <https://github.com/creativeprojects/resticprofile/#resticprofile>`__.

When scheduling restic to run recurringly, please make sure to detect already
running instances before starting the backup.

Space requirements
******************

Restic currently assumes that your backup repository has sufficient space
for the backup operation you are about to perform. This is a realistic
assumption for many cloud providers, but may not be true when backing up
to local disks.

Should you run out of space during the middle of a backup, there will be
some additional data in the repository, but the snapshot will never be
created as it would only be written at the very (successful) end of
the backup operation.  Previous snapshots will still be there and will still
work.

Exit status codes
*****************

Restic returns an exit status code after the backup command is run:

* 0 when the backup was successful (snapshot with all source files created)
* 1 when there was a fatal error (no snapshot created)
* 3 when some source files could not be read (incomplete snapshot with remaining files created)
* further exit codes are documented in :ref:`exit-codes`.

Fatal errors occur for example when restic is unable to write to the backup destination, when
there are network connectivity issues preventing successful communication, or when an invalid
password or command line argument is provided. When restic returns this exit status code, one
should not expect a snapshot to have been created.

Source file read errors occur when restic fails to read one or more files or directories that
it was asked to back up, e.g. due to permission problems. Restic displays the number of source
file read errors that occurred while running the backup. If there are errors of this type,
restic will still try to complete the backup run with all the other files, and create a
snapshot that then contains all but the unreadable files.

For use of these exit status codes in scripts and other automation tools, see :ref:`exit-codes`.
To manually inspect the exit code in e.g. Linux, run ``echo $?``.

Environment Variables
*********************

In addition to command-line options, restic supports passing various options in
environment variables. The following lists these environment variables:

.. code-block:: console

    RESTIC_REPOSITORY_FILE              Name of file containing the repository location (replaces --repository-file)
    RESTIC_REPOSITORY                   Location of repository (replaces -r)
    RESTIC_PASSWORD_FILE                Location of password file (replaces --password-file)
    RESTIC_PASSWORD                     The actual password for the repository
    RESTIC_PASSWORD_COMMAND             Command printing the password for the repository to stdout
    RESTIC_KEY_HINT                     ID of key to try decrypting first, before other keys
    RESTIC_CACERT                       Location(s) of certificate file(s), comma separated if multiple (replaces --cacert)
    RESTIC_TLS_CLIENT_CERT              Location of TLS client certificate and private key (replaces --tls-client-cert)
    RESTIC_CACHE_DIR                    Location of the cache directory
    RESTIC_COMPRESSION                  Compression mode (only available for repository format version 2)
    RESTIC_HOST                         Only consider snapshots for this host / Set the hostname for the snapshot manually (replaces --host)
    RESTIC_PROGRESS_FPS                 Frames per second by which the progress bar is updated
    RESTIC_PACK_SIZE                    Target size for pack files
    RESTIC_READ_CONCURRENCY             Concurrency for file reads

    TMPDIR                              Location for temporary files (except Windows)
    TMP                                 Location for temporary files (only Windows)

    AWS_ACCESS_KEY_ID                   Amazon S3 access key ID
    AWS_SECRET_ACCESS_KEY               Amazon S3 secret access key
    AWS_SESSION_TOKEN                   Amazon S3 temporary session token
    AWS_DEFAULT_REGION                  Amazon S3 default region
    AWS_PROFILE                         Amazon credentials profile (alternative to specifying key and region)
    AWS_SHARED_CREDENTIALS_FILE         Location of the AWS CLI shared credentials file (default: ~/.aws/credentials)
    RESTIC_AWS_ASSUME_ROLE_ARN          Amazon IAM Role ARN to assume using discovered credentials
    RESTIC_AWS_ASSUME_ROLE_SESSION_NAME Session Name to use with the role assumption
    RESTIC_AWS_ASSUME_ROLE_EXTERNAL_ID  External ID to use with the role assumption
    RESTIC_AWS_ASSUME_ROLE_POLICY       Inline Amazion IAM session policy
    RESTIC_AWS_ASSUME_ROLE_REGION       Region to use for IAM calls for the role assumption (default: us-east-1)
    RESTIC_AWS_ASSUME_ROLE_STS_ENDPOINT URL to the STS endpoint (default is determined based on RESTIC_AWS_ASSUME_ROLE_REGION). You generally do not need to set this, advanced use only.

    AZURE_ACCOUNT_NAME                  Account name for Azure
    AZURE_ACCOUNT_KEY                   Account key for Azure
    AZURE_ACCOUNT_SAS                   Shared access signatures (SAS) for Azure
    AZURE_ENDPOINT_SUFFIX               Endpoint suffix for Azure Storage (default: core.windows.net)
    AZURE_FORCE_CLI_CREDENTIAL          Force the use of Azure CLI credentials for authentication

    B2_ACCOUNT_ID                       Account ID or applicationKeyId for Backblaze B2
    B2_ACCOUNT_KEY                      Account Key or applicationKey for Backblaze B2

    GOOGLE_PROJECT_ID                   Project ID for Google Cloud Storage
    GOOGLE_APPLICATION_CREDENTIALS      Application Credentials for Google Cloud Storage (e.g. $HOME/.config/gs-secret-restic-key.json)

    OS_AUTH_URL                         Auth URL for keystone authentication
    OS_REGION_NAME                      Region name for keystone authentication
    OS_USERNAME                         Username for keystone authentication
    OS_USER_ID                          User ID for keystone v3 authentication
    OS_PASSWORD                         Password for keystone authentication
    OS_TENANT_ID                        Tenant ID for keystone v2 authentication
    OS_TENANT_NAME                      Tenant name for keystone v2 authentication

    OS_USER_DOMAIN_NAME                 User domain name for keystone authentication
    OS_USER_DOMAIN_ID                   User domain ID for keystone v3 authentication
    OS_PROJECT_NAME                     Project name for keystone authentication
    OS_PROJECT_DOMAIN_NAME              Project domain name for keystone authentication
    OS_PROJECT_DOMAIN_ID                Project domain ID for keystone v3 authentication
    OS_TRUST_ID                         Trust ID for keystone v3 authentication

    OS_APPLICATION_CREDENTIAL_ID        Application Credential ID (keystone v3)
    OS_APPLICATION_CREDENTIAL_NAME      Application Credential Name (keystone v3)
    OS_APPLICATION_CREDENTIAL_SECRET    Application Credential Secret (keystone v3)

    OS_STORAGE_URL                      Storage URL for token authentication
    OS_AUTH_TOKEN                       Auth token for token authentication

    RCLONE_BWLIMIT                      rclone bandwidth limit

    RESTIC_REST_USERNAME                Restic REST Server username
    RESTIC_REST_PASSWORD                Restic REST Server password

    ST_AUTH                             Auth URL for keystone v1 authentication
    ST_USER                             Username for keystone v1 authentication
    ST_KEY                              Password for keystone v1 authentication

See :ref:`caching` for the rules concerning cache locations when
``RESTIC_CACHE_DIR`` is not set.

The external programs that restic may execute include ``rclone`` (for rclone
backends) and ``ssh`` (for the SFTP backend). These may respond to further
environment variables and configuration files; see their respective manuals.
