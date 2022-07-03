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


Backend Connections
===================

Restic uses a global limit for the number of concurrent connections to a backend.
This limit can be configured using ``-o <backend-name>.connections=5``, for example for
the REST backend the parameter would be ``-o rest.connections=5``. By default restic uses
``5`` connections for each backend, except for the local backend which uses a limit of ``2``.
The defaults should work well in most cases. For high-latency backends it can be beneficial
to increase the number of connections. Please be aware that this increases the resource
consumption of restic and that a too high connection count *will degrade performace*.


CPU Usage
=========

By default, restic uses all available CPU cores. You can set the environment variable
`GOMAXPROCS` to limit the number of used CPU cores. For example to use a single CPU core,
use `GOMAXPROCS=1`. Limiting the number of usable CPU cores, can slightly reduce the memory
usage of restic.


Compression
===========

For a repository using a least repository format version 2, you can configure how data
is compressed with the option ``--compression``. It can be set to ```auto``` (the default,
which will compress very fast), ``max`` (which will trade backup speed and CPU usage for
slightly better compression), or ``off`` (which disables compression). Each setting is
only applied for the single run of restic.
