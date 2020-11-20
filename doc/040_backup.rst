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
    password is correct
    lock repository
    load index files
    start scan
    start backup
    scan finished in 1.837s
    processed 1.720 GiB in 0:12
    Files:        5307 new,     0 changed,     0 unmodified
    Dirs:         1867 new,     0 changed,     0 unmodified
    Added:      1.200 GiB
    snapshot 40dc1520 saved

As you can see, restic created a backup of the directory and was pretty
fast! The specific snapshot just created is identified by a sequence of
hexadecimal characters, ``40dc1520`` in this case.

You can see that restic tells us it processed 1.720 GiB of data, this is the
size of the files and directories in ``~/work`` on the local file system. It
also tells us that only 1.200 GiB was added to the repository. This means that
some of the data was duplicate and restic was able to efficiently reduce it.

If you don't pass the ``--verbose`` option, restic will print less data. You'll
still get a nice live status display. Be aware that the live status shows the
processed files and not the transferred data. Transferred volume might be lower
(due to de-duplication) or higher.

On Windows, the ``--use-fs-snapshot`` option will use Windows' Volume Shadow Copy
Service (VSS) when creating backups. Restic will transparently create a VSS
snapshot for each volume that contains files to backup. Files are read from the
VSS snapshot instead of the regular filesystem. This allows to backup files that are
exclusively locked by another process during the backup.

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

    $ restic -r /srv/restic-repo backup --verbose ~/work
    open repository
    enter password for repository:
    password is correct
    lock repository
    load index files
    using parent snapshot d875ae93
    start scan
    start backup
    scan finished in 1.881s
    processed 1.720 GiB in 0:03
    Files:           0 new,     0 changed,  5307 unmodified
    Dirs:            0 new,     0 changed,  1867 unmodified
    Added:      0 B
    snapshot 79766175 saved

You can even backup individual files in the same repository (not passing
``--verbose`` means less output):

.. code-block:: console

    $ restic -r /srv/restic-repo backup ~/work.txt
    enter password for repository:
    password is correct
    snapshot 249d0210 saved

If you're interested in what restic does, pass ``--verbose`` twice (or
``--verbose=2``) to display detailed information about each file and directory
restic encounters:

.. code-block:: console

    $ echo 'more data foo bar' >> ~/work.txt

    $ restic -r /srv/restic-repo backup --verbose --verbose ~/work.txt
    open repository
    enter password for repository:
    password is correct
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

Please be aware that when you backup different directories (or the
directories to be saved have a variable name component like a
time/date), restic always needs to read all files and only afterwards
can compute which parts of the files need to be saved. When you backup
the same directory again (maybe with new or changed files) restic will
find the old snapshot in the repo and by default only reads those files
that are new or have been modified since the last snapshot. This is
decided based on the following attributes of the file in the file system:

 * Type (file, symlink, or directory?)
 * Modification time
 * Size
 * Inode number (internal number used to reference a file in a file system)

Now is a good time to run ``restic check`` to verify that all data
is properly stored in the repository. You should run this command regularly
to make sure the internal structure of the repository is free of errors.

Excluding Files
***************

You can exclude folders and files by specifying exclude patterns, currently
the exclude options are:

-  ``--exclude`` Specified one or more times to exclude one or more items
-  ``--iexclude`` Same as ``--exclude`` but ignores the case of paths
-  ``--exclude-caches`` Specified once to exclude folders containing a special file
-  ``--exclude-file`` Specified one or more times to exclude items listed in a given file
-  ``--iexclude-file`` Same as ``exclude-file`` but ignores cases like in ``--iexclude``
-  ``--exclude-if-present foo`` Specified one or more times to exclude a folder's content if it contains a file called ``foo`` (optionally having a given header, no wildcards for the file name supported)
-  ``--exclude-larger-than size`` Specified once to excludes files larger than the given size

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

Patterns use `filepath.Glob <https://golang.org/pkg/path/filepath/#Glob>`__ internally,
see `filepath.Match <https://golang.org/pkg/path/filepath/#Match>`__ for
syntax. Patterns are tested against the full path of a file/dir to be saved,
even if restic is passed a relative path to save.

Environment-variables in exclude files are expanded with `os.ExpandEnv <https://golang.org/pkg/os/#ExpandEnv>`__,
so ``/home/$USER/foo`` will be expanded to ``/home/bob/foo`` for the user ``bob``.
To get a literal dollar sign, write ``$$`` to the file. Note that tilde (``~``) expansion does not work, please use the ``$HOME`` environment variable instead.

Patterns need to match on complete path components. For example, the pattern ``foo``:

 * matches ``/dir1/foo/dir2/file`` and ``/dir/foo``
 * does not match ``/dir/foobar`` or ``barfoo``

A trailing ``/`` is ignored, a leading ``/`` anchors the pattern at the root directory.
This means, ``/bin`` matches ``/bin/bash`` but does not match ``/usr/bin/restic``.

Regular wildcards cannot be used to match over the directory separator ``/``.
For example: ``b*ash`` matches ``/bin/bash`` but does not match ``/bin/ash``.

For this, the special wildcard ``**`` can be used to match arbitrary
sub-directories: The pattern ``foo/**/bar`` matches:

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

By specifying the option ``--one-file-system`` you can instruct restic
to only backup files from the file systems the initially specified files
or directories reside on. In other words, it will prevent restic from crossing
filesystem boundaries when performing a backup.

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

This excludes files in ``~/work`` which are larger than 1 MB from the backup.

The default unit for the size value is bytes, so e.g. ``--exclude-larger-than 2048``
would exclude files larger than 2048 bytes (2 kilobytes). To specify other units,
suffix the size value with one of ``k``/``K`` for kilobytes, ``m``/``M`` for megabytes,
``g``/``G`` for gigabytes and ``t``/``T`` for terabytes (e.g. ``1k``, ``10K``, ``20m``,
``20M``,  ``30g``, ``30G``, ``2t`` or ``2T``).

Including Files
***************

By using the ``--files-from`` option you can read the files you want to back
up from one or more folders. This is especially useful if a lot of files have
to be backed up that are not in the same folder or are maybe pre-filtered by
other software.

For example maybe you want to backup files which have a name that matches a
certain pattern:

.. code-block:: console

    $ find /tmp/somefiles | grep 'PATTERN' > /tmp/files_to_backup

You can then use restic to backup the filtered files:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --files-from /tmp/files_to_backup

Incidentally you can also combine ``--files-from`` with the normal files
args:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --files-from /tmp/files_to_backup /tmp/some_additional_file

Paths in the listing file can be absolute or relative. Please note that
patterns listed in a ``--files-from`` file are treated the same way as
exclude patterns are, which means that beginning and trailing spaces are
trimmed and special characters must be escaped. See the documentation
above for more information.

Comparing Snapshots
*******************

Restic has a `diff` command which shows the difference between two snapshots
and displays a small statistic, just pass the command two snapshot IDs:

.. code-block:: console

    $ restic -r /srv/restic-repo diff 5845b002 2ab627a6
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

In filesystems that do not support inode consistency, like FUSE-based ones and pCloud, it is
possible to ignore inode on changed files comparison by passing ``--ignore-inode`` to
``backup`` command.

Reading data from stdin
***********************

Sometimes it can be nice to directly save the output of a program, e.g.
``mysqldump`` so that the SQL can later be restored. Restic supports
this mode of operation, just supply the option ``--stdin`` to the
``backup`` command like this:

.. code-block:: console

    $ set -o pipefail
    $ mysqldump [...] | restic -r /srv/restic-repo backup --stdin

This creates a new snapshot of the output of ``mysqldump``. You can then
use e.g. the fuse mounting option (see below) to mount the repository
and read the file.

By default, the file name ``stdin`` is used, a different name can be
specified with ``--stdin-filename``, e.g. like this:

.. code-block:: console

    $ mysqldump [...] | restic -r /srv/restic-repo backup --stdin --stdin-filename production.sql

The option ``pipefail`` is highly recommended so that a non-zero exit code from
one of the programs in the pipe (e.g. ``mysqldump`` here) makes the whole chain
return a non-zero exit code. Refer to the `Use the Unofficial Bash Strict Mode
<http://redsymbol.net/articles/unofficial-bash-strict-mode/>`__ for more
details on this.


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
    RESTIC_CACHE_DIR                    Location of the cache directory
    RESTIC_PROGRESS_FPS                 Frames per second by which the progress bar is updated

    TMPDIR                              Location for temporary files

    AWS_ACCESS_KEY_ID                   Amazon S3 access key ID
    AWS_SECRET_ACCESS_KEY               Amazon S3 secret access key
    AWS_DEFAULT_REGION                  Amazon S3 default region

    ST_AUTH                             Auth URL for keystone v1 authentication
    ST_USER                             Username for keystone v1 authentication
    ST_KEY                              Password for keystone v1 authentication

    OS_AUTH_URL                         Auth URL for keystone authentication
    OS_REGION_NAME                      Region name for keystone authentication
    OS_USERNAME                         Username for keystone authentication
    OS_PASSWORD                         Password for keystone authentication
    OS_TENANT_ID                        Tenant ID for keystone v2 authentication
    OS_TENANT_NAME                      Tenant name for keystone v2 authentication

    OS_USER_DOMAIN_NAME                 User domain name for keystone authentication
    OS_PROJECT_NAME                     Project name for keystone authentication
    OS_PROJECT_DOMAIN_NAME              Project domain name for keystone authentication

    OS_APPLICATION_CREDENTIAL_ID        Application Credential ID (keystone v3)
    OS_APPLICATION_CREDENTIAL_NAME      Application Credential Name (keystone v3)
    OS_APPLICATION_CREDENTIAL_SECRET    Application Credential Secret (keystone v3)

    OS_STORAGE_URL                      Storage URL for token authentication
    OS_AUTH_TOKEN                       Auth token for token authentication

    B2_ACCOUNT_ID                       Account ID or applicationKeyId for Backblaze B2
    B2_ACCOUNT_KEY                      Account Key or applicationKey for Backblaze B2

    AZURE_ACCOUNT_NAME                  Account name for Azure
    AZURE_ACCOUNT_KEY                   Account key for Azure

    GOOGLE_PROJECT_ID                   Project ID for Google Cloud Storage
    GOOGLE_APPLICATION_CREDENTIALS      Application Credentials for Google Cloud Storage (e.g. $HOME/.config/gs-secret-restic-key.json)

    RCLONE_BWLIMIT                      rclone bandwidth limit

See :ref:`caching` for the rules concerning cache locations when
``RESTIC_CACHE_DIR`` is not set.

The external programs that restic may execute include ``rclone`` (for rclone
backends) and ``ssh`` (for the SFTP backend). These may respond to further
environment variables and configuration files; see their respective manuals.


Exit status codes
*****************

Restic returns one of the following exit status codes after the backup command is run:

 * 0 when the backup was successful (snapshot with all source files created)
 * 1 when there was a fatal error (no snapshot created)
 * 3 when some source files could not be read (incomplete snapshot with remaining files created)

Fatal errors occur for example when restic is unable to write to the backup destination, when
there are network connectivity issues preventing successful communication, or when an invalid
password or command line argument is provided. When restic returns this exit status code, one
should not expect a snapshot to have been created.

Source file read errors occur when restic fails to read one or more files or directories that
it was asked to back up, e.g. due to permission problems. Restic displays the number of source
file read errors that occurred while running the backup. If there are errors of this type,
restic will still try to complete the backup run with all the other files, and create a
snapshot that then contains all but the unreadable files.

One can use these exit status codes in scripts and other automation tools, to make them aware of
the outcome of the backup run. To manually inspect the exit code in e.g. Linux, run ``echo $?``.
