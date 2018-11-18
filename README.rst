|Documentation| |Build Status| |Build status| |Report Card| |Say Thanks| |TestCoverage| |Reviewed by Hound|

Introduction
------------

restic is a backup program that is fast, efficient and secure. It supports the three major operating systems (Linux, macOS, Windows) and a few smaller ones (FreeBSD, OpenBSD).

For detailed usage and installation instructions check out the `documentation <https://restic.readthedocs.io/en/latest>`__.

You can ask questions in our `Discourse forum <https://forum.restic.net>`__.

Quick start
-----------

Once you've `installed
<https://restic.readthedocs.io/en/latest/020_installation.html>`__ restic, start
off with creating a repository for your backups:

.. code-block:: console

    $ restic init --repo /tmp/backup
    enter password for new backend:
    enter password again:
    created restic backend 085b3c76b9 at /tmp/backup
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

and add some data:

.. code-block:: console

    $ restic --repo /tmp/backup backup ~/work
    enter password for repository:
    scan [/home/user/work]
    scanned 764 directories, 1816 files in 0:00
    [0:29] 100.00%  54.732 MiB/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
    duration: 0:29, 54.47MiB/s
    snapshot 40dc1520 saved

Next you can either use ``restic restore`` to restore files or use ``restic
mount`` to mount the repository via fuse and browse the files from previous
snapshots.

For more options check out the `online documentation <https://restic.readthedocs.io/en/latest/>`__.

Backends
--------

Saving a backup on the same machine is nice but not a real backup strategy.
Therefore, restic supports the following backends for storing backups natively:

- `Local directory <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#local>`__
- `sftp server (via SSH) <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#sftp>`__
- `HTTP REST server <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#rest-server>`__ (`protocol <doc/100_references.rst#rest-backend>`__ `rest-server <https://github.com/restic/rest-server>`__)
- `AWS S3 <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#amazon-s3>`__ (either from Amazon or using the `Minio <https://minio.io>`__ server)
- `OpenStack Swift <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#openstack-swift>`__
- `BackBlaze B2 <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#backblaze-b2>`__
- `Microsoft Azure Blob Storage <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#microsoft-azure-blob-storage>`__
- `Google Cloud Storage <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#google-cloud-storage>`__
- And many other services via the `rclone <https://rclone.org>`__ `Backend <https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#other-services-via-rclone>`__

Design Principles
-----------------

Restic is a program that does backups right and was designed with the
following principles in mind:

-  **Easy:** Doing backups should be a frictionless process, otherwise
   you might be tempted to skip it. Restic should be easy to configure
   and use, so that, in the event of a data loss, you can just restore
   it. Likewise, restoring data should not be complicated.

-  **Fast**: Backing up your data with restic should only be limited by
   your network or hard disk bandwidth so that you can backup your files
   every day. Nobody does backups if it takes too much time. Restoring
   backups should only transfer data that is needed for the files that
   are to be restored, so that this process is also fast.

-  **Verifiable**: Much more important than backup is restore, so restic
   enables you to easily verify that all data can be restored.

-  **Secure**: Restic uses cryptography to guarantee confidentiality and
   integrity of your data. The location the backup data is stored is
   assumed not to be a trusted environment (e.g. a shared space where
   others like system administrators are able to access your backups).
   Restic is built to secure your data against such attackers.

-  **Efficient**: With the growth of data, additional snapshots should
   only take the storage of the actual increment. Even more, duplicate
   data should be de-duplicated before it is actually written to the
   storage back end to save precious backup space.

Reproducible Builds
-------------------

The binaries released with each restic version starting at 0.6.1 are
`reproducible <https://reproducible-builds.org/>`__, which means that you can
easily reproduce a byte identical version from the source code for that
release. Instructions on how to do that are contained in the
`builder repository <https://github.com/restic/builder>`__.

News
----

You can follow the restic project on Twitter `@resticbackup <https://twitter.com/resticbackup>`__ or by subscribing to
the `development blog <https://restic.net/blog/>`__.

License
-------

Restic is licensed under `BSD 2-Clause License <https://opensource.org/licenses/BSD-2-Clause>`__. You can find the
complete text in ``LICENSE``.

Sponsorship
-----------

Backend integration tests for Google Cloud Storage and Microsoft Azure Blob
Storage are sponsored by `AppsCode <https://appscode.com>`__!

|AppsCode|

.. |Documentation| image:: https://readthedocs.org/projects/restic/badge/?version=latest
   :target: https://restic.readthedocs.io/en/latest/?badge=latest
.. |Build Status| image:: https://travis-ci.com/restic/restic.svg?branch=master
   :target: https://travis-ci.com/restic/restic
.. |Build status| image:: https://ci.appveyor.com/api/projects/status/nuy4lfbgfbytw92q/branch/master?svg=true
   :target: https://ci.appveyor.com/project/fd0/restic/branch/master
.. |Report Card| image:: https://goreportcard.com/badge/github.com/restic/restic
   :target: https://goreportcard.com/report/github.com/restic/restic
.. |Say Thanks| image:: https://img.shields.io/badge/Say%20Thanks-!-1EAEDB.svg
   :target: https://saythanks.io/to/restic
.. |TestCoverage| image:: https://codecov.io/gh/restic/restic/branch/master/graph/badge.svg
   :target: https://codecov.io/gh/restic/restic
.. |AppsCode| image:: https://cdn.appscode.com/images/logo/appscode/ac-logo-color.png
   :target: https://appscode.com
.. |Reviewed by Hound| image:: https://img.shields.io/badge/Reviewed_by-Hound-8E64B0.svg
   :target: https://houndci.com
