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


#####################
Backing up S3 buckets
#####################

``backup --from-s3`` reads objects from an S3 bucket instead of the local file
system. The repository can use any backend.

.. code-block:: console

    $ restic -r /srv/restic-repo backup --from-s3 s3:s3.amazonaws.com/bucket
    $ restic -r /srv/restic-repo backup --from-s3 s3:https://host/bucket/prefix sub/dir

The location uses the same format as an S3 repository.

Selecting objects
=================

File and directory arguments are relative to the bucket and prefix. Without
arguments, the whole bucket or prefix is backed up. The snapshot contains the
bucket as top-level directory, for example ``/bucket/sub/dir``.

The same applies to ``--files-from``, ``--files-from-verbatim`` and
``--files-from-raw``. Wildcards in ``--files-from`` are expanded by listing the
bucket.

Credentials
===========

Credentials are read like for an S3 repository. The environment variables
``RESTIC_SOURCE_AWS_ACCESS_KEY_ID``, ``RESTIC_SOURCE_AWS_SECRET_ACCESS_KEY`` and
``RESTIC_SOURCE_AWS_DEFAULT_REGION`` take precedence and allow using different
credentials for the bucket and the repository.

Object keys
===========

Object keys are split at ``/`` into directories and files. Keys with empty
components (``a//b``, ``/a``) or the components ``.`` or ``..``, and objects
with the same name as a directory, are skipped.

Zero-sized objects with a key ending in ``/`` are treated as directory markers
and are not stored as files.

Metadata
========

Files get the mode ``0644``, directories ``0755``. The modification time of a
file is the last modified time of the object, truncated to seconds, directories
have a modification time of 1970-01-01.

A file is read again if its size, modification time or ETag changed. An object
which was modified after its directory was listed is backed up in the version
which is read.

Performance
===========

Objects are read with ``--read-concurrency`` parallel requests (default 2);
higher values are usually faster.

Each directory is listed once by the scanner and once while reading files,
``--no-scan`` avoids the first listing.

Up to 16 subdirectories of each directory are listed in the background before
they are needed. ``-o s3source.prefetch=N`` changes the number, ``0`` disables
it.
