<p align="center">
  <img src="doc/logo/logo.png" alt="Restic logo" width="200" height="200">
</p>

<p align="center">
  <strong>Restic</strong> – <strong>Fast</strong>, <strong>efficient</strong>, and <strong>secure</strong> backup program
  <br>
  <a href="https://restic.net/">Website</a> |
  <a href="https://restic.readthedocs.io/">Documentation</a> |
  <a href="https://forum.restic.net/">Forum</a> |
  <a href="https://github.com/restic/restic/issues">Issues</a>
</p>

# Restic

[![Documentation](https://readthedocs.org/projects/restic/badge/?version=latest)](https://restic.readthedocs.io/en/latest/)
[![Build Status](https://github.com/restic/restic/workflows/test/badge.svg)](https://github.com/restic/restic/actions?query=workflow%3Atest)
[![Go Report Card](https://goreportcard.com/badge/github.com/restic/restic)](https://goreportcard.com/report/github.com/restic/restic)

Restic is a backup program that is fast, efficient, and secure. It is supported
on major operating systems (Linux, macOS, and Windows) as well as a few smaller
ones (FreeBSD and OpenBSD).

- **Easy**: Creating backups should be a frictionless process, otherwise it
  may be tempting to skip it. Restic should be easy to configure and use, so
  that you can quickly restore your data in the event of a data loss.

- **Fast**: Nobody does backups if it takes too much time. Backing up your data
  with restic should only be limited by your network or storage bandwidth, so
  that you can back up your files every day. When restoring from backups, only
  the data necessary for restoring is transferred.

- **Verifiable**: Making sure your backups can restore your data is equally as
  important as creating backups. Restic enables you to easily verify the
  integrity of your backups.

- **Secure**: Restic uses cryptography to guarantee confidentiality and
  integrity of your data. Restic does not assume your backup data is located in
  a trusted environement (e.g., a shared space where system administrators are
  able to access your backups).

- **Efficient**: Restic de-duplicates data before it is written to the storage
  backend to save space. A backup snapshot should only occupy the storage of
  the actual increment.

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Supported backends](#supported-backends)
- [Reproducible builds](#reproducible-builds)
- [News](#news)
- [License](#license)
- [Sponsorship](#sponsorship)

## Installation

Restic is available on many platforms. Check out the [documentation](https://restic.readthedocs.io/)
for [installation instructions](https://restic.readthedocs.io/en/stable/020_installation.html).

## Quick start

Once you've installed restic, start off by creating a repository for your
backups:

  ```
  $ restic init --repo /tmp/backup
  enter password for new backend:
  enter password again:
  created restic backend 085b3c76b9 at /tmp/backup
  Please note that knowledge of your password is required to access the repository.
  Losing your password means that your data is irrecoverably lost.
  ```

Then create a backup snapshot of some data:

  ```
  $ restic --repo /tmp/backup backup ~/work
  enter password for repository:
  scan [/home/user/work]
  scanned 764 directories, 1816 files in 0:00
  [0:29] 100.00%  54.732 MiB/s  1.582 GiB / 1.582 GiB  2580 / 2580 items  0 errors  ETA 0:00
  duration: 0:29, 54.47MiB/s
  snapshot 40dc1520 saved
  ```

Next, you can use the `restic restore` command to restore files. You can also
use the `restic mount` command to mount the repository via FUSE and browse the
files from previous snapshots.

Check out the [documentation](https://restic.readthedocs.io/en/latest/) for
more command options. You can also ask questions on our [Discourse forum](https://forum.restic.net).

## Supported backends

Although saving a backup on the same machine is nice, it is not a real backup
strategy. Therefore, restic supports the following backends for storing backups
natively:

- [Local directory](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#local)
- [SFTP server (via SSH)](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#sftp)
- [HTTP REST server](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#rest-server) ([protocol](https://restic.readthedocs.io/en/latest/100_references.html#rest-backend), [rest-server](https://github.com/restic/rest-server))
- [AWS S3](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#amazon-s3) (either from Amazon or using the [MinIO](https://minio.io) server)
- [OpenStack Swift](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#openstack-swift)
- [Backblaze B2](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#backblaze-b2)
- [Microsoft Azure Blob Storage](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#microsoft-azure-blob-storage)
- [Google Cloud Storage](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#google-cloud-storage)
- And many other services via the [Rclone backend](https://restic.readthedocs.io/en/latest/030_preparing_a_new_repo.html#other-services-via-rclone)

## Reproducible builds

The binaries for each restic version release (starting with v0.6.1) are
[reproducible](https://reproducible-builds.org/). This means that you can
reproduce byte-identical copies of restic from the source code. See the
[builder repository](https://github.com/restic/builder) for build instructions.

## News

You can follow the restic project on Twitter [@resticbackup](https://twitter.com/resticbackup)
or by subscribing to the [project blog](https://restic.net/blog/).

## License

Restic is licensed under [BSD 2-Clause License](https://opensource.org/licenses/BSD-2-Clause).
You can find the complete text in [``LICENSE``](LICENSE).

## Sponsorship

Backend integration tests for Google Cloud Storage and Microsoft Azure Blob
Storage are sponsored by [AppsCode](https://appscode.com)!

[![Sponsored by AppsCode](https://cdn.appscode.com/images/logo/appscode/ac-logo-color.png)](https://appscode.com)
