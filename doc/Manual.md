## Building restic

If you are using Mac OS X, you can install restic using the [homebrew](http://brew.sh/) packet manager:

```shell
$ brew tap restic/restic
$ brew install restic
```

If you are using Linux, BSD or Windows, the only way to install restic on your system right now is to compile it from source. restic is written in the Go programming language and you need at least Go version 1.3. See the [Getting started](https://golang.org/doc/install) guide of the Go project for instructions how to install Go.

In order to build restic from source, execute the following steps:

```shell
$ git clone https://github.com/restic/restic
[...]

$ cd restic

$ go run build.go
[...]
```

Usage help is available:

```shell
$ ./restic --help
Usage:
  restic [OPTIONS] <command>

Application Options:
  -r, --repo= Repository directory to backup to/restore from

Help Options:
  -h, --help  Show this help message

Available commands:
  backup     save file/directory
  cache      manage cache
  cat        dump something
  find       find a file/directory
  fsck       check the repository
  init       create repository
  key        manage keys
  list       lists data
  ls         list files
  restore    restore a snapshot
  snapshots  show snapshots
  version    display version
```

Subcommand-specific help is there too:

```shell
$ ./restic backup --help
Usage:
  restic [OPTIONS] backup DIR/FILE [DIR/FILE] [...]

The backup command creates a snapshot of a file or directory

Application Options:
  -r, --repo=        Repository directory to backup to/restore from
      --cache-dir=   Directory to use as a local cache
  -q, --quiet        Do not output comprehensive progress report (false)
      --no-lock      Do not lock the repo, this allows some operations on read-only repos. (false)

Help Options:
  -h, --help         Show this help message

[backup command options]
      -p, --parent=  use this parent snapshot (default: last snapshot in repo that has the same target)
      -f, --force    Force re-reading the target. Overrides the "parent" flag
      -e, --exclude= Exclude a pattern (can be specified multiple times)
```

## Initialize a repository

First, we need to create a "repository". This is the place where your backups will be saved at.

In order to create a repository at `/tmp/backup`, run the following command and enter the same password twice:

```shell
$ restic init --repo /tmp/backup
enter password for new backend:
enter password again:
created restic backend 085b3c76b9 at /tmp/backup
Please note that knowledge of your password is required to access the repository.
Losing your password means that your data is irrecoverably lost.
```

Remembering your password is important! If you lose it, you won't be able to access data stored in the repository.

Attention, at the moment restic only supports real Windows console interaction. If you use emulation environments like
[MSYS2](https://msys2.github.io/) or [Cygwin](https://www.cygwin.com/), which use terminals like Mintty, rxvt,
you'll get an password error:

```shell
$ restic -r /tmp/backup init
enter password for repository: unable to read password: Das Handle ist ungültig.
```

You can workaround this by using a special tool called "winpty" (look [here](https://sourceforge.net/p/msys2/wiki/Porting/)
and [here](https://github.com/rprichard/winpty) for detail information).

```shell
# install winpty on MSYS2
$ pacman -S winpty
$ winpty restic -r /tmp/backup init
```

For automated backups, restic accepts the repository location in the environment variable `RESTIC_REPOSITORY` and also the password in the variable `RESTIC_PASSWORD`.

## Create a snapshot

Now we're ready to backup some data. The contents of a directory at a specific point in time is called a "snapshot" in restic. Run the following command and enter the repository password you chose above again:

```shell
$ restic -r /tmp/backup backup ~/work
enter password for repository:
scan [/home/user/work]
scanned 764 directories, 1816 files in 0:00
[0:29] 100.00%  54.732 MiB/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
duration: 0:29, 54.47MiB/s
snapshot 40dc1520 saved
```

As you can see, restic created a backup of the directory and was pretty fast! The specific snapshot just created is identified by a sequence of hexadecimal characters, `40dc1520` in this case.

If you run the command again, restic will create another snapshot of your data, but this time it's even faster. This is deduplication at work!

```shell
$ restic -r /tmp/backup backup ~/shared/work/web
enter password for repository:
using parent snapshot 40dc1520aa6a07b7b3ae561786770a01951245d2367241e71e9485f18ae8228c
scan [/home/user/work]
scanned 764 directories, 1816 files in 0:00
[0:00] 100.00%  0B/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
duration: 0:00, 6572.38MiB/s
snapshot 79766175 saved
```

You can even backup individual files in the same repository.

```shell
$ restic -r /tmp/backup backup ~/work.txt
scan [/tmp/backup backup ~/work.txt]
scanned 0 directories, 1 files in 0:00
[0:00] 100.00%  0B/s  220B / 220B  1 / 1 items  0 errors  ETA 0:00
duration: 0:00, 0.03MiB/s
snapshot 31f7bd63 saved
```

In fact several hosts may use the same repository to backup directories and files leading to a greater deduplication.

## List all snapshots

Now, you can list all the snapshots stored in the repository:

```shell
$ restic -r /tmp/backup snapshots
enter password for repository:
ID        Date                 Source      Directory
----------------------------------------------------------------------
40dc1520  2015-05-08 21:38:30  kasimir     /home/user/work
79766175  2015-05-08 21:40:19  kasimir     /home/user/work
```

## Restore a snapshot

Restoring a snapshot is as easy as it sounds, just use the following command to restore the contents of the latest snapshot to `/tmp/restore-work`:

```shell
$ restic -r /tmp/backup restore 79766175 --target ~/tmp/restore-work
enter password for repository:
restoring <Snapshot of [/home/user/work] at 2015-05-08 21:40:19.884408621 +0200 CEST> to /tmp/restore-work
```

## Manage repository keys

The `key` command allows you to set multiple access keys or passwords per repository. In fact, you can use the `list`, `add`, `remove` and `passwd` sub-commands to manage these keys very precisely:

```shell
$ restic -r /tmp/backup key list
enter password for repository:
 ID          User        Host        Created
----------------------------------------------------------------------
*eb78040b    username    kasimir   2015-08-12 13:29:57

$ restic -r /tmp/backup key add
enter password for repository:
enter password for new key:
enter password again:
saved new key as <Key of username@kasimir, created on 2015-08-12 13:35:05.316831933 +0200 CEST>

$ restic -r backup key list
enter password for repository:
 ID          User        Host        Created
----------------------------------------------------------------------
 5c657874    username    kasimir   2015-08-12 13:35:05
*eb78040b    username    kasimir   2015-08-12 13:29:57
```

## Check integrity and consistency

Imagine your repository is saved on a server that has a faulty hard drive, or even worse, attackers get privileged access and modify your backup with the intention to make you restore malicious data:

```shell
$ sudo echo "boom" >> backup/index/d795ffa99a8ab8f8e42cec1f814df4e48b8f49129360fb57613df93739faee97
```

In order to detect these things, it is a good idea to regularly use the `check` command to test whether everything is alright, your precious backup data is consistent and the integrity is unharmed:

```shell
$ restic -r /tmp/backup check
Load indexes
ciphertext verification failed
```

Trying to restore a snapshot which has been modified as shown above will yield the same error:

```shell
$ restic -r /tmp/backup restore 79766175 --target ~/tmp/restore-work
Load indexes
ciphertext verification failed
```

## Mount a repository

Browsing your backup as a regular file system is also very easy. First, create a mount point such as `/mnt/restic` and then use the following command to serve the repository with FUSE:

```shell
$ mkdir /mnt/restic
$ restic -r /tmp/backup mount /mnt/restic
enter password for repository:
Now serving /tmp/backup at /tmp/restic
Don't forget to umount after quitting!
```

Windows doesn't support FUSE directly. Projects like [dokan](http://dokan-dev.github.io/) try to fill the gap.
We haven't tested it yet, but we'd like to hear about your experience. For setup information see
[dokan FUSE in dokan's wiki](https://github.com/dokan-dev/dokany/wiki/FUSE).

## Create an SFTP repository

In order to backup data via SFTP, you must first set up a server with SSH and let it know your public key. Passwordless login is really important since restic fails to connect to the repository if the server prompts for credentials.

Once the server is configured, the setup of the SFTP repository can simply be achieved by changing the URL scheme in the `init` command:

```shell
$ restic -r sftp://user@host//tmp/backup init
enter password for new backend:
enter password again:
created restic backend f1c6108821 at sftp://user@host//tmp/backup
Please note that knowledge of your password is required to access the repository.
Losing your password means that your data is irrecoverably lost.
```

Yes, that's really two slash (`/`) characters after the host name, here the directory `/tmp/backup` on the server is meant. If you'd rather like to create a repository in the user's home directory on the server, use the location `sftp://user@host/foo/bar/repo`. In this case the directory is relative to the user's home directory: `foo/bar/repo`.


## Create an Amazon S3 repository

Restic can backup data to any Amazon S3 bucket. However, in this case, changing the URL scheme is not enough since Amazon uses special security credentials to sign HTTP requests. By consequence, you must first setup the following environment variables with the credentials you obtained while creating the bucket.

```shell
$ export AWS_ACCESS_KEY_ID=<MY_ACCESS_KEY>
$ export AWS_SECRET_ACCESS_KEY=<MY_SECRET_ACCESS_KEY>
```

You can then easily initialize a repository that uses your Amazon S3 as a backend.

```shell
$ restic -r s3://s3.amazonaws.com/bucket_name init
enter password for new backend:
enter password again:
created restic backend eefee03bbd at s3://s3.amazonaws.com/bucket_name
Please note that knowledge of your password is required to access the repository.
Losing your password means that your data is irrecoverably lost.
```

For an S3-compatible repository without TLS available, use the alternative URI protocol `s3:http://server:port/bucket_name`.

## Under the hood: Browse repository objects

Internally, a repository stores data of several different types described in the [design documentation](https://github.com/restic/restic/blob/master/doc/Design.md). You can `list` objects such as blobs, packs, index, snapshots, keys or locks with the following command:

```shell
$ restic -r /tmp/backup list snapshots
d369ccc7d126594950bf74f0a348d5d98d9e99f3215082eb69bf02dc9b3e464c
```

The `find` command searches for a given [pattern](http://golang.org/pkg/path/filepath/#Match) in the repository.

```
restic -r backup find test.txt
debug log file restic.log
debug enabled
enter password for repository:
found 1 matching entries in snapshot 196bc5760c909a7681647949e80e5448e276521489558525680acf1bd428af36
  -rw-r--r--   501    20      5 2015-08-26 14:09:57 +0200 CEST path/to/test.txt
```

The `cat` command allows you to display the JSON representation of the objects or its raw content.

```shell
$ restic -r /tmp/backup cat snapshot d369ccc7d126594950bf74f0a348d5d98d9e99f3215082eb69bf02dc9b3e464c
enter password for repository:
{
  "time": "2015-08-12T12:52:44.091448856+02:00",
  "tree": "05cec17e8d3349f402576d02576a2971fc0d9f9776ce2f441c7010849c4ff5af",
  "paths": [
    "/home/user/work"
  ],
  "hostname": "kasimir",
  "username": "username",
  "uid": 501,
  "gid": 20
}
```

