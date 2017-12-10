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

############
Installation
############

Packages
********

Mac OS X
========

If you are using Mac OS X, you can install restic using the
`homebrew <http://brew.sh/>`__ package manager:

.. code-block:: console

    $ brew install restic

Arch Linux
==========

On `Arch Linux <https://www.archlinux.org/>`__, there is a package called ``restic-git``
which can be installed from AUR, e.g. with ``pacaur``:

.. code-block:: console

    $ pacaur -S restic-git

Nix & NixOS
===========

If you are using `Nix <https://nixos.org/nix/>`__ or `NixOS <https://nixos.org/>`__
there is a package available named ``restic``.
It can be installed uisng ``nix-env``:

.. code-block:: console

    $ nix-env --install restic 

Debian
======

On Debian, there's a package called ``restic`` which can be
installed from the official repos, e.g. with ``apt-get``:

.. code-block:: console

    $ apt-get install restic


.. warning:: Please be aware that, at the time of writing, Debian *stable*
   has ``restic`` version 0.3.3 which is very old. The *testing* and *unstable*
   branches have recent versions of ``restic``.

Pre-compiled Binary
*******************

You can download the latest pre-compiled binary from the `restic release
page <https://github.com/restic/restic/releases/latest>`__.

Docker Container
****************

.. note::
   | A docker container is available as a contribution (Thank you!).
   | You can find it at https://github.com/Lobaro/restic-backup-docker

From Source
***********

restic is written in the Go programming language and you need at least
Go version 1.8. Building restic may also work with older versions of Go,
but that's not supported. See the `Getting
started <https://golang.org/doc/install>`__ guide of the Go project for
instructions how to install Go.

In order to build restic from source, execute the following steps:

.. code-block:: console

    $ git clone https://github.com/restic/restic
    [...]

    $ cd restic

    $ go run build.go

You can easily cross-compile restic for all supported platforms, just
supply the target OS and platform via the command-line options like this
(for Windows and FreeBSD respectively):

::

    $ go run build.go --goos windows --goarch amd64

    $ go run build.go --goos freebsd --goarch 386

The resulting binary is statically linked and does not require any
libraries.

At the moment, the only tested compiler for restic is the official Go
compiler. Building restic with gccgo may work, but is not supported.

Autocompletion
**************

Restic can write out a bash compatible autocompletion script:

.. code-block:: console

    $ ./restic autocomplete --help
    The "autocomplete" command generates a shell autocompletion script.

    NOTE: The current version supports Bash only.
          This should work for *nix systems with Bash installed.

By default, the file is written directly to ``/etc/bash_completion.d/``
for convenience, and the command may need superuser rights, e.g.

.. code-block:: console

    $ sudo restic autocomplete

    Usage:
      restic autocomplete [flags]

    Flags:
          --completionfile string   autocompletion file (default "/etc/bash_completion.d/restic.sh")

    Global Flags:
          --json                   set output mode to JSON for commands that support it
          --no-lock                do not lock the repo, this allows some operations on read-only repos
      -o, --option key=value       set extended option (key=value, can be specified multiple times)
      -p, --password-file string   read the repository password from a file
      -q, --quiet                  do not output comprehensive progress report
      -r, --repo string            repository to backup to or restore from (default: $RESTIC_REPOSITORY)


