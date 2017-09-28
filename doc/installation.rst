Installation
============

Packages
--------

Mac OS X
~~~~~~~~~

If you are using Mac OS X, you can install restic using the
`homebrew <http://brew.sh/>`__ package manager:

.. code-block:: console

    $ brew tap restic/restic
    $ brew install restic

Arch Linux
~~~~~~~~~~

On `Arch Linux <https://www.archlinux.org/>`__, there is a package called ``restic-git`` which can be
installed from AUR, e.g. with ``pacaur``:

.. code-block:: console

    $ pacaur -S restic-git
    
Nix & NixOS
~~~~~~~~~~~

If you are using `Nix <https://nixos.org/nix/>`__ or `NixOS <https://nixos.org/>`__
there is a package available named ``restic``.
It can be installed uisng `nix-env`:

.. code-block:: console

    $ nix-env --install restic 

Pre-compiled Binary
-------------------

You can download the latest pre-compiled binary from the `restic release
page <https://github.com/restic/restic/releases/latest>`__.

From Source
-----------

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
