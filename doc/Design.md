This document gives a high-level overview of the design and repository layout of the restic backup program.

Repository Format
=================

All data is stored in a restic repository. A repository is able to store chunks
of data called blobs of several different types, which can later be requested
based on an ID. The ID is the hash (SHA-256) of the content of a blob. All
blobs in a repository are only written once and never modified afterwards. This
allows accessing and even writing to the repository with multiple clients in
parallel. Only the delete operation changes data in the repository.

At the time of writing, the only implemented repository type is based on
directories and files. Such repositories can be accessed locally on the same
system or via the integrated SFTP client. The directory layout is the same for
both access methods. This repository type is described in the following.

Repositories consists of several directories and a file called `version`. This
file contains the version number of the repository. At the moment, this file
is expected to hold the string `1`, with an optional newline character.

For all other blobs stored in the repository, the name for the file is the
lower case hexadecimal representation of the SHA-256 hash of the file's
contents. This allows easily checking all files for accidental modifications
like disk read errors by simply running the program `sha256sum` and comparing
its output to the file name. If the prefix of a filename is unique amongst all
the other files in the same directory, the prefix may be used instead of the
complete filename.

Apart from the `version` file and the files stored below the `keys` directory,
all files are encrypted with AES-256 in counter mode (CTR). The integrity of
the encrypted data is secured by an Poly1305-AES signature.

In the first 16 bytes of each encrypted file the initialisation vector (IV) is
stored. It is followed by the encrypted data and completed by the 16 byte MAC
signature. The format is: `IV || CIPHERTEXT || MAC`. The complete encryption
overhead is 48 byte. For each file, a new random IV is selected.

The basic layout of a sample restic repository is shown below:

    /tmp/restic-repo
    ├── data
    │   ├── 59
    │   │   └── 59fe4bcde59bd6222eba87795e35a90d82cd2f138a27b6835032b7b58173a426
    │   ├── 73
    │   │   └── 73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c
    │   [...]
    ├── keys
    │   └── b02de829beeb3c01a63e6b25cbd421a98fef144f03b9a02e46eff9e2ca3f0bd7
    ├── locks
    ├── snapshots
    │   └── 22a5af1bdc6e616f8a29579458c49627e01b32210d09adb288d1ecda7c5711ec
    ├── tmp
    ├── trees
    │   ├── 21
    │   │   └── 2159dd48f8a24f33c307b750592773f8b71ff8d11452132a7b2e2a6a01611be1
    │   ├── 32
    │   │   └── 32ea976bc30771cebad8285cd99120ac8786f9ffd42141d452458089985043a5
    │   ├── 95
    │   │   └── 95f75feb05a7cc73e328b2efa668b1ea68f65fece55a93bc65aff6cd0bcfeefc
    │   ├── b8
    │   │   └── b8138ab08a4722596ac89c917827358da4672eac68e3c03a8115b88dbf4bfb59
    │   ├── e0
    │   │   └── e01150928f7ad24befd6ec15b087de1b9e0f92edabd8e5cabb3317f8b20ad044
    │   [...]
    └── version

A repository can be initialized with the `restic init` command, e.g.:

    $ restic -r /tmp/restic-repo init

Keys, Encryption and MAC
------------------------

All data stored by restic in the repository is encrypted with AES-256 in
counter mode and signed with Poly1305-AES. For encrypting new data first 16
bytes are read from a cryptographically secure pseudorandom number generator as
a random nonce. This is used both as the IV for counter mode and the nonce for
Poly1305. This operation needs three keys: A 32 byte for AES-256 for
encryption, a 16 byte AES key and a 16 byte key for Poly1305. For details see
the original paper[The Poly1305-AES message-authentication
code](http://cr.yp.to/mac/poly1305-20050329.pdf) by Dan Bernstein. The
ciphertext is stored as IV || CIPHERTEXT || MAC.

The directory `keys` contains key files. These are simple JSON documents which
contain all data that is needed to derive the repository's master signing and
encryption keys from a user's password. The JSON document from the repository
can be pretty-printed for example by using the Python module `json` (shortened
to increase readability):

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
repository password. This is then used with `scrypt`, a key derivation function
(KDF), and the supplied parameters (`N`, `r`, `p` and `salt`) to derive 64 key
bytes. The first 32 bytes are used as the encryption key (for AES-256) and the
last 32 bytes are used as the signing key (for Poly1305-AES). These last 32
bytes are divided into a 16 byte AES key `k` followed by 16 bytes of secret key
`r`. They key `r` is then masked for use with Poly1305 (see the paper for
details).

This signing key is used to compute a MAC over the bytes contained in the
JSON field `data` (after removing the Base64 encoding and not including the
last 32 byte). If the password is incorrect or the key file has been tampered
with, the computed MAC will not match the last 16 bytes of the data, and
restic exits with an error. Otherwise, the data is decrypted with the
encryption key derived from `scrypt`. This yields a JSON document which
contains the master signing and encryption keys for this repository, encoded in
Base64. The command `restic cat masterkey` can be used as follows to decrypt
and pretty-print the master key:

    $ restic -r /tmp/restic-repo cat masterkey
    {
        "sign": {
          "k": "evFWd9wWlndL9jc501268g==",
          "r": "E9eEDnSJZgqwTOkDtOp+Dw=="
        },
        "encrypt": "UQCqa0lKZ94PygPxMRqkePTZnHRYh1k1pX2k2lM2v3Q="
    }

All data in the repository is encrypted and signed with these master keys with
AES-256 in Counter mode and signed with Poly1305-AES as described above.

A repository can have several different passwords, with a key file for each.
This way, the password can be changed without having to re-encrypt all data.

Snapshots
---------

A snapshots represents a directory with all files and sub-directories at a
given point in time. For each backup that is made, a new snapshot is created. A
snapshot is a JSON document that is stored in an encrypted file below the
directory `snapshots` in the repository. The filename is the SHA-256 hash of
the (encrypted) contents. This string is unique and used within restic to
uniquely identify a snapshot.

The command `restic cat snapshot` can be used as follows to decrypt and
pretty-print the contents of a snapshot file:

    $ restic -r /tmp/restic-repo cat snapshot 22a5af1b
    Enter Password for Repository:
    {
      "time": "2015-01-02T18:10:50.895208559+01:00",
      "tree": "",
      "tree": {
        "id": "2da81727b6585232894cfbb8f8bdab8d1eccd3d8f7c92bc934d62e62e618ffdf",
        "size": 282,
        "sid": "b8138ab08a4722596ac89c917827358da4672eac68e3c03a8115b88dbf4bfb59",
        "ssize": 330
      },
      "dir": "/tmp/testdata",
      "hostname": "kasimir",
      "username": "fd0",
      "uid": 1000,
      "gid": 100
    }

Here it can be seen that this snapshot represents the contents of the directory
`/tmp/testdata`. The most important field is `tree`.

All content within a restic repository is referenced according to its SHA-256
hash. Before saving, each file is split into variable sized chunks of data. The
SHA-256 hashes of all chunks are saved in an ordered list which then represents
the content of the file.

In order to relate these plain text hashes to the actual encrypted storage
hashes (which vary due to random IVs), each object contains a list that maps
all referenced plaintext hashes to storage hashes. In the case of the snapshot
data structure listed above, the list only consists of one entry for the
referenced tree, so the field `tree` consists of such a mapping.

Trees and Data
--------------

A snapshot references a tree by the SHA-256 hash of the JSON string
representation of its contents. Trees are saved in a subdirectory of the
directory `trees`. The sub directory's name is the first two characters of the
filename the tree object is stored in.

The command `restic cat tree` can be used to inspect the tree referenced above:

    $ restic -r /tmp/restic-repo cat tree b8138ab08a4722596ac89c917827358da4672eac68e3c03a8115b88dbf4bfb59
    Enter Password for Repository:
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
      ],
      "map": [
        {
          "id": "b26e315b0988ddcd1cee64c351d13a100fedbc9fdbb144a67d1b765ab280b4dc",
          "size": 910,
          "sid": "8b238c8811cc362693e91a857460c78d3acf7d9edb2f111048691976803cf16e",
          "ssize": 958
        }
      ]
    }

A tree contains a list of entries (in the field `nodes`) which contain meta
data like a name and timestamps. When the entry references a directory, the
field `subtree` contains the plain text ID of another tree object. The
associated storage ID can be found in the map object. All referenced plaintext
hashes are mapped to their corresponding storage hashes in the list contained
in the field `map`.

When the command `restic cat tree` is used, the storage hash is needed to print
a tree. The tree referenced above can be dumped as follows:

    $ restic -r /tmp/restic-repo cat tree 8b238c8811cc362693e91a857460c78d3acf7d9edb2f111048691976803cf16e
    Enter Password for Repository:
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
      ],
      "map": [
        {
          "id": "50f77b3b4291e8411a027b9f9b9e64658181cc676ce6ba9958b95f268cb1109d",
          "size": 1234,
          "sid": "00634c46e5f7c055c341acd1201cf8289cabe769f991d6e350f8cd8ce2a52ac3",
          "ssize": 1282
        },
        [...]
      ]
    }

This tree contains a file entry. This time, the `subtree` field is not present
and the `content` field contains a list with one plain text SHA-256 hash. The
storage ID for this ID can in turn be looked up in the map. Data chunks stored
as encrypted and signed files in a sub directory of the directory `data`,
similar to tree objects.

The command `restic cat data` can be used to extract and decrypt data given a
storage hash, e.g. for the data mentioned above:

    $ restic -r /tmp/restic-repo cat blob 00634c46e5f7c055c341acd1201cf8289cabe769f991d6e350f8cd8ce2a52ac3 | sha256sum
    Enter Password for Repository:
    50f77b3b4291e8411a027b9f9b9e64658181cc676ce6ba9958b95f268cb1109d  -

As can be seen from the output of the program `sha256sum`, the hash matches the
plaintext hash from the map included in the tree above, so the correct data has
been returned.

Backups and Deduplication
=========================

For creating a backup, restic scans the target directory for all files,
sub-directories and other entries. The data from each file is split into
variable length chunks cut at offsets defined by a sliding window of 64 byte.
The implementation uses Rabin Fingerprints for implementing this Content
Defined Chunking (CDC).

Files smaller than 512 KiB are not split, chunks are of 512 KiB to 8 MiB in
size. The implementation aims for 1 MiB chunk size on average.

For modified files, only modified chunks have to be saved in a subsequent
backup. This even works if bytes are inserted or removed at arbitrary positions
within the file.

Threat Model
============

The design goals for restic include being able to securely store backups in a
location that is not completely trusted, e.g. a shared system where others can
potentially access the files or (in the case of the system administrator) even
modify or delete them.

General assumptions:

 * The host system a backup is created on is trusted. This is the most basic
   requirement, and essential for creating trustworthy backups.

The restic backup program guarantees the following:

 * Accessing the unencrypted content of stored files and meta data should not
   be possible without a password for the repository. Everything except the
   `version` file and the meta data included for informational purposes in the
   key files is encrypted and then signed.

 * Modifications (intentional or unintentional) can be detected automatically
   on several layers:

     1. For all accesses of data stored in the repository it is checked whether
        the cryptographic hash of the contents matches the storage ID (the
        file's name). This way, modifications (bad RAM, broken harddisk) can be
        detected easily.

     2. Before decrypting any data, the MAC signature on the encrypted data is
        checked. If there has been a modification, the signature check will
        fail. This step happens even before the data is decrypted, so data that
        has been tampered with is not decrypted at all.

However, the restic backup program is not designed to protect against attackers
deleting files at the storage location. There is nothing that can be done about
this. If this needs to be guaranteed, get a secure location without any access
from third parties. If you assume that attackers have write access to your
files at the storage location, attackers are able to figure out (e.g. based on
the timestamps of the stored files) which files belong to what snapshot. When
only these files are deleted, the particular snapshot vanished and all
snapshots depending on data that has been added in the snapshot cannot be
restored completely. Restic is not designed to detect this attack.
