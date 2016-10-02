Thanks for using restic. This document will give you an overview of the basic
functionality provided by restic.

# Building/installing restic

If you are using Mac OS X, you can install restic using the
[homebrew](http://brew.sh/) packet manager:

    $ brew tap restic/restic
    $ brew install restic

On archlinux, there is a package called `restic-git` which can be installed from AUR, e.g. with `pacaur`:

    $ pacaur -S restic-git

At debian stable you can install 'go' directly from the repositories (as root):

     $ apt-get install golang-go

after installation of 'go' go straight forward to 'git clone [...]'

If you are using Linux, BSD or Windows, the only way to install restic on your
system right now is to compile it from source. restic is written in the Go
programming language and you need at least Go version 1.6. Building restic may
also work with older versions of Go, but that's not supported. See the [Getting
started](https://golang.org/doc/install) guide of the Go project for
instructions how to install Go.

In order to build restic from source, execute the following steps:

    $ git clone https://github.com/restic/restic
    [...]

    $ cd restic

    $ go run build.go

At the moment, the only tested compiler for restic is the official Go compiler.
Building restic with gccgo may work, but is not supported.

Usage help is available:

    $ ./restic --help
    Usage:
      restic [OPTIONS] <command>

    Application Options:
      -r, --repo=      Repository directory to backup to/restore from
          --cache-dir= Directory to use as a local cache
      -q, --quiet      Do not output comprehensive progress report (false)
          --no-lock    Do not lock the repo, this allows some operations on read-only repos. (false)
      -o, --option=    Specify options in the form 'foo.key=value'

    Help Options:
      -h, --help       Show this help message

    Available commands:
      backup         save file/directory
      cat            dump something
      check          check the repository
      find           find a file/directory
      forget         removes snapshots from a repository
      init           create repository
      key            manage keys
      list           lists data
      ls             list files
      mount          mount a repository
      prune          removes content from a repository
      rebuild-index  rebuild the index
      restore        restore a snapshot
      snapshots      show snapshots
      unlock         remove locks
      version        display version

Similar to programs such as `git`, restic has a number of sub-commands. You can
see these commands in the listing above. Each sub-command may have own
command-line options, and there is a help option for each command which lists
them, e.g. for the `backup` command:

    $ ./restic backup --help
    Usage:
      restic [OPTIONS] backup DIR/FILE [DIR/FILE] [...]

    The backup command creates a snapshot of a file or directory

    Application Options:
      -r, --repo=               Repository directory to backup to/restore from (/tmp/repo)
      -p, --password-file=      Read the repository password from a file
          --cache-dir=          Directory to use as a local cache
      -q, --quiet               Do not output comprehensive progress report (false)
          --no-lock             Do not lock the repo, this allows some operations on read-only repos. (false)
      -o, --option=             Specify options in the form 'foo.key=value'

    Help Options:
      -h, --help                Show this help message

    [backup command options]
          -p, --parent=         use this parent snapshot (default: last snapshot in repo that has the same target)
          -f, --force           Force re-reading the target. Overrides the "parent" flag
          -e, --exclude=        Exclude a pattern (can be specified multiple times)
              --exclude-file=   Read exclude-patterns from file
              --stdin           read backup data from stdin
              --stdin-filename= file name to use when reading from stdin (stdin)
              --tag=            Add a tag (can be specified multiple times)

Subcommand that support showing progress information such as `backup`, `check` and `prune` will do so unless
the quiet flag `-q` or `--quiet` is set. When running from a non-interactive console progress reporting will
be limited to once every 10 seconds to not fill your logs.

Additionally on Unix systems if `restic` receives a SIGUSR signal the current progress will written to the
standard output so you can check up on the status at will.


# Initialize a repository

First, we need to create a "repository". This is the place where your backups
will be saved at.

In order to create a repository at `/tmp/backup`, run the following command and
enter the same password twice:

    $ restic init --repo /tmp/backup
    enter password for new backend:
    enter password again:
    created restic backend 085b3c76b9 at /tmp/backup
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

Remembering your password is important! If you lose it, you won't be able to
access data stored in the repository.

For automated backups, restic accepts the repository location in the
environment variable `RESTIC_REPOSITORY`. The password can be read from a file
(via the option `--password-file`) or the environment variable
`RESTIC_PASSWORD`.

## Password prompt on Windows

At the moment, restic only supports the default Windows console interaction.
If you use emulation environments like [MSYS2](https://msys2.github.io/) or
[Cygwin](https://www.cygwin.com/), which use terminals like `Mintty` or `rxvt`,
you may get a password error:

You can workaround this by using a special tool called `winpty` (look
[here](https://sourceforge.net/p/msys2/wiki/Porting/) and
[here](https://github.com/rprichard/winpty) for detail information). On MSYS2,
you can install `winpty` as follows:

    $ pacman -S winpty
    $ winpty restic -r /tmp/backup init

# Create a snapshot

Now we're ready to backup some data. The contents of a directory at a specific
point in time is called a "snapshot" in restic. Run the following command and
enter the repository password you chose above again:

    $ restic -r /tmp/backup backup ~/work
    enter password for repository:
    scan [/home/user/work]
    scanned 764 directories, 1816 files in 0:00
    [0:29] 100.00%  54.732 MiB/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
    duration: 0:29, 54.47MiB/s
    snapshot 40dc1520 saved

As you can see, restic created a backup of the directory and was pretty fast!
The specific snapshot just created is identified by a sequence of hexadecimal
characters, `40dc1520` in this case.

If you run the command again, restic will create another snapshot of your data,
but this time it's even faster. This is de-duplication at work!

    $ restic -r /tmp/backup backup ~/shared/work/web
    enter password for repository:
    using parent snapshot 40dc1520aa6a07b7b3ae561786770a01951245d2367241e71e9485f18ae8228c
    scan [/home/user/work]
    scanned 764 directories, 1816 files in 0:00
    [0:00] 100.00%  0B/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
    duration: 0:00, 6572.38MiB/s
    snapshot 79766175 saved

You can even backup individual files in the same repository.

    $ restic -r /tmp/backup backup ~/work.txt
    scan [~/work.txt]
    scanned 0 directories, 1 files in 0:00
    [0:00] 100.00%  0B/s  220B / 220B  1 / 1 items  0 errors  ETA 0:00
    duration: 0:00, 0.03MiB/s
    snapshot 31f7bd63 saved

In fact several hosts may use the same repository to backup directories and
files leading to a greater de-duplication.

You can exclude folders and files by specifying exclude-patterns.  
Either specify them with multiple `--exclude`'s or one `--exclude-file`

    $ cat exclude
    # exclude go-files
    *.go
    # exclude foo/x/y/z/bar foo/x/bar foo/bar
    foo/**/bar
    $ restic -r /tmp/backup backup ~/work --exclude=*.c --exclude-file=exclude

Patterns use [`filepath.Glob`](https://golang.org/pkg/path/filepath/#Glob) internally,
see [`filepath.Match`](https://golang.org/pkg/path/filepath/#Match) for syntax.
Additionally `**` exludes arbitrary subdirectories.  
Environment-variables in exclude-files are expanded with [`os.ExpandEnv`](https://golang.org/pkg/os/#ExpandEnv).

By specifying the option `--one-file-system` you can instruct restic to only
backup files from the file systems the initially specified files or directories
reside on. For example, calling restic like this won't backup `/sys` or
`/dev` on a Linux system:

    $ restic -r /tmp/backup backup --one-file-system /

## Reading data from stdin

Sometimes it can be nice to directly save the output of a program, e.g.
`mysqldump` so that the SQL can later be restored. Restic supports this mode of
operation, just supply the option `--stdin` to the `backup` command like this:

    $ mysqldump [...] | restic -r /tmp/backup backup --stdin

This creates a new snapshot of the output of `mysqldump`. You can then use e.g.
the fuse mounting option (see below) to mount the repository and read the file.

By default, the file name `stdin` is used, a different name can be specified
with `--stdin-filename`, e.g. like this:

    $ mysqldump [...] | restic -r /tmp/backup backup --stdin --stdin-filename production.sql

## Tags

Snapshots can have one or more tags, short strings which add identifying
information. Just specify the tags for a snapshot with `--tag`:

    $ restic -r /tmp/backup backup --tag projectX ~/shared/work/web
    [...]

The tags can later be used to keep (or forget) snapshots.

# List all snapshots

Now, you can list all the snapshots stored in the repository:

    $ restic -r /tmp/backup snapshots
    enter password for repository:
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    40dc1520  2015-05-08 21:38:30  kasimir        /home/user/work
    79766175  2015-05-08 21:40:19  kasimir        /home/user/work
    bdbd3439  2015-05-08 21:45:17  luigi          /home/art
    590c8fc8  2015-05-08 21:47:38  kazik          /srv
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

You can filter the listing by directory path:

    $ restic -r /tmp/backup snapshots --path="/srv"
    enter password for repository:
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    590c8fc8  2015-05-08 21:47:38  kazik          /srv
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

Or filter by host:

    $ restic -r /tmp/backup snapshots --host luigi
    enter password for repository:
    ID        Date                 Host    Tags   Directory
    ----------------------------------------------------------------------
    bdbd3439  2015-05-08 21:45:17  luigi          /home/art
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

Combining filters is also possible.    

# Restore a snapshot

Restoring a snapshot is as easy as it sounds, just use the following command to
restore the contents of the latest snapshot to `/tmp/restore-work`:

    $ restic -r /tmp/backup restore 79766175 --target ~/tmp/restore-work
    enter password for repository:
    restoring <Snapshot of [/home/user/work] at 2015-05-08 21:40:19.884408621 +0200 CEST> to /tmp/restore-work

Use the word `latest` to restore the last backup. You can also combine `latest`
with the `--host` and `--path` filters to choose the last backup for a specific
host, path or both.

    $ restic -r /tmp/backup restore latest --target ~/tmp/restore-work --path "/home/art" --host luigi
    enter password for repository:
    restoring <Snapshot of [/home/art] at 2015-05-08 21:45:17.884408621 +0200 CEST> to /tmp/restore-work


# Manage repository keys

The `key` command allows you to set multiple access keys or passwords per
repository. In fact, you can use the `list`, `add`, `remove` and `passwd`
sub-commands to manage these keys very precisely:

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

# Check integrity and consistency

Imagine your repository is saved on a server that has a faulty hard drive, or
even worse, attackers get privileged access and modify your backup with the
intention to make you restore malicious data:

    $ sudo echo "boom" >> backup/index/d795ffa99a8ab8f8e42cec1f814df4e48b8f49129360fb57613df93739faee97

In order to detect these things, it is a good idea to regularly use the `check`
command to test whether everything is alright, your precious backup data is
consistent and the integrity is unharmed:

    $ restic -r /tmp/backup check
    Load indexes
    ciphertext verification failed

Trying to restore a snapshot which has been modified as shown above will yield
the same error:

    $ restic -r /tmp/backup restore 79766175 --target ~/tmp/restore-work
    Load indexes
    ciphertext verification failed

# Mount a repository

Browsing your backup as a regular file system is also very easy. First, create
a mount point such as `/mnt/restic` and then use the following command to serve
the repository with FUSE:

    $ mkdir /mnt/restic
    $ restic -r /tmp/backup mount /mnt/restic
    enter password for repository:
    Now serving /tmp/backup at /tmp/restic
    Don't forget to umount after quitting!

Mounting repositories via FUSE is not possible on Windows and OpenBSD.

# Create an SFTP repository

In order to backup data via SFTP, you must first set up a server with SSH and
let it know your public key. Passwordless login is really important since
restic fails to connect to the repository if the server prompts for
credentials.

Once the server is configured, the setup of the SFTP repository can simply be
achieved by changing the URL scheme in the `init` command:

    $ restic -r sftp:user@host:/tmp/backup init
    enter password for new backend:
    enter password again:
    created restic backend f1c6108821 at sftp:user@host:/tmp/backup
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

You can also specify a relative (read: no slash (`/`) character at the
beginning) directory, in this case the dir is relative to the remote user's
home directory.

# Create an Amazon S3 repository

Restic can backup data to any Amazon S3 bucket. However, in this case, changing the URL scheme is not enough since Amazon uses special security credentials to sign HTTP requests. By consequence, you must first setup the following environment variables with the credentials you obtained while creating the bucket.

    $ export AWS_ACCESS_KEY_ID=<MY_ACCESS_KEY>
    $ export AWS_SECRET_ACCESS_KEY=<MY_SECRET_ACCESS_KEY>

You can then easily initialize a repository that uses your Amazon S3 as a backend.

    $ restic -r s3:eu-central-1/bucket_name init
    enter password for new backend:
    enter password again:
    created restic backend eefee03bbd at s3:eu-central-1/bucket_name
    Please note that knowledge of your password is required to access the repository.
    Losing your password means that your data is irrecoverably lost.

Fro an s3-compatible server that is not Amazon (like Minio, see below), or is
only available via HTTP, you can specify the URL to the server like this:
`s3:http://server:port/bucket_name`.

## Create a Minio Server repository

[Minio](https://www.minio.io) is an Open Source Object Storage, written in Go and compatible with AWS S3 API.

### Pre-Requisites

* Download and Install [Minio Server](https://minio.io/download/). 
* You can also refer to [https://docs.minio.io](https://docs.minio.io) for step by step guidance on installation and getting started on Minio CLient and Minio Server.

You must first setup the following environment variables with the credentials of your running Minio Server.

    $ export AWS_ACCESS_KEY_ID=<YOUR-MINIO-ACCESS-KEY-ID>
    $ export AWS_SECRET_ACCESS_KEY= <YOUR-MINIO-SECRET-ACCESS-KEY>

Now you can easily initialize restic to use Minio server as backend with this command.

    $ ./restic -r s3:http://localhost:9000/restic init
    enter password for new backend: 
    enter password again: 
    created restic backend 6ad29560f5 at s3:http://localhost:9000/restic1
    Please note that knowledge of your password is required to access
    the repository. Losing your password means that your data is irrecoverably lost. 

# Removing old snapshots

All backup space is finite, so restic allows removing old snapshots. This can
be done either manually (by specifying a snapshot ID to remove) or by using a
policy that describes which snapshots to forget. For all remove operations, two
commands need to be called in sequence: `forget` to remove a snapshot and
`prune` to actually remove the data that was referenced by the snapshot from
the repository.

## Remove a single snapshot

The command `snapshots` can be used to list all snapshots in a repository like this:

    $ restic -r /tmp/backup snapshots
    enter password for repository:
    ID        Date                 Host      Tags  Directory
    ----------------------------------------------------------------------
    40dc1520  2015-05-08 21:38:30  kasimir         /home/user/work
    79766175  2015-05-08 21:40:19  kasimir         /home/user/work
    bdbd3439  2015-05-08 21:45:17  luigi           /home/art
    590c8fc8  2015-05-08 21:47:38  kazik           /srv
    9f0bc19e  2015-05-08 21:46:11  luigi           /srv

In order to remove the snapshot of `/home/art`, use the `forget` command and
specify the snapshot ID on the command line:

    $ restic -r /tmp/backup forget bdbd3439
    enter password for repository:
    removed snapshot d3f01f63

Afterwards this snapshot is removed:

    $ restic -r /tmp/backup snapshots
    enter password for repository:
    ID        Date                 Host     Tags  Directory
    ----------------------------------------------------------------------
    40dc1520  2015-05-08 21:38:30  kasimir        /home/user/work
    79766175  2015-05-08 21:40:19  kasimir        /home/user/work
    590c8fc8  2015-05-08 21:47:38  kazik          /srv
    9f0bc19e  2015-05-08 21:46:11  luigi          /srv

But the data that was referenced by files in this snapshot is still stored in
the repository. To cleanup unreferenced data, the `prune` command must be run:

    $ restic -r /tmp/backup prune
    enter password for repository:

    counting files in repo
    building new index for repo
    [0:00] 100.00%  22 / 22 files
    repository contains 22 packs (8512 blobs) with 100.092 MiB bytes
    processed 8512 blobs: 0 duplicate blobs, 0B duplicate
    load all snapshots
    find data that is still in use for 1 snapshots
    [0:00] 100.00%  1 / 1 snapshots
    found 8433 of 8512 data blobs still in use
    will rewrite 3 packs
    creating new index
    [0:00] 86.36%  19 / 22 files
    saved new index as 544a5084
    done

Afterwards the repository is smaller.

## Removing snapshots according to a policy

Removing snapshots manually is tedious and error-prone, therefore restic allows
specifying which snapshots should be removed automatically according to a
policy. You can specify how many hourly, daily, weekly, monthly and yearly
snapshots to keep, any other snapshots are removed. The most important
command-line parameter here is `--dry-run` which instructs restic to not remove
anything but print which snapshots would be removed.

When `forget` is run with a policy, restic loads the list of all snapshots,
then groups these by host name and list of directories. The policy is then
applied to each group of snapshots separately. This is a safety feature.

The `forget` command accepts the following parameters:

 * `--keep-last n` never delete the `n` last (most recent) snapshots
 * `--keep-hourly n` for the last `n` hours in which a snapshot was made, keep
   only the last snapshot for each hour.
 * `--keep-daily n` for the last `n` days which have one or more snapshots, only
   keep the last one for that day.
 * `--keep-weekly n` for the last `n` weeks which have one or more snapshots, only
   keep the last one for that week.
 * `--keep-monthly n` for the last `n` months which have one or more snapshots, only
   keep the last one for that month.
 * `--keep-yearly n` for the last `n` years which have one or more snapshots, only
   keep the last one for that year.
 * `--keep-tag` keep all snapshots which have all tags specified by this option
   (can be specified multiple times).

Additionally, you can restrict removing snapshots to those which have a
particular hostname with the `--hostname` parameter, or tags with the `--tag`
option. When multiple tags are specified, only the snapshots which have all the
tags are considered.

All the `--keep-*` options above only count hours/days/weeks/months/years which
have a snapshot, so those without a snapshot are ignored.

Let's explain this with an example: Suppose you have only made a backup on each
Sunday for 12 weeks. Then `forget --keep-daily 4` will keep the last four snapshots
for the last four Sundays, but remove the rest. Only counting the days which
have a backup and ignore the ones without is a safety feature: it prevents
restic from removing many snapshots when no new ones are created. If it was
implemented otherwise, running `forget --keep-daily 4` on a Friday would remove
all snapshots!

# Debugging restic

The program can be built with debug support like this:

    $ go run build.go -tags debug

Afterwards, extensive debug messages are written to the file in environment
variable `DEBUG_LOG`, e.g.:

    $ DEBUG_LOG=/tmp/restic-debug.log restic backup ~/work

If you suspect that there is a bug, you can have a look at the debug log.
Please be aware that the debug log might contain sensitive information such as
file and directory names.

The debug log will always contain all log messages restic generates. You can
also instruct restic to print some or all debug messages to stderr. These can
also be limited to e.g. a list of source files or a list of patterns for
function names. The patterns are globbing patterns (see the documentation for
[`path.Glob`](https://golang.org/pkg/path/#Glob)), multiple patterns are
separated by commas. Patterns are case sensitive.

Printing all log messages to the console can be achieved by setting the file
filter to `*`:

    $ DEBUG_FILES=* restic check

If you want restic to just print all debug log messages from the files
`main.go` and `lock.go`, set the environment variable `DEBUG_FILES` like this: 

    $ DEBUG_FILES=main.go,lock.go restic check

The following command line instructs restic to only print debug statements
originating in functions that match the pattern `*unlock*` (case sensitive):

    $ DEBUG_FUNCS=*unlock* restic check

# Under the hood: Browse repository objects

Internally, a repository stores data of several different types described in the [design documentation](https://github.com/restic/restic/blob/master/doc/Design.md). You can `list` objects such as blobs, packs, index, snapshots, keys or locks with the following command:

```shell
$ restic -r /tmp/backup list snapshots
d369ccc7d126594950bf74f0a348d5d98d9e99f3215082eb69bf02dc9b3e464c
```

The `find` command searches for a given
[pattern](http://golang.org/pkg/path/filepath/#Match) in the repository.

    $ restic -r backup find test.txt
    debug log file restic.log
    debug enabled
    enter password for repository:
    found 1 matching entries in snapshot 196bc5760c909a7681647949e80e5448e276521489558525680acf1bd428af36
      -rw-r--r--   501    20      5 2015-08-26 14:09:57 +0200 CEST path/to/test.txt

The `cat` command allows you to display the JSON representation of the objects
or its raw content.

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
