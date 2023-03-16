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

########################
Tuning Backup Parameters
########################

Restic offers a few parameters that allow tuning the backup. The default values should
work well in general although specific use cases can benefit from different non-default
values. As the restic commands evolve over time, the optimal value for each parameter
can also change across restic versions.


Disabling Backup Progress Estimation
====================================

When you start a backup, restic will concurrently count the number of files and
their total size, which is used to estimate how long it will take. This will
cause some extra I/O, which can slow down backups of network file systems or
FUSE mounts. To avoid this overhead at the cost of not seeing a progress
estimate, use the ``--no-scan`` option which disables this file scanning.

Backend Connections
===================

Restic uses a global limit for the number of concurrent connections to a backend.
This limit can be configured using ``-o <backend-name>.connections=5``, for example for
the REST backend the parameter would be ``-o rest.connections=5``. By default restic uses
``5`` connections for each backend, except for the local backend which uses a limit of ``2``.
The defaults should work well in most cases. For high-latency backends it can be beneficial
to increase the number of connections. Please be aware that this increases the resource
consumption of restic and that a too high connection count *will degrade performance*.


CPU Usage
=========

By default, restic uses all available CPU cores. You can set the environment variable
`GOMAXPROCS` to limit the number of used CPU cores. For example to use a single CPU core,
use `GOMAXPROCS=1`. Limiting the number of usable CPU cores, can slightly reduce the memory
usage of restic.


Compression
===========

For a repository using at least repository format version 2, you can configure how data
is compressed with the option ``--compression``. It can be set to ``auto`` (the default,
which will compress very fast), ``max`` (which will trade backup speed and CPU usage for
slightly better compression), or ``off`` (which disables compression). Each setting is
only applied for the single run of restic. The option can also be set via the environment
variable ``RESTIC_COMPRESSION``.


File Read Concurrency
=====================

When backing up files from fast storage like NVMe disks, it can be beneficial to increase
the read concurrency. This can increase the overall performance of the backup operation
by reading more files in parallel. You can specify the concurrency of file reads with the
``RESTIC_READ_CONCURRENCY`` environment variable or the ``--read-concurrency`` option of
the ``backup`` command.


Pack Size
=========

In certain instances, such as very large repositories (in the TiB range) or very fast
upload connections, it is desirable to use larger pack sizes to reduce the number of
files in the repository and improve upload performance.  Notable examples are OpenStack
Swift and some Google Drive Team accounts, where there are hard limits on the total
number of files.  Larger pack sizes can also improve the backup speed for a repository
stored on a local HDD.  This can be achieved by either using the ``--pack-size`` option
or defining the ``$RESTIC_PACK_SIZE`` environment variable.  Restic currently defaults
to a 16 MiB pack size.

The side effect of increasing the pack size is requiring more disk space for temporary pack
files created before uploading.  The space must be available in the system default temp
directory, unless overwritten by setting the ``$TMPDIR`` environment variable.  In addition,
depending on the backend the memory usage can also increase by a similar amount. Restic
requires temporary space according to the pack size, multiplied by the number
of backend connections plus one. For example, if the backend uses 5 connections (the default
for most backends), with a target pack size of 64 MiB, you'll need a *minimum* of 384 MiB
of space in the temp directory. A bit of tuning may be required to strike a balance between
resource usage at the backup client and the number of pack files in the repository.

Note that larger pack files increase the chance that the temporary pack files are written
to disk. An operating system usually caches file write operations in memory and writes
them to disk after a short delay. As larger pack files take longer to upload, this
increases the chance of these files being written to disk. This can increase disk wear
for SSDs.
