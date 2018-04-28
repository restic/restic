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
and Windows.

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
