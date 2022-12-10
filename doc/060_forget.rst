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

All backup space is finite, so restic allows removing old snapshots. This can
be done either manually (by specifying a snapshot ID to remove) or by using a
policy that describes which snapshots to forget. For all remove operations, two
commands need to be called in sequence: ``forget`` to remove snapshots, and
``prune`` to remove the remaining data that was referenced only by the removed
snapshots. The latter can be automated with the ``--prune`` option of ``forget``,
which runs ``prune`` automatically if any snapshots were actually removed.

Pruning snapshots can be a time-consuming process, depending on the
number of snapshots and data to process. During a prune operation, the
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
    
    to repack:            69 blobs / 1.078 MiB
    this removes:         67 blobs / 1.047 MiB
    to delete:             7 blobs / 25.726 KiB
    total prune:          74 blobs / 1.072 MiB
    remaining:            16 blobs / 38.003 KiB
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

Removing snapshots manually is tedious and error-prone, therefore restic allows
specifying a policy (one or more ``--keep-*`` options) for which snapshots to
keep. You can for example define how many hourly, daily, weekly, monthly and
yearly snapshots to keep, and any other snapshots will be removed.

.. warning:: If you use an append-only repository with policy-based snapshot
    removal, some security considerations are important. Please refer to the
    section below for more information.

.. note:: You can always use the ``--dry-run`` option of the ``forget`` command,
    which instructs restic to not remove anything but instead just print what
    actions would be performed.

The ``forget`` command accepts the following policy options:

-  ``--keep-last n`` keep the ``n`` last (most recent) snapshots.
-  ``--keep-hourly n`` for the last ``n`` hours which have one or more
   snapshots, keep only the most recent one for each hour.
-  ``--keep-daily n`` for the last ``n`` days which have one or more
   snapshots, keep only the most recent one for each day.
-  ``--keep-weekly n`` for the last ``n`` weeks which have one or more
   snapshots, keep only the most recent one for each week.
-  ``--keep-monthly n`` for the last ``n`` months which have one or more
   snapshots, keep only the most recent one for each month.
-  ``--keep-yearly n`` for the last ``n`` years which have one or more
   snapshots, keep only the most recent one for each year.
-  ``--keep-tag`` keep all snapshots which have all tags specified by
   this option (can be specified multiple times).
-  ``--keep-within duration`` keep all snapshots having a timestamp within
   the specified duration of the latest snapshot, where ``duration`` is a
   number of years, months, days, and hours. E.g. ``2y5m7d3h`` will keep all
   snapshots made in the two years, five months, seven days and three hours
   before the latest (most recent) snapshot.
-  ``--keep-within-hourly duration`` keep all hourly snapshots made within the
   specified duration of the latest snapshot. The ``duration`` is specified in
   the same way as for ``--keep-within`` and the method for determining hourly
   snapshots is the same as for ``--keep-hourly``.
-  ``--keep-within-daily duration`` keep all daily snapshots made within the
   specified duration of the latest snapshot.
-  ``--keep-within-weekly duration`` keep all weekly snapshots made within the
   specified duration of the latest snapshot.
-  ``--keep-within-monthly duration`` keep all monthly snapshots made within the
   specified duration of the latest snapshot.
-  ``--keep-within-yearly duration`` keep all yearly snapshots made within the
   specified duration of the latest snapshot.

.. note:: All calendar related options (``--keep-{hourly,daily,...}``) work on
    natural time boundaries and *not* relative to when you run ``forget``. Weeks
    are Monday 00:00 to Sunday 23:59, days 00:00 to 23:59, hours :00 to :59, etc.
    They also only count hours/days/weeks/etc which have one or more snapshots.

.. note:: All duration related options (``--keep-{within,-*}``) ignore snapshots
    with a timestamp in the future (relative to when the ``forget`` command is
    run) and these snapshots will hence not be removed.

.. note:: Specifying ``--keep-tag ''`` will match untagged snapshots only.

When ``forget`` is run with a policy, restic first loads the list of all snapshots
and groups them by their host name and paths. The grouping options can be set with
``--group-by``, e.g. using ``--group-by paths,tags`` to instead group snapshots by
paths and tags. The policy is then applied to each group of snapshots individually.
This is a safety feature to prevent accidental removal of unrelated backup sets. To
disable grouping and apply the policy to all snapshots regardless of their host,
paths and tags, use ``--group-by ''`` (that is, an empty value to ``--group-by``).
Note that one would normally set the ``--group-by`` option for the ``backup``
command to the same value.

Additionally, you can restrict the policy to only process snapshots which have a
particular hostname with the ``--host`` parameter, or tags with the ``--tag``
option. When multiple tags are specified, only the snapshots which have all the
tags are considered. For example, the following command removes all but the
latest snapshot of all snapshots that have the tag ``foo``:

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

Let's look at a simple example: Suppose you have only made one backup every
Sunday for 12 weeks:

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

Then ``forget --keep-daily 4`` will keep the last four snapshots, for the last
four Sundays, and remove the other snapshots:

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

The processed snapshots are evaluated against all ``--keep-*`` options but a
snapshot only need to match a single option to be kept (the results are ORed).
This means that the most recent snapshot on a Sunday would match both hourly,
daily and weekly ``--keep-*`` options, and possibly more depending on calendar.

For example, suppose you make one backup every day for 100 years. Then ``forget
--keep-daily 7 --keep-weekly 5 --keep-monthly 12 --keep-yearly 75`` would keep
the most recent 7 daily snapshots and 4 last-day-of-the-week ones (since the 7
dailies already include 1 weekly). Additionally, 12 or 11 last-day-of-the-month
snapshots will be kept (depending on whether one of them ends up being the same
as a daily or weekly). And finally 75 or 74 last-day-of-the-year snapshots are
kept, depending on whether one of them ends up being the same as an already kept
snapshot. All other snapshots are removed.

You might want to maintain the same policy as in the example above, but have
irregular backups. For example, the 7 snapshots specified with ``--keep-daily 7`` 
might be spread over a longer period. If what you want is to keep daily
snapshots for the last week, weekly for the last month, monthly for the last
year and yearly for the last 75 years, you can instead specify ``forget
--keep-within-daily 7d --keep-within-weekly 1m --keep-within-monthly 1y
--keep-within-yearly 75y`` (note that `1w` is not a recognized duration, so
you will have to specify `7d` instead).

For safety reasons, restic refuses to act on an "empty" policy. For example,
if one were to specify ``--keep-last 0`` to forget *all* snapshots in the
repository, restic will respond that no snapshots will be removed. To delete
all snapshots, use ``--keep-last 1`` and then finally remove the last snapshot
manually (by passing the ID to ``forget``).

Security considerations in append-only mode
===========================================

.. note:: TL;DR: With append-only repositories, one should specifically use the
    ``--keep-within`` option of the ``forget`` command when removing snapshots.

To prevent a compromised backup client from deleting its backups (for example
due to a ransomware infection), a repository service/backend can serve the
repository in a so-called append-only mode. This means that the repository is
served in such a way that it can only be written to and read from, while delete
and overwrite operations are denied. Restic's `rest-server`_ features an
append-only mode, but few other standard backends do. To support append-only
with such backends, one can use `rclone`_ as a complement in between the backup
client and the backend service.

.. _rest-server: https://github.com/restic/rest-server/
.. _rclone: https://rclone.org/commands/rclone_serve_restic/

To remove snapshots and recover the corresponding disk space, the ``forget``
and ``prune`` commands require full read, write and delete access to the
repository. If an attacker has this, the protection offered by append-only
mode is naturally void. The usual and recommended setup with append-only
repositories is therefore to use a separate and well-secured client whenever
full access to the repository is needed, e.g. for administrative tasks such
as running ``forget``, ``prune`` and other maintenance commands.

However, even with append-only mode active and a separate, well-secured client
used for administrative tasks, an attacker who is able to add garbage snapshots
to the repository could bring the snapshot list into a state where all the
legitimate snapshots risk being deleted by an unsuspecting administrator that
runs the ``forget`` command with certain ``--keep-*`` options, leaving only the
attacker's useless snapshots.

For example, if the ``forget`` policy is to keep three weekly snapshots, and
the attacker adds an empty snapshot for each of the last three weeks, all with
a timestamp (see the ``backup`` command's ``--time`` option) slightly more
recent than the existing snapshots (but still within the target week), then the
next time the repository administrator (or a scheduled job) runs the ``forget``
command with this policy, the legitimate snapshots will be removed (since the
policy will keep only the most recent snapshot within each week). Even without
running ``prune``, recovering data would be messy and some metadata lost.

To avoid this, ``forget`` policies applied to append-only repositories should
use the ``--keep-within`` option, as this will keep not only the attacker's
snapshots but also the legitimate ones. Assuming the system time is correctly
set when ``forget`` runs, this will allow the administrator to notice problems
with the backup or the compromised host (e.g. by seeing more snapshots than
usual or snapshots with suspicious timestamps). This is, of course, limited to
the specified duration: if ``forget --keep-within 7d`` is run 8 days after the
last good snapshot, then the attacker can still use that opportunity to remove
all legitimate snapshots.

.. _customize-pruning:

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
    * As a size relative to the total repository size (e.g. ``10%``). This means that
      after prune, at most ``10%`` of the total data stored in the repository may be
      unused data. If the repository after prune has a size of 500MB, then at most
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


Recovering from "no free space" errors
**************************************

In some cases when a repository has grown large enough to fill up all disk space or the
allocated quota, then ``prune`` might fail to free space. ``prune`` works in such a way
that a repository remains usable no matter at which point the command is interrupted.
However, this also means that ``prune`` requires some scratch space to work.

In most cases it is sufficient to instruct ``prune`` to use as little scratch space as
possible by running it as ``prune --max-repack-size 0``. Note that for restic versions
before 0.13.0 ``prune --max-repack-size 1`` must be used. Obviously, this can only work
if several snapshots have been removed using ``forget`` before. This then allows the
``prune`` command to actually remove data from the repository. If the command succeeds,
but there is still little free space, then remove a few more snapshots and run ``prune`` again.

If ``prune`` fails to complete, then ``prune --unsafe-recover-no-free-space SOME-ID``
is available as a method of last resort. It allows prune to work with little to no free
space. However, a **failed** ``prune`` run can cause the repository to become
**temporarily unusable**. Therefore, make sure that you have a stable connection to the
repository storage, before running this command. In case the command fails, it may become
necessary to manually remove all files from the `index/` folder of the repository and
run `rebuild-index` afterwards.

To prevent accidental usages of the ``--unsafe-recover-no-free-space`` option it is
necessary to first run ``prune --unsafe-recover-no-free-space SOME-ID`` and then replace
``SOME-ID`` with the requested ID.
