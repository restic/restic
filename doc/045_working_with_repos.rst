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


#########################
Working with repositories
#########################

Listing all snapshots
=====================

Now, you can list all the snapshots stored in the repository:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots
    enter password for repository:
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    40dc1520  2015-05-08 21:38:30  kasimir        /home/user/work
    79766175  2015-05-08 21:40:19  kasimir        /home/user/work
    bdbd3439  2015-05-08 21:45:17  luigi          /home/art
    590c8fc8  2015-05-08 21:47:38  kazik          /srv
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

You can filter the listing by directory path:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots --path="/srv"
    enter password for repository:
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    590c8fc8  2015-05-08 21:47:38  kazik          /srv
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

Or filter by host:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots --host luigi
    enter password for repository:
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    bdbd3439  2015-05-08 21:45:17  luigi          /home/art
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

Combining filters is also possible.

Furthermore you can group the output by the same filters (host, paths, tags):

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots --group-by host

    enter password for repository:
    snapshots for (host [kasimir])
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    40dc1520  2015-05-08 21:38:30  kasimir        /home/user/work
    79766175  2015-05-08 21:40:19  kasimir        /home/user/work
    2 snapshots
    snapshots for (host [luigi])
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    bdbd3439  2015-05-08 21:45:17  luigi          /home/art
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv
    2 snapshots
    snapshots for (host [kazik])
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    590c8fc8  2015-05-08 21:47:38  kazik          /srv
    1 snapshots


Checking integrity and consistency
==================================

Imagine your repository is saved on a server that has a faulty hard
drive, or even worse, attackers get privileged access and modify the
files in your repository with the intention to make you restore
malicious data:

.. code-block:: console

    $ echo "boom" > /srv/restic-repo/index/de30f3231ca2e6a59af4aa84216dfe2ef7339c549dc11b09b84000997b139628

Trying to restore a snapshot which has been modified as shown above
will yield an error:

.. code-block:: console

    $ restic -r /srv/restic-repo --no-cache restore c23e491f --target /tmp/restore-work
    ...
    Fatal: unable to load index de30f323: load <index/de30f3231c>: invalid data returned

In order to detect these things before they become a problem, it's a
good idea to regularly use the ``check`` command to test whether your
repository is healthy and consistent, and that your precious backup
data is unharmed. There are two types of checks that can be performed:

- Structural consistency and integrity, e.g. snapshots, trees and pack files (default)
- Integrity of the actual data that you backed up (enabled with flags, see below)

To verify the structure of the repository, issue the ``check`` command.
If the repository is damaged like in the example above, ``check`` will
detect this and yield the same error as when you tried to restore:

.. code-block:: console

    $ restic -r /srv/restic-repo check
    ...
    load indexes
    error: error loading index de30f323: load <index/de30f3231c>: invalid data returned
    Fatal: LoadIndex returned errors

If the repository structure is intact, restic will show that no errors were found:

.. code-block:: console

    $ restic -r /src/restic-repo check
    ...
    load indexes
    check all packs
    check snapshots, trees and blobs
    no errors were found

By default, the ``check`` command does not verify that the actual data files
on disk in the repository are unmodified, because doing so requires reading
a copy of every data file in the repository. To tell restic to also verify the
integrity of the data files in the repository, use the ``--read-data`` flag:

.. code-block:: console

    $ restic -r /srv/restic-repo check --read-data
    ...
    load indexes
    check all packs
    check snapshots, trees and blobs
    read all data
    [0:00] 100.00%  3 / 3 items
    duration: 0:00
    no errors were found

.. note:: Since ``--read-data`` has to download all data files in the
    repository, beware that it might incur higher bandwidth costs than usual
    and also that it takes more time than the default ``check``.

Alternatively, use the ``--read-data-subset=n/t`` parameter to check only a
subset of the repository data files at a time. The parameter takes two values,
``n`` and ``t``. When the check command runs, all data files in the repository
are logically divided in ``t`` (roughly equal) groups, and only files that
belong to group number ``n`` are checked. For example, the following commands
check all repository data files over 5 separate invocations:

.. code-block:: console

    $ restic -r /srv/restic-repo check --read-data-subset=1/5
    $ restic -r /srv/restic-repo check --read-data-subset=2/5
    $ restic -r /srv/restic-repo check --read-data-subset=3/5
    $ restic -r /srv/restic-repo check --read-data-subset=4/5
    $ restic -r /srv/restic-repo check --read-data-subset=5/5
