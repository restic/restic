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

Restic and json
***************

Restic can output json data if requested with the ``--json`` flag.
The structure of that data varies depending on the circumstance.  The
json output of Most restic commands are documented here.

.. note::
    Not all commands support json output.  If a command does not support json output,
    at the time of writing, it is not supported yet. (feel free to submit a pull request!)

Backup
------

backup has multiple json structures, outlined below.

Status
^^^^^^

+----------------------+---------------------------------------------------------+
|``message_type``      | always "status"                                         |
+----------------------+---------------------------------------------------------+
|``seconds_elapsed``   | Time since backup started                               |
+----------------------+---------------------------------------------------------+
|``seconds_remaining`` | Estimated time remaining                                |
+----------------------+---------------------------------------------------------+
|``percent_done``      | Percentage of data backed up.  (bytes_done/total_bytes) |
+----------------------+---------------------------------------------------------+
|``total_files``       | Total number of files detected                          |
+----------------------+---------------------------------------------------------+
|``files_done``        | Files completed (backed up or skipped)                  |
+----------------------+---------------------------------------------------------+
|``total_bytes``       | Total number of bytes in backup set                     |
+----------------------+---------------------------------------------------------+
|``bytes_done``        | Number of bytes completed                               |
+----------------------+---------------------------------------------------------+
|``error_count``       | Number of errors                                        |
+----------------------+---------------------------------------------------------+
|``current_files``     | List of files currently being backed up                 |
+----------------------+---------------------------------------------------------+

Error
^^^^^

+----------------------+--------------------------------+
| ``message_type``     | always "error"                 |
+----------------------+--------------------------------+
| ``error``            | error message                  |
+----------------------+--------------------------------+
| ``during``           | what restic was trying to do   |
+----------------------+--------------------------------+
| ``item``             | what item was being processed  |
+----------------------+--------------------------------+

Verbose Status
^^^^^^^^^^^^^^

+----------------------+-------------------------------------------+
| ``message_type``     | Always "verbose_status"                   |
+----------------------+-------------------------------------------+
| ``action``           | Either "new", "unchanged" or "modified"   |
+----------------------+-------------------------------------------+
| ``item``             | The item in question                      |
+----------------------+-------------------------------------------+
| ``duration``         | How long it took, in seconds              |
+----------------------+-------------------------------------------+
| ``data_size``        | How big item is                           |
+----------------------+-------------------------------------------+
| ``metadata_size``    | How big the metadata is                   |
+----------------------+-------------------------------------------+
| ``total_files``      | how many total files there are.           |
+----------------------+-------------------------------------------+

Summary
^^^^^^^

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
| ``snapshot_id``           | the ID of the new snapshot                              |
+---------------------------+---------------------------------------------------------+

snapshots
---------

Snapshots returns a single json structure with a number of optional fields.

+----------------+------------------------------------------------------------------------+
| ``hostname``   | contains the hostname of the machine that's being backed up.           |
+----------------+------------------------------------------------------------------------+
| ``username``   | contains the username that the backup command was run as.              |
+----------------+------------------------------------------------------------------------+
| ``excludes``   | contains a list of paths and globs that were excluded from the backup. |
+----------------+------------------------------------------------------------------------+
| ``tags``       | contains a list of tags for the snapshot in question.                  |
+----------------+------------------------------------------------------------------------+
| ``id``         | contains the long snapshot id.                                         |
+----------------+------------------------------------------------------------------------+
| ``short_id``   | contains the short snapshot id.                                        |
+----------------+------------------------------------------------------------------------+
| ``time``       | contains the timestamp of the backup.                                  |
+----------------+------------------------------------------------------------------------+
| ``parent``     | contains the id of the previous backup.                                |
+----------------+------------------------------------------------------------------------+
| ``tree``       | contains something...                                                  |
+----------------+------------------------------------------------------------------------+
| ``paths``      | contains a list of paths that were included in the backup.             |
+----------------+------------------------------------------------------------------------+

cat
---

Cat will return data about various objects in the repository, already in json form.
By specifying ``--json``, it will suppress any non-json messages the command generates.

find
----


+-----------------+------------------------------------------+
| ``path``        | Object path                              |
+-----------------+------------------------------------------+
| ``permissions`` | unix permissions                         |
+-----------------+------------------------------------------+
| ``type``        | what type it is e.g. file, dir, etc...   |
+-----------------+------------------------------------------+
| ``atime``       | Access time                              |
+-----------------+------------------------------------------+
| ``mtime``       | Modification time                        |
+-----------------+------------------------------------------+
| ``ctime``       | Creation time                            |
+-----------------+------------------------------------------+
| ``name``        | Object name                              |
+-----------------+------------------------------------------+
| ``user``        | Name of owner                            |
+-----------------+------------------------------------------+
| ``group``       | Name of group                            |
+-----------------+------------------------------------------+
| ``uid``         | ID of owner                              |
+-----------------+------------------------------------------+
| ``gid``         | ID of group                              |
+-----------------+------------------------------------------+
| ``size``        | size of object in bytes                  |
+-----------------+------------------------------------------+

key list
--------

+--------------+------------------------------------+
| ``current``  | Is currently used key?             |
+--------------+------------------------------------+
| ``id``       | Unique key ID                      |
+--------------+------------------------------------+
| ``userName`` | user who created it                |
+--------------+------------------------------------+
| ``hostName`` | name of machine it was created on  |
+--------------+------------------------------------+
| ``created``  | timestamp when it was created      |
+--------------+------------------------------------+

ls
--

snapshot
^^^^^^^^

+-----------------+-------------------------------------+
| ``time``        | Snapshot time                       |
+-----------------+-------------------------------------+
| ``tree``        | Snapshot tree root                  |
+-----------------+-------------------------------------+
| ``paths``       | List of paths included in snapshot  |
+-----------------+-------------------------------------+
| ``hostname``    | hostname of snapshot                |
+-----------------+-------------------------------------+
| ``username``    | user snapshot was run as            |
+-----------------+-------------------------------------+
| ``uid``         | uid of backup process               |
+-----------------+-------------------------------------+
| ``gid``         | gid of backup process               |
+-----------------+-------------------------------------+
| ``id``          | snapshot id, long form              |
+-----------------+-------------------------------------+
| ``short_id``    | snapshot id, short form             |
+-----------------+-------------------------------------+
| ``struct_type`` | always "snapshot"                   |
+-----------------+-------------------------------------+


node
^^^^

+-----------------+--------------------------+
| ``name``        | node name                |
+-----------------+--------------------------+
| ``type``        | node type                |
+-----------------+--------------------------+
| ``path``        | node path                |
+-----------------+--------------------------+
| ``uid``         | uid of node              |
+-----------------+--------------------------+
| ``gid``         | gid of node              |
+-----------------+--------------------------+
| ``size``        | size in bytes            |
+-----------------+--------------------------+
| ``mode``        | node mode                |
+-----------------+--------------------------+
| ``atime``       | node access time         |
+-----------------+--------------------------+
| ``mtime``       | node modification time   |
+-----------------+--------------------------+
| ``ctime``       | node creation time       |
+-----------------+--------------------------+
| ``struct_type`` | always "node"            |
+-----------------+--------------------------+

stats
-----

+----------------------+---------------------------------------------+
| ``total_size``       | Repository size in bytes                    |
+----------------------+---------------------------------------------+
| ``total_file_count`` | Number of files backed up in the repository |
+----------------------+---------------------------------------------+
