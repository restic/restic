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

You can use the command ``restic ls latest`` or ``restic find foo`` to find the
path to the file within the snapshot. This path you can then pass to
`--include` in verbatim to only restore the single file or directory.

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
    Don't forget to umount after quitting!

Mounting repositories via FUSE is not possible on OpenBSD, Solaris/illumos
and Windows. For Linux, the ``fuse`` kernel module needs to be loaded. For
FreeBSD, you may need to install FUSE and load the kernel module (``kldload
fuse``).

Restic supports storage and preservation of hard links. However, since
hard links exist in the scope of a filesystem by definition, restoring
hard links from a fuse mount should be done by a program that preserves
hard links. A program that does so is ``rsync``, used with the option
--hard-links.

Printing files to stdout
========================

Sometimes it's helpful to print files to stdout so that other programs can read
the data directly. This can be achieved by using the `dump` command, like this:

.. code-block:: console

    $ restic -r /srv/restic-repo dump latest production.sql | mysql

If you have saved multiple different things into the same repo, the ``latest``
snapshot may not be the right one. For example, consider the following
snapshots in a repo:

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
