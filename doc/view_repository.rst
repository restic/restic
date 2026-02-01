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

The ``restic list`` allows listing objects in the repository based on type.
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
command to study an ``type`` object with an ``ID`` in more detail. The only exception to
this singular/plural ``type`` is ``ìndex``, which is used in both commands ``restic list index`` and
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
