Local Cache
===========

In order to speed up certain operations, restic manages a local cache of data.
This document describes the data structures for the local cache. This document
describes the cache with version 1.

Versions
--------

The cache directory is selected according to the `XDG base dir specification
<http://standards.freedesktop.org/basedir-spec/basedir-spec-latest.html>`. It
contains a file named `version`, which contains a single ASCII integer line
that stands for the current version of the cache. If a lower version number is
found the cache is recreated with the current version. If a higher version
number is found the cache is ignored and left as is.

Snapshots
---------

The sub-directory `snapshots` stores cached data for each snapshot in a single
file named after the complete snapshot ID. The snapshot ID is either the
storage ID for the snapshot or, if it has been amended (e.g. by adding tags)
the contents of the JSON field `original`.

A snapshot cache file consists of several blocks, each prepended by a four byte
length field (`uint32`) in little-endian encoding. All blocks are encrypted and
authenticated with the repository master keys.

The first block contains the snapshot data itself, encoded in JSON as it is
saved in the repo. Afterwards follow all (encrypted) tree objects of this
repository, sorted by the node name in depth-first order. The tree objects are
embedded in the following JSON structure:

.. code:: json

    {
        "path": "/",
        "id": "2da81727b6585232894cfbb8f8bdab8d1eccd3d8f7c92bc934d62e62e618ffdf",
        "tree": {
          "nodes": [
            {
              "name": "testdata",
              "type": "dir",
              "mode": 493,
              "mtime": "2014-12-22T14:47:59.912418701+01:00",
              "atime": "2014-12-06T17:49:21.748468803+01:00",
              "ctime": "2014-12-22T14:47:59.912418701+01:00",
              "uid": 1000,
              "gid": 100,
              "user": "fd0",
              "inode": 409704562,
              "content": null,
              "subtree": "b26e315b0988ddcd1cee64c351d13a100fedbc9fdbb144a67d1b765ab280b4dc"
            }
          ]
        }
    }
