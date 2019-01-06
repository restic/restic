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

#############
Participating
#############

*********
Debugging
*********

The program can be built with debug support like this:

.. code-block:: console

    $ go run build.go -mod=vendor -tags debug

For Go < 1.11, the option ``-mod=vendor`` needs to be removed.

Afterwards, extensive debug messages are written to the file in
environment variable ``DEBUG_LOG``, e.g.:

.. code-block:: console

    $ DEBUG_LOG=/tmp/restic-debug.log restic backup ~/work

If you suspect that there is a bug, you can have a look at the debug
log. Please be aware that the debug log might contain sensitive
information such as file and directory names.

The debug log will always contain all log messages restic generates. You
can also instruct restic to print some or all debug messages to stderr.
These can also be limited to e.g. a list of source files or a list of
patterns for function names. The patterns are globbing patterns (see the
documentation for `path.Glob <https://golang.org/pkg/path/#Glob>`__), multiple
patterns are separated by commas. Patterns are case sensitive.

Printing all log messages to the console can be achieved by setting the
file filter to ``*``:

.. code-block:: console

    $ DEBUG_FILES=* restic check

If you want restic to just print all debug log messages from the files
``main.go`` and ``lock.go``, set the environment variable
``DEBUG_FILES`` like this:

.. code-block:: console

    $ DEBUG_FILES=main.go,lock.go restic check

The following command line instructs restic to only print debug
statements originating in functions that match the pattern ``*unlock*``
(case sensitive):

.. code-block:: console

    $ DEBUG_FUNCS=*unlock* restic check


************
Contributing
************

Contributions are welcome! Please **open an issue first** (or add a
comment to an existing issue) if you plan to work on any code or add a
new feature. This way, duplicate work is prevented and we can discuss
your ideas and design first.

More information and a description of the development environment can be
found in `CONTRIBUTING.md <https://github.com/restic/restic/blob/master/CONTRIBUTING.md>`__.
A document describing the design of restic and the data structures stored on the
back end is contained in `Design <https://restic.readthedocs.io/en/latest/design.html>`__.

If you'd like to start contributing to restic, but don't know exactly
what do to, have a look at this great article by Dave Cheney:
`Suggestions for contributing to an Open Source
project <https://dave.cheney.net/2016/03/12/suggestions-for-contributing-to-an-open-source-project>`__
A few issues have been tagged with the label ``help wanted``, you can
start looking at those:
https://github.com/restic/restic/labels/help%20wanted

********
Security
********

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

*************
Compatibility
*************

Backward compatibility for backups is important so that our users are
always able to restore saved data. Therefore restic follows `Semantic
Versioning <https://semver.org>`__ to clearly define which versions are
compatible. The repository and data structures contained therein are
considered the "Public API" in the sense of Semantic Versioning. This
goes for all released versions of restic, this may not be the case for
the master branch.

We guarantee backward compatibility of all repositories within one major
version; as long as we do not increment the major version, data can be
read and restored. We strive to be fully backward compatible to all
prior versions.

**********************
Building documentation
**********************

The restic documentation is built with `Sphinx <https://www.sphinx-doc.org>`__,
therefore building it locally requires a recent Python version and requirements listed in ``doc/requirements.txt``.
This example will guide you through the process using `virtualenv <https://virtualenv.pypa.io>`__:

::

  $ virtualenv venv # create virtual python environment
  $ source venv/bin/activate # activate the virtual environment
  $ cd doc
  $ pip install -r requirements.txt # install dependencies
  $ make html # build html documentation
  $ # open _build/html/index.html with your favorite browser
