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


************************
Diving into a Repository
************************
The following section dives into the commands developers could use
to extract certain data from a repository.

Listing different file types in the repository
==============================================

The ``restic list`` command allows listing objects in the repository based on type.
The allowed types are (in alphabetic order):

- blobs
- index
- keys
- locks
- packs
- snapshots

With the exception of ``blobs`` all output - in text mode - contains zero or more
``IDs`` of the given type, one ``ID`` per output line.

The output for ``blobs`` contains one or more lines of output of the form
``blob-type blob-ID``, where ``blob-type`` is either ``data`` or ``tree``, and ``blob-ID``
is the ``sha256sum`` of the ``blob``.

The output of the ``restic list 'type-plural'`` is most commonly used for the ``restic cat 'type' ID``
command to study a ``type`` object with an ``ID`` in more detail. The only exception to
this singular/plural ``type`` is ``index``, which is used in both commands ``restic list index`` and
``restic cat index <ID>``.

The examples below are using part of the standard file structure for testing restic itself.
Here is the ``ls`` output of the one and only snapshot in this test repository:

.. code-block:: console

    $ restic -r /srv/restic-repo ls 4254d65c
    snapshot 4254d65c of [/srv/restic-repo/testdata/0/for_cmd_ls] at 2026-01-17 17:26:41.972899252 +0000 UTC by user@kasimir filtered by []:
    /srv/restic-repo/testdata
    /srv/restic-repo/testdata/0
    /srv/restic-repo/testdata/0/for_cmd_ls
    /srv/restic-repo/testdata/0/for_cmd_ls/file1.txt
    /srv/restic-repo/testdata/0/for_cmd_ls/file2.txt
    /srv/restic-repo/testdata/0/for_cmd_ls/python.py

Inspecting this repository with ``restic list snapshots`` produces:

.. code-block:: console

    $ restic -r /srv/restic-repo list snapshots -q
    4254d65c92208eda22b852b390bd5401ca4c500be7a022c70e7c33de68ca2143
    $ restic -r /srv/restic-repo cat snapshot 4254d65c92208eda22b852b390bd5401ca4c500be7a022c70e7c33de68ca2143 -q
    {
      "time": "2026-01-17T17:26:41.972899252Z",
      "tree": "db9e90f7f1761ab892b3ae25e3838bbd697499b985e9b47d3a1da09e0bd8ca68",
      ...
      "summary": {
        "backup_start": "2026-01-17T17:26:41.972899252Z",
        "backup_end": "2026-01-17T17:26:42.581012438Z",
        ...
      }
    }

The index contains 2 packfiles, one for trees and one for the actual file data:

.. code-block:: console

    $ restic -r /srv/restic-repo list index -q
    a1828d209e760f0fd143aa79e530de0a377d7affd1cd0964d9cb2ad3c77e0d8b
    $ restic -r /srv/restic-repo cat index a1828d209e760f0fd143aa79e530de0a377d7affd1cd0964d9cb2ad3c77e0d8b | jq
    {
      "packs": [
        {
          "id": "953e5381138bdc44da23740a83065809dd4021f45ce4e351b577dc4c07f81314",
          "blobs": [
            {
              "id": "124323c57d74fb8944c98fb69ce67a41a107cb6d2ed304cf50c8529cc137aafd",
              "type": "data",
              "offset": 0,
              "length": 59,
              "uncompressed_length": 18
            },
            ...
          ]
        },
        {
          "id": "75bca8556f47d16362e58e757ea89a34b28fb96aedcc314bea35d468e5cb665c",
          "blobs": [
            {
              "id": "6dfdc53cc3b45a6bf519a7fb80a54f6ef3e3ea859f51d3e85a6235177606f1f9",
              "type": "tree",
              "offset": 0,
              "length": 271,
              "uncompressed_length": 353
            },
            ...
          ]
        }
      ]
    }

And this is the list of blobs:

.. code-block:: console

    $ restic -r /srv/restic-repo list blobs -q
    data 124323c57d74fb8944c98fb69ce67a41a107cb6d2ed304cf50c8529cc137aafd
    data 37cc0b45af245d93abaecba73a600a8d577b39e4a1fdc2dcdf93ad63b1e167bd
    data 5dfb8bc8a35175bf011d10ac7bc3a6b8d42b7743ac188be8c1bf0b215f9b7bf5
    tree 6dfdc53cc3b45a6bf519a7fb80a54f6ef3e3ea859f51d3e85a6235177606f1f9
    tree 73947e98d4025179347363401eb41f148dc29a1d1735bfb96a08a6036422108c
    tree 6d1daddbb3f280be0f25e708618576e003c2a87516a9aa31e98205ae0a152ab5
    tree 2e89c815e31c377629ef77fa1c156d1ad794b9f09d9d3b113e00e8eab36ceb98
    tree db9e90f7f1761ab892b3ae25e3838bbd697499b985e9b47d3a1da09e0bd8ca68
    tree d2524f3358bffbfe7349ca73df4bd3f23f5b252a9ba887481eda7e696b506dd4
    tree 4d8f5a6c6e90a2d69ae4b2f8e4f7f5851ccc4fa2cd3314f81de6c929453994fe

The other types ``keys``, ``locks`` and ``packs`` are used in the same way as the type ``index``.

.. code-block:: console

    $ restic -r /srv/restic-repo list packs -q
    953e5381138bdc44da23740a83065809dd4021f45ce4e351b577dc4c07f81314
    75bca8556f47d16362e58e757ea89a34b28fb96aedcc314bea35d468e5cb665c


.. _view-repository-objects:

Inspecting repository objects
=============================

The ``cat`` command is used to inspect and print internal repository objects to stdout.
This is primarily useful for debugging, understanding repository structure, or
recovering data from a damaged repository. The command supports the object types described
below. To get a list of objects of a given type, use the ``restic list`` command
as described in the previous section.

For details about the individual data structures, see the :ref:`repository-format` section.

.. note::

    The output format for ``masterkey``, ``config``, ``snapshot``, ``tree``, ``index``,
    ``key``, and ``lock`` is JSON. The output for ``blob`` and ``pack`` is raw
    binary data. If the output of ``cat`` is sent to a file or command, or when specifying
    ``--json`` or ``--quiet``, then any extra messages that the command generates on stdout
    will be suppressed. Errors are still printed on stderr.

masterkey
---------

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
------

Prints the repository configuration in JSON format. This includes settings such as
the repository version and chunker polynomial. No additional ID argument is required.

Example::

    $ restic -r /srv/restic-repo cat config
    repository c528f271 opened (version 2, compression level auto)
    {
      "version": 2,
      "id": "c528f27103c1cfc08ca3df9331c5b04a73b9c6822ae30f33bbf1d9342b9a1bf0",
      "chunker_polynomial": "255b9ca195d755"
    }

snapshot ID
-----------

Prints the metadata for a specific snapshot in JSON format. The snapshot ID
can be the full snapshot ID or a unique prefix. The output includes the
snapshot timestamp, paths, tags, hostname, username, and the root tree ID,
and a number of additional fields.

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
-------------------------

Prints the root tree of a snapshot or a specific subfolder if specified.
This outputs the tree in JSON format. The tree contains nodes representing files
and directories. To view the tree structure in a human-readable format, you can
use the ``restic ls`` command instead. For better readability, pipe the output
to ``jq``.

Example::

    # Print the root tree of a snapshot
    $ restic -r /srv/restic-repo cat tree 251c2e584489c16e19f3c4544a0ef23e3ebf65a7cc23c68f31035ddf885d92ad | jq .
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
    $ restic -r /srv/restic-repo cat tree 251c2e584489c16e19f3c4544a0ef23e3ebf65a7cc23c68f31035ddf885d92ad:subfolder/path | jq .
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
-------

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
--------

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
------

Prints information about a specific key in JSON format. This includes the key
creation time, username, hostname, and the encrypted master key data.

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
-------

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
-------

Prints the raw binary content of a pack file. Pack files contain multiple
encrypted blobs and are the fundamental storage unit in restic. The command
will verify the hash and warn if it doesn't match the pack ID.

Example::

    $ restic -r /srv/restic-repo cat pack fe975e242e458b2c18bd00538ae16eaba3077fe1b615b00366294c0c3a52c664
    [binary output]
