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
Scripting
#########################

This is a list of how certain tasks may be accomplished when you use
restic via scripts.

Check if a repository is already initialized
********************************************

You may find a need to check if a repository is already initialized,
perhaps to prevent your script from trying to initialize a repository multiple
times (the ``init`` command contains a check to prevent overwriting existing
repositories). The command ``cat config`` may be used for this purpose:

.. code-block:: console

    $ restic -r /srv/restic-repo cat config
    Fatal: repository does not exist: unable to open config file: stat /srv/restic-repo/config: no such file or directory
    Is there a repository at the following location?
    /srv/restic-repo

If a repository does not exist, restic (since 0.17.0) will return exit code ``10``
and print a corresponding error message. Older versions return exit code ``1``.
Note that restic will also return exit code ``1`` if a different error is encountered
(e.g.: incorrect password to ``cat config``) and it may print a different error message.
If there are no errors, restic will return a zero exit code and print the repository
metadata.

Exit codes
**********

Restic commands return an exit code that signals whether the command was successful.
The following table provides a general description, see the help of each command for
a more specific description.

.. warning::
    New exit codes will be added over time. If an unknown exit code is returned, then it
    MUST be treated as a command failure.

+-----+----------------------------------------------------+
| 0   | Command was successful                             |
+-----+----------------------------------------------------+
| 1   | Command failed, see command help for more details  |
+-----+----------------------------------------------------+
| 2   | Go runtime error                                   |
+-----+----------------------------------------------------+
| 3   | ``backup`` command could not read some source data |
+-----+----------------------------------------------------+
| 10  | Repository does not exist (since restic 0.17.0)    |
+-----+----------------------------------------------------+
| 11  | Failed to lock repository (since restic 0.17.0)    |
+-----+----------------------------------------------------+
| 130 | Restic was interrupted using SIGINT or SIGSTOP     |
+-----+----------------------------------------------------+

JSON output
***********

Restic outputs JSON data to ``stdout`` if requested with the ``--json`` flag.
The structure of that data varies depending on the circumstance.  The
JSON output of most restic commands are documented here.

.. note::
    Not all commands support JSON output.  If a command does not support JSON output,
    feel free to submit a pull request!

.. warning::
    We try to keep the JSON output backwards compatible. However, new message types
    or fields may be added at any time. Similarly, enum-like fields for which a fixed
    list of allowed values is documented may be extended at any time.


Output formats
--------------

Currently only the output on ``stdout`` is JSON formatted. Errors printed on ``stderr``
are still printed as plain text messages. The generated JSON output uses one of the
following two formats.

Single JSON document
^^^^^^^^^^^^^^^^^^^^

Several commands output a single JSON document that can be parsed in its entirety.
Depending on the command, the output consists of either a single or multiple lines.

JSON lines
^^^^^^^^^^

Several commands, in particular long running ones or those that generate a large output,
use a format also known as JSON lines. It consists of a stream of new-line separated JSON
messages. You can determine the nature of the message using the ``message_type`` field.

backup
------

The ``backup`` command uses the JSON lines format with the following message types.

Status
^^^^^^

+----------------------+------------------------------------------------------------+
|``message_type``      | Always "status"                                            |
+----------------------+------------------------------------------------------------+
|``seconds_elapsed``   | Time since backup started                                  |
+----------------------+------------------------------------------------------------+
|``seconds_remaining`` | Estimated time remaining                                   |
+----------------------+------------------------------------------------------------+
|``percent_done``      | Percentage of data backed up (bytes_done/total_bytes)      |
+----------------------+------------------------------------------------------------+
|``total_files``       | Total number of files detected                             |
+----------------------+------------------------------------------------------------+
|``files_done``        | Files completed (backed up to repo)                        |
+----------------------+------------------------------------------------------------+
|``total_bytes``       | Total number of bytes in backup set                        |
+----------------------+------------------------------------------------------------+
|``bytes_done``        | Number of bytes completed (backed up to repo)              |
+----------------------+------------------------------------------------------------+
|``error_count``       | Number of errors                                           |
+----------------------+------------------------------------------------------------+
|``current_files``     | List of files currently being backed up                    |
+----------------------+------------------------------------------------------------+

Error
^^^^^

+----------------------+-------------------------------------------+
| ``message_type``     | Always "error"                            |
+----------------------+-------------------------------------------+
| ``error.message``    | Error message                             |
+----------------------+-------------------------------------------+
| ``during``           | What restic was trying to do              |
+----------------------+-------------------------------------------+
| ``item``             | Usually, the path of the problematic file |
+----------------------+-------------------------------------------+

Verbose Status
^^^^^^^^^^^^^^

Verbose status provides details about the progress, including details about backed up files.

+----------------------+-----------------------------------------------------------+
| ``message_type``     | Always "verbose_status"                                   |
+----------------------+-----------------------------------------------------------+
| ``action``           | Either "new", "unchanged", "modified" or "scan_finished"  |
+----------------------+-----------------------------------------------------------+
| ``item``             | The item in question                                      |
+----------------------+-----------------------------------------------------------+
| ``duration``         | How long it took, in seconds                              |
+----------------------+-----------------------------------------------------------+
| ``data_size``        | How big the item is                                       |
+----------------------+-----------------------------------------------------------+
| ``metadata_size``    | How big the metadata is                                   |
+----------------------+-----------------------------------------------------------+
| ``total_files``      | Total number of files                                     |
+----------------------+-----------------------------------------------------------+

Summary
^^^^^^^

Summary is the last output line in a successful backup.

+---------------------------+---------------------------------------------------------+
| ``message_type``          | Always "summary"                                        |
+---------------------------+---------------------------------------------------------+
| ``files_new``             | Number of new files                                     |
+---------------------------+---------------------------------------------------------+
| ``files_changed``         | Number of files that changed                            |
+---------------------------+---------------------------------------------------------+
| ``files_unmodified``      | Number of files that did not change                     |
+---------------------------+---------------------------------------------------------+
| ``dirs_new``              | Number of new directories                               |
+---------------------------+---------------------------------------------------------+
| ``dirs_changed``          | Number of directories that changed                      |
+---------------------------+---------------------------------------------------------+
| ``dirs_unmodified``       | Number of directories that did not change               |
+---------------------------+---------------------------------------------------------+
| ``data_blobs``            | Number of data blobs                                    |
+---------------------------+---------------------------------------------------------+
| ``tree_blobs``            | Number of tree blobs                                    |
+---------------------------+---------------------------------------------------------+
| ``data_added``            | Amount of (uncompressed) data added, in bytes           |
+---------------------------+---------------------------------------------------------+
| ``data_added_packed``     | Amount of data added (after compression), in bytes      |
+---------------------------+---------------------------------------------------------+
| ``total_files_processed`` | Total number of files processed                         |
+---------------------------+---------------------------------------------------------+
| ``total_bytes_processed`` | Total number of bytes processed                         |
+---------------------------+---------------------------------------------------------+
| ``total_duration``        | Total time it took for the operation to complete        |
+---------------------------+---------------------------------------------------------+
| ``snapshot_id``           | ID of the new snapshot. Field is omitted if snapshot    |
|                           | creation was skipped                                    |
+---------------------------+---------------------------------------------------------+


cat
---

The ``cat`` command returns data about various objects in the repository, which
are stored in JSON form. Specifying ``--json``  or ``--quiet`` will suppress any
non-JSON messages the command generates.


diff
----

The ``diff`` command uses the JSON lines format with the following message types.

change
^^^^^^

+------------------+--------------------------------------------------------------+
| ``message_type`` | Always "change"                                              |
+------------------+--------------------------------------------------------------+
| ``path``         | Path that has changed                                        |
+------------------+--------------------------------------------------------------+
| ``modifier``     | Type of change, a concatenation of the following characters: |
|                  | "+" = added, "-" = removed, "T" = entry type changed,        |
|                  | "M" = file content changed, "U" = metadata changed,          |
|                  | "?" = bitrot detected                                        |
+------------------+--------------------------------------------------------------+

statistics
^^^^^^^^^^

+---------------------+----------------------------+
| ``message_type``    | Always "statistics"        |
+---------------------+----------------------------+
| ``source_snapshot`` | ID of first snapshot       |
+---------------------+----------------------------+
| ``target_snapshot`` | ID of second snapshot      |
+---------------------+----------------------------+
| ``changed_files``   | Number of changed files    |
+---------------------+----------------------------+
| ``added``           | DiffStat object, see below |
+---------------------+----------------------------+
| ``removed``         | DiffStat object, see below |
+---------------------+----------------------------+

DiffStat object

+----------------+-------------------------------------------+
| ``files``      | Number of changed files                   |
+----------------+-------------------------------------------+
| ``dirs``       | Number of changed directories             |
+----------------+-------------------------------------------+
| ``others``     | Number of changed other directory entries |
+----------------+-------------------------------------------+
| ``data_blobs`` | Number of data blobs                      |
+----------------+-------------------------------------------+
| ``tree_blobs`` | Number of tree blobs                      |
+----------------+-------------------------------------------+
| ``bytes``      | Number of bytes                           |
+----------------+-------------------------------------------+


find
----

The ``find`` command outputs a single JSON document containing an array of JSON
objects with matches for your search term.  These matches are organized by snapshot.

If the ``--blob`` or ``--tree`` option is passed, then the output is an array of
Blob objects.


+-----------------+----------------------------------------------+
| ``hits``        | Number of matches in the snapshot            |
+-----------------+----------------------------------------------+
| ``snapshot``    | ID of the snapshot                           |
+-----------------+----------------------------------------------+
| ``matches``     | Array of Match objects detailing a match     |
+-----------------+----------------------------------------------+

Match object

+-----------------+----------------------------------------------+
| ``path``        | Object path                                  |
+-----------------+----------------------------------------------+
| ``permissions`` | UNIX permissions                             |
+-----------------+----------------------------------------------+
| ``type``        | Object type e.g. file, dir, etc...           |
+-----------------+----------------------------------------------+
| ``atime``       | Access time                                  |
+-----------------+----------------------------------------------+
| ``mtime``       | Modification time                            |
+-----------------+----------------------------------------------+
| ``ctime``       | Change time                                  |
+-----------------+----------------------------------------------+
| ``name``        | Object name                                  |
+-----------------+----------------------------------------------+
| ``user``        | Name of owner                                |
+-----------------+----------------------------------------------+
| ``group``       | Name of group                                |
+-----------------+----------------------------------------------+
| ``inode``       | Inode number                                 |
+-----------------+----------------------------------------------+
| ``mode``        | UNIX file mode, shorthand of ``permissions`` |
+-----------------+----------------------------------------------+
| ``device_id``   | OS specific device identifier                |
+-----------------+----------------------------------------------+
| ``links``       | Number of hardlinks                          |
+-----------------+----------------------------------------------+
| ``uid``         | ID of owner                                  |
+-----------------+----------------------------------------------+
| ``gid``         | ID of group                                  |
+-----------------+----------------------------------------------+
| ``size``        | Size of object in bytes                      |
+-----------------+----------------------------------------------+

Blob object

+-----------------+--------------------------------------------+
| ``object_type`` | Either "blob" or "tree"                    |
+-----------------+--------------------------------------------+
| ``id``          | ID of found blob                           |
+-----------------+--------------------------------------------+
| ``path``        | Path in snapshot                           |
+-----------------+--------------------------------------------+
| ``parent_tree`` | Parent tree blob, only set for type "blob" |
+-----------------+--------------------------------------------+
| ``snapshot``    | Snapshot ID                                |
+-----------------+--------------------------------------------+
| ``time``        | Snapshot timestamp                         |
+-----------------+--------------------------------------------+


forget
------

The ``forget`` command prints a single JSON document containing an array of
ForgetGroups. If specific snapshot IDs are specified, then no output is generated.

The ``prune`` command does not yet support JSON such that ``forget --prune``
results in a mix of JSON and text output.

ForgetGroup
^^^^^^^^^^^

+-------------+-----------------------------------------------------------+
| ``tags``    | Tags identifying the snapshot group                       |
+-------------+-----------------------------------------------------------+
| ``host``    | Host identifying the snapshot group                       |
+-------------+-----------------------------------------------------------+
| ``paths``   | Paths identifying the snapshot group                      |
+-------------+-----------------------------------------------------------+
| ``keep``    | Array of Snapshot objects that are kept                   |
+-------------+-----------------------------------------------------------+
| ``remove``  | Array of Snapshot objects that were removed               |
+-------------+-----------------------------------------------------------+
| ``reasons`` | Array of Reason objects describing why a snapshot is kept |
+-------------+-----------------------------------------------------------+

Snapshot object

+---------------------+--------------------------------------------------+
| ``time``            | Timestamp of when the backup was started         |
+---------------------+--------------------------------------------------+
| ``parent``          | ID of the parent snapshot                        |
+---------------------+--------------------------------------------------+
| ``tree``            | ID of the root tree blob                         |
+---------------------+--------------------------------------------------+
| ``paths``           | List of paths included in the backup             |
+---------------------+--------------------------------------------------+
| ``hostname``        | Hostname of the backed up machine                |
+---------------------+--------------------------------------------------+
| ``username``        | Username the backup command was run as           |
+---------------------+--------------------------------------------------+
| ``uid``             | ID of owner                                      |
+---------------------+--------------------------------------------------+
| ``gid``             | ID of group                                      |
+---------------------+--------------------------------------------------+
| ``excludes``        | List of paths and globs excluded from the backup |
+---------------------+--------------------------------------------------+
| ``tags``            | List of tags for the snapshot in question        |
+---------------------+--------------------------------------------------+
| ``program_version`` | restic version used to create snapshot           |
+---------------------+--------------------------------------------------+
| ``id``              | Snapshot ID                                      |
+---------------------+--------------------------------------------------+
| ``short_id``        | Snapshot ID, short form                          |
+---------------------+--------------------------------------------------+

Reason object

+----------------+-----------------------------------------------------------+
| ``snapshot``   | Snapshot object, including ``id`` and ``short_id`` fields |
+----------------+-----------------------------------------------------------+
| ``matches``    | Array containing descriptions of the matching criteria    |
+----------------+-----------------------------------------------------------+
| ``counters``   | Object containing counters used by the policies           |
+----------------+-----------------------------------------------------------+


init
----

The ``init`` command uses the JSON lines format, but only outputs a single message.

+------------------+--------------------------------+
| ``message_type`` | Always "initialized"           |
+------------------+--------------------------------+
| ``id``           | ID of the created repository   |
+------------------+--------------------------------+
| ``repository``   | URL of the repository          |
+------------------+--------------------------------+


key list
--------

The ``key list`` command returns an array of objects with the following structure.

+--------------+------------------------------------+
| ``current``  | Is currently used key?             |
+--------------+------------------------------------+
| ``id``       | Unique key ID                      |
+--------------+------------------------------------+
| ``userName`` | User who created it                |
+--------------+------------------------------------+
| ``hostName`` | Name of machine it was created on  |
+--------------+------------------------------------+
| ``created``  | Timestamp when it was created      |
+--------------+------------------------------------+


.. _ls json:

ls
--

The ``ls`` command uses the JSON lines format with the following message types.
As an exception, the ``struct_type`` field is used to determine the message type.

snapshot
^^^^^^^^

+------------------+--------------------------------------------------+
| ``message_type`` | Always "snapshot"                                |
+------------------+--------------------------------------------------+
| ``struct_type``  | Always "snapshot" (deprecated)                   |
+------------------+--------------------------------------------------+
| ``time``         | Timestamp of when the backup was started         |
+------------------+--------------------------------------------------+
| ``parent``       | ID of the parent snapshot                        |
+------------------+--------------------------------------------------+
| ``tree``         | ID of the root tree blob                         |
+------------------+--------------------------------------------------+
| ``paths``        | List of paths included in the backup             |
+------------------+--------------------------------------------------+
| ``hostname``     | Hostname of the backed up machine                |
+------------------+--------------------------------------------------+
| ``username``     | Username the backup command was run as           |
+------------------+--------------------------------------------------+
| ``uid``          | ID of owner                                      |
+------------------+--------------------------------------------------+
| ``gid``          | ID of group                                      |
+------------------+--------------------------------------------------+
| ``excludes``     | List of paths and globs excluded from the backup |
+------------------+--------------------------------------------------+
| ``tags``         | List of tags for the snapshot in question        |
+------------------+--------------------------------------------------+
| ``id``           | Snapshot ID                                      |
+------------------+--------------------------------------------------+
| ``short_id``     | Snapshot ID, short form                          |
+------------------+--------------------------------------------------+


node
^^^^

+------------------+----------------------------+
| ``message_type`` | Always "node"              |
+------------------+----------------------------+
| ``struct_type``  | Always "node" (deprecated) |
+------------------+----------------------------+
| ``name``         | Node name                  |
+------------------+----------------------------+
| ``type``         | Node type                  |
+------------------+----------------------------+
| ``path``         | Node path                  |
+------------------+----------------------------+
| ``uid``          | UID of node                |
+------------------+----------------------------+
| ``gid``          | GID of node                |
+------------------+----------------------------+
| ``size``         | Size in bytes              |
+------------------+----------------------------+
| ``mode``         | Node mode                  |
+------------------+----------------------------+
| ``atime``        | Node access time           |
+------------------+----------------------------+
| ``mtime``        | Node modification time     |
+------------------+----------------------------+
| ``ctime``        | Node creation time         |
+------------------+----------------------------+
| ``inode``        | Inode number of node       |
+------------------+----------------------------+


restore
-------

The ``restore`` command uses the JSON lines format with the following message types.

Status
^^^^^^

+----------------------+------------------------------------------------------------+
|``message_type``      | Always "status"                                            |
+----------------------+------------------------------------------------------------+
|``seconds_elapsed``   | Time since restore started                                 |
+----------------------+------------------------------------------------------------+
|``percent_done``      | Percentage of data restored (bytes_restored/total_bytes)   |
+----------------------+------------------------------------------------------------+
|``total_files``       | Total number of files detected                             |
+----------------------+------------------------------------------------------------+
|``files_restored``    | Files restored                                             |
+----------------------+------------------------------------------------------------+
|``files_skipped``     | Files skipped due to overwrite setting                     |
+----------------------+------------------------------------------------------------+
|``total_bytes``       | Total number of bytes in restore set                       |
+----------------------+------------------------------------------------------------+
|``bytes_restored``    | Number of bytes restored                                   |
+----------------------+------------------------------------------------------------+
|``bytes_skipped``     | Total size of skipped files                                |
+----------------------+------------------------------------------------------------+

Error
^^^^^

+----------------------+-------------------------------------------+
| ``message_type``     | Always "error"                            |
+----------------------+-------------------------------------------+
| ``error.message``    | Error message                             |
+----------------------+-------------------------------------------+
| ``during``           | Always "restore"                          |
+----------------------+-------------------------------------------+
| ``item``             | Usually, the path of the problematic file |
+----------------------+-------------------------------------------+

Verbose Status
^^^^^^^^^^^^^^

Verbose status provides details about the progress, including details about restored files.
Only printed if `--verbose=2` is specified.

+----------------------+-----------------------------------------------------------+
| ``message_type``     | Always "verbose_status"                                   |
+----------------------+-----------------------------------------------------------+
| ``action``           | Either "restored", "updated", "unchanged" or "deleted"    |
+----------------------+-----------------------------------------------------------+
| ``item``             | The item in question                                      |
+----------------------+-----------------------------------------------------------+
| ``size``             | Size of the item in bytes                                 |
+----------------------+-----------------------------------------------------------+

Summary
^^^^^^^

+----------------------+------------------------------------------------------------+
|``message_type``      | Always "summary"                                           |
+----------------------+------------------------------------------------------------+
|``seconds_elapsed``   | Time since restore started                                 |
+----------------------+------------------------------------------------------------+
|``total_files``       | Total number of files detected                             |
+----------------------+------------------------------------------------------------+
|``files_restored``    | Files restored                                             |
+----------------------+------------------------------------------------------------+
|``files_skipped``     | Files skipped due to overwrite setting                     |
+----------------------+------------------------------------------------------------+
|``total_bytes``       | Total number of bytes in restore set                       |
+----------------------+------------------------------------------------------------+
|``bytes_restored``    | Number of bytes restored                                   |
+----------------------+------------------------------------------------------------+
|``bytes_skipped``     | Total size of skipped files                                |
+----------------------+------------------------------------------------------------+


snapshots
---------

The snapshots command returns a single JSON object, an array with objects of the structure outlined below.

+---------------------+--------------------------------------------------+
| ``time``            | Timestamp of when the backup was started         |
+---------------------+--------------------------------------------------+
| ``parent``          | ID of the parent snapshot                        |
+---------------------+--------------------------------------------------+
| ``tree``            | ID of the root tree blob                         |
+---------------------+--------------------------------------------------+
| ``paths``           | List of paths included in the backup             |
+---------------------+--------------------------------------------------+
| ``hostname``        | Hostname of the backed up machine                |
+---------------------+--------------------------------------------------+
| ``username``        | Username the backup command was run as           |
+---------------------+--------------------------------------------------+
| ``uid``             | ID of owner                                      |
+---------------------+--------------------------------------------------+
| ``gid``             | ID of group                                      |
+---------------------+--------------------------------------------------+
| ``excludes``        | List of paths and globs excluded from the backup |
+---------------------+--------------------------------------------------+
| ``tags``            | List of tags for the snapshot in question        |
+---------------------+--------------------------------------------------+
| ``program_version`` | restic version used to create snapshot           |
+---------------------+--------------------------------------------------+
| ``summary``         | Snapshot statistics, see "Summary object"        |
+---------------------+--------------------------------------------------+
| ``id``              | Snapshot ID                                      |
+---------------------+--------------------------------------------------+
| ``short_id``        | Snapshot ID, short form                          |
+---------------------+--------------------------------------------------+

Summary object

The contained statistics reflect the information at the point in time when the snapshot
was created.

+---------------------------+---------------------------------------------------------+
| ``backup_start``          | Time at which the backup was started                    |
+---------------------------+---------------------------------------------------------+
| ``backup_end``            | Time at which the backup was completed                  |
+---------------------------+---------------------------------------------------------+
| ``files_new``             | Number of new files                                     |
+---------------------------+---------------------------------------------------------+
| ``files_changed``         | Number of files that changed                            |
+---------------------------+---------------------------------------------------------+
| ``files_unmodified``      | Number of files that did not change                     |
+---------------------------+---------------------------------------------------------+
| ``dirs_new``              | Number of new directories                               |
+---------------------------+---------------------------------------------------------+
| ``dirs_changed``          | Number of directories that changed                      |
+---------------------------+---------------------------------------------------------+
| ``dirs_unmodified``       | Number of directories that did not change               |
+---------------------------+---------------------------------------------------------+
| ``data_blobs``            | Number of data blobs                                    |
+---------------------------+---------------------------------------------------------+
| ``tree_blobs``            | Number of tree blobs                                    |
+---------------------------+---------------------------------------------------------+
| ``data_added``            | Amount of (uncompressed) data added, in bytes           |
+---------------------------+---------------------------------------------------------+
| ``data_added_packed``     | Amount of data added (after compression), in bytes      |
+---------------------------+---------------------------------------------------------+
| ``total_files_processed`` | Total number of files processed                         |
+---------------------------+---------------------------------------------------------+
| ``total_bytes_processed`` | Total number of bytes processed                         |
+---------------------------+---------------------------------------------------------+


stats
-----

The stats command returns a single JSON object.

+------------------------------+-----------------------------------------------------+
| ``total_size``               | Repository size in bytes                            |
+------------------------------+-----------------------------------------------------+
| ``total_file_count``         | Number of files backed up in the repository         |
+------------------------------+-----------------------------------------------------+
| ``total_blob_count``         | Number of blobs in the repository                   |
+------------------------------+-----------------------------------------------------+
| ``snapshots_count``          | Number of processed snapshots                       |
+------------------------------+-----------------------------------------------------+
| ``total_uncompressed_size``  | Repository size in bytes if blobs were uncompressed |
+------------------------------+-----------------------------------------------------+
| ``compression_ratio``        | Factor by which the already compressed data         |
|                              | has shrunk due to compression                       |
+------------------------------+-----------------------------------------------------+
| ``compression_progress``     | Percentage of already compressed data               |
+------------------------------+-----------------------------------------------------+
| ``compression_space_saving`` | Overall space saving due to compression             |
+------------------------------+-----------------------------------------------------+


version
-------

The version command returns a single JSON object.

+----------------+--------------------+
| ``version``    | restic version     |
+----------------+--------------------+
| ``go_version`` | Go compile version |
+----------------+--------------------+
| ``go_os``      | Go OS              |
+----------------+--------------------+
| ``go_arch``    | Go architecture    |
+----------------+--------------------+
