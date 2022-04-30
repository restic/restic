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

For a repository using at least repository format version 2, you can configure how data
is compressed with the option ``--compression``. It can be set to ```auto``` (the default,
which will compress very fast), ``max`` (which will trade backup speed and CPU usage for
slightly better compression), or ``off`` (which disables compression). Each setting is
only applied for the single run of restic. The option can also be set via the environment
variable ``RESTIC_COMPRESSION``.


Pack Size
=========

In certain instances, such as very large repositories, it is desired to have larger pack
sizes to reduce the number of files in the repository.  Notable examples are OpenStack
Swift and some Google Drive Team accounts, where there are hard limits on the total
number of files.  This can be achieved by either using the ``--min-packsize`` flag
or defining the ``$RESTIC_MIN_PACKSIZE`` environment variable.  Restic currently defaults
to a 16MB minimum pack size.

The side effect of increasing the pack size is increased client memory usage.  A bit of
tuning may be required to strike a balance between memory usage and number of pack files.

Restic uses the majority of it's memory according to the pack size, multiplied by the number
of parallel writers. For example, if you have 4 parallel writers (restic creates one per
available CPU), With a minimum pack size of 64 (Megabytes), you'll get a *minimum* of 256MB
of memory usage.
