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

These are up-to-date binaries that are built in a reproducible and verifiable
way, which you can download and run without having to do additional
installation work.

Please see the :ref:`official_binaries` section below for various downloads.
Official binaries can be updated in place by using the ``restic self-update``
command.

Alpine Linux
============

On `Alpine Linux <https://www.alpinelinux.org>`__ you can install restic from
the official community repos using ``apk``:

.. code-block:: console

    $ apk add restic

Arch Linux
==========

On `Arch Linux <https://www.archlinux.org/>`__, you can install restic from the
official community repos using the Pacman package manager:

.. code-block:: console

    $ pacman -S restic

Debian
======

On `Debian <https://www.debian.org/>`__, you can install restic from the
official repos using ``apt-get``:

.. code-block:: console

    $ apt-get install restic


Fedora
======

On `Fedora <https://getfedora.org/>`__, restic can be installed using the DNF
package manager:

.. code-block:: console

    $ dnf install restic

If you have previously installed restic via Copr, remove the Copr repo as
follows to avoid any conflicts:

.. code-block:: console

   $ dnf copr remove copart/restic

macOS
=====

If you are using macOS, you can install restic using the
`Homebrew <https://brew.sh/>`__ package manager:

.. code-block:: console

    $ brew install restic

You can also install it using the `MacPorts <https://www.macports.org/>`__
package manager:

.. code-block:: console

    $ sudo port install restic

Nix & NixOS
===========

If you are using `Nix <https://nixos.org/nix/>`__ or
`NixOS <https://nixos.org/>`__ there is a package named ``restic`` which can
be installed using ``nix-env``:

.. code-block:: console

    $ nix-env --install restic

OpenBSD
=======

On `OpenBSD <https://www.openbsd.org/>`__ 6.3 and greater, you can install
restic using ``pkg_add``:

.. code-block:: console

    # pkg_add restic

FreeBSD
=======

On `FreeBSD <https://www.freebsd.org/>`__ (11 and probably later versions), you
can install restic using ``pkg``:

.. code-block:: console

    # pkg install restic

openSUSE
========

On `openSUSE <https://www.opensuse.org/>`__ (Leap 15.0 and greater, and
Tumbleweed), you can install restic using the Zypper package manager:

.. code-block:: console

    # zypper install restic

RHEL & CentOS
=============

For RHEL7/CentOS, you can try installing restic via the Copr repository:

.. code-block:: console

    $ yum install yum-plugin-copr
    $ yum copr enable copart/restic
    $ yum install restic

If that doesn't work, you can try adding the repository directly.
For CentOS 6 use:

.. code-block:: console

    $ yum-config-manager --add-repo https://copr.fedorainfracloud.org/coprs/copart/restic/repo/epel-6/copart-restic-epel-6.repo

For CentOS 7 use:

.. code-block:: console

    $ yum-config-manager --add-repo https://copr.fedorainfracloud.org/coprs/copart/restic/repo/epel-7/copart-restic-epel-7.repo

Solus
=====

Restic can be installed from the official repo of Solus via the ``eopkg``
package manager:

.. code-block:: console

    $ eopkg install restic

Windows
=======

On Windows, restic can be installed using the `Scoop <https://scoop.sh/>`__
package manager:

.. code-block:: console

    scoop install restic

Using this installation method, ``restic.exe`` will automatically be available
in the ``PATH``. It can be called from cmd.exe or PowerShell by typing
``restic``.


.. _official_binaries:

Official binaries
*****************

Stable releases
===============

You can download the latest stable release versions of restic from the `restic
release page <https://github.com/restic/restic/releases/latest>`__. These builds
are considered stable and releases are made regularly in a controlled manner.

There's both pre-compiled binaries for different platforms as well as the source
code available for download. Just download and run the one matching your system.

The official binaries can be updated in place using the ``restic self-update``
command (needs restic 0.9.3 or later):

.. code-block:: console

    $ restic version
    restic 0.9.3 compiled with go1.11.2 on linux/amd64

    $ restic self-update
    find latest release of restic at GitHub
    latest version is 0.9.4
    download file SHA256SUMS
    download SHA256SUMS
    download file SHA256SUMS
    download SHA256SUMS.asc
    GPG signature verification succeeded
    download restic_0.9.4_linux_amd64.bz2
    downloaded restic_0.9.4_linux_amd64.bz2
    saved 12115904 bytes in ./restic
    successfully updated restic to version 0.9.4

    $ restic version
    restic 0.9.4 compiled with go1.12.1 on linux/amd64

The ``self-update`` command uses the GPG signature on the files uploaded to
GitHub to verify their authenticity. No external programs are necessary.

.. note:: Please be aware that the user executing the ``restic self-update``
   command must have the permission to replace the restic binary.
   If you want to save the downloaded restic binary into a different file, pass
   the file name via the option ``--output``.

Unstable builds
===============

Another option is to use the latest builds for the master branch, available on
the `restic beta download site
<https://beta.restic.net/?sort=time&order=desc>`__. These too are pre-compiled
and ready to run, and a new version is built every time a push is made to the
master branch.

Windows
=======

On Windows, put the `restic.exe` binary into `%SystemRoot%\\System32` to use
restic in scripts without the need for absolute paths to the binary. This
requires administrator rights.

Docker container
****************

We're maintaining a bare docker container with just a few files and the restic
binary, you can get it with `docker pull` like this:

.. code-block:: console

    $ docker pull restic/restic

.. note::
   | Another docker container which offers more configuration options is
   | available as a contribution (Thank you!). You can find it at
   | https://github.com/Lobaro/restic-backup-docker

From source
***********

Restic is written in the Go programming language and you need at least
Go version 1.13. Building restic may also work with older versions of Go,
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

    The "generate" command writes automatically generated files (like the man pages
    and the auto-completion files for bash and zsh).

    Usage:
      restic generate [flags] [command]

    Flags:
          --bash-completion file   write bash completion file
      -h, --help                   help for generate
          --man directory          write man pages to directory
          --zsh-completion file    write zsh completion file

Example for using sudo to write a bash completion script directly to the system-wide
location:

.. code-block:: console

    $ sudo ./restic generate --bash-completion /etc/bash_completion.d/restic
    writing bash completion file to /etc/bash_completion.d/restic
