..
  Normally, there are no heading levels assigned to certain characters as the structure is
  determined from the succession of headings. However, this convention is used in Python's
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

This section covers environment variables and how certain tasks may be accomplished
when you use restic via scripts.

.. _environment-variables:

Environment Variables
*********************

In addition to command-line options, restic supports passing various options in
environment variables, which are listed below.

.. code-block:: console

    RESTIC_REPOSITORY_FILE              Name of file containing the repository location (replaces --repository-file)
    RESTIC_REPOSITORY                   Location of repository (replaces -r)
    RESTIC_PASSWORD_FILE                Location of password file (replaces --password-file)
    RESTIC_PASSWORD                     The actual password for the repository
    RESTIC_PASSWORD_COMMAND             Command printing the password for the repository to stdout
    RESTIC_KEY_HINT                     ID of key to try decrypting first, before other keys
    RESTIC_CACERT                       Location(s) of certificate file(s), comma separated if multiple (replaces --cacert)
    RESTIC_TLS_CLIENT_CERT              Location of TLS client certificate and private key (replaces --tls-client-cert)
    RESTIC_CACHE_DIR                    Location of the cache directory
    RESTIC_COMPRESSION                  Compression mode (only available for repository format version 2)
    RESTIC_HOST                         Only consider snapshots for this host / Set the hostname for the snapshot manually (replaces --host)
    RESTIC_PROGRESS_FPS                 Frames per second by which the progress bar is updated
    RESTIC_PACK_SIZE                    Target size for pack files
    RESTIC_READ_CONCURRENCY             Concurrency for file reads

    TMPDIR                              Location for temporary files (except Windows)
    TMP                                 Location for temporary files (only Windows)

    AWS_ACCESS_KEY_ID                   Amazon S3 access key ID
    AWS_SECRET_ACCESS_KEY               Amazon S3 secret access key
    AWS_SESSION_TOKEN                   Amazon S3 temporary session token
    AWS_DEFAULT_REGION                  Amazon S3 default region
    AWS_PROFILE                         Amazon credentials profile (alternative to specifying key and region)
    AWS_SHARED_CREDENTIALS_FILE         Location of the AWS CLI shared credentials file (default: ~/.aws/credentials)
    RESTIC_AWS_ASSUME_ROLE_ARN          Amazon IAM Role ARN to assume using discovered credentials
    RESTIC_AWS_ASSUME_ROLE_SESSION_NAME Session Name to use with the role assumption
    RESTIC_AWS_ASSUME_ROLE_EXTERNAL_ID  External ID to use with the role assumption
    RESTIC_AWS_ASSUME_ROLE_POLICY       Inline Amazion IAM session policy
    RESTIC_AWS_ASSUME_ROLE_REGION       Region to use for IAM calls for the role assumption (default: us-east-1)
    RESTIC_AWS_ASSUME_ROLE_STS_ENDPOINT URL to the STS endpoint (default is determined based on RESTIC_AWS_ASSUME_ROLE_REGION). You generally do not need to set this, advanced use only.

    AZURE_ACCOUNT_NAME                  Account name for Azure
    AZURE_ACCOUNT_KEY                   Account key for Azure
    AZURE_ACCOUNT_SAS                   Shared access signatures (SAS) for Azure
    AZURE_ENDPOINT_SUFFIX               Endpoint suffix for Azure Storage (default: core.windows.net)
    AZURE_FORCE_CLI_CREDENTIAL          Force the use of Azure CLI credentials for authentication

    B2_ACCOUNT_ID                       Account ID or applicationKeyId for Backblaze B2
    B2_ACCOUNT_KEY                      Account Key or applicationKey for Backblaze B2

    GOOGLE_PROJECT_ID                   Project ID for Google Cloud Storage
    GOOGLE_APPLICATION_CREDENTIALS      Application Credentials for Google Cloud Storage (e.g. $HOME/.config/gs-secret-restic-key.json)

    OS_AUTH_URL                         Auth URL for keystone authentication
    OS_REGION_NAME                      Region name for keystone authentication
    OS_USERNAME                         Username for keystone authentication
    OS_USER_ID                          User ID for keystone v3 authentication
    OS_PASSWORD                         Password for keystone authentication
    OS_TENANT_ID                        Tenant ID for keystone v2 authentication
    OS_TENANT_NAME                      Tenant name for keystone v2 authentication

    OS_USER_DOMAIN_NAME                 User domain name for keystone authentication
    OS_USER_DOMAIN_ID                   User domain ID for keystone v3 authentication
    OS_PROJECT_NAME                     Project name for keystone authentication
    OS_PROJECT_DOMAIN_NAME              Project domain name for keystone authentication
    OS_PROJECT_DOMAIN_ID                Project domain ID for keystone v3 authentication
    OS_TRUST_ID                         Trust ID for keystone v3 authentication

    OS_APPLICATION_CREDENTIAL_ID        Application Credential ID (keystone v3)
    OS_APPLICATION_CREDENTIAL_NAME      Application Credential Name (keystone v3)
    OS_APPLICATION_CREDENTIAL_SECRET    Application Credential Secret (keystone v3)

    OS_STORAGE_URL                      Storage URL for token authentication
    OS_AUTH_TOKEN                       Auth token for token authentication

    RCLONE_BWLIMIT                      rclone bandwidth limit

    RESTIC_REST_USERNAME                Restic REST Server username
    RESTIC_REST_PASSWORD                Restic REST Server password

    ST_AUTH                             Auth URL for keystone v1 authentication
    ST_USER                             Username for keystone v1 authentication
    ST_KEY                              Password for keystone v1 authentication

See :ref:`caching` for the rules concerning cache locations when
``RESTIC_CACHE_DIR`` is not set.

The external programs that restic may execute include ``rclone`` (for rclone
backends) and ``ssh`` (for the SFTP backend). These may respond to further
environment variables and configuration files; see their respective manuals.

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

.. _exit-codes:

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
| 12  | Wrong password (since restic 0.17.1)               |
+-----+----------------------------------------------------+
| 130 | Restic was interrupted using SIGINT or SIGSTOP     |
+-----+----------------------------------------------------+

.. _JSON output:

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


Exit errors
-----------

Fatal errors will result in a final JSON message on ``stderr`` before the process exits.
It will hold the error message and the exit code.

.. note::
    Some errors cannot be caught and reported this way,
    such as Go runtime errors or command line parsing errors.

+------------------+-----------------------------+--------+
| ``message_type`` | Always "exit_error"         | string |
+------------------+-----------------------------+--------+
| ``code``         | Exit code (see above chart) | int    |
+------------------+-----------------------------+--------+
| ``message``      | Error message               | string |
+------------------+-----------------------------+--------+

Output formats
--------------

Commands print their main JSON output on ``stdout``.
The generated JSON output uses one of the following two formats.

.. note::
    Not all messages and errors have been converted to JSON yet.
    Feel free to submit a pull request!

The datatypes specified in the following sections roughly correspond to the underlying
Go types and are mapped to the JSON types as follows:

- ``int32``, ``int64``, ``uint32``, ``uint64`` and ``float64`` are encoded as number.
- ``bool`` and ``string`` correspond to the respective types.
- ``[]`` in front of a type indicates that the field is an array of the respective type.
- ``time.Time`` is encoded as a string in RFC3339 format.
- ``os.FileMode`` is encoded as an ``uint32``.

If a field contains a default value like ``0`` or ``""``, it may be omitted from the JSON output.

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

+-----------------------+-----------------------------------------------------+----------+
| ``message_type``      | Always "status"                                     | string   |
+-----------------------+-----------------------------------------------------+----------+
| ``seconds_elapsed``   | Time since backup started                           | uint64   |
+-----------------------+-----------------------------------------------------+----------+
| ``seconds_remaining`` | Estimated time remaining                            | uint64   |
+-----------------------+-----------------------------------------------------+----------+
| ``percent_done``      | Fraction of data backed up (bytes_done/total_bytes) | float64  |
+-----------------------+-----------------------------------------------------+----------+
| ``total_files``       | Total number of files detected                      | uint64   |
+-----------------------+-----------------------------------------------------+----------+
| ``files_done``        | Files completed (backed up to repo)                 | uint64   |
+-----------------------+-----------------------------------------------------+----------+
| ``total_bytes``       | Total number of bytes in backup set                 | uint64   |
+-----------------------+-----------------------------------------------------+----------+
| ``bytes_done``        | Number of bytes completed (backed up to repo)       | uint64   |
+-----------------------+-----------------------------------------------------+----------+
| ``error_count``       | Number of errors                                    | uint64   |
+-----------------------+-----------------------------------------------------+----------+
| ``current_files``     | List of files currently being backed up             | []string |
+-----------------------+-----------------------------------------------------+----------+

Error
^^^^^

These errors are printed on ``stderr``.

+-------------------+-------------------------------------------+--------+
| ``message_type``  | Always "error"                            | string |
+-------------------+-------------------------------------------+--------+
| ``error.message`` | Error message                             | string |
+-------------------+-------------------------------------------+--------+
| ``during``        | What restic was trying to do              | string |
+-------------------+-------------------------------------------+--------+
| ``item``          | Usually, the path of the problematic file | string |
+-------------------+-------------------------------------------+--------+

Verbose Status
^^^^^^^^^^^^^^

Verbose status provides details about the progress, including details about backed up files.

+---------------------------+----------------------------------------------------------+---------+
| ``message_type``          | Always "verbose_status"                                  | string  |
+---------------------------+----------------------------------------------------------+---------+
| ``action``                | Either "new", "unchanged", "modified" or "scan_finished" | string  |
+---------------------------+----------------------------------------------------------+---------+
| ``item``                  | The item in question                                     | string  |
+---------------------------+----------------------------------------------------------+---------+
| ``duration``              | How long it took, in seconds                             | float64 |
+---------------------------+----------------------------------------------------------+---------+
| ``data_size``             | How big the item is                                      | uint64  |
+---------------------------+----------------------------------------------------------+---------+
| ``data_size_in_repo``     | How big the item is in the repository                    | uint64  |
+---------------------------+----------------------------------------------------------+---------+
| ``metadata_size``         | How big the metadata is                                  | uint64  |
+---------------------------+----------------------------------------------------------+---------+
| ``metadata_size_in_repo`` | How big the metadata is in the repository                | uint64  |
+---------------------------+----------------------------------------------------------+---------+
| ``total_files``           | Total number of files                                    | uint64  |
+---------------------------+----------------------------------------------------------+---------+

Summary
^^^^^^^

Summary is the last output line in a successful backup.

+---------------------------+------------------------------------------------------+-----------+
| ``message_type``          | Always "summary"                                     | string    |
+---------------------------+------------------------------------------------------+-----------+
| ``dry_run``               | Whether the backup was a dry run                     | bool      |
+---------------------------+------------------------------------------------------+-----------+
| ``files_new``             | Number of new files                                  | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``files_changed``         | Number of files that changed                         | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``files_unmodified``      | Number of files that did not change                  | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``dirs_new``              | Number of new directories                            | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``dirs_changed``          | Number of directories that changed                   | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``dirs_unmodified``       | Number of directories that did not change            | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``data_blobs``            | Number of data blobs added                           | int64     |
+---------------------------+------------------------------------------------------+-----------+
| ``tree_blobs``            | Number of tree blobs added                           | int64     |
+---------------------------+------------------------------------------------------+-----------+
| ``data_added``            | Amount of (uncompressed) data added, in bytes        | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``data_added_packed``     | Amount of data added (after compression), in bytes   | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``total_files_processed`` | Total number of files processed                      | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``total_bytes_processed`` | Total number of bytes processed                      | uint64    |
+---------------------------+------------------------------------------------------+-----------+
| ``backup_start``          | Time at which the backup was started                 | time.Time |
+---------------------------+------------------------------------------------------+-----------+
| ``backup_end``            | Time at which the backup was completed               | time.Time |
+---------------------------+------------------------------------------------------+-----------+
| ``total_duration``        | Total time it took for the operation to complete     | float64   |
+---------------------------+------------------------------------------------------+-----------+
| ``snapshot_id``           | ID of the new snapshot. Field is omitted if snapshot | string    |
|                           | creation was skipped                                 |           |
+---------------------------+------------------------------------------------------+-----------+


cat
---

The ``cat`` command is used to inspect and print internal repository objects to stdout.
This is primarily useful for debugging, understanding repository structure, or
recovering data from a damaged repository. The command supports the following object types:

masterkey
^^^^^^^^^

Prints the master encryption key in JSON format. This contains the master
encryption and message authentication keys for the repository (encoded in Base64).
No additional ID argument is required.

Example::

    $ restic -r /srv/restic-repo cat masterkey
    repository c528f271 opened (version 2, compression level auto)
    {
      "mac": {
        "k": "...omitted base64...",
        "r": "...omitted base64..."
      },
      "encrypt": "...omitted base64..."
    }

config
^^^^^^

Prints the repository configuration in JSON format. This includes settings such as
the repository version, chunker polynomial.
No additional ID argument is required.

Example::

    $ restic -r /srv/restic-repo cat config
    repository c528f271 opened (version 2, compression level auto)
    {
      "version": 2,
      "id": "c528f27103c1cfc08ca3df9331c5b04a73b9c6822ae30f33bbf1d9342b9a1bf0",
      "chunker_polynomial": "255b9ca195d755"
    }

snapshot ID
^^^^^^^^^^^

Prints the metadata for a specific snapshot in JSON format. The snapshot ID
can be the full snapshot ID or a unique prefix. The output includes the
snapshot timestamp, paths, tags, hostname, username, and the root tree ID, and a number of additional .

Example::

    $ restic -r /srv/restic-repo cat snapshot 251c2e58
    repository c528f271 opened (version 2, compression level auto)
    {
      "time": "2026-02-19T22:44:34.833377676-08:00",
      "parent": "b204fa5c0a8d3cd37fd97b23d0bf20e9c1c0a2144c609ecbf5ce62bb264e0e14",
      "tree": "eff44e81e07d4ffbb50c5e7012b4d74fce63c19791cab62a6e8df387fd71e1dd",
      "paths": [
        "/home/myuser/test-assets"
      ],
      "hostname": "myhost",
      "username": "myuser",
      "uid": 1000,
      "gid": 1000,
      "tags": [
        "assets"
      ],
      "program_version": "restic 0.18.1",
      "summary": {
        "backup_start": "2026-02-19T22:44:34.833377676-08:00",
        "backup_end": "2026-02-19T22:44:35.510749043-08:00",
        "files_new": 0,
        "files_changed": 0,
        "files_unmodified": 32,
        "dirs_new": 0,
        "dirs_changed": 0,
        "dirs_unmodified": 17,
        "data_blobs": 0,
        "tree_blobs": 0,
        "data_added": 0,
        "data_added_packed": 0,
        "total_files_processed": 32,
        "total_bytes_processed": 2525938
      }
    }

tree snapshot[:subfolder]
^^^^^^^^^^^^^^^^^^^^^^^^^

Prints the tree structure for a snapshot, optionally limited to a subfolder.
This outputs the tree in JSON format. The tree contains nodes representing files
and directories. To view the tree structure in a human-readable format, you can
use the ``restic ls`` command instead. For better readability, pipe the output
to ``jq``.

Example::

    # Print the root tree of a snapshot
    $ restic -r /srv/restic-repo cat tree latest | jq .
    {
      "nodes": [
        {
          "name": "test-assets",
          "type": "dir",
          "mode": 2147484141,
          "mtime": "2026-02-19T22:14:58.41375581-08:00",
          "atime": "2026-02-19T22:14:58.41375581-08:00",
          "ctime": "2026-02-19T22:14:58.41375581-08:00",
          "uid": 1000,
          "gid": 1000,
          "user": "myuser",
          "group": "mygroup",
          "inode": 1698305,
          "device_id": 41,
          "content": null,
          "subtree": "2f15b17652357a2be758e58a76549eda0f7fb155d9a7bf3081234d8a028601ac"
        }
      ]
    }

    # Print a specific subfolder within a snapshot
    $ restic -r /srv/restic-repo cat tree latest:subfolder/path | jq .
    {
      "nodes": [
        {
          "name": ".git",
          "type": "dir",
          "mode": 2147484141,
          "mtime": "2026-02-19T22:14:58.415878457-08:00",
          "atime": "2026-02-19T22:14:58.415878457-08:00",
          "ctime": "2026-02-19T22:14:58.415878457-08:00",
          "uid": 1000,
          "gid": 1000,
          "user": "myuser",
          "group": "mygroup",
          "inode": 1698306,
          "device_id": 41,
          "content": null,
          "subtree": "7e85cd4922528fac27879965a4ffa64d52ad912b14f1e23e788f5de432ed2072"
        },
        {
          "name": "README.md",
          "type": "file",
          "mode": 420,
          "mtime": "2026-02-19T22:14:58.411265656-08:00",
          "atime": "2026-02-19T22:14:58.411265656-08:00",
          "ctime": "2026-02-19T22:14:58.411265656-08:00",
          "uid": 1000,
          "gid": 1000,
          "user": "myuser",
          "group": "mygroup",
          "inode": 1698370,
          "device_id": 41,
          "size": 50,
          "links": 1,
          "content": [
            "0499644cc8e5f947be5df73c15b673b96067631d213c751b51951d65fad7b3f4"
          ]
        },
        ...additional output omitted...
      ]
    }


blob ID
^^^^^^^

Prints the raw binary content of a blob (data or tree). The ID can be
either a data blob or a tree blob. The output is the decrypted content in its
original binary format. For tree blobs, you can pipe the output to ``jq`` for
readable JSON.

Example::

    # Print a tree blob and view as JSON (requires jq)
    $ restic -r /srv/restic-repo cat blob 2f15b17652357a2be758e58a76549eda0f7fb155d9a7bf3081234d8a028601ac | jq .
    {
      "nodes": [
        {
          "name": ".git",
          "type": "dir",
          "mode": 2147484141,
          "mtime": "2026-02-19T22:14:58.415878457-08:00",
          "atime": "2026-02-19T22:14:58.415878457-08:00",
          "ctime": "2026-02-19T22:14:58.415878457-08:00",
          "uid": 1000,
          "gid": 1000,
          "user": "myuser",
          "group": "mygroup",
          "inode": 1698306,
          "device_id": 41,
          "content": null,
          "subtree": "7e85cd4922528fac27879965a4ffa64d52ad912b14f1e23e788f5de432ed2072"
        },
        {
          "name": "README.md",
          "type": "file",
          "mode": 420,
          "mtime": "2026-02-19T22:14:58.411265656-08:00",
          "atime": "2026-02-19T22:14:58.411265656-08:00",
          "ctime": "2026-02-19T22:14:58.411265656-08:00",
          "uid": 1000,
          "gid": 1000,
          "user": "myuser",
          "group": "mygroup",
          "inode": 1698370,
          "device_id": 41,
          "size": 50,
          "links": 1,
          "content": [
            "0499644cc8e5f947be5df73c15b673b96067631d213c751b51951d65fad7b3f4"
          ]
        },
        ...additional output omitted...
      ]
    }

    # Print a data blob
    $ restic -r /srv/restic-repo cat blob 0499644cc8e5f947be5df73c15b673b96067631d213c751b51951d65fad7b3f4
    repository c528f271 opened (version 2, compression level auto)
    [0:00] 100.00%  1 / 1 index files loaded
    [blob contents]

    # Print a data blob and verify the hash
    $ restic -r /srv/restic-repo cat blob 0499644cc8e5f947be5df73c15b673b96067631d213c751b51951d65fad7b3f4 | sha256sum
    0499644cc8e5f947be5df73c15b673b96067631d213c751b51951d65fad7b3f4  -

index ID
^^^^^^^^

Prints the index file content in JSON format. The index maps blobs to pack files
that contain them.

Example::

    $ restic -r /srv/restic-repo cat index c3c0855e22febcfbd3897a2b8bd84c2c0d0356fffe93744c5a853bf453f3e921 | jq .
    {
      "packs": [
        {
          "id": "fe975e242e458b2c18bd00538ae16eaba3077fe1b615b00366294c0c3a52c664",
          "blobs": [
            {
              "id": "0223497a0b8b033aa58a3a521b8629869386cf7ab0e2f101963d328aa62193f7",
              "type": "data",
              "offset": 0,
              "length": 328,
              "uncompressed_length": 478
            },
            {
              "id": "81765af2daef323061dcbc5e61fc16481cb74b3bac9ad8a174b186523586f6c5",
              "type": "data",
              "offset": 328,
              "length": 181,
              "uncompressed_length": 189
            },
            ...additional similarly structured objects omitted...
          ]
        },
        {
          "id": "d558c02687114c0a0cb060d0a0e49f7f0fcdda4c86790bedae49025f88059ee0",
          "blobs": [
            {
              "id": "59f06b13a43d7bf7fcaccef0c47b23c9d129901bead6901ff8776f14c126fadb",
              "type": "tree",
              "offset": 0,
              "length": 258,
              "uncompressed_length": 377
            },
            {
              "id": "7a82ac2ee96828b667155f3255bbcc15af06740df6bad5513bc1159e28e2cfef",
              "type": "tree",
              "offset": 258,
              "length": 261,
              "uncompressed_length": 375
            },
            ...additional similarly structured objects omitted...
          ]
        }
      ]
    }

key ID
^^^^^^

Prints information about a specific key in JSON format. This includes the key
creation time, username, hostname, and the associated key ID.

Example::

    $ restic -r /srv/restic-repo cat key 33aa6685814c417df6eca8dfd5cbf7e0000dd197cdda223aa9443190b619e5c7
    repository c528f271 opened (version 2, compression level auto)
    {
      "created": "2026-02-19T22:16:30.938425666-08:00",
      "username": "myuser",
      "hostname": "myhost",
      "kdf": "scrypt",
      "N": 32768,
      "r": 8,
      "p": 5,
      "salt": "...omitted...",
      "data": "...omitted..."
    }


lock ID
^^^^^^^

Prints information about a repository lock in JSON format. Locks are created
during repository operations to prevent concurrent access.

Example::

    $ restic -r /srv/restic-repo cat lock 623da7f58063e4acd52a9995049e96b83a7ab39d158304ec83e3554c33bcbf63
    repository c528f271 opened (version 2, compression level auto)
    {
      "time": "2026-02-19T22:37:46.85328066-08:00",
      "exclusive": false,
      "hostname": "myhost",
      "username": "myuser",
      "pid": 1698519,
      "uid": 1000,
      "gid": 1000
    }

pack ID
^^^^^^^

Prints the raw binary content of a pack file. Pack files contain multiple
encrypted blobs and are the fundamental storage unit in restic. The command
will verify the hash and warn if it doesn't match the pack ID.

Example::

    $ restic -r /srv/restic-repo cat pack fe975e242e458b2c18bd00538ae16eaba3077fe1b615b00366294c0c3a52c664
    [binary output]

.. note::

    The output format for ``masterkey``, ``config``, ``snapshot``, ``tree``, ``index``,
    ``key``, and ``lock`` is JSON. The output for ``blob`` and ``pack`` is raw
    binary data. Specifying ``--json`` or ``--quiet`` will suppress any non-JSON
    messages that the command generates.

check
-----

The ``check`` command uses the JSON lines format with the following message types.

Status
^^^^^^

+--------------------------+------------------------------------------------------------------------------------------------+----------+
| ``message_type``         | Always "summary"                                                                               | string   |
+--------------------------+------------------------------------------------------------------------------------------------+----------+
| ``num_errors``           | Number of errors                                                                               | int64    |
+--------------------------+------------------------------------------------------------------------------------------------+----------+
| ``broken_packs``         | Run "restic repair packs ID..." and "restic repair snapshots --forget" to remove damaged files | []string |
+--------------------------+------------------------------------------------------------------------------------------------+----------+
| ``suggest_repair_index`` | Run "restic repair index"                                                                      | bool     |
+--------------------------+------------------------------------------------------------------------------------------------+----------+
| ``suggest_prune``        | Run "restic prune"                                                                             | bool     |
+--------------------------+------------------------------------------------------------------------------------------------+----------+

Error
^^^^^

These errors are printed on ``stderr``.

+------------------+---------------------------------------------------------------------+--------+
| ``message_type`` | Always "error"                                                      | string |
+------------------+---------------------------------------------------------------------+--------+
| ``message``      | Error message. May change in arbitrary ways across restic versions. | string |
+------------------+---------------------------------------------------------------------+--------+


diff
----

The ``diff`` command uses the JSON lines format with the following message types.

change
^^^^^^

+------------------+--------------------------------------------------------------+--------+
| ``message_type`` | Always "change"                                              | string |
+------------------+--------------------------------------------------------------+--------+
| ``path``         | Path that has changed                                        | string |
+------------------+--------------------------------------------------------------+--------+
| ``modifier``     | Type of change, a concatenation of the following characters: | string |
|                  | "+" = added, "-" = removed, "T" = entry type changed,        |        |
|                  | "M" = file content changed, "U" = metadata changed,          |        |
|                  | "?" = bitrot detected                                        |        |
+------------------+--------------------------------------------------------------+--------+

statistics
^^^^^^^^^^

+---------------------+-------------------------+--------------------+
| ``message_type``    | Always "statistics"     | string             |
+---------------------+-------------------------+--------------------+
| ``source_snapshot`` | ID of first snapshot    | string             |
+---------------------+-------------------------+--------------------+
| ``target_snapshot`` | ID of second snapshot   | string             |
+---------------------+-------------------------+--------------------+
| ``changed_files``   | Number of changed files | int64              |
+---------------------+-------------------------+--------------------+
| ``added``           | Added items             | `DiffStat object`_ |
+---------------------+-------------------------+--------------------+
| ``removed``         | Removed items           | `DiffStat object`_ |
+---------------------+-------------------------+--------------------+

.. _DiffStat object:

DiffStat object

+----------------+-------------------------------------------+--------+
| ``files``      | Number of changed files                   | int64  |
+----------------+-------------------------------------------+--------+
| ``dirs``       | Number of changed directories             | int64  |
+----------------+-------------------------------------------+--------+
| ``others``     | Number of changed other directory entries | int64  |
+----------------+-------------------------------------------+--------+
| ``data_blobs`` | Number of data blobs                      | int64  |
+----------------+-------------------------------------------+--------+
| ``tree_blobs`` | Number of tree blobs                      | int64  |
+----------------+-------------------------------------------+--------+
| ``bytes``      | Number of bytes                           | uint64 |
+----------------+-------------------------------------------+--------+

.. _find:

find
----

The ``find`` command outputs a single JSON document containing an array of JSON
objects with matches for your search term.  These matches are organized by snapshot.

If the ``--blob``, ``--tree`` or ``--pack`` option is passed, then the output is
an array of `Blob objects`_.

+--------------+-----------------------------------+--------------------+
| ``hits``     | Number of matches in the snapshot | uint64             |
+--------------+-----------------------------------+--------------------+
| ``snapshot`` | ID of the snapshot                | string             |
+--------------+-----------------------------------+--------------------+
| ``matches``  | Details of a match                | [] `Match object`_ |
+--------------+-----------------------------------+--------------------+

.. _Match object:

Match object

+-----------------+----------------------------------------------+-------------+
| ``path``        | Object path                                  | string      |
+-----------------+----------------------------------------------+-------------+
| ``permissions`` | UNIX permissions                             | string      |
+-----------------+----------------------------------------------+-------------+
| ``name``        | Object name                                  | string      |
+-----------------+----------------------------------------------+-------------+
| ``type``        | Object type e.g. file, dir, etc...           | string      |
+-----------------+----------------------------------------------+-------------+
| ``atime``       | Access time                                  | time.Time   |
+-----------------+----------------------------------------------+-------------+
| ``mtime``       | Modification time                            | time.Time   |
+-----------------+----------------------------------------------+-------------+
| ``ctime``       | Change time                                  | time.Time   |
+-----------------+----------------------------------------------+-------------+
| ``user``        | Name of owner                                | string      |
+-----------------+----------------------------------------------+-------------+
| ``group``       | Name of group                                | string      |
+-----------------+----------------------------------------------+-------------+
| ``inode``       | Inode number                                 | uint64      |
+-----------------+----------------------------------------------+-------------+
| ``mode``        | UNIX file mode, shorthand of ``permissions`` | os.FileMode |
+-----------------+----------------------------------------------+-------------+
| ``device_id``   | OS specific device identifier                | uint64      |
+-----------------+----------------------------------------------+-------------+
| ``links``       | Number of hardlinks                          | uint64      |
+-----------------+----------------------------------------------+-------------+
| ``link_target`` | Target of a symlink                          | string      |
+-----------------+----------------------------------------------+-------------+
| ``uid``         | ID of owner                                  | uint32      |
+-----------------+----------------------------------------------+-------------+
| ``gid``         | ID of group                                  | uint32      |
+-----------------+----------------------------------------------+-------------+
| ``size``        | Size of object in bytes                      | uint64      |
+-----------------+----------------------------------------------+-------------+

.. _Blob objects:

Blob objects

+-----------------+--------------------------------------------+-----------+
| ``object_type`` | Either "blob" or "tree"                    | string    |
+-----------------+--------------------------------------------+-----------+
| ``id``          | ID of found blob                           | string    |
+-----------------+--------------------------------------------+-----------+
| ``path``        | Path in snapshot                           | string    |
+-----------------+--------------------------------------------+-----------+
| ``parent_tree`` | Parent tree blob, only set for type "blob" | string    |
+-----------------+--------------------------------------------+-----------+
| ``snapshot``    | Snapshot ID                                | string    |
+-----------------+--------------------------------------------+-----------+
| ``time``        | Snapshot timestamp                         | time.Time |
+-----------------+--------------------------------------------+-----------+


forget
------

The ``forget`` command prints a single JSON document containing an array of
ForgetGroups. If specific snapshot IDs are specified, then no output is generated.

The ``prune`` command does not yet support JSON such that ``forget --prune``
results in a mix of JSON and text output.

ForgetGroup
^^^^^^^^^^^

+-------------+---------------------------------------------------------------+-------------------------+
| ``tags``    | Tags identifying the snapshot group                           | []string                |
+-------------+---------------------------------------------------------------+-------------------------+
| ``host``    | Host identifying the snapshot group                           | string                  |
+-------------+---------------------------------------------------------------+-------------------------+
| ``paths``   | Paths identifying the snapshot group                          | []string                |
+-------------+---------------------------------------------------------------+-------------------------+
| ``keep``    | Array of Snapshot that are kept                               | [] `Snapshot object`_   |
+-------------+---------------------------------------------------------------+-------------------------+
| ``remove``  | Array of Snapshot that were removed                           | [] `Snapshot object`_   |
+-------------+---------------------------------------------------------------+-------------------------+
| ``reasons`` | Array of KeepReason objects describing why a snapshot is kept | [] `KeepReason object`_ |
+-------------+---------------------------------------------------------------+-------------------------+

.. _Snapshot object:

Snapshot object

+---------------------+--------------------------------------------------+---------------------------+
| ``time``            | Timestamp of when the backup was started         | time.Time                 |
+---------------------+--------------------------------------------------+---------------------------+
| ``parent``          | ID of the parent snapshot                        | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``tree``            | ID of the root tree blob                         | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``paths``           | List of paths included in the backup             | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``hostname``        | Hostname of the backed up machine                | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``username``        | Username the backup command was run as           | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``uid``             | ID of owner                                      | uint32                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``gid``             | ID of group                                      | uint32                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``excludes``        | List of paths and globs excluded from the backup | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``tags``            | List of tags for the snapshot in question        | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``program_version`` | restic version used to create snapshot           | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``summary``         | Snapshot statistics                              | `SnapshotSummary object`_ |
+---------------------+--------------------------------------------------+---------------------------+
| ``id``              | Snapshot ID                                      | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``short_id``        | Snapshot ID, short form (deprecated)             | string                    |
+---------------------+--------------------------------------------------+---------------------------+

.. _KeepReason object:

KeepReason object

+--------------+--------------------------------------------------------+--------------------+
| ``snapshot`` | Snapshot described by this object                      | `Snapshot object`_ |
+--------------+--------------------------------------------------------+--------------------+
| ``matches``  | Array containing descriptions of the matching criteria | []string           |
+--------------+--------------------------------------------------------+--------------------+


init
----

The ``init`` command uses the JSON lines format, but only outputs a single message.

+------------------+------------------------------+--------+
| ``message_type`` | Always "initialized"         | string |
+------------------+------------------------------+--------+
| ``id``           | ID of the created repository | string |
+------------------+------------------------------+--------+
| ``repository``   | URL of the repository        | string |
+------------------+------------------------------+--------+


key list
--------

The ``key list`` command returns an array of objects with the following structure.

+--------------+-----------------------------------+-----------------+
| ``current``  | Is currently used key?            | bool            |
+--------------+-----------------------------------+-----------------+
| ``id``       | Unique key ID                     | string          |
+--------------+-----------------------------------+-----------------+
| ``userName`` | User who created it               | string          |
+--------------+-----------------------------------+-----------------+
| ``hostName`` | Name of machine it was created on | string          |
+--------------+-----------------------------------+-----------------+
| ``created``  | Timestamp when it was created     | local time.Time |
+--------------+-----------------------------------+-----------------+


.. _ls json:

ls
--

The ``ls`` command uses the JSON lines format with the following message types.
As an exception, the ``struct_type`` field is used to determine the message type.

snapshot
^^^^^^^^

+---------------------+--------------------------------------------------+---------------------------+
| ``message_type``    | Always "snapshot"                                | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``struct_type``     | Always "snapshot" (deprecated)                   | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``time``            | Timestamp of when the backup was started         | time.Time                 |
+---------------------+--------------------------------------------------+---------------------------+
| ``parent``          | ID of the parent snapshot                        | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``tree``            | ID of the root tree blob                         | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``paths``           | List of paths included in the backup             | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``hostname``        | Hostname of the backed up machine                | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``username``        | Username the backup command was run as           | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``uid``             | ID of owner                                      | uint32                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``gid``             | ID of group                                      | uint32                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``excludes``        | List of paths and globs excluded from the backup | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``tags``            | List of tags for the snapshot in question        | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``program_version`` | restic version used to create snapshot           | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``summary``         | Snapshot statistics                              | `SnapshotSummary object`_ |
+---------------------+--------------------------------------------------+---------------------------+
| ``id``              | Snapshot ID                                      | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``short_id``        | Snapshot ID, short form (deprecated)             | string                    |
+---------------------+--------------------------------------------------+---------------------------+


node
^^^^

+------------------+----------------------------+-------------+
| ``message_type`` | Always "node"              | string      |
+------------------+----------------------------+-------------+
| ``struct_type``  | Always "node" (deprecated) | string      |
+------------------+----------------------------+-------------+
| ``name``         | Node name                  | string      |
+------------------+----------------------------+-------------+
| ``type``         | Node type                  | string      |
+------------------+----------------------------+-------------+
| ``path``         | Node path                  | string      |
+------------------+----------------------------+-------------+
| ``uid``          | UID of node                | uint32      |
+------------------+----------------------------+-------------+
| ``gid``          | GID of node                | uint32      |
+------------------+----------------------------+-------------+
| ``size``         | Size in bytes              | uint64      |
+------------------+----------------------------+-------------+
| ``mode``         | Node mode                  | os.FileMode |
+------------------+----------------------------+-------------+
| ``permissions``  | Node mode as string        | string      |
+------------------+----------------------------+-------------+
| ``atime``        | Node access time           | time.Time   |
+------------------+----------------------------+-------------+
| ``mtime``        | Node modification time     | time.Time   |
+------------------+----------------------------+-------------+
| ``ctime``        | Node creation time         | time.Time   |
+------------------+----------------------------+-------------+
| ``inode``        | Inode number of node       | uint64      |
+------------------+----------------------------+-------------+


restore
-------

The ``restore`` command uses the JSON lines format with the following message types.

Status
^^^^^^

+---------------------+----------------------------------------------------------+---------+
| ``message_type``    | Always "status"                                          | string  |
+---------------------+----------------------------------------------------------+---------+
| ``seconds_elapsed`` | Time since restore started                               | uint64  |
+---------------------+----------------------------------------------------------+---------+
| ``percent_done``    | Percentage of data restored (bytes_restored/total_bytes) | float64 |
+---------------------+----------------------------------------------------------+---------+
| ``total_files``     | Total number of files detected                           | uint64  |
+---------------------+----------------------------------------------------------+---------+
| ``files_restored``  | Files restored                                           | uint64  |
+---------------------+----------------------------------------------------------+---------+
| ``files_skipped``   | Files skipped due to overwrite setting                   | uint64  |
+---------------------+----------------------------------------------------------+---------+
| ``files_deleted``   | Files deleted                                            | uint64  |
+---------------------+----------------------------------------------------------+---------+
| ``total_bytes``     | Total number of bytes in restore set                     | uint64  |
+---------------------+----------------------------------------------------------+---------+
| ``bytes_restored``  | Number of bytes restored                                 | uint64  |
+---------------------+----------------------------------------------------------+---------+
| ``bytes_skipped``   | Total size of skipped files                              | uint64  |
+---------------------+----------------------------------------------------------+---------+

Error
^^^^^

These errors are printed on ``stderr``.

+-------------------+-------------------------------------------+--------+
| ``message_type``  | Always "error"                            | string |
+-------------------+-------------------------------------------+--------+
| ``error.message`` | Error message                             | string |
+-------------------+-------------------------------------------+--------+
| ``during``        | Always "restore"                          | string |
+-------------------+-------------------------------------------+--------+
| ``item``          | Usually, the path of the problematic file | string |
+-------------------+-------------------------------------------+--------+

Verbose Status
^^^^^^^^^^^^^^

Verbose status provides details about the progress, including details about restored files.
Only printed if `--verbose=2` is specified.

+------------------+--------------------------------------------------------+--------+
| ``message_type`` | Always "verbose_status"                                | string |
+------------------+--------------------------------------------------------+--------+
| ``action``       | Either "restored", "updated", "unchanged" or "deleted" | string |
+------------------+--------------------------------------------------------+--------+
| ``item``         | The item in question                                   | string |
+------------------+--------------------------------------------------------+--------+
| ``size``         | Size of the item in bytes                              | uint64 |
+------------------+--------------------------------------------------------+--------+

Summary
^^^^^^^

+---------------------+----------------------------------------+--------+
| ``message_type``    | Always "summary"                       | string |
+---------------------+----------------------------------------+--------+
| ``seconds_elapsed`` | Time since restore started             | uint64 |
+---------------------+----------------------------------------+--------+
| ``total_files``     | Total number of files detected         | uint64 |
+---------------------+----------------------------------------+--------+
| ``files_restored``  | Files restored                         | uint64 |
+---------------------+----------------------------------------+--------+
| ``files_skipped``   | Files skipped due to overwrite setting | uint64 |
+---------------------+----------------------------------------+--------+
| ``files_deleted``   | Files deleted                          | uint64 |
+---------------------+----------------------------------------+--------+
| ``total_bytes``     | Total number of bytes in restore set   | uint64 |
+---------------------+----------------------------------------+--------+
| ``bytes_restored``  | Number of bytes restored               | uint64 |
+---------------------+----------------------------------------+--------+
| ``bytes_skipped``   | Total size of skipped files            | uint64 |
+---------------------+----------------------------------------+--------+


snapshots
---------

The snapshots command returns a single JSON array with objects of the structure outlined below.

+---------------------+--------------------------------------------------+---------------------------+
| ``time``            | Timestamp of when the backup was started         | time.Time                 |
+---------------------+--------------------------------------------------+---------------------------+
| ``parent``          | ID of the parent snapshot                        | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``tree``            | ID of the root tree blob                         | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``paths``           | List of paths included in the backup             | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``hostname``        | Hostname of the backed up machine                | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``username``        | Username the backup command was run as           | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``uid``             | ID of owner                                      | uint32                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``gid``             | ID of group                                      | uint32                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``excludes``        | List of paths and globs excluded from the backup | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``tags``            | List of tags for the snapshot in question        | []string                  |
+---------------------+--------------------------------------------------+---------------------------+
| ``program_version`` | restic version used to create snapshot           | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``summary``         | Snapshot statistics                              | `SnapshotSummary object`_ |
+---------------------+--------------------------------------------------+---------------------------+
| ``id``              | Snapshot ID                                      | string                    |
+---------------------+--------------------------------------------------+---------------------------+
| ``short_id``        | Snapshot ID, short form (deprecated)             | string                    |
+---------------------+--------------------------------------------------+---------------------------+

.. _SnapshotSummary object:

SnapshotSummary object

The contained statistics reflect the information at the point64 in time when the snapshot
was created.

+---------------------------+----------------------------------------------------+-----------+
| ``backup_start``          | Time at which the backup was started               | time.Time |
+---------------------------+----------------------------------------------------+-----------+
| ``backup_end``            | Time at which the backup was completed             | time.Time |
+---------------------------+----------------------------------------------------+-----------+
| ``files_new``             | Number of new files                                | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``files_changed``         | Number of files that changed                       | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``files_unmodified``      | Number of files that did not change                | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``dirs_new``              | Number of new directories                          | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``dirs_changed``          | Number of directories that changed                 | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``dirs_unmodified``       | Number of directories that did not change          | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``data_blobs``            | Number of data blobs added                         | int64     |
+---------------------------+----------------------------------------------------+-----------+
| ``tree_blobs``            | Number of tree blobs added                         | int64     |
+---------------------------+----------------------------------------------------+-----------+
| ``data_added``            | Amount of (uncompressed) data added, in bytes      | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``data_added_packed``     | Amount of data added (after compression), in bytes | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``total_files_processed`` | Total number of files processed                    | uint64    |
+---------------------------+----------------------------------------------------+-----------+
| ``total_bytes_processed`` | Total number of bytes processed                    | uint64    |
+---------------------------+----------------------------------------------------+-----------+


stats
-----

The stats command returns a single JSON object.

+------------------------------+-----------------------------------------------------+---------+
| ``total_size``               | Repository size in bytes                            | uint64  |
+------------------------------+-----------------------------------------------------+---------+
| ``total_file_count``         | Number of files backed up in the repository         | uint64  |
+------------------------------+-----------------------------------------------------+---------+
| ``total_blob_count``         | Number of blobs in the repository                   | uint64  |
+------------------------------+-----------------------------------------------------+---------+
| ``snapshots_count``          | Number of processed snapshots                       | uint64  |
+------------------------------+-----------------------------------------------------+---------+
| ``total_uncompressed_size``  | Repository size in bytes if blobs were uncompressed | uint64  |
+------------------------------+-----------------------------------------------------+---------+
| ``compression_ratio``        | Factor by which the already compressed data         | float64 |
|                              | has shrunk due to compression                       |         |
+------------------------------+-----------------------------------------------------+---------+
| ``compression_progress``     | Percentage of already compressed data               | float64 |
+------------------------------+-----------------------------------------------------+---------+
| ``compression_space_saving`` | Overall space saving due to compression             | float64 |
+------------------------------+-----------------------------------------------------+---------+

tag
---

The ``tag`` command uses the JSON lines format with the following message types.

Changed
^^^^^^^

+---------------------+--------------------------------------+--------+
| ``message_type``    | Always "changed"                     | string |
+---------------------+--------------------------------------+--------+
| ``old_snapshot_id`` | ID of the snapshot before the change | string |
+---------------------+--------------------------------------+--------+
| ``new_snapshot_id`` | ID of the snapshot after the change  | string |
+---------------------+--------------------------------------+--------+

Summary
^^^^^^^

+-----------------------+-----------------------------------+--------+
| ``message_type``      | Always "summary"                  | string |
+-----------------------+-----------------------------------+--------+
| ``changed_snapshots`` | Total number of changed snapshots | int64  |
+-----------------------+-----------------------------------+--------+

version
-------

The version command returns a single JSON object.

+------------------+--------------------+--------+
| ``message_type`` | Always "version"   | string |
+------------------+--------------------+--------+
| ``version``      | restic version     | string |
+------------------+--------------------+--------+
| ``go_version``   | Go compile version | string |
+------------------+--------------------+--------+
| ``go_os``        | Go OS              | string |
+------------------+--------------------+--------+
| ``go_arch``      | Go architecture    | string |
+------------------+--------------------+--------+
