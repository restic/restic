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

.. Warning::

   Pruning snapshots can be a very time-consuming process, taking nearly
   as long as backups themselves. During a prune operation, the index is
   locked and backups cannot be completed. Performance improvements are 
   planned for this feature.

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
    removed snapshot d3f01f63

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

    counting files in repo
    building new index for repo
    [0:00] 100.00%  22 / 22 files
    repository contains 22 packs (8512 blobs) with 100.092 MiB bytes
    processed 8512 blobs: 0 duplicate blobs, 0B duplicate
    load all snapshots
    find data that is still in use for 1 snapshots
    [0:00] 100.00%  1 / 1 snapshots
    found 8433 of 8512 data blobs still in use
    will rewrite 3 packs
    creating new index
    [0:00] 86.36%  19 / 22 files
    saved new index as 544a5084
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
    counting files in repo
    building new index for repo
    [0:00] 100.00%  37 / 37 packs
    repository contains 37 packs (5521 blobs) with 151.012 MiB bytes
    processed 5521 blobs: 0 duplicate blobs, 0B duplicate
    load all snapshots
    find data that is still in use for 1 snapshots
    [0:00] 100.00%  1 / 1 snapshots
    found 5323 of 5521 data blobs still in use, removing 198 blobs
    will delete 0 packs and rewrite 27 packs, this frees 22.106 MiB
    creating new index
    [0:00] 100.00%  30 / 30 packs
    saved new index as b49f3e68
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

Multiple policies will be ORed together so as to be as inclusive as possible
for keeping snapshots.

Additionally, you can restrict removing snapshots to those which have a
particular hostname with the ``--hostname`` parameter, or tags with the
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

   $ restic forget --tag foo,tag bar --keep-last 1

All the ``--keep-*`` options above only count
hours/days/weeks/months/years which have a snapshot, so those without a
snapshot are ignored.

For safety reasons, restic refuses to act on an "empty" policy. For example,
if one were to specify ``--keep-last 0`` to forget *all* snapshots in the
repository, restic will respond that no snapshots will be removed. To delete
all snapshots, use ``--keep-last 1`` and then finally remove the last
snapshot ID manually (by passing the ID to ``forget``).

All snapshots are evaluated against all matching ``--keep-*`` counts. A
single snapshot on 2017-09-30 (Sun) will count as a daily, weekly and monthly.

Let's explain this with an example: Suppose you have only made a backup
on each Sunday for 12 weeks. Then ``forget --keep-daily 4`` will keep
the last four snapshots for the last four Sundays, but remove the rest.
Only counting the days which have a backup and ignore the ones without
is a safety feature: it prevents restic from removing many snapshots
when no new ones are created. If it was implemented otherwise, running
``forget --keep-daily 4`` on a Friday would remove all snapshots!

Another example: Suppose you make daily backups for 100 years. Then
``forget --keep-daily 7 --keep-weekly 5 --keep-monthly 12 --keep-yearly 75``
will keep the most recent 7 daily snapshots, then 4 (remember, 7 dailies
already include a week!) last-day-of-the-weeks and 11 or 12
last-day-of-the-months (11 or 12 depends if the 5 weeklies cross a month).
And finally 75 last-day-of-the-year snapshots. All other snapshots are
removed.

