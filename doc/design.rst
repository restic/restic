
Terminology
===========

This section introduces terminology used in this document.

*Repository*: All data produced during a backup is sent to and stored in
a repository in a structured form, for example in a file system
hierarchy with several subdirectories. A repository implementation must
be able to fulfill a number of operations, e.g. list the contents.

*Blob*: A Blob combines a number of data bytes with identifying
information like the SHA-256 hash of the data and its length.

*Pack*: A Pack combines one or more Blobs, e.g. in a single file.

*Snapshot*: A Snapshot stands for the state of a file or directory that
has been backed up at some point in time. The state here means the
content and meta data like the name and modification time for the file
or the directory and its contents.

*Storage ID*: A storage ID is the SHA-256 hash of the content stored in
the repository. This ID is required in order to load the file from the
repository.

Repository Format
=================

All data is stored in a restic repository. A repository is able to store
data of several different types, which can later be requested based on
an ID. This so-called "storage ID" is the SHA-256 hash of the content of
a file. All files in a repository are only written once and never
modified afterwards. Writing should occur atomically to prevent concurrent
operations from reading incomplete files. This allows accessing and even
writing to the repository with multiple clients in parallel. Only the ``prune``
operation removes data from the repository.

Repositories consist of several directories and a top-level file called
``config``. For all other files stored in the repository, the name for
the file is the lower case hexadecimal representation of the storage ID,
which is the SHA-256 hash of the file's contents. This allows for easy
verification of files for accidental modifications, like disk read
errors, by simply running the program ``sha256sum`` on the file and
comparing its output to the file name. If the prefix of a filename is
unique amongst all the other files in the same directory, the prefix may
be used instead of the complete filename.

Apart from the files stored within the ``keys`` directory, all files are
encrypted with AES-256 in counter mode (CTR). The integrity of the
encrypted data is secured by a Poly1305-AES message authentication code
(sometimes also referred to as a "signature").

In the first 16 bytes of each encrypted file the initialisation vector
(IV) is stored. It is followed by the encrypted data and completed by
the 16 byte MAC. The format is: ``IV || CIPHERTEXT || MAC``. The
complete encryption overhead is 32 bytes. For each file, a new random IV
is selected.

The file ``config`` is encrypted this way and contains a JSON document
like the following:

.. code:: json

    {
      "version": 2,
      "id": "5956a3f67a6230d4a92cefb29529f10196c7d92582ec305fd71ff6d331d6271b",
      "chunker_polynomial": "25b468838dcb75"
    }

After decryption, restic first checks that the version field contains a
version number that it understands, otherwise it aborts. At the moment, the
version is expected to be 1 or 2. The list of changes in the repository
format is contained in the section "Changes" below.

The field ``id`` holds a unique ID which consists of 32 random bytes, encoded
in hexadecimal. This uniquely identifies the repository, regardless if it is
accessed via a remote storage backend or locally. The field
``chunker_polynomial`` contains a parameter that is used for splitting large
files into smaller chunks (see below).

Repository Layout
-----------------

The ``local`` and ``sftp`` backends are implemented using files and
directories stored in a file system. The directory layout is the same
for both backend types and is also used for all other remote backends.

The basic layout of a repository is shown here:

::

    /tmp/restic-repo
    ├── config
    ├── data
    │   ├── 21
    │   │   └── 2159dd48f8a24f33c307b750592773f8b71ff8d11452132a7b2e2a6a01611be1
    │   ├── 32
    │   │   └── 32ea976bc30771cebad8285cd99120ac8786f9ffd42141d452458089985043a5
    │   ├── 59
    │   │   └── 59fe4bcde59bd6222eba87795e35a90d82cd2f138a27b6835032b7b58173a426
    │   ├── 73
    │   │   └── 73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c
    │   [...]
    ├── index
    │   ├── c38f5fb68307c6a3e3aa945d556e325dc38f5fb68307c6a3e3aa945d556e325d
    │   └── ca171b1b7394d90d330b265d90f506f9984043b342525f019788f97e745c71fd
    ├── keys
    │   └── b02de829beeb3c01a63e6b25cbd421a98fef144f03b9a02e46eff9e2ca3f0bd7
    ├── locks
    ├── snapshots
    │   └── 22a5af1bdc6e616f8a29579458c49627e01b32210d09adb288d1ecda7c5711ec
    └── tmp

A local repository can be initialized with the ``restic init`` command, e.g.:

.. code-block:: console

    $ restic -r /tmp/restic-repo init

The local and sftp backends will auto-detect and accept all layouts described
in the following sections, so that remote repositories mounted locally e.g. via
fuse can be accessed. The layout auto-detection can be overridden by specifying
the option ``-o local.layout=default``, valid values are ``default`` and
``s3legacy``. The option for the sftp backend is named ``sftp.layout``, for the
s3 backend ``s3.layout``.

S3 Legacy Layout
----------------

Unfortunately during development the Amazon S3 backend uses slightly different
paths (directory names use singular instead of plural for ``key``,
``lock``, and ``snapshot`` files), and the pack files are stored directly below
the ``data`` directory. The S3 Legacy repository layout looks like this:

::

    /config
    /data
     ├── 2159dd48f8a24f33c307b750592773f8b71ff8d11452132a7b2e2a6a01611be1
     ├── 32ea976bc30771cebad8285cd99120ac8786f9ffd42141d452458089985043a5
     ├── 59fe4bcde59bd6222eba87795e35a90d82cd2f138a27b6835032b7b58173a426
     ├── 73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c
    [...]
    /index
     ├── c38f5fb68307c6a3e3aa945d556e325dc38f5fb68307c6a3e3aa945d556e325d
     └── ca171b1b7394d90d330b265d90f506f9984043b342525f019788f97e745c71fd
    /key
     └── b02de829beeb3c01a63e6b25cbd421a98fef144f03b9a02e46eff9e2ca3f0bd7
    /lock
    /snapshot
     └── 22a5af1bdc6e616f8a29579458c49627e01b32210d09adb288d1ecda7c5711ec

The S3 backend understands and accepts both forms, new backends are
always created with the default layout for compatibility reasons.

Pack Format
===========

All files in the repository except Key and Pack files just contain raw
data, stored as ``IV || Ciphertext || MAC``. Pack files may contain one
or more Blobs of data.

A Pack's structure is as follows:

::

    EncryptedBlob1 || ... || EncryptedBlobN || EncryptedHeader || Header_Length

At the end of the Pack file is a header, which describes the content.
The header is encrypted and authenticated. ``Header_Length`` is the
length of the encrypted header encoded as a four byte integer in
little-endian encoding. Placing the header at the end of a file allows
writing the blobs in a continuous stream as soon as they are read during
the backup phase. This reduces code complexity and avoids having to
re-write a file once the pack is complete and the content and length of
the header is known.

All the blobs (``EncryptedBlob1``, ``EncryptedBlobN`` etc.) are
authenticated and encrypted independently. This enables repository
reorganisation without having to touch the encrypted Blobs. In addition
it also allows efficient indexing, for only the header needs to be read
in order to find out which Blobs are contained in the Pack. Since the
header is authenticated, authenticity of the header can be checked
without having to read the complete Pack.

After decryption, a Pack's header consists of the following elements:

::

    Type_Blob1 || Data_Blob1 ||
    [...]
    Type_BlobN || Data_BlobN ||

The Blob type field is a single byte. What follows it depends on the type. The
following Blob types are defined:

+-----------+----------------------+-------------------------------------------------------------------------------+
| Type      | Meaning              |  Data                                                                         |
+===========+======================+===============================================================================+
| 0b00      | data blob            |  ``Length(encrypted_blob) || Hash(plaintext_blob)``                           |
+-----------+----------------------+-------------------------------------------------------------------------------+
| 0b01      | tree blob            |  ``Length(encrypted_blob) || Hash(plaintext_blob)``                           |
+-----------+----------------------+-------------------------------------------------------------------------------+
| 0b10      | compressed data blob |  ``Length(encrypted_blob) || Length(plaintext_blob) || Hash(plaintext_blob)`` |
+-----------+----------------------+-------------------------------------------------------------------------------+
| 0b11      | compressed tree blob |  ``Length(encrypted_blob) || Length(plaintext_blob) || Hash(plaintext_blob)`` |
+-----------+----------------------+-------------------------------------------------------------------------------+

This is enough to calculate the offsets for all the Blobs in the Pack.
The length fields are encoded as four byte integers in little-endian
format. In the Data column, ``Length(plaintext_blob)`` means the length
of the decrypted and uncompressed data a blob consists of.

All other types are invalid, more types may be added in the future. The
compressed types are only valid for repository format version 2. Data and
tree blobs may be compressed with the zstandard compression algorithm.

In repository format version 1, data and tree blobs should be stored in
separate pack files. In version 2, they must be stored in separate files.
Compressed and non-compress blobs of the same type may be mixed in a pack
file.

For reconstructing the index or parsing a pack without an index, first
the last four bytes must be read in order to find the length of the
header. Afterwards, the header can be read and parsed, which yields all
plaintext hashes, types, offsets and lengths of all included blobs.

Unpacked Data Format
====================

Individual files for the index, locks or snapshots are encrypted
and authenticated like Data and Tree Blobs, so the outer structure is
``IV || Ciphertext || MAC`` again. In repository format version 1 the
plaintext always consists of a JSON document which must either be an
object or an array.

Repository format version 2 adds support for compression. The plaintext
now starts with a header to indicate the encoding version to distinguish
it from plain JSON and to allow for further evolution of the storage format:
``encoding_version || data``
The ``encoding_version`` field is encoded as one byte.
For backwards compatibility the encoding versions '[' (0x5b) and '{' (0x7b)
are used to mark that the whole plaintext (including the encoding version
byte) should treated as JSON document.

For new data the encoding version is currently always ``2``. For that
version ``data`` contains a JSON document compressed using the zstandard
compression algorithm.

Indexing
========

Index files contain information about Data and Tree Blobs and the Packs
they are contained in and store this information in the repository. When
the local cached index is not accessible any more, the index files can
be downloaded and used to reconstruct the index. The file encoding is
described in the "Unpacked Data Format" section. The plaintext consists
of a JSON document like the following:

.. code:: json

    {
      "supersedes": [
        "ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452"
      ],
      "packs": [
        {
          "id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
          "blobs": [
            {
              "id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
              "type": "data",
              "offset": 0,
              "length": 38,
              // no 'uncompressed_length' as blob is not compressed
            },
            {
              "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
              "type": "tree",
              "offset": 38,
              "length": 112,
              "uncompressed_length": 511,
            },
            {
              "id": "d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66",
              "type": "data",
              "offset": 150,
              "length": 123,
              "uncompressed_length": 234,
            }
          ]
        }, [...]
      ]
    }

This JSON document lists Packs and the blobs contained therein. In this
example, the Pack ``73d04e61`` contains two data Blobs and one Tree
blob, the plaintext hashes are listed afterwards. The ``length`` field
corresponds to ``Length(encrypted_blob)`` in the pack file header.
Field ``uncompressed_length`` is only present for compressed blobs and
therefore is never present in version 1. It is set to the value of
``Length(blob)``.

The field ``supersedes`` lists the storage IDs of index files that have
been replaced with the current index file. This happens when index files
are repacked, for example when old snapshots are removed and Packs are
recombined.

There may be an arbitrary number of index files, containing information
on non-disjoint sets of Packs. The number of packs described in a single
file is chosen so that the file size is kept below 8 MiB.

Keys, Encryption and MAC
========================

All data stored by restic in the repository is encrypted with AES-256 in
counter mode and authenticated using Poly1305-AES. For encrypting new
data first 16 bytes are read from a cryptographically secure
pseudo-random number generator as a random nonce. This is used both as
the IV for counter mode and the nonce for Poly1305. This operation needs
three keys: A 32 byte for AES-256 for encryption, a 16 byte AES key and
a 16 byte key for Poly1305. For details see the original paper `The
Poly1305-AES message-authentication
code <https://cr.yp.to/mac/poly1305-20050329.pdf>`__ by Dan Bernstein.
The data is then encrypted with AES-256 and afterwards a message
authentication code (MAC) is computed over the ciphertext, everything is
then stored as IV \|\| CIPHERTEXT \|\| MAC.

The directory ``keys`` contains key files. These are simple JSON
documents which contain all data that is needed to derive the
repository's master encryption and message authentication keys from a
user's password. The JSON document from the repository can be
pretty-printed for example by using the Python module ``json``
(shortened to increase readability):

::

    $ python -mjson.tool /tmp/restic-repo/keys/b02de82*
    {
        "hostname": "kasimir",
        "username": "fd0"
        "kdf": "scrypt",
        "N": 65536,
        "r": 8,
        "p": 1,
        "created": "2015-01-02T18:10:13.48307196+01:00",
        "data": "tGwYeKoM0C4j4/9DFrVEmMGAldvEn/+iKC3te/QE/6ox/V4qz58FUOgMa0Bb1cIJ6asrypCx/Ti/pRXCPHLDkIJbNYd2ybC+fLhFIJVLCvkMS+trdywsUkglUbTbi+7+Ldsul5jpAj9vTZ25ajDc+4FKtWEcCWL5ICAOoTAxnPgT+Lh8ByGQBH6KbdWabqamLzTRWxePFoYuxa7yXgmj9A==",
        "salt": "uW4fEI1+IOzj7ED9mVor+yTSJFd68DGlGOeLgJELYsTU5ikhG/83/+jGd4KKAaQdSrsfzrdOhAMftTSih5Ux6w==",
    }

When the repository is opened by restic, the user is prompted for the
repository password. This is then used with ``scrypt``, a key derivation
function (KDF), and the supplied parameters (``N``, ``r``, ``p`` and
``salt``) to derive 64 key bytes. The first 32 bytes are used as the
encryption key (for AES-256) and the last 32 bytes are used as the
message authentication key (for Poly1305-AES). These last 32 bytes are
divided into a 16 byte AES key ``k`` followed by 16 bytes of secret key
``r``. The key ``r`` is then masked for use with Poly1305 (see the paper
for details).

Those keys are used to authenticate and decrypt the bytes contained in
the JSON field ``data`` with AES-256 and Poly1305-AES as if they were
any other blob (after removing the Base64 encoding). If the
password is incorrect or the key file has been tampered with, the
computed MAC will not match the last 16 bytes of the data, and restic
exits with an error. Otherwise, the data yields a JSON document
which contains the master encryption and message authentication keys for
this repository (encoded in Base64). The command
``restic cat masterkey`` can be used as follows to decrypt and
pretty-print the master key:

.. code-block:: console

    $ restic -r /tmp/restic-repo cat masterkey
    {
        "mac": {
          "k": "evFWd9wWlndL9jc501268g==",
          "r": "E9eEDnSJZgqwTOkDtOp+Dw=="
        },
        "encrypt": "UQCqa0lKZ94PygPxMRqkePTZnHRYh1k1pX2k2lM2v3Q=",
    }

All data in the repository is encrypted and authenticated with these
master keys. For encryption, the AES-256 algorithm in Counter mode is
used. For message authentication, Poly1305-AES is used as described
above.

A repository can have several different passwords, with a key file for
each. This way, the password can be changed without having to re-encrypt
all data.

Snapshots
=========

A snapshot represents a directory with all files and sub-directories at
a given point in time. For each backup that is made, a new snapshot is
created. A snapshot is a JSON document that is stored in a file below
the directory ``snapshots`` in the repository. It uses the file encoding
described in the "Unpacked Data Format" section. The filename
is the storage ID. This string is unique and used within restic to
uniquely identify a snapshot.

The command ``restic cat snapshot`` can be used as follows to decrypt
and pretty-print the contents of a snapshot file:

.. code-block:: console

    $ restic -r /tmp/restic-repo cat snapshot 251c2e58
    enter password for repository:
    {
      "time": "2015-01-02T18:10:50.895208559+01:00",
      "tree": "2da81727b6585232894cfbb8f8bdab8d1eccd3d8f7c92bc934d62e62e618ffdf",
      "dir": "/tmp/testdata",
      "hostname": "kasimir",
      "username": "fd0",
      "uid": 1000,
      "gid": 100,
      "tags": [
        "NL"
      ]
    }

Here it can be seen that this snapshot represents the contents of the
directory ``/tmp/testdata``. The most important field is ``tree``. When
the meta data (e.g. the tags) of a snapshot change, the snapshot needs
to be re-encrypted and saved. This will change the storage ID, so in
order to relate these seemingly different snapshots, a field
``original`` is introduced which contains the ID of the original
snapshot, e.g. after adding the tag ``DE`` to the snapshot above it
becomes:

.. code-block:: console

    $ restic -r /tmp/restic-repo cat snapshot 22a5af1b
    enter password for repository:
    {
      "time": "2015-01-02T18:10:50.895208559+01:00",
      "tree": "2da81727b6585232894cfbb8f8bdab8d1eccd3d8f7c92bc934d62e62e618ffdf",
      "dir": "/tmp/testdata",
      "hostname": "kasimir",
      "username": "fd0",
      "uid": 1000,
      "gid": 100,
      "tags": [
        "NL",
        "DE"
      ],
      "original": "251c2e5841355f743f9d4ffd3260bee765acee40a6229857e32b60446991b837"
    }

Once introduced, the ``original`` field is not modified when the
snapshot's meta data is changed again.

All content within a restic repository is referenced according to its
SHA-256 hash. Before saving, each file is split into variable sized
Blobs of data. The SHA-256 hashes of all Blobs are saved in an ordered
list which then represents the content of the file.

In order to relate these plaintext hashes to the actual location within
a Pack file, an index is used. If the index is not available, the
header of all data Blobs can be read.

Trees and Data
==============

A snapshot references a tree by the SHA-256 hash of the JSON string
representation of its contents. Trees and data are saved in pack files
in a subdirectory of the directory ``data``.

The command ``restic cat blob`` can be used to inspect the tree
referenced above (piping the output of the command to ``jq .`` so that
the JSON is indented):

.. code-block:: console

    $ restic -r /tmp/restic-repo cat blob 2da81727b6585232894cfbb8f8bdab8d1eccd3d8f7c92bc934d62e62e618ffdf | jq .
    enter password for repository:
    {
      "nodes": [
        {
          "name": "testdata",
          "type": "dir",
          "mode": 493,
          "mtime": "2014-12-22T14:47:59.912418701+01:00",
          "atime": "2014-12-06T17:49:21.748468803+01:00",
          "ctime": "2014-12-22T14:47:59.912418701+01:00",
          "uid": 1000,
          "gid": 100,
          "user": "fd0",
          "inode": 409704562,
          "content": null,
          "subtree": "b26e315b0988ddcd1cee64c351d13a100fedbc9fdbb144a67d1b765ab280b4dc"
        }
      ]
    }

A tree contains a list of entries (in the field ``nodes``) which contain
meta data like a name and timestamps. When the entry references a
directory, the field ``subtree`` contains the plain text ID of another
tree object.

When the command ``restic cat blob`` is used, the plaintext ID is needed
to print a tree. The tree referenced above can be dumped as follows:

.. code-block:: console

    $ restic -r /tmp/restic-repo cat blob b26e315b0988ddcd1cee64c351d13a100fedbc9fdbb144a67d1b765ab280b4dc
    enter password for repository:
    {
      "nodes": [
        {
          "name": "testfile",
          "type": "file",
          "mode": 420,
          "mtime": "2014-12-06T17:50:23.34513538+01:00",
          "atime": "2014-12-06T17:50:23.338468713+01:00",
          "ctime": "2014-12-06T17:50:23.34513538+01:00",
          "uid": 1000,
          "gid": 100,
          "user": "fd0",
          "inode": 416863351,
          "size": 1234,
          "links": 1,
          "content": [
            "50f77b3b4291e8411a027b9f9b9e64658181cc676ce6ba9958b95f268cb1109d"
          ]
        },
        [...]
      ]
    }

This tree contains a file entry. This time, the ``subtree`` field is not
present and the ``content`` field contains a list with one plain text
SHA-256 hash.

The command ``restic cat blob`` can also be used to extract and decrypt
data given a plaintext ID, e.g. for the data mentioned above:

.. code-block:: console

    $ restic -r /tmp/restic-repo cat blob 50f77b3b4291e8411a027b9f9b9e64658181cc676ce6ba9958b95f268cb1109d | sha256sum
    enter password for repository:
    50f77b3b4291e8411a027b9f9b9e64658181cc676ce6ba9958b95f268cb1109d  -

As can be seen from the output of the program ``sha256sum``, the hash
matches the plaintext hash from the map included in the tree above, so
the correct data has been returned.

Locks
=====

The restic repository structure is designed in a way that allows
parallel access of multiple instance of restic and even parallel writes.
However, there are some functions that work more efficient or even
require exclusive access of the repository. In order to implement these
functions, restic processes are required to create a lock on the
repository before doing anything.

Locks come in two types: Exclusive and non-exclusive locks. At most one
process can have an exclusive lock on the repository, and during that
time there must not be any other locks (exclusive and non-exclusive).
There may be multiple non-exclusive locks in parallel.

A lock is a file in the subdir ``locks`` whose filename is the storage
ID of the contents. It is stored in the file encoding described in the
"Unpacked Data Format" section and contains the following JSON structure:

.. code:: json

    {
      "time": "2015-06-27T12:18:51.759239612+02:00",
      "exclusive": false,
      "hostname": "kasimir",
      "username": "fd0",
      "pid": 13607,
      "uid": 1000,
      "gid": 100
    }

The field ``exclusive`` defines the type of lock. When a new lock is to
be created, restic checks all locks in the repository. When a lock is
found, it is tested if the lock is stale, which is the case for locks
with timestamps older than 30 minutes. If the lock was created on the
same machine, even for younger locks it is tested whether the process is
still alive by sending a signal to it. If that fails, restic assumes
that the process is dead and considers the lock to be stale.

When a new lock is to be created and no other conflicting locks are
detected, restic creates a new lock, waits, and checks if other locks
appeared in the repository. Depending on the type of the other locks and
the lock to be created, restic either continues or fails.

Read and Write Ordering
=======================
The repository format allows writing (e.g. backup) and reading (e.g. restore)
to happen concurrently. As the data for each snapshot in a repository spans
multiple files (snapshot, index and packs), it is necessary to follow certain
rules regarding the order in which files are read and written. These ordering
rules also guarantee that repository modifications always maintain a correct
repository even if the client or the storage backend crashes for example due
to a power cut or the (network) connection between both is interrupted.

The correct order to access data in a repository is derived from the following
set of invariants that must be maintained at **any time** in a correct
repository. *Must* in the following is a strict requirement and will lead to
data loss if not followed. *Should* will require steps to fix a repository
(e.g. rebuilding the index) if not followed, but should not cause data loss.
*existing* means that the referenced data is **durably** stored in the repository.

- A snapshot *must* only reference an existing tree blob.
- A reachable tree blob *must* only reference tree and data blobs that exist
  (recursively). *Reachable* means that the tree blob is reachable starting from
  a snapshot.
- An index *must* only reference valid blobs in existing packs.
- All blobs referenced by a snapshot *should* be listed in an index.

This leads to the following recommended order to store data in a repository.
First, pack files, which contain data and tree blobs, must be written. Then the
indexes which reference blobs in these already written pack files. And finally
the corresponding snapshots.

Note that there is no need for a specific write order of data and tree blobs
during a backup as the blobs only become referenced once the corresponding
snapshot is uploaded.

Reading data should follow the opposite order compared to writing. Only once a
snapshot was written, it is guaranteed that all required data exists in the
repository. This especially means that the list of snapshots to read should be
collected before loading the repository index. The other way round can lead to
a race condition where a recently written snapshot is loaded but not its
accompanying index, which results in a failure to access the snapshot's tree
blob.

For removing or rewriting data from a repository the following rules must be
followed, which are derived from the above invariants.

- A client removing data *must* acquire an exclusive lock first to prevent
  conflicts with other clients.
- A pack *must* be removed from the referencing index before it is deleted.
- Rewriting a pack *must* write the new pack, update the index (add an updated
  index and delete the old one) and only then delete the old pack.


Backups and Deduplication
=========================

For creating a backup, restic scans the source directory for all files,
sub-directories and other entries. The data from each file is split into
variable length Blobs cut at offsets defined by a sliding window of 64
bytes. The implementation uses Rabin Fingerprints for implementing this
Content Defined Chunking (CDC). An irreducible polynomial is selected at
random and saved in the file ``config`` when a repository is
initialized, so that watermark attacks are much harder.

Files smaller than 512 KiB are not split, Blobs are of 512 KiB to 8 MiB
in size. The implementation aims for 1 MiB Blob size on average.

For modified files, only modified Blobs have to be saved in a subsequent
backup. This even works if bytes are inserted or removed at arbitrary
positions within the file.

Threat Model
============

The design goals for restic include being able to securely store backups
in a location that is not completely trusted (e.g., a shared system where
others can potentially access the files) or even modify or delete them in
the case of the system administrator.

General assumptions:

-  The host system a backup is created on is trusted. This is the most
   basic requirement, and it is essential for creating trustworthy backups.
-  The user uses an authentic copy of restic.
-  The user does not share the repository password with an attacker.
-  The restic backup program is not designed to protect against attackers
   deleting files at the storage location. There is nothing that can be
   done about this. If this needs to be guaranteed, get a secure location
   without any access from third parties.
-  The whole repository is re-encrypted if a key is leaked. With the current
   key management design, it is impossible to securely revoke a leaked key
   without re-encrypting the whole repository.
-  Advances in cryptography attacks against the cryptographic primitives used
   by restic (i.e., AES-256-CTR-Poly1305-AES and SHA-256) have not occurred. Such
   advances could render the confidentiality or integrity protections provided
   by restic useless.
-  Sufficient advances in computing have not occurred to make brute-force
   attacks against restic's cryptographic protections feasible.

The restic backup program guarantees the following:

-  Unencrypted content of stored files and metadata cannot be accessed
   without a password for the repository. Everything except the metadata
   included for informational purposes in the key files is encrypted and
   authenticated. The cache is also encrypted to prevent metadata 
   leaks.
-  Modifications to data stored in the repository (due to bad RAM, broken
   harddisk, etc.) can be detected.
-  Data that has been tampered will not be decrypted.

With the aforementioned assumptions and guarantees in mind, the following are
examples of things an adversary could achieve in various circumstances.

An adversary with read access to your backup storage location could:

-  Attempt a brute force password guessing attack against a copy of the
   repository (please use strong passwords with sufficient entropy).
-  Infer which packs probably contain trees via file access patterns.
-  Infer the size of backups by using creation timestamps of repository objects.

An adversary with network access could:

-  Attempt to DoS the server storing the backup repository or the network 
   connection between client and server.
-  Determine from where you create your backups (i.e., the location where the
   requests originate).
-  Determine where you store your backups (i.e., which provider/target system).
-  Infer the size of backups by observing network traffic.

The following are examples of the implications associated with violating some
of the aforementioned assumptions.

An adversary who compromises (via malware, physical access, etc.) the host
system making backups could:

-  Render the entire backup process untrustworthy (e.g., intercept password, 
   copy files, manipulate data).
-  Create snapshots (containing garbage data) which cover all modified files
   and wait until a trusted host has used ``forget`` often enough to remove all
   correct snapshots.
-  Create a garbage snapshot for every existing snapshot with a slightly
   different timestamp and wait until certain ``forget`` configurations have been
   run, thereby removing all correct snapshots at once.

An adversary with write access to your files at the storage location could:

-  Delete or manipulate your backups, thereby impairing your ability to restore
   files from the compromised storage location.
-  Determine which files belong to what snapshot (e.g., based on the timestamps
   of the stored files). When only these files are deleted, the particular
   snapshot vanishes and all snapshots depending on data that has been added in
   the snapshot cannot be restored completely. Restic is not designed to detect
   this attack.

An adversary who compromises a host system with append-only (read+write allowed,
delete+overwrite denied) access to the backup repository could:

-  Capture the password and decrypt backups from the past and in the future
   (see the "leaked key" example below for related information).
-  Render new backups untrustworthy *after* the host has been compromised
   (due to having complete control over new backups). An attacker cannot delete
   or manipulate old backups. As such, restoring old snapshots created *before*
   a host compromise remains possible.
-  Potentially manipulate the use of the ``forget`` command into deleting all
   legitimate snapshots, keeping only bogus snapshots added by the attacker.
   Ransomware might try this in order to leave only one option to get your data
   back: paying the ransom. For safe use of ``forget``, please see the
   corresponding documentation on removing backup snapshots and append-only mode.

An adversary who has a leaked (decrypted) key for a repository could:

-  Decrypt existing and future backup data. If multiple hosts backup into the
   same repository, an attacker will get access to the backup data of every host.
   Note that since the local encryption key gives access to the master key, a
   password change will not prevent this. Changing the master key can currently
   only be done using the ``copy`` command, which moves the data into a new
   repository with a new master key, or by making a completely new repository
   and new backup.

Changes
=======

Repository Version 2
--------------------

 * Support compression for blobs (data/tree) and index / lock / snapshot files
