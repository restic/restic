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

#####################
Restoring from backup
#####################

Restoring from a snapshot
=========================

Restoring a snapshot is as easy as it sounds, just use the following
command to restore the contents of the latest snapshot to
``/tmp/restore-work``:

.. code-block:: console

    $ restic -r /srv/restic-repo restore 79766175 --target /tmp/restore-work
    enter password for repository:
    restoring <Snapshot of [/home/user/work] at 2015-05-08 21:40:19.884408621 +0200 CEST> to /tmp/restore-work

Use the word ``latest`` to restore the last backup. You can also combine
``latest`` with the ``--host`` and ``--path`` filters to choose the last
backup for a specific host, path or both.

.. code-block:: console

    $ restic -r /srv/restic-repo restore latest --target /tmp/restore-art --path "/home/art" --host luigi
    enter password for repository:
    restoring <Snapshot of [/home/art] at 2015-05-08 21:45:17.884408621 +0200 CEST> to /tmp/restore-art

Use ``--exclude`` and ``--include`` to restrict the restore to a subset of
files in the snapshot. For example, to restore a single file:

.. code-block:: console

    $ restic -r /srv/restic-repo restore 79766175 --target /tmp/restore-work --include /work/foo
    enter password for repository:
    restoring <Snapshot of [/home/user/work] at 2015-05-08 21:40:19.884408621 +0200 CEST> to /tmp/restore-work

This will restore the file ``foo`` to ``/tmp/restore-work/work/foo``.

To only restore a specific subfolder, you can use the ``<snapshot>:<subfolder>``
syntax, where ``snapshot`` is the ID of a snapshot (or the string ``latest``)
and ``subfolder`` is a path within the snapshot.

.. code-block:: console

    $ restic -r /srv/restic-repo restore 79766175:/work --target /tmp/restore-work --include /foo
    enter password for repository:
    restoring <Snapshot of [/home/user/work] at 2015-05-08 21:40:19.884408621 +0200 CEST> to /tmp/restore-work

This will restore the file ``foo`` to ``/tmp/restore-work/foo``.

You can use the command ``restic ls latest`` or ``restic find foo`` to find the
path to the file within the snapshot. This path you can then pass to
``--include`` in verbatim to only restore the single file or directory.

There are case insensitive variants of ``--exclude`` and ``--include`` called
``--iexclude`` and ``--iinclude``. These options will behave the same way but
ignore the casing of paths.

There are also ``--include-file``, ``--exclude-file``, ``--iinclude-file`` and
``--iexclude-file`` flags that read the include and exclude patterns from a file.

Restoring symbolic links on windows is only possible when the user has
``SeCreateSymbolicLinkPrivilege`` privilege or is running as admin. This is a
restriction of windows not restic.

Restoring full security descriptors on Windows is only possible when the user has
``SeRestorePrivilege``, ``SeSecurityPrivilege`` and ``SeTakeOwnershipPrivilege`` 
privilege or is running as admin. This is a restriction of Windows not restic.
If either of these conditions are not met, only the DACL will be restored.

By default, restic does not restore files as sparse. Use ``restore --sparse`` to
enable the creation of sparse files if supported by the filesystem. Then restic
will restore long runs of zero bytes as holes in the corresponding files.
Reading from a hole returns the original zero bytes, but it does not consume
disk space. Note that the exact location of the holes can differ from those in
the original file, as their location is determined while restoring and is not
stored explicitly.

Restoring in-place
------------------

.. note::

    Restoring data in-place can leave files in a partially restored state if the ``restore``
    operation is interrupted. To ensure you can revert back to the previous state, create
    a current ``backup`` before restoring a different snapshot.

By default, the ``restore`` command overwrites already existing files at the target
directory. This behavior can be configured via the ``--overwrite`` option. The following
values are supported:

* ``--overwrite always`` (default): always overwrites already existing files. ``restore``
  will verify the existing file content and only restore mismatching parts to minimize
  downloads. Updates the metadata of all files.
* ``--overwrite if-changed``: like the previous case, but speeds up the file content check
  by assuming that files with matching size and modification time (mtime) are already up to date.
  In case of a mismatch, the full file content is verified. Updates the metadata of all files.
* ``--overwrite if-newer``: only overwrite existing files if the file in the snapshot has a
  newer modification time (mtime).
* ``--overwrite never``: never overwrite existing files.

Delete files not in snapshot
----------------------------

When restoring into a directory that already contains files, it can be useful to remove all
files that do not exist in the snapshot. For this, pass the ``--delete`` option to the ``restore``
command. The command will then **delete all files** from the target directory that do not
exist in the snapshot.

The ``--delete`` option also allows overwriting a non-empty directory if the snapshot contains a
file with the same name.

.. warning::

    Always use the ``--dry-run -vv`` option to verify what would be deleted before running the actual
    command.

When specifying ``--include`` or ``--exclude`` options, only files or directories matched by those
options will be deleted. For example, the command
``restic -r /srv/restic-repo restore 79766175:/work --target /tmp/restore-work --include /foo --delete``
would only delete files within ``/tmp/restore-work/foo``.

Dry run
-------

As restore operations can take a long time, it can be useful to perform a dry-run to
see what would be restored without having to run the full restore operation. The
restore command supports the ``--dry-run`` option and prints information about the
restored files when specifying ``--verbose=2``.

.. code-block:: console

    $ restic restore --target /tmp/restore-work --dry-run --verbose=2 latest

    unchanged /restic/internal/walker/walker.go with size 2.812 KiB
    updated   /restic/internal/walker/walker_test.go with size 11.143 KiB
    restored  /restic/restic with size 35.318 MiB
    restored  /restic
    [...]
    Summary: Restored 9072 files/dirs (153.597 MiB) in 0:00

Files with already up to date content are reported as ``unchanged``. Files whose content
was modified are ``updated`` and files that are new are shown as ``restored``. Directories
and other file types like symlinks are always reported as ``restored``.

To reliably determine which files would be updated, a dry-run also verifies the content of
already existing files according to the specified overwrite behavior. To skip these checks
either specify ``--overwrite never`` or specify a non-existing ``--target`` directory.

Restore using mount
===================

Browsing your backup as a regular file system is also very easy. First,
create a mount point such as ``/mnt/restic`` and then use the following
command to serve the repository with FUSE:

.. code-block:: console

    $ mkdir /mnt/restic
    $ restic -r /srv/restic-repo mount /mnt/restic
    enter password for repository:
    Now serving /srv/restic-repo at /mnt/restic
    Use another terminal or tool to browse the contents of this folder.
    When finished, quit with Ctrl-c here or umount the mountpoint.

Mounting repositories via FUSE is only possible on Linux, macOS and FreeBSD.
On Linux, the ``fuse`` kernel module needs to be loaded and the ``fusermount``
command needs to be in the ``PATH``. On macOS, you need `FUSE-T
<https://www.fuse-t.org/>`__ or `FUSE for macOS <https://osxfuse.github.io/>`__.
On FreeBSD, you may need to install FUSE and load the kernel module (``kldload fuse``).

Restic supports storage and preservation of hard links. However, since
hard links exist in the scope of a filesystem by definition, restoring
hard links from a fuse mount should be done by a program that preserves
hard links. A program that does so is ``rsync``, used with the option
``--hard-links``.

.. note:: ``restic mount`` is mostly useful if you want to restore just a few
   files out of a snapshot, or to check which files are contained in a snapshot.
   To restore many files or a whole snapshot, ``restic restore`` is the best
   alternative, often it is *significantly* faster.

Printing files to stdout
========================

Sometimes it's helpful to print files to stdout so that other programs can read
the data directly. This can be achieved by using the `dump` command, like this:

.. code-block:: console

    $ restic -r /srv/restic-repo dump latest production.sql | mysql

If you have saved multiple different things into the same repo, the ``latest``
snapshot may not be the right one. For example, consider the following
snapshots in a repository:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots
    ID        Date                 Host        Tags        Directory
    ----------------------------------------------------------------------
    562bfc5e  2018-07-14 20:18:01  mopped                  /home/user/file1
    bbacb625  2018-07-14 20:18:07  mopped                  /home/other/work
    e922c858  2018-07-14 20:18:10  mopped                  /home/other/work
    098db9d5  2018-07-14 20:18:13  mopped                  /production.sql
    b62f46ec  2018-07-14 20:18:16  mopped                  /home/user/file1
    1541acae  2018-07-14 20:18:18  mopped                  /home/other/work
    ----------------------------------------------------------------------

Here, restic would resolve ``latest`` to the snapshot ``1541acae``, which does
not contain the file we'd like to print at all (``production.sql``).  In this
case, you can pass restic the snapshot ID of the snapshot you like to restore:

.. code-block:: console

    $ restic -r /srv/restic-repo dump 098db9d5 production.sql | mysql

Or you can pass restic a path that should be used for selecting the latest
snapshot. The path must match the patch printed in the "Directory" column,
e.g.:

.. code-block:: console

    $ restic -r /srv/restic-repo dump --path /production.sql latest production.sql | mysql

It is also possible to ``dump`` the contents of a whole folder structure to
stdout. To retain the information about the files and folders Restic will
output the contents in the tar (default) or zip format:

.. code-block:: console

    $ restic -r /srv/restic-repo dump latest /home/other/work > restore.tar

.. code-block:: console

    $ restic -r /srv/restic-repo dump -a zip latest /home/other/work > restore.zip

The folder content is then contained at ``/home/other/work`` within the archive.
To include the folder content at the root of the archive, you can use the ``<snapshot>:<subfolder>`` syntax:

.. code-block:: console

    $ restic -r /srv/restic-repo dump latest:/home/other/work / > restore.tar

It is also possible to ``dump`` the contents of a selected snapshot and folder
structure to a file using the ``--target`` flag.

.. code-block:: console

    $ restic -r /srv/restic-repo dump latest / --target /home/linux.user/output.tar -a tar
