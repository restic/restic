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

The place where your backups will be saved is called a "repository". This is
simply a directory containing a set of subdirectories and files created by
restic to store your backups, some corresponding metadata and encryption keys.

To access the repository, a password (also called a key) must be specified. A
repository can hold multiple keys that can all be used to access the repository.

This chapter explains how to create ("init") such a repository. The repository
can be stored locally, or on some remote server or service. We'll first cover
using a local repository; the remaining sections of this chapter cover all the
other options. You can skip to the next chapter once you've read the relevant
section here.

For automated backups, restic supports specifying the repository location in the
environment variable ``RESTIC_REPOSITORY``. Restic can also read the repository
location from a file specified via the ``--repository-file`` option or the
environment variable ``RESTIC_REPOSITORY_FILE``.

For automating the supply of the repository password to restic, several options
exist:

 * Setting the environment variable ``RESTIC_PASSWORD``

 * Specifying the path to a file with the password via the option
   ``--password-file`` or the environment variable ``RESTIC_PASSWORD_FILE``

 * Configuring a program to be called when the password is needed via the
   option ``--password-command`` or the environment variable
   ``RESTIC_PASSWORD_COMMAND``
   
The ``init`` command has an option called ``--repository-version`` which can
be used to explicitly set the version of the new repository. By default, the
current stable version is used (see table below). The alias ``latest`` will
always resolve to the latest repository version. Have a look at the `design
documentation <https://github.com/restic/restic/blob/master/doc/design.rst>`__
for more details.

The below table shows which restic version is required to use a certain
repository version, as well as notable features introduced in the various
versions.

+--------------------+-------------------------+---------------------+------------------+
| Repository version | Required restic version | Major new features  | Comment          |
+====================+=========================+=====================+==================+
| ``1``              | Any                     |                     |                  |
+--------------------+-------------------------+---------------------+------------------+
| ``2``              | 0.14.0 or newer         | Compression support | Current default  |
+--------------------+-------------------------+---------------------+------------------+


Local
*****

In order to create a repository at ``/srv/restic-repo``, run the following
command and enter the same password twice:

.. code-block:: console

    $ restic init --repo /srv/restic-repo
    enter password for new repository:
    enter password again:
    created restic repository 085b3c76b9 at /srv/restic-repo
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

.. warning::

   Remembering your password is important! If you lose it, you won't be
   able to access data stored in the repository.

.. warning::

   On Linux, storing the backup repository on a CIFS (SMB) share is not
   recommended due to compatibility issues. Either use another backend
   or set the environment variable `GODEBUG` to `asyncpreemptoff=1`.
   Refer to GitHub issue `#2659 <https://github.com/restic/restic/issues/2659>`_ for further explanations.

SFTP
****

In order to backup data via SFTP, you must first set up a server with
SSH and let it know your public key. Passwordless login is important
since automatic backups are not possible if the server prompts for
credentials.

Once the server is configured, the setup of the SFTP repository can
simply be achieved by changing the URL scheme in the ``init`` command:

.. code-block:: console

    $ restic -r sftp:user@host:/srv/restic-repo init
    enter password for new repository:
    enter password again:
    created restic repository f1c6108821 at sftp:user@host:/srv/restic-repo
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

You can also specify a relative (read: no slash (``/``) character at the
beginning) directory, in this case the dir is relative to the remote
user's home directory.

Also, if the SFTP server is enforcing domain-confined users, you can
specify the user this way: ``user@domain@host``.

.. note:: Please be aware that sftp servers do not expand the tilde character
          (``~``) normally used as an alias for a user's home directory. If you
          want to specify a path relative to the user's home directory, pass a
          relative path to the sftp backend.

If you need to specify a port number or IPv6 address, you'll need to use
URL syntax. E.g., the repository ``/srv/restic-repo`` on ``[::1]`` (localhost)
at port 2222 with username ``user`` can be specified as

::

    sftp://user@[::1]:2222//srv/restic-repo

Note the double slash: the first slash separates the connection settings from
the path, while the second is the start of the path. To specify a relative
path, use one slash.

Alternatively, you can create an entry in the ``ssh`` configuration file,
usually located in your home directory at ``~/.ssh/config`` or in
``/etc/ssh/ssh_config``:

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

.. note:: Please be aware that sftp servers close connections when no data is
          received by the client. This can happen when restic is processing huge
          amounts of unchanged data. To avoid this issue add the following lines 
          to the client's .ssh/config file:

::

    ServerAliveInterval 60
    ServerAliveCountMax 240
          
          
REST Server
***********

In order to backup data to the remote server via HTTP or HTTPS protocol,
you must first set up a remote `REST
server <https://github.com/restic/rest-server>`__ instance. Once the
server is configured, accessing it is achieved by changing the URL
scheme like this:

.. code-block:: console

    $ restic -r rest:http://host:8000/ init

Depending on your REST server setup, you can use HTTPS protocol,
password protection, multiple repositories or any combination of
those features. The TCP/IP port is also configurable. Here
are some more examples:

.. code-block:: console

    $ restic -r rest:https://host:8000/ init
    $ restic -r rest:https://user:pass@host:8000/ init
    $ restic -r rest:https://user:pass@host:8000/my_backup_repo/ init

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
    enter password for new repository:
    enter password again:
    created restic repository eefee03bbd at s3:s3.amazonaws.com/bucket_name
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

If needed, you can manually specify the region to use by either setting the
environment variable ``AWS_DEFAULT_REGION`` or calling restic with an option
parameter like ``-o s3.region="us-east-1"``. If the region is not specified,
the default region is used. Afterwards, the S3 server (at least for AWS,
``s3.amazonaws.com``) will redirect restic to the correct endpoint.

When using temporary credentials make sure to include the session token via
then environment variable ``AWS_SESSION_TOKEN``.

Until version 0.8.0, restic used a default prefix of ``restic``, so the files
in the bucket were placed in a directory named ``restic``. If you want to
access a repository created with an older version of restic, specify the path
after the bucket name like this:

.. code-block:: console

    $ restic -r s3:s3.amazonaws.com/bucket_name/restic [...]

For an S3-compatible server that is not Amazon (like Minio, see below),
or is only available via HTTP, you can specify the URL to the server
like this: ``s3:http://server:port/bucket_name``.
          
.. note:: restic expects `path-style URLs <https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingBucket.html#access-bucket-intro>`__
          like for example ``s3.us-west-2.amazonaws.com/bucket_name``.
          Virtual-hosted–style URLs like ``bucket_name.s3.us-west-2.amazonaws.com``,
          where the bucket name is part of the hostname are not supported. These must
          be converted to path-style URLs instead, for example ``s3.us-west-2.amazonaws.com/bucket_name``.

.. note:: Certain S3-compatible servers do not properly implement the
          ``ListObjectsV2`` API, most notably Ceph versions before v14.2.5. On these
          backends, as a temporary workaround, you can provide the
          ``-o s3.list-objects-v1=true`` option to use the older
          ``ListObjects`` API instead. This option may be removed in future
          versions of restic.


Minio Server
************

`Minio <https://www.minio.io>`__ is an Open Source Object Storage,
written in Go and compatible with Amazon S3 API.

-  Download and Install `Minio
   Server <https://minio.io/downloads/#minio-server>`__.
-  You can also refer to https://docs.minio.io for step by step guidance
   on installation and getting started on Minio Client and Minio Server.

You must first setup the following environment variables with the
credentials of your Minio Server.

.. code-block:: console

    $ export AWS_ACCESS_KEY_ID=<YOUR-MINIO-ACCESS-KEY-ID>
    $ export AWS_SECRET_ACCESS_KEY= <YOUR-MINIO-SECRET-ACCESS-KEY>

Now you can easily initialize restic to use Minio server as a backend with
this command.

.. code-block:: console

    $ ./restic -r s3:http://localhost:9000/restic init
    enter password for new repository:
    enter password again:
    created restic repository 6ad29560f5 at s3:http://localhost:9000/restic1
    Please note that knowledge of your password is required to access
    the repository. Losing your password means that your data is irrecoverably lost.

Wasabi
************

`Wasabi <https://wasabi.com>`__ is a low cost Amazon S3 conformant object storage provider.
Due to it's S3 conformance, Wasabi can be used as a storage provider for a restic repository.

-  Create a Wasabi bucket using the `Wasabi Console <https://console.wasabisys.com>`__.
-  Determine the correct Wasabi service URL for your bucket `here <https://wasabi-support.zendesk.com/hc/en-us/articles/360015106031-What-are-the-service-URLs-for-Wasabi-s-different-regions->`__.

You must first setup the following environment variables with the
credentials of your Wasabi account.

.. code-block:: console

    $ export AWS_ACCESS_KEY_ID=<YOUR-WASABI-ACCESS-KEY-ID>
    $ export AWS_SECRET_ACCESS_KEY=<YOUR-WASABI-SECRET-ACCESS-KEY>

Now you can easily initialize restic to use Wasabi as a backend with
this command.

.. code-block:: console

    $ ./restic -r s3:https://<WASABI-SERVICE-URL>/<WASABI-BUCKET-NAME> init
    enter password for new repository:
    enter password again:
    created restic repository xxxxxxxxxx at s3:https://<WASABI-SERVICE-URL>/<WASABI-BUCKET-NAME>
    Please note that knowledge of your password is required to access
    the repository. Losing your password means that your data is irrecoverably lost.

Alibaba Cloud (Aliyun) Object Storage System (OSS)
**************************************************

`Alibaba OSS <https://www.alibabacloud.com/product/oss/>`__ is an
encrypted, secure, cost-effective, and easy-to-use object storage
service that enables you to store, back up, and archive large amounts
of data in the cloud.

Alibaba OSS is S3 compatible so it can be used as a storage provider
for a restic repository with a couple of extra parameters.

-  Determine the correct `Alibaba OSS region endpoint <https://www.alibabacloud.com/help/doc-detail/31837.htm>`__ - this will be something like ``oss-eu-west-1.aliyuncs.com``
-  You'll need the region name too - this will be something like ``oss-eu-west-1``

You must first setup the following environment variables with the
credentials of your Alibaba OSS account.

.. code-block:: console

    $ export AWS_ACCESS_KEY_ID=<YOUR-OSS-ACCESS-KEY-ID>
    $ export AWS_SECRET_ACCESS_KEY=<YOUR-OSS-SECRET-ACCESS-KEY>

Now you can easily initialize restic to use Alibaba OSS as a backend with
this command.

.. code-block:: console

    $ ./restic -o s3.bucket-lookup=dns -o s3.region=<OSS-REGION> -r s3:https://<OSS-ENDPOINT>/<OSS-BUCKET-NAME> init
    enter password for new backend:
    enter password again:
    created restic backend xxxxxxxxxx at s3:https://<OSS-ENDPOINT>/<OSS-BUCKET-NAME>
    Please note that knowledge of your password is required to access
    the repository. Losing your password means that your data is irrecoverably lost.

For example with an actual endpoint:

.. code-block:: console

    $ restic -o s3.bucket-lookup=dns -o s3.region=oss-eu-west-1 -r s3:https://oss-eu-west-1.aliyuncs.com/bucketname init

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
   $ export OS_USER_ID=<MY_USER_ID>
   $ export OS_PASSWORD=<MY_PASSWORD>
   $ export OS_USER_DOMAIN_NAME=<MY_DOMAIN_NAME>
   $ export OS_USER_DOMAIN_ID=<MY_DOMAIN_ID>
   $ export OS_PROJECT_NAME=<MY_PROJECT_NAME>
   $ export OS_PROJECT_DOMAIN_NAME=<MY_PROJECT_DOMAIN_NAME>
   $ export OS_PROJECT_DOMAIN_ID=<MY_PROJECT_DOMAIN_ID>
   $ export OS_TRUST_ID=<MY_TRUST_ID>

   # For keystone v3 application credential authentication (application credential id)
   $ export OS_AUTH_URL=<MY_AUTH_URL>
   $ export OS_APPLICATION_CREDENTIAL_ID=<MY_APPLICATION_CREDENTIAL_ID>
   $ export OS_APPLICATION_CREDENTIAL_SECRET=<MY_APPLICATION_CREDENTIAL_SECRET>

   # For keystone v3 application credential authentication (application credential name)
   $ export OS_AUTH_URL=<MY_AUTH_URL>
   $ export OS_USERNAME=<MY_USERNAME>
   $ export OS_USER_DOMAIN_NAME=<MY_DOMAIN_NAME>
   $ export OS_APPLICATION_CREDENTIAL_NAME=<MY_APPLICATION_CREDENTIAL_NAME>
   $ export OS_APPLICATION_CREDENTIAL_SECRET=<MY_APPLICATION_CREDENTIAL_SECRET>

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
   enter password for new repository:
   enter password again:
   created restic repository eefee03bbd at swift:container_name:/path
   Please note that knowledge of your password is required to access the repository.
   Losing your password means that your data is irrecoverably lost.

The policy of the new container created by restic can be changed using environment variable:

.. code-block:: console

   $ export SWIFT_DEFAULT_CONTAINER_POLICY=<MY_CONTAINER_POLICY>


Backblaze B2
************

.. warning::

   Due to issues with error handling in the current B2 library that restic uses,
   the recommended way to utilize Backblaze B2 is by using its S3-compatible API.
   
   Follow the documentation to `generate S3-compatible access keys`_ and then
   setup restic as described at :ref:`Amazon S3`. This is expected to work better
   than using the Backblaze B2 backend directly.

   Different from the B2 backend, restic's S3 backend will only hide no longer
   necessary files. Thus, make sure to setup lifecycle rules to eventually
   delete hidden files.

Restic can backup data to any Backblaze B2 bucket. You need to first setup the
following environment variables with the credentials you can find in the
dashboard on the "Buckets" page when signed into your B2 account:

.. code-block:: console

    $ export B2_ACCOUNT_ID=<MY_APPLICATION_KEY_ID>
    $ export B2_ACCOUNT_KEY=<MY_APPLICATION_KEY>

To get application keys, a user can go to the App Keys section of the Backblaze
account portal.  You must create a master application key first.  From there, you
can generate a standard Application Key.  Please note that the Application Key
should be treated like a password and will only appear once.  If an Application
Key is forgotten, you must generate a new one.

For more information on application keys, refer to the Backblaze `documentation <https://www.backblaze.com/b2/docs/application_keys.html>`__.

.. note:: As of version 0.9.2, restic supports both master and non-master `application keys <https://www.backblaze.com/b2/docs/application_keys.html>`__. If using a non-master application key, ensure that it is created with at least **read and write** access to the B2 bucket. On earlier versions of restic, a master application key is required.

You can then initialize a repository stored at Backblaze B2. If the
bucket does not exist yet and the credentials you passed to restic have the
privilege to create buckets, it will be created automatically:

.. code-block:: console

    $ restic -r b2:bucketname:path/to/repo init
    enter password for new repository:
    enter password again:
    created restic repository eefee03bbd at b2:bucketname:path/to/repo
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

Note that the bucket name must be unique across all of B2.

The number of concurrent connections to the B2 service can be set with the ``-o
b2.connections=10`` switch. By default, at most five parallel connections are
established.

.. _generate S3-compatible access keys: https://help.backblaze.com/hc/en-us/articles/360047425453-Getting-Started-with-the-S3-Compatible-API

Microsoft Azure Blob Storage
****************************

You can also store backups on Microsoft Azure Blob Storage. Export the Azure
account name and key as follows:

.. code-block:: console

    $ export AZURE_ACCOUNT_NAME=<ACCOUNT_NAME>
    $ export AZURE_ACCOUNT_KEY=<SECRET_KEY>

or

.. code-block:: console

    $ export AZURE_ACCOUNT_NAME=<ACCOUNT_NAME>
    $ export AZURE_ACCOUNT_SAS=<SAS_TOKEN>

Afterwards you can initialize a repository in a container called ``foo`` in the
root path like this:

.. code-block:: console

    $ restic -r azure:foo:/ init
    enter password for new repository:
    enter password again:

    created restic repository a934bac191 at azure:foo:/
    [...]

The number of concurrent connections to the Azure Blob Storage service can be set with the
``-o azure.connections=10`` switch. By default, at most five parallel connections are
established.

Google Cloud Storage
********************

.. note:: Google Cloud Storage is not the same service as Google Drive - to use
          the latter, please see :ref:`other-services` for instructions on using
          the rclone backend.

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

Alternatively, you can specify an existing access token directly:

.. code-block:: console

    $ export GOOGLE_ACCESS_TOKEN=ya29.a0AfH6SMC78...

If ``GOOGLE_ACCESS_TOKEN`` is set all other authentication mechanisms are
disabled. The access token must have at least the
``https://www.googleapis.com/auth/devstorage.read_write`` scope. Keep in mind
that access tokens are short-lived (usually one hour), so they are not suitable
if creating a backup takes longer than that, for instance.

Once authenticated, you can use the ``gs:`` backend type to create a new
repository in the bucket ``foo`` at the root path:

.. code-block:: console

    $ restic -r gs:foo:/ init
    enter password for new repository:
    enter password again:

    created restic repository bde47d6254 at gs:foo/
    [...]

The number of concurrent connections to the GCS service can be set with the
``-o gs.connections=10`` switch. By default, at most five parallel connections are
established.

.. _service account: https://cloud.google.com/iam/docs/service-accounts
.. _create a service account key: https://cloud.google.com/iam/docs/creating-managing-service-account-keys#iam-service-account-keys-create-console
.. _default authentication material: https://cloud.google.com/docs/authentication/production

.. _other-services:

Other Services via rclone
*************************

The program `rclone`_ can be used to access many other different services and
store data there. First, you need to install and `configure`_ rclone.  The
general backend specification format is ``rclone:<remote>:<path>``, the
``<remote>:<path>`` component will be directly passed to rclone. When you
configure a remote named ``foo``, you can then call restic as follows to
initiate a new repository in the path ``bar`` in the remote ``foo``:

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

The rclone backend has three additional options:

 * ``-o rclone.program`` specifies the path to rclone, the default value is just ``rclone``
 * ``-o rclone.args`` allows setting the arguments passed to rclone, by default this is ``serve restic --stdio --b2-hard-delete``
 * ``-o rclone.timeout`` specifies timeout for waiting on repository opening, the default value is ``1m``

The reason for the ``--b2-hard-delete`` parameters can be found in the corresponding GitHub `issue #1657`_.

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

    $ restic -o rclone.program="ssh user@remotehost rclone" -r rclone:b2:foo/bar

With these options, restic works with local files. It uses rclone and
credentials stored on ``remotehost`` to communicate with B2. All data (except
credentials) is encrypted/decrypted locally, then sent/received via
``remotehost`` to/from B2.

A more advanced version of this setup forbids specific hosts from removing
files in a repository. See the `blog post by Simon Ruderich
<https://ruderich.org/simon/notes/append-only-backups-with-restic-and-rclone>`_
for details and the documentation for the ``forget`` command to learn about
important security considerations.

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
`here <https://www.msys2.org/wiki/Porting/>`__ and
`here <https://github.com/rprichard/winpty>`__ for detail information).
On MSYS2, you can install ``winpty`` as follows:

.. code-block:: console

    $ pacman -S winpty
    $ winpty restic -r /srv/restic-repo init


Group accessible repositories
*****************************

Since restic version 0.14 local and SFTP repositories can be made
accessible to members of a system group. To control this we have to change
the group permissions of the top-level ``config`` file and restic will use
this as a hint to determine what permissions to apply to newly created
files. By default ``restic init`` sets repositories up to be group
inaccessible.

In order to give group members read-only access we simply add the read
permission bit to all repository files with ``chmod``:

.. code-block:: console

    $ chmod -R g+r /srv/restic-repo

This serves two purposes: 1) it sets the read permission bit on the
repository config file triggering restic's logic to create new files as
group accessible and 2) it actually allows the group read access to the
files.

.. note:: By default files on Unix systems are created with a user's
          primary group as defined by the gid (group id) field in
          ``/etc/passwd``. See `passwd(5)
          <https://manpages.debian.org/latest/passwd/passwd.5.en.html>`_.

For read-write access things are a bit more complicated. When users other
than the repository creator add new files in the repository they will be
group-owned by this user's primary group by default, not that of the
original repository owner, meaning the original creator wouldn't have
access to these files. That's hardly what you'd want.

To make this work we can employ the help of the ``setgid`` permission bit
available on Linux and most other Unix systems. This permission bit makes
newly created directories inherit both the group owner (gid) and setgid bit
from the parent directory. Setting this bit requires root but since it
propagates down to any new directories we only have to do this priviledged
setup once:

.. code-block:: console

    # find /srv/restic-repo -type d -exec chmod g+s '{}' \;
    $ chmod -R g+rw /srv/restic-repo

This sets the ``setgid`` bit on all existing directories in the repository
and then grants read/write permissions for group access.

.. note:: To manage who has access to the repository you can use
          ``usermod`` on Linux systems, to change which group controls
          repository access ``chgrp -R`` is your friend.
