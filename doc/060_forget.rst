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
Removing backup snapshots
#########################

All backup space is finite, so restic allows removing old snapshots.
This can be done either manually (by specifying a snapshot ID to remove)
or by using a policy that describes which snapshots to forget. For all
remove operations, two commands need to be called in sequence:
``forget`` to remove a snapshot and ``prune`` to actually remove the
data that was referenced by the snapshot from the repository. This can
be automated with the ``--prune`` option of the ``forget`` command,
which runs ``prune`` automatically if snapshots have been removed.

Pruning snapshots can be a time-consuming process, depending on the
amount of snapshots and data to process. During a prune operation, the
repository is locked and backups cannot be completed. Please plan your
pruning so that there's time to complete it and it doesn't interfere with
regular backup runs.

It is advisable to run ``restic check`` after pruning, to make sure
you are alerted, should the internal data structures of the repository
be damaged.

Remove a single snapshot
************************

The command ``snapshots`` can be used to list all snapshots in a
repository like this:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots
    enter password for repository:
    ID        Date                 Host      Tags  Directory
    ----------------------------------------------------------------------
    40dc1520  2015-05-08 21:38:30  kasimir         /home/user/work
    79766175  2015-05-08 21:40:19  kasimir         /home/user/work
    bdbd3439  2015-05-08 21:45:17  luigi           /home/art
    590c8fc8  2015-05-08 21:47:38  kazik           /srv
    9f0bc19e  2015-05-08 21:46:11  luigi           /srv

In order to remove the snapshot of ``/home/art``, use the ``forget``
command and specify the snapshot ID on the command line:

.. code-block:: console

    $ restic -r /srv/restic-repo forget bdbd3439
    enter password for repository:
    removed snapshot bdbd3439

Afterwards this snapshot is removed:

.. code-block:: console

    $ restic -r /srv/restic-repo snapshots
    enter password for repository:
    ID        Date                 Host     Tags  Directory
    ----------------------------------------------------------------------
    40dc1520  2015-05-08 21:38:30  kasimir        /home/user/work
    79766175  2015-05-08 21:40:19  kasimir        /home/user/work
    590c8fc8  2015-05-08 21:47:38  kazik          /srv
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

But the data that was referenced by files in this snapshot is still
stored in the repository. To cleanup unreferenced data, the ``prune``
command must be run:

.. code-block:: console

    $ restic -r /srv/restic-repo prune
    enter password for repository:
    repository 33002c5e opened successfully, password is correct
    loading all snapshots...
    loading indexes...
    finding data that is still in use for 4 snapshots
    [0:00] 100.00%  4 / 4 snapshots
    searching used packs...
    collecting packs for deletion and repacking
    [0:00] 100.00%  5 / 5 packs processed
    
    to repack:           69 blobs / 1.078 MiB
    this removes         67 blobs / 1.047 MiB
    to delete:            7 blobs / 25.726 KiB
    total prune:         74 blobs / 1.072 MiB
    remaining:           16 blobs / 38.003 KiB
    unused size after prune: 0 B (0.00% of remaining size)
    
    repacking packs
    [0:00] 100.00%  2 / 2 packs repacked
    rebuilding index
    [0:00] 100.00%  3 / 3 packs processed
    deleting obsolete index files
    [0:00] 100.00%  3 / 3 files deleted
    removing 3 old packs
    [0:00] 100.00%  3 / 3 files deleted
    done

Afterwards the repository is smaller.

You can automate this two-step process by using the ``--prune`` switch
to ``forget``:

.. code-block:: console

    $ restic forget --keep-last 1 --prune
    snapshots for host mopped, directories /home/user/work:

    keep 1 snapshots:
    ID        Date                 Host        Tags        Directory
    ----------------------------------------------------------------------
    4bba301e  2017-02-21 10:49:18  mopped                  /home/user/work

    remove 1 snapshots:
    ID        Date                 Host        Tags        Directory
    ----------------------------------------------------------------------
    8c02b94b  2017-02-21 10:48:33  mopped                  /home/user/work

    1 snapshots have been removed, running prune
    loading all snapshots...
    loading indexes...
    finding data that is still in use for 1 snapshots
    [0:00] 100.00%  1 / 1 snapshots
    searching used packs...
    collecting packs for deletion and repacking
    [0:00] 100.00%  5 / 5 packs processed
    
    to repack:           69 blobs / 1.078 MiB
    this removes         67 blobs / 1.047 MiB
    to delete:            7 blobs / 25.726 KiB
    total prune:         74 blobs / 1.072 MiB
    remaining:           16 blobs / 38.003 KiB
    unused size after prune: 0 B (0.00% of remaining size)
    
    repacking packs
    [0:00] 100.00%  2 / 2 packs repacked
    rebuilding index
    [0:00] 100.00%  3 / 3 packs processed
    deleting obsolete index files
    [0:00] 100.00%  3 / 3 files deleted
    removing 3 old packs
    [0:00] 100.00%  3 / 3 files deleted
    done

Removing snapshots according to a policy
****************************************

Removing snapshots manually is tedious and error-prone, therefore restic
allows specifying which snapshots should be removed automatically
according to a policy. You can specify how many hourly, daily, weekly,
monthly and yearly snapshots to keep, any other snapshots are removed.
The most important command-line parameter here is ``--dry-run`` which
instructs restic to not remove anything but print which snapshots would
be removed.

When ``forget`` is run with a policy, restic loads the list of all
snapshots, then groups these by host name and list of directories. The grouping
options can be set with ``--group-by``, to only group snapshots by paths and
tags use ``--group-by paths,tags``. The policy is then applied to each group of
snapshots separately. This is a safety feature.

The ``forget`` command accepts the following parameters:

-  ``--keep-last n`` never delete the ``n`` last (most recent) snapshots
-  ``--keep-hourly n`` for the last ``n`` hours in which a snapshot was
   made, keep only the last snapshot for each hour.
-  ``--keep-daily n`` for the last ``n`` days which have one or more
   snapshots, only keep the last one for that day.
-  ``--keep-weekly n`` for the last ``n`` weeks which have one or more
   snapshots, only keep the last one for that week.
-  ``--keep-monthly n`` for the last ``n`` months which have one or more
   snapshots, only keep the last one for that month.
-  ``--keep-yearly n`` for the last ``n`` years which have one or more
   snapshots, only keep the last one for that year.
-  ``--keep-tag`` keep all snapshots which have all tags specified by
   this option (can be specified multiple times).
-  ``--keep-within duration`` keep all snapshots which have been made within
   the duration of the latest snapshot. ``duration`` needs to be a number of
   years, months, days, and hours, e.g. ``2y5m7d3h`` will keep all snapshots
   made in the two years, five months, seven days, and three hours before the
   latest snapshot.
-  ``--keep-within-hourly duration`` keep all hourly snapshots made within
   specified duration of the latest snapshot. The duration is specified in 
   the same way as for ``--keep-within`` and the method for determining
   hourly snapshots is the same as for ``--keep-hourly``.
-  ``--keep-within-daily duration`` keep all daily snapshots made within
   specified duration of the latest snapshot.
-  ``--keep-within-weekly duration`` keep all weekly snapshots made within
   specified duration of the latest snapshot.
-  ``--keep-within-monthly duration`` keep all monthly snapshots made within
   specified duration of the latest snapshot.
-  ``--keep-within-yearly duration`` keep all yearly snapshots made within
   specified duration of the latest snapshot.

.. note:: All calendar related ``--keep-*`` options work on the natural time
    boundaries and not relative to when you run the ``forget`` command. Weeks
    are Monday 00:00 -> Sunday 23:59, days 00:00 to 23:59, hours :00 to :59, etc.

.. note:: Specifying ``--keep-tag ''`` will match untagged snapshots only.

Multiple policies will be ORed together so as to be as inclusive as possible
for keeping snapshots.

Additionally, you can restrict removing snapshots to those which have a
particular hostname with the ``--host`` parameter, or tags with the
``--tag`` option. When multiple tags are specified, only the snapshots
which have all the tags are considered. For example, the following command
removes all but the latest snapshot of all snapshots that have the tag ``foo``:

.. code-block:: console

   $ restic forget --tag foo --keep-last 1

This command removes all but the last snapshot of all snapshots that have
either the ``foo`` or ``bar`` tag set:

.. code-block:: console

   $ restic forget --tag foo --tag bar --keep-last 1

To only keep the last snapshot of all snapshots with both the tag ``foo`` and
``bar`` set use:

.. code-block:: console

   $ restic forget --tag foo,bar --keep-last 1

To ensure only untagged snapshots are considered, specify the empty string '' as
the tag.

.. code-block:: console

   $ restic forget --tag '' --keep-last 1

All the ``--keep-*`` options above only count
hours/days/weeks/months/years which have a snapshot, so those without a
snapshot are ignored.

For safety reasons, restic refuses to act on an "empty" policy. For example,
if one were to specify ``--keep-last 0`` to forget *all* snapshots in the
repository, restic will respond that no snapshots will be removed. To delete
all snapshots, use ``--keep-last 1`` and then finally remove the last
snapshot ID manually (by passing the ID to ``forget``).

All snapshots are evaluated against all matching ``--keep-*`` counts. A
single snapshot on 2017-09-30 (Sat) will count as a daily, weekly and monthly.

Let's explain this with an example: Suppose you have only made a backup
on each Sunday for 12 weeks:

.. code-block:: console

   $ restic snapshots
   repository f00c6e2a opened successfully, password is correct
   ID        Time                 Host        Tags        Paths
   ---------------------------------------------------------------
   0a1f9759  2019-09-01 11:00:00  mopped                  /home/user/work
   46cfe4d5  2019-09-08 11:00:00  mopped                  /home/user/work
   f6b1f037  2019-09-15 11:00:00  mopped                  /home/user/work
   eb430a5d  2019-09-22 11:00:00  mopped                  /home/user/work
   8cf1cb9a  2019-09-29 11:00:00  mopped                  /home/user/work
   5d33b116  2019-10-06 11:00:00  mopped                  /home/user/work
   b9553125  2019-10-13 11:00:00  mopped                  /home/user/work
   e1a7b58b  2019-10-20 11:00:00  mopped                  /home/user/work
   8f8018c0  2019-10-27 11:00:00  mopped                  /home/user/work
   59403279  2019-11-03 11:00:00  mopped                  /home/user/work
   dfee9fb4  2019-11-10 11:00:00  mopped                  /home/user/work
   e1ae2f40  2019-11-17 11:00:00  mopped                  /home/user/work
   ---------------------------------------------------------------
   12 snapshots

Then ``forget --keep-daily 4`` will keep the last four snapshots for the last
four Sundays, but remove the rest:

.. code-block:: console

   $ restic forget --keep-daily 4 --dry-run
   repository f00c6e2a opened successfully, password is correct
   Applying Policy: keep the last 4 daily snapshots
   keep 4 snapshots:
   ID        Time                 Host        Tags        Reasons         Paths
   -------------------------------------------------------------------------------
   8f8018c0  2019-10-27 11:00:00  mopped                  daily snapshot  /home/user/work
   59403279  2019-11-03 11:00:00  mopped                  daily snapshot  /home/user/work
   dfee9fb4  2019-11-10 11:00:00  mopped                  daily snapshot  /home/user/work
   e1ae2f40  2019-11-17 11:00:00  mopped                  daily snapshot  /home/user/work
   -------------------------------------------------------------------------------
   4 snapshots

   remove 8 snapshots:
   ID        Time                 Host        Tags        Paths
   ---------------------------------------------------------------
   0a1f9759  2019-09-01 11:00:00  mopped                  /home/user/work
   46cfe4d5  2019-09-08 11:00:00  mopped                  /home/user/work
   f6b1f037  2019-09-15 11:00:00  mopped                  /home/user/work
   eb430a5d  2019-09-22 11:00:00  mopped                  /home/user/work
   8cf1cb9a  2019-09-29 11:00:00  mopped                  /home/user/work
   5d33b116  2019-10-06 11:00:00  mopped                  /home/user/work
   b9553125  2019-10-13 11:00:00  mopped                  /home/user/work
   e1a7b58b  2019-10-20 11:00:00  mopped                  /home/user/work
   ---------------------------------------------------------------
   8 snapshots

The result of the ``forget --keep-daily`` operation does not depend on when it
is run, it will only count the days for which a snapshot exists. This is a
safety feature: it prevents restic from removing snapshots when no new ones are
created. Otherwise, running ``forget --keep-daily 4`` on a Friday (without any
snapshot Monday to Thursday) would remove all snapshots!

Another example: Suppose you make daily backups for 100 years. Then
``forget --keep-daily 7 --keep-weekly 5 --keep-monthly 12 --keep-yearly 75``
will keep the most recent 7 daily snapshots, then 4 (remember, 7 dailies
already include a week!) last-day-of-the-weeks and 11 or 12
last-day-of-the-months (11 or 12 depends if the 5 weeklies cross a month).
And finally 75 last-day-of-the-year snapshots. All other snapshots are
removed.

You might want to maintain the same policy as for the example above, but have
irregular backups. For example, the 7 snapshots specified with ``--keep-daily 7`` 
might be spread over a longer period. If what you want is to keep daily snapshots
for a week, weekly for a month, monthly for a year and yearly for 75 years, you 
could specify:
``forget --keep-daily-within 7d --keep-weekly-within 1m --keep-monthly-within 1y
--keep-yearly-within 75y``
(Note that `1w` is not a recognized duration, so you will have to specify 
`7d` instead)

Customize pruning
*****************

To understand the custom options, we first explain how the pruning process works:

1. All snapshots and directories within snapshots are scanned to determine
   which data is still in use.
2. For all files in the repository, restic finds out if the file is fully
   used, partly used or completely unused.
3. Completely unused files are marked for deletion. Fully used files are kept.
   A partially used file is either kept or marked for repacking depending on user
   options.

   Note that for repacking, restic must download the file from the repository
   storage and re-upload the needed data in the repository. This can be very
   time-consuming for remote repositories.
4. After deciding what to do, ``prune`` will actually perform the repack, modify
   the index according to the changes and delete the obsolete files.

The ``prune`` command accepts the following options:

-  ``--max-unused limit`` allow unused data up to the specified limit within the repository.
   This allows restic to keep partly used files instead of repacking them.

   The limit can be specified in several ways:

    * As an absolute size (e.g. ``200M``). If you want to minimize the space
      used by your repository, pass ``0`` to this option.
    * As a size relative to the total repo size (e.g. ``10%``). This means that
      after prune, at most ``10%`` of the total data stored in the repo may be
      unused data. If the repo after prune has as size of 500MB, then at most
      50MB may be unused.
    * If the string ``unlimited`` is passed, there is no limit for partly
      unused files. This means that as long as some data is still used within
      a file stored in the repo, restic will just leave it there. Use this if
      you want to minimize the time and bandwidth used by the ``prune``
      operation. Note that metadata will still be repacked.

   Restic tries to repack as little data as possible while still ensuring this 
   limit for unused data. The default value is 5%.

- ``--max-repack-size size`` if set limits the total size of files to repack.
  As ``prune`` first stores all repacked files and deletes the obsolete files at the end,
  this option might be handy if you expect many files to be repacked and fear to run low
  on storage. 

- ``--repack-cacheable-only`` if set to true only files which contain
  metadata and would be stored in the cache are repacked. Other pack files are
  not repacked if this option is set. This allows a very fast repacking
  using only cached data. It can, however, imply that the unused data in
  your repository exceeds the value given by ``--max-unused``.
  The default value is false.

-  ``--dry-run`` only show what ``prune`` would do.

-  ``--verbose`` increased verbosity shows additional statistics for ``prune``.
