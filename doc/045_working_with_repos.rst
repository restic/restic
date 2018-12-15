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


Checking a repo's integrity and consistency
===========================================

Imagine your repository is saved on a server that has a faulty hard
drive, or even worse, attackers get privileged access and modify your
backup with the intention to make you restore malicious data:

.. code-block:: console

    $ echo "boom" >> backup/index/d795ffa99a8ab8f8e42cec1f814df4e48b8f49129360fb57613df93739faee97

In order to detect these things, it is a good idea to regularly use the
``check`` command to test whether everything is alright, your precious
backup data is consistent and the integrity is unharmed:

.. code-block:: console

    $ restic -r /srv/restic-repo check
    Load indexes
    ciphertext verification failed

Trying to restore a snapshot which has been modified as shown above will
yield the same error:

.. code-block:: console

    $ restic -r /srv/restic-repo restore 79766175 --target /tmp/restore-work
    Load indexes
    ciphertext verification failed

By default, ``check`` command does not check that repository data files 
are unmodified. Use ``--read-data`` parameter to check all repository
data files:

.. code-block:: console

    $ restic -r /srv/restic-repo check --read-data
    load indexes
    check all packs
    check snapshots, trees and blobs
    read all data

Use ``--read-data-subset=n/t`` parameter to check subset of repository data
files. The parameter takes two values, ``n`` and ``t``. All repository data 
files are logically devided in ``t`` roughly equal groups and only files that
belong to the group number ``n`` are checked. For example, the following 
commands check all repository data files over 5 separate invocations:

.. code-block:: console

    $ restic -r /srv/restic-repo check --read-data-subset=1/5
    $ restic -r /srv/restic-repo check --read-data-subset=2/5
    $ restic -r /srv/restic-repo check --read-data-subset=3/5
    $ restic -r /srv/restic-repo check --read-data-subset=4/5
    $ restic -r /srv/restic-repo check --read-data-subset=5/5

