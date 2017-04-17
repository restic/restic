Development
===========

Contribute
----------
Contributions are welcome! Please **open an issue first** (or add a
comment to an existing issue) if you plan to work on any code or add a
new feature. This way, duplicate work is prevented and we can discuss
your ideas and design first.

More information and a description of the development environment can be
found in `CONTRIBUTING.md <CONTRIBUTING.md>`__.
A document describing the design of restic and the data structures stored on the
back end is contained in `Design <https://restic.readthedocs.io/en/latest/design.html>`__.

If you'd like to start contributing to restic, but don't know exactly
what do to, have a look at this great article by Dave Cheney:
`Suggestions for contributing to an Open Source
project <http://dave.cheney.net/2016/03/12/suggestions-for-contributing-to-an-open-source-project>`__
A few issues have been tagged with the label ``help wanted``, you can
start looking at those:
https://github.com/restic/restic/labels/help%20wanted

Security
--------
**Important**: If you discover something that you believe to be a
possible critical security problem, please do *not* open a GitHub issue
but send an email directly to alexander@bumpern.de. If possible, please
encrypt your email using the following PGP key
(`0x91A6868BD3F7A907 <https://pgp.mit.edu/pks/lookup?op=get&search=0xCF8F18F2844575973F79D4E191A6868BD3F7A907>`__):

::

    pub   4096R/91A6868BD3F7A907 2014-11-01
          Key fingerprint = CF8F 18F2 8445 7597 3F79  D4E1 91A6 868B D3F7 A907
          uid                          Alexander Neumann <alexander@bumpern.de>
          sub   4096R/D5FC2ACF4043FDF1 2014-11-01

Compatibility
-------------

Backward compatibility for backups is important so that our users are
always able to restore saved data. Therefore restic follows `Semantic
Versioning <http://semver.org>`__ to clearly define which versions are
compatible. The repository and data structures contained therein are
considered the "Public API" in the sense of Semantic Versioning. This
goes for all released versions of restic, this may not be the case for
the master branch.

We guarantee backward compatibility of all repositories within one major
version; as long as we do not increment the major version, data can be
read and restored. We strive to be fully backward compatible to all
prior versions.

Building documentation
----------------------

The restic documentation is built with `Sphinx <http://sphinx-doc.org>`__,
therefore building it locally requires a recent Python version and requirements listed in ``doc/requirements.txt``.
This example will guide you through the process using `virtualenv <https://virtualenv.pypa.io>`__:

::

  $ virtualenv venv # create virtual python environment
  $ source venv/bin/activate # activate the virtual environment
  $ cd doc
  $ pip install -r requirements.txt # install dependencies
  $ make html # build html documentation
  $ # open _build/html/index.html with your favorite browser
