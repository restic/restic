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
perhaps to prevent your script from initializing a repository multiple
times. The command ``cat config`` may be used for this purpose:

.. code-block:: console

    $ restic -r /srv/restic-repo cat config
    Fatal: unable to open config file: stat /srv/restic-repo/config: no such file or directory
    Is there a repository at the following location?
    /srv/restic-repo

If a repository does not exist, restic will return a non-zero exit code
and print an error message. Note that restic will also return a non-zero
exit code if a different error is encountered (e.g.: incorrect password
to ``cat config``) and it may print a different error message. If there
are no errors, restic will return a zero exit code and print the repository
metadata.

Restic and JSON
***************

Restic can output json data if requested with the ``--json`` flag.
The structure of that data varies depending on the circumstance.  The
json output of most restic commands are documented here.

.. note::
    Not all commands support json output.  If a command does not support json output,
    feel free to submit a pull request!

Backup
------

The backup command has multiple json structures, outlined below.

During the backup process, Restic will print out a stream of new-line separated JSON
messages.  You can determine the nature of the message by the ``message_type`` field.

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
|``files_done``        | Files completed (backed up or confirmed in repo)           |
+----------------------+------------------------------------------------------------+
|``total_bytes``       | Total number of bytes in backup set                        |
+----------------------+------------------------------------------------------------+
|``bytes_done``        | Number of bytes completed (backed up or confirmed in repo) |
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
| ``error``            | Error message                             |
+----------------------+-------------------------------------------+
| ``during``           | What restic was trying to do              |
+----------------------+-------------------------------------------+
| ``item``             | Usually, the path of the problematic file |
+----------------------+-------------------------------------------+

Verbose Status
^^^^^^^^^^^^^^

Verbose status is a status line that supplements 

+----------------------+-------------------------------------------+
| ``message_type``     | Always "verbose_status"                   |
+----------------------+-------------------------------------------+
| ``action``           | Either "new", "unchanged" or "modified"   |
+----------------------+-------------------------------------------+
| ``item``             | The item in question                      |
+----------------------+-------------------------------------------+
| ``duration``         | How long it took, in seconds              |
+----------------------+-------------------------------------------+
| ``data_size``        | How big the item is                       |
+----------------------+-------------------------------------------+
| ``metadata_size``    | How big the metadata is                   |
+----------------------+-------------------------------------------+
| ``total_files``      | Total number of files                     |
+----------------------+-------------------------------------------+

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
| ``data_added``            | Amount of data added, in bytes                          |
+---------------------------+---------------------------------------------------------+
| ``total_files_processed`` | Total number of files processed                         |
+---------------------------+---------------------------------------------------------+
| ``total_bytes_processed`` | Total number of bytes processed                         |
+---------------------------+---------------------------------------------------------+
| ``total_duration``        | Total time it took for the operation to complete        |
+---------------------------+---------------------------------------------------------+
| ``snapshot_id``           | The short ID of the new snapshot                        |
+---------------------------+---------------------------------------------------------+

snapshots
---------

The snapshots command returns a single JSON object, an array with the structure outlined below.

+----------------+------------------------------------------------------------------------+
| ``hostname``   | The hostname of the machine that's being backed up                     |
+----------------+------------------------------------------------------------------------+
| ``username``   | The username that the backup command was run as                        |
+----------------+------------------------------------------------------------------------+
| ``excludes``   | A list of paths and globs that were excluded from the backup           |
+----------------+------------------------------------------------------------------------+
| ``tags``       | A list of tags for the snapshot in question                            |
+----------------+------------------------------------------------------------------------+
| ``id``         | The long snapshot ID                                                   |
+----------------+------------------------------------------------------------------------+
| ``short_id``   | The short snapshot ID                                                  |
+----------------+------------------------------------------------------------------------+
| ``time``       | The timestamp of when the backup was started                           |
+----------------+------------------------------------------------------------------------+
| ``parent``     | The ID of the previous snapshot                                        |
+----------------+------------------------------------------------------------------------+
| ``tree``       | The ID of the root tree blob                                           |
+----------------+------------------------------------------------------------------------+
| ``paths``      | A list of paths that were included in the backup                       |
+----------------+------------------------------------------------------------------------+

cat
---

Cat will return data about various objects in the repository, already in json form.
By specifying ``--json``, it will suppress any non-json messages the command generates.

find
----

The find command outputs an array of json objects with matches for your search term.  These
matches are organized by snapshot.

Snapshot
^^^^^^^^

+-----------------+----------------------------------------------+
| ``hits``        | The number of matches in the snapshot        |
+-----------------+----------------------------------------------+
| ``snapshot``    | The long ID of the snapshot                  |
+-----------------+----------------------------------------------+
| ``matches``     | Array of JSON objects detailing a match.     |
+-----------------+----------------------------------------------+


Match
^^^^^

+-----------------+----------------------------------------------+
| ``path``        | Object path                                  |
+-----------------+----------------------------------------------+
| ``permissions`` | UNIX permissions                             |
+-----------------+----------------------------------------------+
| ``type``        | what type it is e.g. file, dir, etc...       |
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
| ``mode``        | UNIX file mode, shorthand of ``permissions`` |
+-----------------+----------------------------------------------+
| ``device_id``   | Unique machine Identifier                    |
+-----------------+----------------------------------------------+
| ``links``       | Number of hardlinks                          |
+-----------------+----------------------------------------------+
| ``uid``         | ID of owner                                  |
+-----------------+----------------------------------------------+
| ``gid``         | ID of group                                  |
+-----------------+----------------------------------------------+
| ``size``        | Size of object in bytes                      |
+-----------------+----------------------------------------------+

key list
--------

The key list command returns an array of objects with the following structure.

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

ls
--

The ls command spits out a series of newline-separated JSON objects,
the nature of which can be determined by the ``struct_type`` field.

snapshot
^^^^^^^^

+-----------------+-------------------------------------+
| ``time``        | Snapshot time                       |
+-----------------+-------------------------------------+
| ``tree``        | Snapshot tree root                  |
+-----------------+-------------------------------------+
| ``paths``       | List of paths included in snapshot  |
+-----------------+-------------------------------------+
| ``hostname``    | Hostname of snapshot                |
+-----------------+-------------------------------------+
| ``username``    | User snapshot was run as            |
+-----------------+-------------------------------------+
| ``uid``         | ID of owner                         |
+-----------------+-------------------------------------+
| ``gid``         | ID of group                         |
+-----------------+-------------------------------------+
| ``id``          | Snapshot ID, long form              |
+-----------------+-------------------------------------+
| ``short_id``    | Snapshot ID, short form             |
+-----------------+-------------------------------------+
| ``struct_type`` | Always "snapshot"                   |
+-----------------+-------------------------------------+


node
^^^^

+-----------------+--------------------------+
| ``name``        | Node name                |
+-----------------+--------------------------+
| ``type``        | Node type                |
+-----------------+--------------------------+
| ``path``        | Node path                |
+-----------------+--------------------------+
| ``uid``         | UID of node              |
+-----------------+--------------------------+
| ``gid``         | GID of node              |
+-----------------+--------------------------+
| ``size``        | Size in bytes            |
+-----------------+--------------------------+
| ``mode``        | Node mode                |
+-----------------+--------------------------+
| ``atime``       | Node access time         |
+-----------------+--------------------------+
| ``mtime``       | Node modification time   |
+-----------------+--------------------------+
| ``ctime``       | Node creation time       |
+-----------------+--------------------------+
| ``struct_type`` | Always "node"            |
+-----------------+--------------------------+

stats
-----

+----------------------+---------------------------------------------+
| ``total_size``       | Repository size in bytes                    |
+----------------------+---------------------------------------------+
| ``total_file_count`` | Number of files backed up in the repository |
+----------------------+---------------------------------------------+
| ``total_blob_count`` | Number of blobs in the repository           |
+----------------------+---------------------------------------------+
