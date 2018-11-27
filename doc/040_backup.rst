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

If you run the command again, restic will create another snapshot of
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
``--verbose 2``) to display detailed information about each file and directory
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

Including and Excluding Files
*****************************

You can exclude folders and files by specifying exclude patterns, currently
the exclude options are:

-  ``--exclude`` Specified one or more times to exclude one or more items
-  ``--exclude-caches`` Specified once to exclude folders containing a special file
-  ``--exclude-file`` Specified one or more times to exclude items listed in a given file
-  ``--exclude-if-present`` Specified one or more times to exclude a folders content
   if it contains a given file (optionally having a given header)

 Let's say we have a file called ``excludes.txt`` with the following content:

::

    # exclude go-files
    *.go
    # exclude foo/x/y/z/bar foo/x/bar foo/bar
    foo/**/bar

It can be used like this:

.. code-block:: console

    $ restic -r /srv/restic-repo backup ~/work --exclude="*.c" --exclude-file=excludes.txt

This instruct restic to exclude files matching the following criteria:

 * All files matching ``*.go`` (second line in ``excludes.txt``)
 * All files and sub-directories named ``bar`` which reside somewhere below a directory called ``foo`` (fourth line in ``excludes.txt``)
 * All files matching ``*.c`` (parameter ``--exclude``)

Please see ``restic help backup`` for more specific information about each exclude option.

Patterns use `filepath.Glob <https://golang.org/pkg/path/filepath/#Glob>`__ internally,
see `filepath.Match <https://golang.org/pkg/path/filepath/#Match>`__ for
syntax. Patterns are tested against the full path of a file/dir to be saved,
even if restic is passed a relative path to save. Environment-variables in
exclude-files are expanded with `os.ExpandEnv <https://golang.org/pkg/os/#ExpandEnv>`__,
so `/home/$USER/foo` will be expanded to `/home/bob/foo` for the user `bob`. To
get a literal dollar sign, write `$$` to the file.

Patterns need to match on complete path components. For example, the pattern ``foo``:

 * matches ``/dir1/foo/dir2/file`` and ``/dir/foo``
 * does not match ``/dir/foobar`` or ``barfoo``

A trailing ``/`` is ignored, a leading ``/`` anchors the
pattern at the root directory. This means, ``/bin`` matches ``/bin/bash`` but
does not match ``/usr/bin/restic``.

Regular wildcards cannot be used to match over the
directory separator ``/``. For example: ``b*ash`` matches ``/bin/bash`` but does not match
``/bin/ash``.

For this, the special wildcard ``**`` can be used to match arbitrary
sub-directories: The pattern ``foo/**/bar`` matches:

 * ``/dir1/foo/dir2/bar/file``
 * ``/foo/bar/file``
 * ``/tmp/foo/bar``

By specifying the option ``--one-file-system`` you can instruct restic
to only backup files from the file systems the initially specified files
or directories reside on. For example, calling restic like this won't
backup ``/sys`` or ``/dev`` on a Linux system:

.. code-block:: console

    $ restic -r /srv/restic-repo backup --one-file-system /

.. note:: ``--one-file-system`` is currently unsupported on Windows, and will
    cause the backup to immediately fail with an error.

By using the ``--files-from`` option you can read the files you want to
backup from one or more files. This is especially useful if a lot of files have
to be backed up that are not in the same folder or are maybe pre-filtered
by other software.

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

Paths in the listing file can be absolute or relative.

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

Reading data from stdin
***********************

Sometimes it can be nice to directly save the output of a program, e.g.
``mysqldump`` so that the SQL can later be restored. Restic supports
this mode of operation, just supply the option ``--stdin`` to the
``backup`` command like this:

.. code-block:: console

    $ mysqldump [...] | restic -r /srv/restic-repo backup --stdin

This creates a new snapshot of the output of ``mysqldump``. You can then
use e.g. the fuse mounting option (see below) to mount the repository
and read the file.

By default, the file name ``stdin`` is used, a different name can be
specified with ``--stdin-filename``, e.g. like this:

.. code-block:: console

    $ mysqldump [...] | restic -r /srv/restic-repo backup --stdin --stdin-filename production.sql

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
environment variables. The following list of environment variables:

.. code-block:: console

    RESTIC_REPOSITORY                   Location of repository (replaces -r)
    RESTIC_PASSWORD_FILE                Location of password file (replaces --password-file)
    RESTIC_PASSWORD                     The actual password for the repository

    AWS_ACCESS_KEY_ID                   Amazon S3 access key ID
    AWS_SECRET_ACCESS_KEY               Amazon S3 secret access key

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
    OS_PROJECT_DOMAIN_NAME              PRoject domain name for keystone authentication

    OS_STORAGE_URL                      Storage URL for token authentication
    OS_AUTH_TOKEN                       Auth token for token authentication

    B2_ACCOUNT_ID                       Account ID or applicationKeyId for Backblaze B2
    B2_ACCOUNT_KEY                      Account Key or applicationKey for Backblaze B2

    AZURE_ACCOUNT_NAME                  Account name for Azure
    AZURE_ACCOUNT_KEY                   Account key for Azure

    GOOGLE_PROJECT_ID                   Project ID for Google Cloud Storage
    GOOGLE_APPLICATION_CREDENTIALS      Application Credentials for Google Cloud Storage (e.g. $HOME/.config/gs-secret-restic-key.json)

    RCLONE_BWLIMIT                      rclone bandwidth limit



