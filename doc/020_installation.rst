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
Official binaries can be updated in place by using the ``restic self-update``
command.

Alpine Linux
============

On `Alpine Linux <https://www.alpinelinux.org>`__ you can install the ``restic``
package from the official community repos, e.g. using ``apk``:

.. code-block:: console

    $ apk add restic

Arch Linux
==========

On `Arch Linux <https://www.archlinux.org/>`__, there is a package called ``restic``
installed from the official community repos, e.g. with ``pacman -S``:

.. code-block:: console

    $ pacman -S restic

Debian
======

On Debian, there's a package called ``restic`` which can be
installed from the official repos, e.g. with ``apt-get``:

.. code-block:: console

    $ apt-get install restic


Fedora
======

restic can be installed using ``dnf``:

.. code-block:: console

    $ dnf install restic

If you used restic from copr previously, remove the copr repo as follows to
avoid any conflicts:

.. code-block:: console

   $ dnf copr remove copart/restic

macOS
=====

If you are using macOS, you can install restic using the
`homebrew <https://brew.sh/>`__ package manager:

.. code-block:: console

    $ brew install restic

You may also install it using `MacPorts <https://www.macports.org/>`__:

.. code-block:: console

    $ sudo port install restic

Nix & NixOS
===========

If you are using `Nix <https://nixos.org/nix/>`__ or `NixOS <https://nixos.org/>`__
there is a package available named ``restic``.
It can be installed using ``nix-env``:

.. code-block:: console

    $ nix-env --install restic

OpenBSD
=======

On OpenBSD 6.3 and greater, you can install restic using ``pkg_add``:

.. code-block:: console

    # pkg_add restic

FreeBSD
=======

On FreeBSD (11 and probably later versions), you can install restic using ``pkg install``:

.. code-block:: console

    # pkg install restic

openSUSE
========

On openSUSE (leap 15.0 and greater, and tumbleweed), you can install restic using the ``zypper`` package manager:

.. code-block:: console

    # zypper install restic

RHEL & CentOS
=============

For RHEL / CentOS Stream 8 & 9 restic can be installed from the EPEL repository:

.. code-block:: console

    $ dnf install epel-release
    $ dnf install restic

For RHEL7/CentOS there is a copr repository available, you can try the following:

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

Solus
=====

restic can be installed from the official repo of Solus via the ``eopkg`` package manager:

.. code-block:: console

    $ eopkg install restic

Windows
=======

restic can be installed using `Scoop <https://scoop.sh/>`__:

.. code-block:: console

    scoop install restic

Using this installation method, ``restic.exe`` will automatically be available
in the ``PATH``. It can be called from cmd.exe or PowerShell by typing ``restic``.


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

On your first installation, if you desire, you can verify the integrity of your
downloads by testing the SHA-256 checksums listed in ``SHA256SUMS`` and verifying
the integrity of the file ``SHA256SUMS`` with the PGP signature in ``SHA256SUMS.asc``. 
The PGP signature was created using the key (`0x91A6868BD3F7A907 <https://restic.net/gpg-key-alex.asc>`__):

::

    pub   4096R/91A6868BD3F7A907 2014-11-01
          Key fingerprint = CF8F 18F2 8445 7597 3F79  D4E1 91A6 868B D3F7 A907
          uid                          Alexander Neumann <alexander@bumpern.de>
          sub   4096R/D5FC2ACF4043FDF1 2014-11-01

Once downloaded, the official binaries can be updated in place using the 
``restic self-update`` command (needs restic 0.9.3 or later):

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

Unstable Builds
===============

Another option is to use the latest builds for the master branch, available on
the `restic beta download site
<https://beta.restic.net/?sort=time&order=desc>`__. These too are pre-compiled
and ready to run, and a new version is built every time a push is made to the
master branch.

Windows
=======

On Windows, put the `restic.exe` binary into `%SystemRoot%\\System32` to use restic
in scripts without the need for absolute paths to the binary. This requires
administrator rights.

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
Go version 1.15. Building restic may also work with older versions of Go,
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

    $ go run build.go --goos solaris --goarch amd64

The resulting binary is statically linked and does not require any
libraries.

At the moment, the only tested compiler for restic is the official Go
compiler. Building restic with gccgo may work, but is not supported.

Autocompletion
**************

Restic can write out man pages and bash/fish/zsh compatible autocompletion scripts:

.. code-block:: console

    $ ./restic generate --help

    The "generate" command writes automatically generated files (like the man pages
    and the auto-completion files for bash, fish and zsh).

    Usage:
      restic generate [flags] [command]

    Flags:
          --bash-completion file   write bash completion file
          --fish-completion file   write fish completion file
      -h, --help                   help for generate
          --man directory          write man pages to directory
          --zsh-completion file    write zsh completion file

Example for using sudo to write a bash completion script directly to the system-wide location:

.. code-block:: console

    $ sudo ./restic generate --bash-completion /etc/bash_completion.d/restic
    writing bash completion file to /etc/bash_completion.d/restic

.. note:: The path for the ``--bash-completion`` option may vary depending on
   the operating system used, e.g. ``/usr/share/bash-completion/completions/restic``
   in Debian and derivatives. Please look up the correct path in the appropriate
   documentation.
