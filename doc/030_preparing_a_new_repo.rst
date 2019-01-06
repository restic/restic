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

##########################
Preparing a new repository
##########################

The place where your backups will be saved is called a "repository".
This chapter explains how to create ("init") such a repository. The repository
can be stored locally, or on some remote server or service. We'll first cover
using a local repository; the remaining sections of this chapter cover all the
other options. You can skip to the next chapter once you've read the relevant
section here.

For automated backups, restic accepts the repository location in the
environment variable ``RESTIC_REPOSITORY``. For the password, several options
exist:

 * Setting the environment variable ``RESTIC_PASSWORD``

 * Specifying the path to a file with the password via the option
   ``--password-file`` or the environment variable ``RESTIC_PASSWORD_FILE``

 * Configuring a program to be called when the password is needed via the
   option ``--password-command`` or the environment variable
   ``RESTIC_PASSWORD_COMMAND``

Local
*****

In order to create a repository at ``/srv/restic-repo``, run the following
command and enter the same password twice:

.. code-block:: console

    $ restic init --repo /srv/restic-repo
    enter password for new backend:
    enter password again:
    created restic backend 085b3c76b9 at /srv/restic-repo
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

.. warning::

   Remembering your password is important! If you lose it, you won't be
   able to access data stored in the repository.

SFTP
****

In order to backup data via SFTP, you must first set up a server with
SSH and let it know your public key. Passwordless login is really
important since restic fails to connect to the repository if the server
prompts for credentials.

Once the server is configured, the setup of the SFTP repository can
simply be achieved by changing the URL scheme in the ``init`` command:

.. code-block:: console

    $ restic -r sftp:user@host:/srv/restic-repo init
    enter password for new backend:
    enter password again:
    created restic backend f1c6108821 at sftp:user@host:/srv/restic-repo
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

You can also specify a relative (read: no slash (``/``) character at the
beginning) directory, in this case the dir is relative to the remote
user's home directory.

.. note:: Please be aware that sftp servers do not expand the tilde character
          (``~``) normally used as an alias for a user's home directory. If you
          want to specify a path relative to the user's home directory, pass a
          relative path to the sftp backend.

The backend config string does not allow specifying a port. If you need
to contact an sftp server on a different port, you can create an entry
in the ``ssh`` file, usually located in your user's home directory at
``~/.ssh/config`` or in ``/etc/ssh/ssh_config``:

::

    Host foo
        User bar
        Port 2222

Then use the specified host name ``foo`` normally (you don't need to
specify the user name in this case):

::

    $ restic -r sftp:foo:/srv/restic-repo init

You can also add an entry with a special host name which does not exist,
just for use with restic, and use the ``Hostname`` option to set the
real host name:

::

    Host restic-backup-host
        Hostname foo
        User bar
        Port 2222

Then use it in the backend specification:

::

    $ restic -r sftp:restic-backup-host:/srv/restic-repo init

Last, if you'd like to use an entirely different program to create the
SFTP connection, you can specify the command to be run with the option
``-o sftp.command="foobar"``.


REST Server
***********

In order to backup data to the remote server via HTTP or HTTPS protocol,
you must first set up a remote `REST
server <https://github.com/restic/rest-server>`__ instance. Once the
server is configured, accessing it is achieved by changing the URL
scheme like this:

.. code-block:: console

    $ restic -r rest:http://host:8000/

Depending on your REST server setup, you can use HTTPS protocol,
password protection, multiple repositories or any combination of
those features. The TCP/IP port is also configurable. Here
are some more examples:

.. code-block:: console

    $ restic -r rest:https://host:8000/
    $ restic -r rest:https://user:pass@host:8000/
    $ restic -r rest:https://user:pass@host:8000/my_backup_repo/

If you use TLS, restic will use the system's CA certificates to verify the
server certificate. When the verification fails, restic refuses to proceed and
exits with an error. If you have your own self-signed certificate, or a custom
CA certificate should be used for verification, you can pass restic the
certificate filename via the ``--cacert`` option. It will then verify that the
server's certificate is contained in the file passed to this option, or signed
by a CA certificate in the file. In this case, the system CA certificates are
not considered at all.

REST server uses exactly the same directory structure as local backend,
so you should be able to access it both locally and via HTTP, even
simultaneously.

Amazon S3
*********

Restic can backup data to any Amazon S3 bucket. However, in this case,
changing the URL scheme is not enough since Amazon uses special security
credentials to sign HTTP requests. By consequence, you must first setup
the following environment variables with the credentials you obtained
while creating the bucket.

.. code-block:: console

    $ export AWS_ACCESS_KEY_ID=<MY_ACCESS_KEY>
    $ export AWS_SECRET_ACCESS_KEY=<MY_SECRET_ACCESS_KEY>

You can then easily initialize a repository that uses your Amazon S3 as
a backend. If the bucket does not exist it will be created in the
default location:

.. code-block:: console

    $ restic -r s3:s3.amazonaws.com/bucket_name init
    enter password for new backend:
    enter password again:
    created restic backend eefee03bbd at s3:s3.amazonaws.com/bucket_name
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

It is not possible at the moment to have restic create a new bucket in a
different location, so you need to create it using a different program.
Afterwards, the S3 server (``s3.amazonaws.com``) will redirect restic to
the correct endpoint.

Until version 0.8.0, restic used a default prefix of ``restic``, so the files
in the bucket were placed in a directory named ``restic``. If you want to
access a repository created with an older version of restic, specify the path
after the bucket name like this:

.. code-block:: console

    $ restic -r s3:s3.amazonaws.com/bucket_name/restic [...]

For an S3-compatible server that is not Amazon (like Minio, see below),
or is only available via HTTP, you can specify the URL to the server
like this: ``s3:http://server:port/bucket_name``.

Minio Server
************

`Minio <https://www.minio.io>`__ is an Open Source Object Storage,
written in Go and compatible with AWS S3 API.

-  Download and Install `Minio
   Server <https://minio.io/downloads/#minio-server>`__.
-  You can also refer to https://docs.minio.io for step by step guidance
   on installation and getting started on Minio Client and Minio Server.

You must first setup the following environment variables with the
credentials of your Minio Server.

.. code-block:: console

    $ export AWS_ACCESS_KEY_ID=<YOUR-MINIO-ACCESS-KEY-ID>
    $ export AWS_SECRET_ACCESS_KEY= <YOUR-MINIO-SECRET-ACCESS-KEY>

Now you can easily initialize restic to use Minio server as backend with
this command.

.. code-block:: console

    $ ./restic -r s3:http://localhost:9000/restic init
    enter password for new backend:
    enter password again:
    created restic backend 6ad29560f5 at s3:http://localhost:9000/restic1
    Please note that knowledge of your password is required to access
    the repository. Losing your password means that your data is irrecoverably lost.

OpenStack Swift
***************

Restic can backup data to an OpenStack Swift container. Because Swift supports
various authentication methods, credentials are passed through environment
variables. In order to help integration with existing OpenStack installations,
the naming convention of those variables follows the official Python Swift client:

.. code-block:: console

   # For keystone v1 authentication
   $ export ST_AUTH=<MY_AUTH_URL>
   $ export ST_USER=<MY_USER_NAME>
   $ export ST_KEY=<MY_USER_PASSWORD>

   # For keystone v2 authentication (some variables are optional)
   $ export OS_AUTH_URL=<MY_AUTH_URL>
   $ export OS_REGION_NAME=<MY_REGION_NAME>
   $ export OS_USERNAME=<MY_USERNAME>
   $ export OS_PASSWORD=<MY_PASSWORD>
   $ export OS_TENANT_ID=<MY_TENANT_ID>
   $ export OS_TENANT_NAME=<MY_TENANT_NAME>

   # For keystone v3 authentication (some variables are optional)
   $ export OS_AUTH_URL=<MY_AUTH_URL>
   $ export OS_REGION_NAME=<MY_REGION_NAME>
   $ export OS_USERNAME=<MY_USERNAME>
   $ export OS_PASSWORD=<MY_PASSWORD>
   $ export OS_USER_DOMAIN_NAME=<MY_DOMAIN_NAME>
   $ export OS_PROJECT_NAME=<MY_PROJECT_NAME>
   $ export OS_PROJECT_DOMAIN_NAME=<MY_PROJECT_DOMAIN_NAME>

   # For authentication based on tokens
   $ export OS_STORAGE_URL=<MY_STORAGE_URL>
   $ export OS_AUTH_TOKEN=<MY_AUTH_TOKEN>


Restic should be compatible with an `OpenStack RC file
<https://docs.openstack.org/user-guide/common/cli-set-environment-variables-using-openstack-rc.html>`__
in most cases.

Once environment variables are set up, a new repository can be created. The
name of the Swift container and optional path can be specified. If
the container does not exist, it will be created automatically:

.. code-block:: console

   $ restic -r swift:container_name:/path init   # path is optional
   enter password for new backend:
   enter password again:
   created restic backend eefee03bbd at swift:container_name:/path
   Please note that knowledge of your password is required to access the repository.
   Losing your password means that your data is irrecoverably lost.

The policy of the new container created by restic can be changed using environment variable:

.. code-block:: console

   $ export SWIFT_DEFAULT_CONTAINER_POLICY=<MY_CONTAINER_POLICY>


Backblaze B2
************

Restic can backup data to any Backblaze B2 bucket. You need to first setup the
following environment variables with the credentials you can find in the
dashboard in on the "Buckets" page when signed into your B2 account:

.. code-block:: console

    $ export B2_ACCOUNT_ID=<MY_APPLICATION_KEY_ID>
    $ export B2_ACCOUNT_KEY=<MY_SECRET_ACCOUNT_KEY>

.. note:: In case you want to use Backblaze Application Keys replace <MY_APPLICATION_KEY_ID> and <MY_SECRET_ACCOUNT_KEY> with <applicationKeyId> and <applicationKey> respectively.

You can then initialize a repository stored at Backblaze B2. If the
bucket does not exist yet and the credentials you passed to restic have the
privilege to create buckets, it will be created automatically:

.. code-block:: console

    $ restic -r b2:bucketname:path/to/repo init
    enter password for new backend:
    enter password again:
    created restic backend eefee03bbd at b2:bucketname:path/to/repo
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

Note that the bucket name must be unique across all of B2.

The number of concurrent connections to the B2 service can be set with the ``-o
b2.connections=10`` switch. By default, at most five parallel connections are
established.

Microsoft Azure Blob Storage
****************************

You can also store backups on Microsoft Azure Blob Storage. Export the Azure
account name and key as follows:

.. code-block:: console

    $ export AZURE_ACCOUNT_NAME=<ACCOUNT_NAME>
    $ export AZURE_ACCOUNT_KEY=<SECRET_KEY>

Afterwards you can initialize a repository in a container called ``foo`` in the
root path like this:

.. code-block:: console

    $ restic -r azure:foo:/ init
    enter password for new backend:
    enter password again:

    created restic backend a934bac191 at azure:foo:/
    [...]

The number of concurrent connections to the Azure Blob Storage service can be set with the
``-o azure.connections=10`` switch. By default, at most five parallel connections are
established.

Google Cloud Storage
********************

Restic supports Google Cloud Storage as a backend and connects via a `service account`_.

For normal restic operation, the service account must have the
``storage.objects.{create,delete,get,list}`` permissions for the bucket. These
are included in the "Storage Object Admin" role.
``restic init`` can create the repository bucket. Doing so requires the
``storage.buckets.create`` permission ("Storage Admin" role). If the bucket
already exists, that permission is unnecessary.

To use the Google Cloud Storage backend, first `create a service account key`_
and download the JSON credentials file.
Second, find the Google Project ID that you can see in the Google Cloud
Platform console at the "Storage/Settings" menu. Export the path to the JSON
key file and the project ID as follows:

.. code-block:: console

    $ export GOOGLE_PROJECT_ID=123123123123
    $ export GOOGLE_APPLICATION_CREDENTIALS=$HOME/.config/gs-secret-restic-key.json

Restic uses  Google's client library to generate `default authentication material`_,
which means if you're running in Google Container Engine or are otherwise
located on an instance with default service accounts then these should work out of 
the box.

Once authenticated, you can use the ``gs:`` backend type to create a new
repository in the bucket ``foo`` at the root path:

.. code-block:: console

    $ restic -r gs:foo:/ init
    enter password for new backend:
    enter password again:

    created restic backend bde47d6254 at gs:foo2/
    [...]

The number of concurrent connections to the GCS service can be set with the
``-o gs.connections=10`` switch. By default, at most five parallel connections are
established.

.. _service account: https://cloud.google.com/storage/docs/authentication#service_accounts
.. _create a service account key: https://cloud.google.com/storage/docs/authentication#generating-a-private-key
.. _default authentication material: https://developers.google.com/identity/protocols/application-default-credentials

Other Services via rclone
*************************

The program `rclone`_ can be used to access many other different services and
store data there. First, you need to install and `configure`_ rclone.  The
general backend specification format is ``rclone:<remote>:<path>``, the
``<remote>:<path>`` component will be directly passed to rclone. When you
configure a remote named ``foo``, you can then call restic as follows to
initiate a new repository in the path ``bar`` in the repo:

.. code-block:: console

    $ restic -r rclone:foo:bar init

Restic takes care of starting and stopping rclone.

As a more concrete example, suppose you have configured a remote named
``b2prod`` for Backblaze B2 with rclone, with a bucket called ``yggdrasil``.
You can then use rclone to list files in the bucket like this:

.. code-block:: console

    $ rclone ls b2prod:yggdrasil

In order to create a new repository in the root directory of the bucket, call
restic like this:

.. code-block:: console

    $ restic -r rclone:b2prod:yggdrasil init

If you want to use the path ``foo/bar/baz`` in the bucket instead, pass this to
restic:

.. code-block:: console

    $ restic -r rclone:b2prod:yggdrasil/foo/bar/baz init

Listing the files of an empty repository directly with rclone should return a
listing similar to the following:

.. code-block:: console

    $ rclone ls b2prod:yggdrasil/foo/bar/baz
        155 bar/baz/config
        448 bar/baz/keys/4bf9c78049de689d73a56ed0546f83b8416795295cda12ec7fb9465af3900b44

Rclone can be `configured with environment variables`_, so for instance
configuring a bandwidth limit for rclone can be achieved by setting the
``RCLONE_BWLIMIT`` environment variable:

.. code-block:: console

    $ export RCLONE_BWLIMIT=1M

For debugging rclone, you can set the environment variable ``RCLONE_VERBOSE=2``.

The rclone backend has two additional options:

 * ``-o rclone.program`` specifies the path to rclone, the default value is just ``rclone``
 * ``-o rclone.args`` allows setting the arguments passed to rclone, by default this is ``serve restic --stdio --b2-hard-delete --drive-use-trash=false``

The reason for the two last parameters (``--b2-hard-delete`` and
``--drive-use-trash=false``) can be found in the corresponding GitHub `issue #1657`_.

In order to start rclone, restic will build a list of arguments by joining the
following lists (in this order): ``rclone.program``, ``rclone.args`` and as the
last parameter the value that follows the ``rclone:`` prefix of the repository
specification.

So, calling restic like this

.. code-block:: console

    $ restic -o rclone.program="/path/to/rclone" \
      -o rclone.args="serve restic --stdio --bwlimit 1M --b2-hard-delete --verbose" \
      -r rclone:b2:foo/bar

runs rclone as follows:

.. code-block:: console

    $ /path/to/rclone serve restic --stdio --bwlimit 1M --b2-hard-delete --verbose b2:foo/bar

Manually setting ``rclone.program`` also allows running a remote instance of
rclone e.g. via SSH on a server, for example:

.. code-block:: console

    $ restic -o rclone.program="ssh user@host rclone" -r rclone:b2:foo/bar

The rclone command may also be hard-coded in the SSH configuration or the
user's public key, in this case it may be sufficient to just start the SSH
connection (and it's irrelevant what's passed after ``rclone:`` in the
repository specification):

.. code-block:: console

    $ restic -o rclone.program="ssh user@host" -r rclone:x

.. _rclone: https://rclone.org/
.. _configure: https://rclone.org/docs/
.. _configured with environment variables: https://rclone.org/docs/#environment-variables
.. _issue #1657: https://github.com/restic/restic/pull/1657#issuecomment-377707486

Password prompt on Windows
**************************

At the moment, restic only supports the default Windows console
interaction. If you use emulation environments like
`MSYS2 <https://msys2.github.io/>`__ or
`Cygwin <https://www.cygwin.com/>`__, which use terminals like
``Mintty`` or ``rxvt``, you may get a password error.

You can workaround this by using a special tool called ``winpty`` (look
`here <https://sourceforge.net/p/msys2/wiki/Porting/>`__ and
`here <https://github.com/rprichard/winpty>`__ for detail information).
On MSYS2, you can install ``winpty`` as follows:

.. code-block:: console

    $ pacman -S winpty
    $ winpty restic -r /srv/restic-repo init

