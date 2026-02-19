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

**********
Debug Logs
**********

Set the environment variable ``DEBUG_LOG`` to let restic write extensive debug
messages to the specified filed, e.g.:

.. code-block:: console

    $ DEBUG_LOG=/tmp/restic-debug.log restic backup ~/work

If you suspect that there is a bug, you can have a look at the debug
log. Please be aware that the debug log might contain sensitive
information such as file and directory names.

The debug log will always contain all log messages restic generates. You
can also instruct restic to print some or all debug messages to stderr.
These can also be limited to e.g. a list of source files or a list of
patterns for function names. The patterns are globbing patterns (see the
documentation for `filepath.Match <https://pkg.go.dev/path/filepath#Match>`__).
Multiple patterns are separated by commas. Patterns are case sensitive.

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


*********
Debugging
*********

The program can be built with debug support like this:

.. code-block:: console

    $ go run build.go -tags debug

This will make the ``restic debug <subcommand>`` available which can be used to
inspect internal data structures.

In addition, this enables profiling flags such as ``--cpu-profile`` and
``--mem-profile`` which can help with investigation performance and memory usage
issues. See ``restic help`` for more details and a few additional
``--...-profile`` flags.

Running Restic with profiling enabled generates a ``.pprof`` file such as
``cpu.pprof``. To view a profile in a web browser, first make sure that the
``dot`` command from `Graphviz <https://graphviz.org/>`__ is in the PATH. Then,
run ``go tool pprof -http : cpu.pprof``.


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
project <https://dave.cheney.net/2016/03/12/suggestions-for-contributing-to-an-open-source-project>`__.
A few issues have been tagged with the label ``help wanted``, you can
start looking at `those <https://github.com/restic/restic/labels/help%3A%20wanted>`_.


*************
Writing tests
*************

In case you want or need to create tests for an enhancement or a new feature of restic,
here is a brief description of how to write tests.

Tests are typically falling into two categories: functional tests (unit tests) and integration tests.
Functional tests will verify the correct workings of a function or a set of functions.
See more on integration tests below.

The restic test package located in ``internal/test``, here named ``rtest``,
provides the following very basic test functions:
::

 rtest.Equals(t, a, b, msg)         compares two values, fails test if a != b, optional ``msg`` string
                                    for detailed message
 rtest.Assert(t, a == b, "msg", more variables a, b) checks for a condition to be true,
                                    <msg> is a format string to represent the values of <a and b>
 rtest.OK(t, err)                   expects err to be ``nil``, otherwise fails test
 rtest.OKs(t, errs)                 expects a slice of errs to be ``nil``


Functional tests
================

The packages in ``internal/...`` often provide ``Test*(...)`` functions and structs or
have a dedicated test package like ``internal/backend/test``.
A good starting points are also the `testing.go` files that exist in several places.
Functional tests are stored in the same directory as their function, e.g.
``internal/<sub-component>/<function>_test.go``.

Tests in ``internal`` that need a full repository should just create one using the
memory backend by calling ``repository.TestRepository(t)``.

``checker.TestCheckRepo()`` can be used to verify the repository integrity.

In general, in-memory operation should be preferred over creating temporary files.
If necessary, temporary files can be stored in ``t.TempDir()``. However, in most cases test code
only requires a few of the methods provided by a ``Repository``.
Then some helper like ``data.TestTreeMap`` can be used or just create a basic mock yourself.

For backends the test suite in ``internal/backend/test`` is mandatory.

To temporarily enable feature flags in tests, use
``defer feature.TestSetFlag(t, feature.Flag, feature.DeviceIDForHardlinks, true)()``.
Such tests must not run in parallel as this changes global state.


Integration tests
=================

The classical helpers for integration tests are, amongst others:

- ``env, cleanup := withTestEnvironment(t)``: build an environment for tests
- ``datafile := testSetupBackupData(t, env)``: initialize a repo, unpack standard backup tree structure
- ``testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)``: backup all of the standard tree structure
- ``testListSnapshots(t, env.gopts, <n>)``: check that there are <n> snapshots in the repository
- ``testRunCheck(t, env.gopts)``: check that the repository is sound and happy
- the above mentioned ``rtest.OK()``, ``rtest.Equals()``, ``rtest.Assert()`` helpers
- ``withCaptureStdout()`` and ``withTermStatus()`` wrappers: both functions are found in ``cmd/restic/integration_helpers_test.go`` for creating an enviroment where one can analyze the output created by the ``testRunXXX()`` command, particularly when checking JSON output

Integration tests test the overall workings of a command. Integration tests are used for commands and
are stored in the same directory ``cmd/restic``. The recommended naming convention is
``cmd_<command>_integration_test.go``.
See the ``cmd/restic/*_integration_test.go`` files for further details.
A lot of the base helpers are found in ``cmd/restic/integration_helpers_test.go``.

This is a typical setting for an integration test:

- run a ``backup``, compare number of files backup with the expected number of files
- run a ``backup``, run the ``ls`` command with a ``sort`` option and compare actual output with the expected output.

For all backup related functions there is a directory tree which can be used for a
default backup, to be found at ``cmd/restic/testdata/backup-data.tar.gz``.
In this compressed archive you will find files, hardlinked files,
symlinked files, an empty directory and a simple directory structure which is good for testing purposes.

Commands that require a ``progress.Printer`` should either be wrapped in ``withTermStatus`` or ``withCaptureStdout``.
If you want to analyze JSON output, you use ``withCaptureStdout()``.
It returns the  generated output in a ``*bytes.Buffer``.
JSON output can be unmarshalled to produce the approriate go structures; see
``cmd/restic/cmd_find_integration_test.go`` as an example.

Example: this is a typical setup for a backup / find scenario
::

 import (
   ... // all your other imports here

   rtest "github.com/restic/restic/internal/test"
 )

 // setup test
 env, cleanup := withTestEnvironment(t)
 defer cleanup()

 // init repository and expand compressed archive into a tree structure
 testSetupBackupData(t, env)

 // run one backup
 opts := BackupOptions{}
 testRunBackup(t, env.testdata+"/0", []string{"."}, opts, env.gopts)

 // make sure we have exactly one snapshot
 testListSnapshots(t, env.gopts, 1)

 // run command ``restic XXX``
 // notice that we use the existing wrapper 'testRunXXXX', here 'testRunFind'.
 // whenever possible use the existing wrapper or modify an existing wrapper
 // to suit your extra needs.
 // any remotely complex assertion should be extracted into a reusable helper function.
 // ``testRunFind()`` uses ``withCaptureStdout()`` to capture output text (in ``results``)
 results := testRunFind(t, false, FindOptions{}, env.gopts, "testfile")

 // there is always a ``\n`` at  the end of the output!
 lines := strings.Split(string(results), "\n")

 // make sure that we have correct output
 rtest.Assert(t, len(lines) == 2, "expected one file, found (%v) in repo", len(lines)-1)


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
considered the "Public API" in the sense of Semantic Versioning.

Once version 1.0.0 is released, we guarantee backward compatibility of
all repositories within one major version; as long as we do not
increment the major version, data can be read and restored. We strive
to be fully backward compatible to all prior versions.

During initial development (versions prior to 1.0.0), maintainers and
developers will do their utmost to keep backwards compatibility and
stability, although there might be breaking changes without increasing
the major version.

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
