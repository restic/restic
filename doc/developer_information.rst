Developer Information
#####################

Reproducible Builds
*******************

This section describes how to reproduce the official released binaries for
restic for version 0.9.3 and later. The binary produced depends on the
following things:

 * The source code for the release
 * The exact version of the official `Go compiler <https://golang.org>`__ used to produce the binaries (running ``restic version`` will print this)
 * The architecture and operating system the Go compiler runs on (Linux, ``amd64``)
 * The path where the source code is extracted to (``/restic``)
 * The path to the Go compiler (``/usr/local/go``)
 * The build tags (for official binaries, it's the tag ``selfupdate``)
 * The environment variables (mostly ``$GOOS``, ``$GOARCH``, ``$CGO_ENABLED``)

In addition, The compressed ZIP files for Windows depends on the modification
timestamp of the binary contained in it. In order to reproduce the exact same
ZIP file every time, we update the timestamp of the file ``VERSION`` in the
source code archive and set the timezone to Europe/Berlin.

In the following example, we'll use the file ``restic-0.9.3.tar.gz`` and Go
1.11.1 to reproduce the released binaries.

1. Download and extract the Go compiler into ``/usr/local/go``:

.. code::

    # cd /usr/local
    # curl -L https://dl.google.com/go/go1.11.1.linux-amd64.tar.gz | tar xz

2. Extract the restic source code into ``/restic``

.. code::

    # mkdir /restic
    # cd /restic
    # TZ=Europe/Berlin curl -L https://github.com/restic/restic/releases/download/v0.9.3/restic-0.9.3.tar.gz | tar xz --strip-components=1

3. Build the binaries for Windows and Linux:

.. code::

    $ export PATH=/usr/local/go/bin:$PATH
    $ go version
    go version go1.11.1 linux/amd64

    $ GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=vendor -ldflags "-s -w" -tags selfupdate -o restic_linux_amd64 ./cmd/restic
    $ bzip2 restic_linux_amd64

    $ GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -mod=vendor -ldflags "-s -w" -tags selfupdate -o restic_windows_amd64.exe ./cmd/restic
    $ touch --reference VERSION restic_windows_amd64.exe
    $ TZ=Europe/Berlin zip -q -X restic_windows_amd64.zip restic_windows_amd64.exe
