|Documentation| |Build Status| |Build status| |Report Card|

Introduction
------------

restic is a backup program that is fast, efficient and secure.

For detailed usage and installation instructions check out the `documentation <https://restic.readthedocs.io/en/latest>`__.

Quick start
-----------
Once you've `installed <https://restic.readthedocs.io/en/latest/installation.html>`__ restic, start off with creating a repository for your backups::

    $ restic init --repo /tmp/backup
    enter password for new backend:
    enter password again:
    created restic backend 085b3c76b9 at /tmp/backup
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

and add some data::

    $ restic -r /tmp/backup backup ~/work
    enter password for repository:
    scan [/home/user/work]
    scanned 764 directories, 1816 files in 0:00
    [0:29] 100.00%  54.732 MiB/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
    duration: 0:29, 54.47MiB/s
    snapshot 40dc1520 saved

For more options check out the `usage guide <https://restic.readthedocs.io/en/latest/usage.html>`__.

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

News
----

You can follow the restic project on Twitter `@resticbackup <https://twitter.com/resticbackup>`__ or by subscribing to
the `development blog <https://restic.github.io/blog/>`__.

License
-------

Restic is licensed under "BSD 2-Clause License". You can find the
complete text in ``LICENSE``.

.. |Documentation| image:: https://readthedocs.org/projects/restic/badge/?version=latest
   :target: https://restic.readthedocs.io/en/latest/?badge=latest
.. |Build Status| image:: https://travis-ci.org/restic/restic.svg?branch=master
   :target: https://travis-ci.org/restic/restic
.. |Build status| image:: https://ci.appveyor.com/api/projects/status/nuy4lfbgfbytw92q/branch/master?svg=true
   :target: https://ci.appveyor.com/project/fd0/restic/branch/master
.. |Report Card| image:: http://goreportcard.com/badge/github.com/restic/restic
   :target: http://goreportcard.com/report/github.com/restic/restic
