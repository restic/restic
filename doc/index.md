Welcome to restic
=================

![restic logo](logo/logo.svg)

restic is a backup program that is fast, efficient and secure. On the left you
can find an overview of the documentation. The project's homepage is
<https://restic.github.io>, the source code repository can be found on GitHub
at the URL <https://github.com/restic/restic>.

Building and viewing the documentation
--------------------------------------

The documentation you're currently viewing may not match the version of restic
you have installed. If you cloned the repository manually, you can find the
right documentation in the directory `doc/`. If you're viewing this online at
<https://restic.readthedocs.io>, there is a small menu at the bottom left of
this page, where you can select the version.

The restic documentation is built with [MkDocs](http://www.mkdocs.org). After
installing it, you can edit and view the documentation locally by running:

    $ mkdocs serve
    INFO    -  Building documentation...
    INFO    -  Cleaning site directory
    [I 160221 12:33:57 server:271] Serving on http://127.0.0.1:8000

Afterwards visit the URL with a browser.

Design Principles
-----------------

Restic is a program that does backups right and was designed with the following
principles in mind:

 * **Easy:** Doing backups should be a frictionless process, otherwise you might be
   tempted to skip it.  Restic should be easy to configure and use, so that, in
   the event of a data loss, you can just restore it. Likewise,
   restoring data should not be complicated.

 * **Fast**: Backing up your data with restic should only be limited by your
   network or hard disk bandwidth so that you can backup your files every day.
   Nobody does backups if it takes too much time. Restoring backups should only
   transfer data that is needed for the files that are to be restored, so that
   this process is also fast.

 * **Verifiable**: Much more important than backup is restore, so restic enables
   you to easily verify that all data can be restored.

 * **Secure**: Restic uses cryptography to guarantee confidentiality and integrity
   of your data. The location the backup data is stored is assumed not to be a
   trusted environment (e.g. a shared space where others like system
   administrators are able to access your backups). Restic is built to secure
   your data against such attackers.

 * **Efficient**: With the growth of data, additional snapshots should only take
   the storage of the actual increment. Even more, duplicate data should be
   de-duplicated before it is actually written to the storage back end to save
   precious backup space.

Compatibility
-------------

Backward compatibility for backups is important so that our users are always
able to restore saved data. Therefore restic follows [Semantic
Versioning](http://semver.org) to clearly define which versions are compatible.
The repository and data structures contained therein are considered the "Public
API" in the sense of Semantic Versioning. This goes for all released versions
of restic, this may not be the case for the master branch.

We guarantee backward compatibility of all repositories within one major version;
as long as we do not increment the major version, data can be read and restored.
We strive to be fully backward compatible to all prior versions.

Contribute and Documentation
----------------------------

Contributions are welcome! More information can be found in the document
[`CONTRIBUTING.md`](https://github.com/restic/restic/blob/master/CONTRIBUTING.md).

Contact
-------

If you discover a bug, find something surprising or if you would like to
discuss or ask something, please [open a github
issue](https://github.com/restic/restic/issues/new). If you would like to chat
about restic, there is also the IRC channel #restic on irc.freenode.net.

**Important**: If you discover something that you believe to be a possible
critical security problem, please do *not* open a GitHub issue but send an
email directly to alexander@bumpern.de. If possible, please encrypt your email
using the following PGP key
([0x91A6868BD3F7A907](https://pgp.mit.edu/pks/lookup?op=get&search=0xCF8F18F2844575973F79D4E191A6868BD3F7A907)):

```
pub   4096R/91A6868BD3F7A907 2014-11-01
      Key fingerprint = CF8F 18F2 8445 7597 3F79  D4E1 91A6 868B D3F7 A907
      uid                          Alexander Neumann <alexander@bumpern.de>
      sub   4096R/D5FC2ACF4043FDF1 2014-11-01
```

Talks
-----

The following talks will be or have been given about restic:

 * 2016-01-31: Lightning Talk at the Go Devroom at FOSDEM 2016, Brussels, Belgium
 * 2016-01-29: [restic - Backups mal richtig](https://media.ccc.de/v/c4.openchaos.2016.01.restic): Public lecture in German at [CCC Cologne e.V.](https://koeln.ccc.de) in Cologne, Germany
 * 2015-08-23: [A Solution to the Backup Inconvenience](https://programm.froscon.de/2015/events/1515.html): Lecture at [FROSCON 2015](https://www.froscon.de) in Bonn, Germany
 * 2015-02-01: [Lightning Talk at FOSDEM 2015](https://www.youtube.com/watch?v=oM-MfeflUZ8&t=11m40s): A short introduction (with slightly outdated command line)
 * 2015-01-27: [Talk about restic at CCC Aachen](https://videoag.fsmpi.rwth-aachen.de/?view=player&lectureid=4442#content) (in German)

License
=======

Restic is licensed under "BSD 2-Clause License". You can find the complete text
in the file `LICENSE`.
