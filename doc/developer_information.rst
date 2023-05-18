Developer Information
#####################

Reproducible Builds
*******************

This section describes how to reproduce the official released binaries for
restic for version 0.10.0 and later. For restic versions down to 0.9.3 please
refer to the documentation for the respective version. The binary produced
depends on the following things:

 * The source code for the release
 * The exact version of the official `Go compiler <https://go.dev>`__ used to produce the binaries (running ``restic version`` will print this)
 * The architecture and operating system the Go compiler runs on (Linux, ``amd64``)
 * The build tags (for official binaries, it's the tag ``selfupdate``)
 * The path where the source code is extracted to (``/restic``)
 * The path to the Go compiler (``/usr/local/go``)
 * The path to the Go workspace (``GOPATH=/home/build/go``)
 * Other environment variables (mostly ``$GOOS``, ``$GOARCH``, ``$CGO_ENABLED``)

In addition, The compressed ZIP files for Windows depends on the modification
timestamp and filename of the binary contained in it. In order to reproduce the
exact same ZIP file every time, we update the timestamp of the file ``VERSION``
in the source code archive and set the timezone to Europe/Berlin.

In the following example, we'll use the file ``restic-0.14.0.tar.gz`` and Go
1.19 to reproduce the released binaries.

1. Determine the Go compiler version used to build the released binaries, then download and extract the Go compiler into ``/usr/local/go``:

.. code::

    $ restic version
    restic 0.14.0 compiled with go1.19 on linux/amd64
    $ cd /usr/local
    $ curl -L https://dl.google.com/go/go1.19.linux-amd64.tar.gz | tar xz

2. Extract the restic source code into ``/restic``

.. code::

    $ mkdir /restic
    $ cd /restic
    $ TZ=Europe/Berlin curl -L https://github.com/restic/restic/releases/download/v0.14.0/restic-0.14.0.tar.gz | tar xz --strip-components=1

3. Build the binaries for Windows and Linux:

.. code::

    $ export PATH=/usr/local/go/bin:$PATH
    $ export GOPATH=/home/build/go
    $ go version
    go version go1.19 linux/amd64

    $ GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -tags selfupdate -o restic_linux_amd64 ./cmd/restic
    $ bzip2 restic_linux_amd64

    $ GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -tags selfupdate -o restic_0.14.0_windows_amd64.exe ./cmd/restic
    $ touch --reference VERSION restic_0.14.0_windows_amd64.exe
    $ TZ=Europe/Berlin zip -q -X restic_0.14.0_windows_amd64.zip restic_0.14.0_windows_amd64.exe

Building the Official Binaries
******************************

The released binaries for restic are built using a Docker container. You can
find it on `Docker Hub <https://hub.docker.com/r/restic/builder>`__ as
``restic/builder``, the ``Dockerfile`` and instructions on how to build the
container can be found in the `GitHub repository
<https://github.com/restic/builder>`__

The container serves the following goals:
 * Have a very controlled environment which is independent from the local system
 * Make it easy to have the correct version of the Go compiler at the right path
 * Make it easy to pass in the source code to build at a well-defined path

The following steps are necessary to build the binaries:

1. Either build the container (see the instructions in the `repository's README <https://github.com/restic/builder>`__). Alternatively, download the container from the hub:

.. code::

    docker pull restic/builder

2. Extract the source code somewhere:

.. code::

    tar xvzf restic-0.14.0.tar.gz

3. Create a directory to place the resulting binaries in:

.. code::

    mkdir output

3. Mount the source code and the output directory in the container and run the default command, which starts ``helpers/build-release-binaries/main.go``:

.. code::

    docker run --rm \
        --volume "$PWD/restic-0.14.0:/restic" \
        --volume "$PWD/output:/output" \
        restic/builder \
        go run helpers/build-release-binaries/main.go --version 0.14.0

4. If anything goes wrong, you can enable debug output like this:

.. code::

    docker run --rm \
        --volume "$PWD/restic-0.14.0:/restic" \
        --volume "$PWD/output:/output" \
        restic/builder \
        go run helpers/build-release-binaries/main.go --version 0.14.0 --verbose

Prepare a New Release
*********************

Publishing a new release of restic requires many different steps. We've
automated this in the Go program ``helpers/prepare-release/main.go`` which also
includes checking that e.g. the changelog is correctly generated. The only
required argument is the new version number (in `Semantic Versioning
<https://semver.org/>`__ format ``MAJOR.MINOR.PATCH``):

.. code::

    go run helpers/prepare-release/main.go 0.14.0

Checks can be skipped on demand via flags, please see ``--help`` for details.
