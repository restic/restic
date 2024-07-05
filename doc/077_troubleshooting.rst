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

.. _troubleshooting:

#########################
Troubleshooting
#########################

The repository format used by restic is designed to be error resistant. In
particular, commands like, for example, ``backup`` or ``prune`` can be interrupted
at *any* point in time without damaging the repository. You might have to run
``unlock`` manually though, but that's it.

However, a repository might be damaged if some of its files are damaged or lost.
This can occur due to hardware failures, accidentally removing files from the
repository or bugs in the implementation of restic.

The following steps will help you recover a repository. This guide does not cover
all possible types of repository damages. Thus, if the steps do not work for you
or you are unsure how to proceed, then ask for help. Please always include the
check output discussed in the next section and what steps you've taken to repair
the repository so far.

* `Forum <https://forum.restic.net/>`_
* Our IRC channel ``#restic`` on ``irc.libera.chat``

Make sure that you **use the latest available restic version**. It can contain
bugfixes, and improvements to simplify the repair of a repository. It might also
contain a fix for your repository problems!


1. Find out what is damaged
***************************

The first step is always to check the repository.

.. code-block:: console

  $ restic check --read-data

  using temporary cache in /tmp/restic-check-cache-1418935501
  repository 12345678 opened (version 2, compression level auto)
  created new cache in /tmp/restic-check-cache-1418935501
  create exclusive lock for repository
  load indexes
  check all packs
  check snapshots, trees and blobs
  error for tree 7ef8ebab:
    id 7ef8ebabc59aadda1a237d23ca7abac487b627a9b86508aa0194690446ff71f6 not found in repository
  [0:02] 100.00%  7 / 7 snapshots
  read all data
  [0:05] 100.00%  25 / 25 packs
  Fatal: repository contains errors

.. note::

  This will download the whole repository. If retrieving data from the backend is
  expensive, then omit the ``--read-data`` option. Keep a copy of the check output
  as it might be necessary later on!

If the output contains warnings that the ``ciphertext verification failed`` for
some blobs in the repository, then please ask for help in the forum or our IRC
channel. These errors are often caused by hardware problems which **must** be
investigated and fixed. Otherwise, the backup will be damaged again and again.

Similarly, if a repository is repeatedly damaged, please open an `issue on GitHub
<https://github.com/restic/restic/issues/new/choose>`_ as this could indicate a bug
somewhere. Please include the check output and additional information that might
help locate the problem.

If ``check`` detects damaged pack files, it will show instructions on how to repair
them using the ``repair pack`` command. Use that command instead of the "Repair the
index" section in this guide.


2. Backup the repository
************************

Create a full copy of the repository if possible. Or at the very least make a
copy of the ``index`` and ``snapshots`` folders. This will allow you to roll back
the repository if the repair procedure fails. If your repository resides in a
cloud storage, then you can for example use `rclone <https://rclone.org/>`_ to
make such a copy.

Please disable all regular operations on the repository to prevent unexpected
changes. Especially, ``forget`` or ``prune`` must be disabled as they could
remove data unexpectedly.

.. warning::

   If you suspect hardware problems, then you *must* investigate those first.
   Otherwise, the repository will soon be damaged again.

Please take the time to understand what the commands described in the following
do. If you are unsure, then ask for help in the forum or our IRC channel. Search
whether your issue is already known and solved. Please take a look at the
`forum`_ and `GitHub issues <https://github.com/restic/restic/issues>`_.


3. Repair the index
*******************

.. note::

  If the `check` command tells you to run `restic repair pack`, then use that
  command instead. It will repair the damaged pack files and also update the index.

Restic relies on its index to contain correct information about what data is
stored in the repository. Thus, the first step to repair a repository is to
repair the index:

.. code-block:: console

    $ restic repair index

    repository a14e5863 opened (version 2, compression level auto)
    loading indexes...
    getting pack files to read...
    removing not found pack file 83ad44f59b05f6bce13376b022ac3194f24ca19e7a74926000b6e316ec6ea5a4
    rebuilding index
    [0:00] 100.00%  27 / 27 packs processed
    deleting obsolete index files
    [0:00] 100.00%  3 / 3 files deleted
    done

This ensures that no longer existing files are removed from the index. All later
steps to repair the repository rely on a correct index. That is, you must always
repair the index first!

Please note that it is not recommended to repair the index unless the repository
is actually damaged.


4. Run all backups (optional)
*****************************

With a correct index, the ``backup`` command guarantees that newly created
snapshots can be restored successfully. It can also heal older snapshots,
if the missing data is also contained in the new snapshot.

Therefore, it is recommended to run all your ``backup`` tasks again. In some
cases, this is enough to fully repair the repository.


5. Remove missing data from snapshots
*************************************

If your repository is still missing data, then you can use the ``repair snapshots``
command to remove all inaccessible data from the snapshots. That is, this will
result in a limited amount of data loss. Using the ``--forget`` option, the
command will automatically remove the original, damaged snapshots.

.. code-block:: console

  $ restic repair snapshots --forget

  snapshot 6979421e of [/home/user/restic/restic] at 2022-11-02 20:59:18.617503315 +0100 CET by user@host
    file "/restic/internal/fuse/snapshots_dir.go": removed missing content
    file "/restic/internal/restorer/restorer_unix_test.go": removed missing content
    file "/restic/internal/walker/walker.go": removed missing content
  saved new snapshot 7b094cea
  removed old snapshot 6979421e

  modified 1 snapshots

If you did not add the ``--forget`` option, then you have to manually delete all
modified snapshots using the ``forget`` command. In the example above, you'd have
to run ``restic forget 6979421e``.


6. Check the repository again
*****************************

Phew, we're almost done now. To make sure that the repository has been successfully
repaired please run ``check`` again.

.. code-block:: console

  $ restic check --read-data

  using temporary cache in /tmp/restic-check-cache-2569290785
  repository a14e5863 opened (version 2, compression level auto)
  created new cache in /tmp/restic-check-cache-2569290785
  create exclusive lock for repository
  load indexes
  check all packs
  check snapshots, trees and blobs
  [0:00] 100.00%  7 / 7 snapshots
  read all data
  [0:00] 100.00%  25 / 25 packs
  no errors were found

If the ``check`` command did not complete with ``no errors were found``, then
the repository is still damaged. At this point, please ask for help at the
`forum`_ or our IRC channel ``#restic`` on ``irc.libera.chat``.
