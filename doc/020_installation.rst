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

Note that if at any point the package you’re trying to use is outdated, you
always have the option to use an official binary from the restic project.

These are up to date binaries, built in a reproducible and verifiable way, that
you can download and run without having to do additional installation work.

Please see the :ref:`official_binaries` section below for various downloads.

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
   
RHEL & CentOS
=============

restic can be installed via copr repository, for RHEL7/CentOS you can try the following:

.. code-block:: console

    $ yum install yum-plugin-copr
    $ yum copr enable copart/restic
    $ yum install restic

If that doesn't work, you can try adding the repository directly, for CentOS6 use:

.. code-block:: console

    $ yum-config-manager --add-repo https://copr.fedorainfracloud.org/coprs/copart/restic/repo/epel-6/copart-restic-epel-6.repo

For CentOS7 use:

.. code-block:: console

    $ yum-config-manager --add-repo https://copr.fedorainfracloud.org/coprs/copart/restic/repo/epel-7/copart-restic-epel-7.repo

Fedora
======

restic can be installed via copr repository.

.. code-block:: console

    $ dnf install dnf-plugin-core
    $ dnf copr enable copart/restic
    $ dnf install restic

Solus
=====

restic can be installed from the official repo of Solus via the ``eopkg`` package manager:

.. code-block:: console

    $ eopkg install restic
    
OpenBSD
=======

On OpenBSD 6.3 and greater, you can install restic using ``pkg_add``:

.. code-block:: console

    # pkg_add restic

.. _official_binaries:

Official Binaries
*****************

Stable Releases
===============

You can download the latest stable release versions of restic from the `restic
release page <https://github.com/restic/restic/releases/latest>`__. These builds
are considered stable and releases are made regularly in a controlled manner.

There's both pre-compiled binaries for different platforms as well as the source
code available for download. Just download and run the one matching your system.

Unstable Builds
===============

Another option is to use the latest builds for the master branch, available on
the `restic beta download site
<https://beta.restic.net/?sort=time&order=desc>`__. These too are pre-compiled
and ready to run, and a new version is built every time a push is made to the
master branch.

Windows
=======

On Windows, put the `restic.exe` into `%SystemRoot%\System32` to use restic
in scripts without the need for absolute paths to the binary. This requires
Admin rights.

Docker Container
****************

We're maintaining a bare docker container with just a few files and the restic
binary, you can get it with `docker pull` like this:

.. code-block:: console

    $ docker pull restic/restic

.. note::
   | Another docker container which offers more configuration options is
   | available as a contribution (Thank you!). You can find it at
   | https://github.com/Lobaro/restic-backup-docker

From Source
***********

restic is written in the Go programming language and you need at least
Go version 1.9. Building restic may also work with older versions of Go,
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

.. code-block:: console

    $ go run build.go --goos windows --goarch amd64

    $ go run build.go --goos freebsd --goarch 386

    $ go run build.go --goos linux --goarch arm --goarm 6
    
The resulting binary is statically linked and does not require any
libraries.

At the moment, the only tested compiler for restic is the official Go
compiler. Building restic with gccgo may work, but is not supported.

Autocompletion
**************

Restic can write out man pages and bash/zsh compatible autocompletion scripts:

.. code-block:: console

    $ ./restic generate --help

    The "generate" command writes automatically generated files like the man pages
    and the auto-completion files for bash and zsh).

    Usage:
      restic generate [command] [flags]

    Flags:
          --bash-completion file   write bash completion file
      -h, --help                   help for generate
          --man directory          write man pages to directory
          --zsh-completion file    write zsh completion file

Example for using sudo to write a bash completion script directly to the system-wide location:

.. code-block:: console

    $ sudo ./restic generate --bash-completion /etc/bash_completion.d/restic
    writing bash completion file to /etc/bash_completion.d/restic
