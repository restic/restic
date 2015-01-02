This document gives a high-level overview of the design and repository layout of the restic backup program.

Repository
==========

All data is stored in a restic repository, which contains of several directories and a file called `version`.
This file contains the version number of the repository.
At the moment, the file `version` is expected to hold the string `1`, with an optional newline character.

For all other files stored in the repository, the name for a file is the lower case hexadecimal representation of the SHA-256 hash of the file's contents.
This allows easily checking all files for accidental modifications like disk read errors by simply running the program `sha256sum` and comparing its output to the file name.
If the prefix of a filename is unique amongst all the other files in the same directory, the prefix may be used instead of the complete filename.

Apart from the `version` file and the files stored below the `keys` directory, all other files are encrypted with AES-256 in counter mode (CTR).
The integrity of each file is secured by an HMAC signature using SHA-256.
The encrypted data is stored in the following format: IV || CIPHERTEXT || HMAC.
For each file, a new random IV is selected.
The complete encryption overhead is 48 byte.

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
    ├── maps
    │   └── 3c0721e5c3f5d2d78a12664b568a1bc992d17b993d41079599f8437ed66192fe
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
    │   └── e0
    │       └── e01150928f7ad24befd6ec15b087de1b9e0f92edabd8e5cabb3317f8b20ad044
    └── version

The directory `keys` contains key files, which are simple JSON documents containing all data that is needed to derive the repository's master signing and encryption keys from a user's password.
The JSON document from the repository can be pretty-printed for example by using the Python module `json` (shortened to increase readability):

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

When the repository is opened by restic, the user is prompted for the repository password.
This can then be used with the key derivation function (KDF) `scrypt` and the supplied parameters (`N`, `r`, `p` and `salt`) to derive 64 key bytes.
The first 32 bytes are used as the encryption key (for AES-256) and the last 32 bytes are used as the signing key (for HMAC-SHA256).
This signing key is then used to compute an HMAC over the bytes contained in the JSON field `data`.
If the password is incorrect or the key file has been tampered with, the computed HMAC will not match the last 32 bytes of the data, and restic exits with an error.
Otherwise, the data is decrypted with the derived encryption key.
The resulting JSON document contains the master signing and encryption keys for this repository.
This way, there can be several different passwords for a single restic repository and the password can be changed.


Backups and Deduplication
=========================

For creating a backup, restic scans the target directory for all files, sub-directories and other entries.


Thread Model
============


