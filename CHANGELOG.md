# Table of Contents

* [Changelog for 0.17.0](#changelog-for-restic-0170-2024-07-26)
* [Changelog for 0.16.5](#changelog-for-restic-0165-2024-07-01)
* [Changelog for 0.16.4](#changelog-for-restic-0164-2024-02-04)
* [Changelog for 0.16.3](#changelog-for-restic-0163-2024-01-14)
* [Changelog for 0.16.2](#changelog-for-restic-0162-2023-10-29)
* [Changelog for 0.16.1](#changelog-for-restic-0161-2023-10-24)
* [Changelog for 0.16.0](#changelog-for-restic-0160-2023-07-31)
* [Changelog for 0.15.2](#changelog-for-restic-0152-2023-04-24)
* [Changelog for 0.15.1](#changelog-for-restic-0151-2023-01-30)
* [Changelog for 0.15.0](#changelog-for-restic-0150-2023-01-12)
* [Changelog for 0.14.0](#changelog-for-restic-0140-2022-08-25)
* [Changelog for 0.13.0](#changelog-for-restic-0130-2022-03-26)
* [Changelog for 0.12.1](#changelog-for-restic-0121-2021-08-03)
* [Changelog for 0.12.0](#changelog-for-restic-0120-2021-02-14)
* [Changelog for 0.11.0](#changelog-for-restic-0110-2020-11-05)
* [Changelog for 0.10.0](#changelog-for-restic-0100-2020-09-19)
* [Changelog for 0.9.6](#changelog-for-restic-096-2019-11-22)
* [Changelog for 0.9.5](#changelog-for-restic-095-2019-04-23)
* [Changelog for 0.9.4](#changelog-for-restic-094-2019-01-06)
* [Changelog for 0.9.3](#changelog-for-restic-093-2018-10-13)
* [Changelog for 0.9.2](#changelog-for-restic-092-2018-08-06)
* [Changelog for 0.9.1](#changelog-for-restic-091-2018-06-10)
* [Changelog for 0.9.0](#changelog-for-restic-090-2018-05-21)
* [Changelog for 0.8.3](#changelog-for-restic-083-2018-02-26)
* [Changelog for 0.8.2](#changelog-for-restic-082-2018-02-17)
* [Changelog for 0.8.1](#changelog-for-restic-081-2017-12-27)
* [Changelog for 0.8.0](#changelog-for-restic-080-2017-11-26)
* [Changelog for 0.7.3](#changelog-for-restic-073-2017-09-20)
* [Changelog for 0.7.2](#changelog-for-restic-072-2017-09-13)
* [Changelog for 0.7.1](#changelog-for-restic-071-2017-07-22)
* [Changelog for 0.7.0](#changelog-for-restic-070-2017-07-01)
* [Changelog for 0.6.1](#changelog-for-restic-061-2017-06-01)
* [Changelog for 0.6.0](#changelog-for-restic-060-2017-05-29)


# Changelog for restic 0.17.0 (2024-07-26)
The following sections list the changes in restic 0.17.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #3600: Handle unreadable xattrs in folders above `backup` source
 * Fix #4209: Fix slow SFTP upload performance
 * Fix #4503: Correct hardlink handling in `stats` command
 * Fix #4568: Prevent `forget --keep-tags <invalid>` from deleting all snapshots
 * Fix #4615: Make `find` not sometimes ignore directories
 * Fix #4656: Properly report ID of newly added keys
 * Fix #4703: Shutdown cleanly when receiving SIGTERM
 * Fix #4709: Correct `--no-lock` handling of `ls` and `tag` commands
 * Fix #4760: Fix possible error on concurrent cache cleanup
 * Fix #4850: Handle UTF-16 password files in `key` command correctly
 * Fix #4902: Update snapshot summary on `rewrite`
 * Chg #956: Return exit code 10 and 11 for non-existing and locked repository
 * Chg #4540: Require at least ARMv6 for ARM binaries
 * Chg #4602: Deprecate legacy index format and `s3legacy` repository layout
 * Chg #4627: Redesign backend error handling to improve reliability
 * Chg #4707: Disable S3 anonymous authentication by default
 * Chg #4744: Include full key ID in JSON output of `key list`
 * Enh #662: Optionally skip snapshot creation if nothing changed
 * Enh #693: Include snapshot size in `snapshots` output
 * Enh #805: Add bitrot detection to `diff` command
 * Enh #828: Improve features of the `repair packs` command
 * Enh #1786: Support repositories with empty password
 * Enh #2348: Add `--delete` option to `restore` command
 * Enh #3067: Add extended options to configure Windows Shadow Copy Service
 * Enh #3406: Improve `dump` performance for large files
 * Enh #3806: Optimize and make `prune` command resumable
 * Enh #4006: (alpha) Store deviceID only for hardlinks
 * Enh #4048: Add support for FUSE-T with `mount` on macOS
 * Enh #4251: Support reading backup from a command's standard output
 * Enh #4287: Support connection to rest-server using unix socket
 * Enh #4354: Significantly reduce `prune` memory usage
 * Enh #4437: Make `check` command create non-existent cache directory
 * Enh #4472: Support AWS Assume Role for S3 backend
 * Enh #4547: Add `--json` option to `version` command
 * Enh #4549: Add `--ncdu` option to `ls` command
 * Enh #4573: Support rewriting host and time metadata in snapshots
 * Enh #4583: Ignore `s3.storage-class` archive tiers for metadata
 * Enh #4590: Speed up `mount` command's error detection
 * Enh #4601: Add support for feature flags
 * Enh #4611: Back up more file metadata on Windows
 * Enh #4664: Make `ls` use `message_type` field in JSON output
 * Enh #4676: Make `key` command's actions separate sub-commands
 * Enh #4678: Add `--target` option to the `dump` command
 * Enh #4708: Back up and restore SecurityDescriptors on Windows
 * Enh #4733: Allow specifying `--host` via environment variable
 * Enh #4737: Include snapshot ID in `reason` field of `forget` JSON output
 * Enh #4764: Support forgetting all snapshots
 * Enh #4768: Allow specifying custom User-Agent for outgoing requests
 * Enh #4781: Add `restore` options to read include/exclude patterns from files
 * Enh #4807: Support Extended Attributes on Windows NTFS
 * Enh #4817: Make overwrite behavior of `restore` customizable
 * Enh #4839: Add dry-run support to `restore` command

## Details

 * Bugfix #3600: Handle unreadable xattrs in folders above `backup` source

   When backup sources are specified using absolute paths, `backup` also includes
   information about the parent folders of the backup sources in the snapshot.

   If the extended attributes for some of these folders could not be read due to
   missing permissions, this caused the backup to fail. This has now been fixed.

   https://github.com/restic/restic/issues/3600
   https://github.com/restic/restic/pull/4668
   https://forum.restic.net/t/parent-directories-above-the-snapshot-source-path-fatal-error-permission-denied/7216

 * Bugfix #4209: Fix slow SFTP upload performance

   Since restic 0.12.1, the upload speed of the sftp backend to a remote server has
   regressed significantly. This has now been fixed.

   https://github.com/restic/restic/issues/4209
   https://github.com/restic/restic/pull/4782

 * Bugfix #4503: Correct hardlink handling in `stats` command

   If files on different devices had the same inode ID, the `stats` command did not
   correctly calculate the snapshot size. This has now been fixed.

   https://github.com/restic/restic/pull/4503
   https://github.com/restic/restic/pull/4006
   https://forum.restic.net/t/possible-bug-in-stats/6461/8

 * Bugfix #4568: Prevent `forget --keep-tags <invalid>` from deleting all snapshots

   Running `forget --keep-tags <invalid>`, where `<invalid>` is a tag that does not
   exist in the repository, would remove all snapshots. This is especially
   problematic if the tag name contains a typo.

   The `forget` command now fails with an error if all snapshots in a snapshot
   group would be deleted. This prevents the above example from deleting all
   snapshots.

   It is possible to temporarily disable the new check by setting the environment
   variable `RESTIC_FEATURES=safe-forget-keep-tags=false`. Note that this feature
   flag will be removed in the next minor restic version.

   https://github.com/restic/restic/pull/4568
   https://github.com/restic/restic/pull/4764

 * Bugfix #4615: Make `find` not sometimes ignore directories

   In some cases, the `find` command ignored empty or moved directories. This has
   now been fixed.

   https://github.com/restic/restic/pull/4615

 * Bugfix #4656: Properly report ID of newly added keys

   `restic key add` now reports the ID of the newly added key. This simplifies
   selecting a specific key using the `--key-hint key` option.

   https://github.com/restic/restic/issues/4656
   https://github.com/restic/restic/pull/4657

 * Bugfix #4703: Shutdown cleanly when receiving SIGTERM

   Previously, when restic received the SIGTERM signal it would terminate
   immediately, skipping cleanup and potentially causing issues like stale locks
   being left behind. This primarily effected containerized restic invocations that
   use SIGTERM, but could also be triggered via a simple `killall restic`.

   This has now been fixed, such that restic shuts down cleanly when receiving the
   SIGTERM signal.

   https://github.com/restic/restic/pull/4703

 * Bugfix #4709: Correct `--no-lock` handling of `ls` and `tag` commands

   The `ls` command never locked the repository. This has now been fixed, with the
   old behavior still being supported using `ls --no-lock`. The latter invocation
   also works with older restic versions.

   The `tag` command erroneously accepted the `--no-lock` command. This command now
   always requires an exclusive lock.

   https://github.com/restic/restic/pull/4709

 * Bugfix #4760: Fix possible error on concurrent cache cleanup

   If multiple restic processes concurrently cleaned up no longer existing files
   from the cache, this could cause some of the processes to fail with an `no such
   file or directory` error. This has now been fixed.

   https://github.com/restic/restic/issues/4760
   https://github.com/restic/restic/pull/4761

 * Bugfix #4850: Handle UTF-16 password files in `key` command correctly

   Previously, `key add` and `key passwd` did not properly decode UTF-16 encoded
   passwords read from a password file. This has now been fixed to correctly match
   the encoding when opening a repository.

   https://github.com/restic/restic/issues/4850
   https://github.com/restic/restic/pull/4851

 * Bugfix #4902: Update snapshot summary on `rewrite`

   Restic previously did not recalculate the total number of files and bytes
   processed when files were excluded from a snapshot by the `rewrite` command.
   This has now been fixed.

   https://github.com/restic/restic/issues/4902
   https://github.com/restic/restic/pull/4905

 * Change #956: Return exit code 10 and 11 for non-existing and locked repository

   If a repository does not exist or cannot be locked, restic previously always
   returned exit code 1. This made it difficult to distinguish these cases from
   other errors.

   Restic now returns exit code 10 if the repository does not exist, and exit code
   11 if the repository could be not locked due to a conflicting lock.

   https://github.com/restic/restic/issues/956
   https://github.com/restic/restic/pull/4884

 * Change #4540: Require at least ARMv6 for ARM binaries

   The official release binaries of restic now require at least ARMv6 support for
   ARM platforms.

   https://github.com/restic/restic/issues/4540
   https://github.com/restic/restic/pull/4542

 * Change #4602: Deprecate legacy index format and `s3legacy` repository layout

   Support for the legacy index format used by restic before version 0.2.0 has been
   deprecated and will be removed in the next minor restic version. You can use
   `restic repair index` to update the index to the current format.

   It is possible to temporarily reenable support for the legacy index format by
   setting the environment variable `RESTIC_FEATURES=deprecate-legacy-index=false`.
   Note that this feature flag will be removed in the next minor restic version.

   Support for the `s3legacy` repository layout used for the S3 backend before
   restic 0.7.0 has been deprecated and will be removed in the next minor restic
   version. You can migrate your S3 repository to the current layout using
   `RESTIC_FEATURES=deprecate-s3-legacy-layout=false restic migrate s3_layout`.

   It is possible to temporarily reenable support for the `s3legacy` layout by
   setting the environment variable
   `RESTIC_FEATURES=deprecate-s3-legacy-layout=false`. Note that this feature flag
   will be removed in the next minor restic version.

   https://github.com/restic/restic/issues/4602
   https://github.com/restic/restic/pull/4724
   https://github.com/restic/restic/pull/4743

 * Change #4627: Redesign backend error handling to improve reliability

   Restic now downloads pack files in large chunks instead of using a streaming
   download. This prevents failures due to interrupted streams. The `restore`
   command now also retries downloading individual blobs that could not be
   retrieved.

   HTTP requests that are stuck for more than two minutes while uploading or
   downloading are now forcibly interrupted. This ensures that stuck requests are
   retried after a short timeout.

   Attempts to access a missing or truncated file will no longer be retried. This
   avoids unnecessary retries in those cases. All other backend requests are
   retried for up to 15 minutes. This ensures that temporarily interrupted network
   connections can be tolerated.

   If a download yields a corrupt file or blob, then the download will be retried
   once.

   Most parts of the new backend error handling can temporarily be disabled by
   setting the environment variable `RESTIC_FEATURES=backend-error-redesign=false`.
   Note that this feature flag will be removed in the next minor restic version.

   https://github.com/restic/restic/issues/4627
   https://github.com/restic/restic/issues/4193
   https://github.com/restic/restic/issues/4515
   https://github.com/restic/restic/issues/1523
   https://github.com/restic/restic/pull/4605
   https://github.com/restic/restic/pull/4792
   https://github.com/restic/restic/pull/4520
   https://github.com/restic/restic/pull/4800
   https://github.com/restic/restic/pull/4784
   https://github.com/restic/restic/pull/4844

 * Change #4707: Disable S3 anonymous authentication by default

   When using the S3 backend with anonymous authentication, it continuously tried
   to retrieve new authentication credentials, causing bad performance.

   Now, to use anonymous authentication, it is necessary to pass the extended
   option `-o s3.unsafe-anonymous-auth=true` to restic.

   It is possible to temporarily revert to the old behavior by setting the
   environment variable `RESTIC_FEATURES=explicit-s3-anonymous-auth=false`. Note
   that this feature flag will be removed in the next minor restic version.

   https://github.com/restic/restic/issues/4707
   https://github.com/restic/restic/pull/4908

 * Change #4744: Include full key ID in JSON output of `key list`

   The JSON output of the `key list` command has changed to include the full key ID
   instead of just a shortened version of the ID, as the latter can be ambiguous in
   some rare cases. To derive the short ID, please truncate the full ID down to
   eight characters.

   https://github.com/restic/restic/issues/4744
   https://github.com/restic/restic/pull/4745

 * Enhancement #662: Optionally skip snapshot creation if nothing changed

   The `backup` command always created a snapshot even if nothing in the backup set
   changed compared to the parent snapshot.

   Restic now supports the `--skip-if-unchanged` option for the `backup` command,
   which omits creating a snapshot if the new snapshot's content would be identical
   to that of the parent snapshot.

   https://github.com/restic/restic/issues/662
   https://github.com/restic/restic/pull/4816

 * Enhancement #693: Include snapshot size in `snapshots` output

   The `snapshots` command now prints the size for snapshots created using this or
   a future restic version. To achieve this, the `backup` command now stores the
   backup summary statistics in the snapshot.

   The text output of the `snapshots` command only shows the snapshot size. The
   other statistics are only included in the JSON output. To inspect these
   statistics use `restic snapshots --json` or `restic cat snapshot <snapshotID>`.

   https://github.com/restic/restic/issues/693
   https://github.com/restic/restic/pull/4705
   https://github.com/restic/restic/pull/4913

 * Enhancement #805: Add bitrot detection to `diff` command

   The output of the `diff` command now includes the modifier `?` for files to
   indicate bitrot in backed up files. The `?` will appear whenever there is a
   difference in content while the metadata is exactly the same.

   Since files with unchanged metadata are normally not read again when creating a
   backup, the detection is only effective when the right-hand side of the diff has
   been created with `backup --force`.

   https://github.com/restic/restic/issues/805
   https://github.com/restic/restic/pull/4526

 * Enhancement #828: Improve features of the `repair packs` command

   The `repair packs` command has been improved to also be able to process
   truncated pack files. The `check` and `check --read-data` command will provide
   instructions on using the command if necessary to repair a repository. See the
   guide at https://restic.readthedocs.io/en/stable/077_troubleshooting.html for
   further instructions.

   https://github.com/restic/restic/issues/828
   https://github.com/restic/restic/pull/4644
   https://github.com/restic/restic/pull/4882

 * Enhancement #1786: Support repositories with empty password

   Restic previously required a password to create or operate on repositories.
   Using the new option `--insecure-no-password` it is now possible to disable this
   requirement. Restic will not prompt for a password when using this option.

   For security reasons, the option must always be specified when operating on
   repositories with an empty password, and specifying `--insecure-no-password`
   while also passing a password to restic via a CLI option or environment variable
   results in an error.

   The `init` and `copy` commands add the related `--from-insecure-no-password`
   option, which applies to the source repository. The `key add` and `key passwd`
   commands add the `--new-insecure-no-password` option to add or set an empty
   password.

   https://github.com/restic/restic/issues/1786
   https://github.com/restic/restic/issues/4326
   https://github.com/restic/restic/pull/4698
   https://github.com/restic/restic/pull/4808

 * Enhancement #2348: Add `--delete` option to `restore` command

   The `restore` command now supports a `--delete` option that allows removing
   files and directories from the target directory that do not exist in the
   snapshot. This option also allows files in the snapshot to replace non-empty
   directories having the same name.

   To check that only expected files are deleted, add the `--dry-run --verbose=2`
   options.

   https://github.com/restic/restic/issues/2348
   https://github.com/restic/restic/pull/4881

 * Enhancement #3067: Add extended options to configure Windows Shadow Copy Service

   Previous, restic always used a 120 seconds timeout and unconditionally created
   VSS snapshots for all volume mount points on disk. This behavior can now be
   fine-tuned by the following new extended options (available only on Windows):

   - `-o vss.timeout`: Time that VSS can spend creating snapshot before timing out
   (default: 120s) - `-o vss.exclude-all-mount-points`: Exclude mountpoints from
   snapshotting on all volumes (default: false) - `-o vss.exclude-volumes`:
   Semicolon separated list of volumes to exclude from snapshotting - `-o
   vss.provider`: VSS provider identifier which will be used for snapshotting

   For example, change VSS timeout to five minutes and disable snapshotting of
   mount points on all volumes:

   Restic backup --use-fs-snapshot -o vss.timeout=5m -o
   vss.exclude-all-mount-points=true

   Exclude drive `d:`, mount point `c:\mnt` and a specific volume from
   snapshotting:

   Restic backup --use-fs-snapshot -o
   vss.exclude-volumes="d:\;c:\mnt\;\\?\Volume{e2e0315d-9066-4f97-8343-eb5659b35762}"

   Uses 'Microsoft Software Shadow Copy provider 1.0' instead of the default
   provider:

   Restic backup --use-fs-snapshot -o
   vss.provider={b5946137-7b9f-4925-af80-51abd60b20d5}

   https://github.com/restic/restic/pull/3067

 * Enhancement #3406: Improve `dump` performance for large files

   The `dump` command now retrieves the data chunks for a file in parallel. This
   improves the download performance by up to as many times as the configured
   number of parallel backend connections.

   https://github.com/restic/restic/issues/3406
   https://github.com/restic/restic/pull/4796

 * Enhancement #3806: Optimize and make `prune` command resumable

   Previously, if the `prune` command was interrupted, a later `prune` run would
   start repacking pack files from the start, as `prune` did not update the index
   while repacking.

   The `prune` command now supports resuming interrupted prune runs. The update of
   the repository index has also been optimized to use less memory and only rewrite
   parts of the index that have changed.

   https://github.com/restic/restic/issues/3806
   https://github.com/restic/restic/pull/4812

 * Enhancement #4006: (alpha) Store deviceID only for hardlinks

   Set `RESTIC_FEATURES=device-id-for-hardlinks` to enable this alpha feature. The
   feature flag will be removed after repository format version 3 becomes available
   or be replaced with a different solution.

   When creating backups from a filesystem snapshot, for example created using
   BTRFS subvolumes, the deviceID of the filesystem changes compared to previous
   snapshots. This prevented restic from deduplicating the directory metadata of a
   snapshot.

   When this alpha feature is enabled, the deviceID is only stored for hardlinks,
   which significantly reduces the metadata duplication for most backups.

   https://github.com/restic/restic/pull/4006

 * Enhancement #4048: Add support for FUSE-T with `mount` on macOS

   The restic `mount` command now supports creating FUSE mounts using FUSE-T on
   macOS.

   https://github.com/restic/restic/issues/4048
   https://github.com/restic/restic/pull/4825

 * Enhancement #4251: Support reading backup from a command's standard output

   The `backup` command now supports the `--stdin-from-command` option. When using
   this option, the arguments to `backup` are interpreted as a command instead of
   paths to back up. `backup` then executes the given command and stores the
   standard output from it in the backup, similar to the what the `--stdin` option
   does. This also enables restic to verify that the command completes with exit
   code zero. A non-zero exit code causes the backup to fail.

   Note that the `--stdin` option does not have to be specified at the same time,
   and that the `--stdin-filename` option also applies to `--stdin-from-command`.

   Example: `restic backup --stdin-from-command --stdin-filename dump.sql mysqldump
   [...]`

   https://github.com/restic/restic/issues/4251
   https://github.com/restic/restic/pull/4410

 * Enhancement #4287: Support connection to rest-server using unix socket

   Restic now supports using a unix socket to connect to a rest-server version
   0.13.0 or later. This allows running restic as follows:

   ```
   rest-server --listen unix:/tmp/rest.socket --data /path/to/data &
   restic -r rest:http+unix:///tmp/rest.socket:/my_backup_repo/ [...]
   ```

   https://github.com/restic/restic/issues/4287
   https://github.com/restic/restic/pull/4655

 * Enhancement #4354: Significantly reduce `prune` memory usage

   The `prune` command has been optimized to use up to 60% less memory. The memory
   usage should now be roughly similar to creating a backup.

   https://github.com/restic/restic/pull/4354
   https://github.com/restic/restic/pull/4812

 * Enhancement #4437: Make `check` command create non-existent cache directory

   Previously, if a custom cache directory was specified for the `check` command,
   but the directory did not exist, `check` continued with the cache disabled.

   The `check` command now attempts to create the cache directory before
   initializing the cache.

   https://github.com/restic/restic/issues/4437
   https://github.com/restic/restic/pull/4805
   https://github.com/restic/restic/pull/4883

 * Enhancement #4472: Support AWS Assume Role for S3 backend

   Previously only credentials discovered via the Minio discovery methods were used
   to authenticate.

   However, there are many circumstances where the discovered credentials have
   lower permissions and need to assume a specific role. This is now possible using
   the following new environment variables:

   - RESTIC_AWS_ASSUME_ROLE_ARN - RESTIC_AWS_ASSUME_ROLE_SESSION_NAME -
   RESTIC_AWS_ASSUME_ROLE_EXTERNAL_ID - RESTIC_AWS_ASSUME_ROLE_REGION (defaults to
   us-east-1) - RESTIC_AWS_ASSUME_ROLE_POLICY - RESTIC_AWS_ASSUME_ROLE_STS_ENDPOINT

   https://github.com/restic/restic/issues/4472
   https://github.com/restic/restic/pull/4474

 * Enhancement #4547: Add `--json` option to `version` command

   Restic now supports outputting restic version along with the Go version, OS and
   architecture used to build restic in JSON format using `version --json`.

   https://github.com/restic/restic/issues/4547
   https://github.com/restic/restic/pull/4553

 * Enhancement #4549: Add `--ncdu` option to `ls` command

   NCDU (NCurses Disk Usage) is a tool to analyse disk usage of directories. It has
   an option to save a directory tree and analyse it later.

   The `ls` command now supports outputting snapshot information in the NCDU format
   using the `--ncdu` option. Example usage: `restic ls latest --ncdu | ncdu -f -`

   https://github.com/restic/restic/issues/4549
   https://github.com/restic/restic/pull/4550
   https://github.com/restic/restic/pull/4911

 * Enhancement #4573: Support rewriting host and time metadata in snapshots

   The `rewrite` command now supports rewriting the host and/or time metadata of a
   snapshot using the new `--new-host` and `--new-time` options.

   https://github.com/restic/restic/pull/4573

 * Enhancement #4583: Ignore `s3.storage-class` archive tiers for metadata

   Restic used to store all files on S3 using the specified `s3.storage-class`.

   Now, restic will only use non-archive storage tiers for metadata, to avoid
   problems when accessing a repository. To restore any data, it is still necessary
   to manually warm up the required data beforehand.

   NOTE: There is no official cold storage support in restic, use this option at
   your own risk.

   https://github.com/restic/restic/issues/4583
   https://github.com/restic/restic/pull/4584

 * Enhancement #4590: Speed up `mount` command's error detection

   The `mount` command now checks for the existence of the mountpoint before
   opening the repository, leading to quicker error detection.

   https://github.com/restic/restic/pull/4590

 * Enhancement #4601: Add support for feature flags

   Restic now supports feature flags that can be used to enable and disable
   experimental features. The flags can be set using the environment variable
   `RESTIC_FEATURES`. To get a list of currently supported feature flags, use the
   `features` command.

   https://github.com/restic/restic/issues/4601
   https://github.com/restic/restic/pull/4666

 * Enhancement #4611: Back up more file metadata on Windows

   Previously, restic did not back up all common Windows-specific metadata.

   Restic now stores file creation time and file attributes like the hidden,
   read-only and encrypted flags when backing up files and folders on Windows.

   https://github.com/restic/restic/pull/4611

 * Enhancement #4664: Make `ls` use `message_type` field in JSON output

   The `ls` command was the only restic command that used the `struct_type` field
   in its JSON output format to specify the message type.

   The JSON output of the `ls` command now also includes the `message_type` field,
   which is consistent with other commands. The `struct_type` field is still
   included, but now deprecated.

   https://github.com/restic/restic/pull/4664

 * Enhancement #4676: Make `key` command's actions separate sub-commands

   Each of the `add`, `list`, `remove` and `passwd` actions provided by the `key`
   command is now a separate sub-command and have its own documentation which can
   be invoked using `restic key <add|list|remove|passwd> --help`.

   https://github.com/restic/restic/issues/4676
   https://github.com/restic/restic/pull/4685

 * Enhancement #4678: Add `--target` option to the `dump` command

   Restic `dump` always printed to the standard output. It now supports specifying
   a `--target` file to write its output to.

   https://github.com/restic/restic/issues/4678
   https://github.com/restic/restic/pull/4682
   https://github.com/restic/restic/pull/4692

 * Enhancement #4708: Back up and restore SecurityDescriptors on Windows

   Restic now backs up and restores SecurityDescriptors for files and folders on
   Windows which includes owner, group, discretionary access control list (DACL)
   and system access control list (SACL).

   This requires the user to be a member of backup operators or the application
   must be run as admin. If that is not the case, only the current user's owner,
   group and DACL will be backed up, and during restore only the DACL of the backed
   up file will be restored, with the current user's owner and group being set on
   the restored file.

   https://github.com/restic/restic/pull/4708

 * Enhancement #4733: Allow specifying `--host` via environment variable

   Restic commands that operate on snapshots, such as `restic backup` and `restic
   snapshots`, support the `--host` option to specify the hostname for grouping
   snapshots.

   Such commands now also support specifying the hostname via the environment
   variable `RESTIC_HOST`. Note that `--host` still takes precedence over the
   environment variable.

   https://github.com/restic/restic/issues/4733
   https://github.com/restic/restic/pull/4734

 * Enhancement #4737: Include snapshot ID in `reason` field of `forget` JSON output

   The JSON output of the `forget` command now includes `id` and `short_id` of
   snapshots in the `reason` field.

   https://github.com/restic/restic/pull/4737

 * Enhancement #4764: Support forgetting all snapshots

   The `forget` command now supports the `--unsafe-allow-remove-all` option, which
   removes all snapshots in the repository.

   This option must always be combined with a snapshot filter (by host, path or
   tag). For example, the command `forget --tag example --unsafe-allow-remove-all`
   removes all snapshots with the tag "example".

   https://github.com/restic/restic/pull/4764

 * Enhancement #4768: Allow specifying custom User-Agent for outgoing requests

   Restic now supports setting a custom `User-Agent` for outgoing HTTP requests
   using the global option `--http-user-agent` or the `RESTIC_HTTP_USER_AGENT`
   environment variable.

   https://github.com/restic/restic/issues/4768
   https://github.com/restic/restic/pull/4810

 * Enhancement #4781: Add `restore` options to read include/exclude patterns from files

   Restic now supports reading include and exclude patterns from files using the
   `--include-file`, `--exclude-file`, `--iinclude-file` and `--iexclude-file`
   options of the `restore` command.

   https://github.com/restic/restic/issues/4781
   https://github.com/restic/restic/pull/4811

 * Enhancement #4807: Support Extended Attributes on Windows NTFS

   Restic now backs up and restores Extended Attributes for files and folders on
   Windows NTFS.

   https://github.com/restic/restic/pull/4807

 * Enhancement #4817: Make overwrite behavior of `restore` customizable

   The `restore` command now supports an `--overwrite` option to configure whether
   already existing files are overwritten. The overwrite behavior can be configured
   using the following option values:

   - `--overwrite always` (default): Always overwrites already existing files. The
   `restore` command will verify the existing file content and only restore
   mismatching parts to minimize downloads. Updates the metadata of all files. -
   `--overwrite if-changed`: Like `always`, but speeds up the file content check by
   assuming that files with matching size and modification time (mtime) are already
   up to date. In case of a mismatch, the full file content is verified like with
   `always`. Updates the metadata of all files. - `--overwrite if-newer`: Like
   `always`, but only overwrites existing files when the file in the snapshot has a
   newer modification time (mtime) than the existing file. - `--overwrite never`:
   Never overwrites existing files.

   https://github.com/restic/restic/issues/4817
   https://github.com/restic/restic/issues/200
   https://github.com/restic/restic/issues/407
   https://github.com/restic/restic/issues/2662
   https://github.com/restic/restic/pull/4837
   https://github.com/restic/restic/pull/4838
   https://github.com/restic/restic/pull/4864
   https://github.com/restic/restic/pull/4921

 * Enhancement #4839: Add dry-run support to `restore` command

   The `restore` command now supports the `--dry-run` option to perform a dry run.
   Pass the `--verbose=2` option to see which files would remain unchanged, and
   which would be updated or freshly restored.

   https://github.com/restic/restic/pull/4839


# Changelog for restic 0.16.5 (2024-07-01)
The following sections list the changes in restic 0.16.5 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Enh #4799: Add option to force use of Azure CLI credential
 * Enh #4873: Update dependencies

## Details

 * Enhancement #4799: Add option to force use of Azure CLI credential

   A new environment variable `AZURE_FORCE_CLI_CREDENTIAL=true` allows forcing the
   use of Azure CLI credential, ignoring other credentials like managed identity.

   https://github.com/restic/restic/pull/4799

 * Enhancement #4873: Update dependencies

   A few potentially vulnerable dependencies were updated.

   https://github.com/restic/restic/issues/4873
   https://github.com/restic/restic/pull/4878


# Changelog for restic 0.16.4 (2024-02-04)
The following sections list the changes in restic 0.16.4 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #4677: Downgrade zstd library to fix rare data corruption at max. compression
 * Enh #4529: Add extra verification of data integrity before upload

## Details

 * Bugfix #4677: Downgrade zstd library to fix rare data corruption at max. compression

   In restic 0.16.3, backups where the compression level was set to `max` (using
   `--compression max`) could in rare and very specific circumstances result in
   data corruption due to a bug in the library used for compressing data. Restic
   0.16.1 and 0.16.2 were not affected.

   Restic now uses the previous version of the library used to compress data, the
   same version used by restic 0.16.2. Please note that the `auto` compression
   level (which restic uses by default) was never affected, and even if you used
   `max` compression, chances of being affected by this issue are small.

   To check a repository for any corruption, run `restic check --read-data`. This
   will download and verify the whole repository and can be used at any time to
   completely verify the integrity of a repository. If the `check` command detects
   anomalies, follow the suggested steps.

   https://github.com/restic/restic/issues/4677
   https://github.com/restic/restic/pull/4679

 * Enhancement #4529: Add extra verification of data integrity before upload

   Hardware issues, or a bug in restic or its dependencies, could previously cause
   corruption in the files restic created and stored in the repository. Detecting
   such corruption previously required explicitly running the `check --read-data`
   or `check --read-data-subset` commands.

   To further ensure data integrity, even in the case of hardware issues or
   software bugs, restic now performs additional verification of the files about to
   be uploaded to the repository.

   These extra checks will increase CPU usage during backups. They can therefore,
   if absolutely necessary, be disabled using the `--no-extra-verify` global
   option. Please note that this should be combined with more active checking using
   the previously mentioned check commands.

   https://github.com/restic/restic/issues/4529
   https://github.com/restic/restic/pull/4681


# Changelog for restic 0.16.3 (2024-01-14)
The following sections list the changes in restic 0.16.3 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #4560: Improve errors for irregular files on Windows
 * Fix #4574: Support backup of deduplicated files on Windows again
 * Fix #4612: Improve error handling for `rclone` backend
 * Fix #4624: Correct `restore` progress information if an error occurs
 * Fix #4626: Improve reliability of restoring large files

## Details

 * Bugfix #4560: Improve errors for irregular files on Windows

   Since Go 1.21, most filesystem reparse points on Windows are considered to be
   irregular files. This caused restic to show an `error: invalid node type ""`
   error message for those files.

   This error message has now been improved and includes the relevant file path:
   `error: nodeFromFileInfo path/to/file: unsupported file type "irregular"`. As
   irregular files are not required to behave like regular files, it is not
   possible to provide a generic way to back up those files.

   https://github.com/restic/restic/issues/4560
   https://github.com/restic/restic/pull/4620
   https://forum.restic.net/t/windows-backup-error-invalid-node-type/6875

 * Bugfix #4574: Support backup of deduplicated files on Windows again

   With the official release builds of restic 0.16.1 and 0.16.2, it was not
   possible to back up files that were deduplicated by the corresponding Windows
   Server feature. This also applied to restic versions built using Go
   1.21.0-1.21.4.

   The Go version used to build restic has now been updated to fix this.

   https://github.com/restic/restic/issues/4574
   https://github.com/restic/restic/pull/4621

 * Bugfix #4612: Improve error handling for `rclone` backend

   Since restic 0.16.0, if rclone encountered an error while listing files, this
   could in rare circumstances cause restic to assume that there are no files.
   Although unlikely, this situation could result in data loss if it were to happen
   right when the `prune` command is listing existing snapshots.

   Error handling has now been improved to detect and work around this case.

   https://github.com/restic/restic/issues/4612
   https://github.com/restic/restic/pull/4618

 * Bugfix #4624: Correct `restore` progress information if an error occurs

   If an error occurred while restoring a snapshot, this could cause the `restore`
   progress bar to show incorrect information. In addition, if a data file could
   not be loaded completely, then errors would also be reported for some already
   restored files.

   Error reporting of the `restore` command has now been made more accurate.

   https://github.com/restic/restic/pull/4624
   https://forum.restic.net/t/errors-restoring-with-restic-on-windows-server-s3/6943

 * Bugfix #4626: Improve reliability of restoring large files

   In some cases restic failed to restore large files that frequently contain the
   same file chunk. In combination with certain backends, this could result in
   network connection timeouts that caused incomplete restores.

   Restic now includes special handling for such file chunks to ensure reliable
   restores.

   https://github.com/restic/restic/pull/4626
   https://forum.restic.net/t/errors-restoring-with-restic-on-windows-server-s3/6943


# Changelog for restic 0.16.2 (2023-10-29)
The following sections list the changes in restic 0.16.2 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #4540: Restore ARMv5 support for ARM binaries
 * Fix #4545: Repair documentation build on Read the Docs

## Details

 * Bugfix #4540: Restore ARMv5 support for ARM binaries

   The official release binaries for restic 0.16.1 were accidentally built to
   require ARMv7. The build process is now updated to restore support for ARMv5.

   Please note that restic 0.17.0 will drop support for ARMv5 and require at least
   ARMv6.

   https://github.com/restic/restic/issues/4540

 * Bugfix #4545: Repair documentation build on Read the Docs

   For restic 0.16.1, no documentation was available at
   https://restic.readthedocs.io/ .

   The documentation build process is now updated to work again.

   https://github.com/restic/restic/pull/4545


# Changelog for restic 0.16.1 (2023-10-24)
The following sections list the changes in restic 0.16.1 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #4513: Make `key list` command honor `--no-lock`
 * Fix #4516: Do not try to load password on command line autocomplete
 * Fix #4523: Update zstd library to fix possible data corruption at max. compression
 * Chg #4532: Update dependencies and require Go 1.19 or newer
 * Enh #229: Show progress bar while loading the index
 * Enh #4128: Automatically set `GOMAXPROCS` in resource-constrained containers
 * Enh #4480: Allow setting REST password and username via environment variables
 * Enh #4511: Include inode numbers in JSON output for `find` and `ls` commands
 * Enh #4519: Add config option to set SFTP command arguments

## Details

 * Bugfix #4513: Make `key list` command honor `--no-lock`

   The `key list` command now supports the `--no-lock` options. This allows
   determining which keys a repo can be accessed by without the need for having
   write access (e.g., read-only sftp access, filesystem snapshot).

   https://github.com/restic/restic/issues/4513
   https://github.com/restic/restic/pull/4514

 * Bugfix #4516: Do not try to load password on command line autocomplete

   The command line autocompletion previously tried to load the repository
   password. This could cause the autocompletion not to work. Now, this step gets
   skipped.

   https://github.com/restic/restic/issues/4516
   https://github.com/restic/restic/pull/4526

 * Bugfix #4523: Update zstd library to fix possible data corruption at max. compression

   In restic 0.16.0, backups where the compression level was set to `max` (using
   `--compression max`) could in rare and very specific circumstances result in
   data corruption due to a bug in the library used for compressing data.

   Restic now uses the latest version of the library used to compress data, which
   includes a fix for this issue. Please note that the `auto` compression level
   (which restic uses by default) was never affected, and even if you used `max`
   compression, chances of being affected by this issue were very small.

   To check a repository for any corruption, run `restic check --read-data`. This
   will download and verify the whole repository and can be used at any time to
   completely verify the integrity of a repository. If the `check` command detects
   anomalies, follow the suggested steps.

   To simplify any needed repository repair and minimize data loss, there is also a
   new and experimental `repair packs` command that salvages all valid data from
   the affected pack files (see `restic help repair packs` for more information).

   https://github.com/restic/restic/issues/4523
   https://github.com/restic/restic/pull/4530

 * Change #4532: Update dependencies and require Go 1.19 or newer

   We have updated all dependencies. Since some libraries require newer Go standard
   library features, support for Go 1.18 has been dropped, which means that restic
   now requires at least Go 1.19 to build.

   https://github.com/restic/restic/pull/4532
   https://github.com/restic/restic/pull/4533

 * Enhancement #229: Show progress bar while loading the index

   Restic did not provide any feedback while loading index files. Now, there is a
   progress bar that shows the index loading progress.

   https://github.com/restic/restic/issues/229
   https://github.com/restic/restic/pull/4419

 * Enhancement #4128: Automatically set `GOMAXPROCS` in resource-constrained containers

   When running restic in a Linux container with CPU-usage limits, restic now
   automatically adjusts `GOMAXPROCS`. This helps to reduce the memory consumption
   on hosts with many CPU cores.

   https://github.com/restic/restic/issues/4128
   https://github.com/restic/restic/pull/4485
   https://github.com/restic/restic/pull/4531

 * Enhancement #4480: Allow setting REST password and username via environment variables

   Previously, it was only possible to specify the REST-server username and
   password in the repository URL, or by using the `--repository-file` option. This
   meant it was not possible to use authentication in contexts where the repository
   URL is stored in publicly accessible way.

   Restic now allows setting the username and password using the
   `RESTIC_REST_USERNAME` and `RESTIC_REST_PASSWORD` variables.

   https://github.com/restic/restic/pull/4480

 * Enhancement #4511: Include inode numbers in JSON output for `find` and `ls` commands

   Restic used to omit the inode numbers in the JSON messages emitted for nodes by
   the `ls` command as well as for matches by the `find` command. It now includes
   those values whenever they are available.

   https://github.com/restic/restic/pull/4511

 * Enhancement #4519: Add config option to set SFTP command arguments

   When using the `sftp` backend, scenarios where a custom identity file was needed
   for the SSH connection, required the full command to be specified: `-o
   sftp.command='ssh user@host:port -i /ssh/my_private_key -s sftp'`

   Now, the `-o sftp.args=...` option can be passed to restic to specify custom
   arguments for the SSH command executed by the SFTP backend. This simplifies the
   above example to `-o sftp.args='-i /ssh/my_private_key'`.

   https://github.com/restic/restic/issues/4241
   https://github.com/restic/restic/pull/4519


# Changelog for restic 0.16.0 (2023-07-31)
The following sections list the changes in restic 0.16.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #2565: Support "unlimited" in `forget --keep-*` options
 * Fix #3311: Support non-UTF8 paths as symlink target
 * Fix #4199: Avoid lock refresh issues on slow network connections
 * Fix #4274: Improve lock refresh handling after standby
 * Fix #4319: Correctly clean up status bar output of the `backup` command
 * Fix #4333: `generate` and `init` no longer silently ignore unexpected arguments
 * Fix #4400: Ignore missing folders in `rest` backend
 * Chg #4176: Fix JSON message type of `scan_finished` for the `backup` command
 * Chg #4201: Require Go 1.20 for Solaris builds
 * Enh #426: Show progress bar during restore
 * Enh #719: Add `--retry-lock` option
 * Enh #1495: Sort snapshots by timestamp in `restic find`
 * Enh #1759: Add `repair index` and `repair snapshots` commands
 * Enh #1926: Allow certificate paths to be passed through environment variables
 * Enh #2359: Provide multi-platform Docker images
 * Enh #2468: Add support for non-global Azure clouds
 * Enh #2679: Reduce file fragmentation for local backend
 * Enh #3328: Reduce memory usage by up to 25%
 * Enh #3397: Improve accuracy of ETA displayed during backup
 * Enh #3624: Keep oldest snapshot when there are not enough snapshots
 * Enh #3698: Add support for Managed / Workload Identity to `azure` backend
 * Enh #3871: Support `<snapshot>:<subfolder>` syntax to select subfolders
 * Enh #3941: Support `--group-by` for backup parent selection
 * Enh #4130: Cancel current command if cache becomes unusable
 * Enh #4159: Add `--human-readable` option to `ls` and `find` commands
 * Enh #4188: Include restic version in snapshot metadata
 * Enh #4220: Add `jq` binary to Docker image
 * Enh #4226: Allow specifying region of new buckets in the `gs` backend
 * Enh #4375: Add support for extended attributes on symlinks

## Details

 * Bugfix #2565: Support "unlimited" in `forget --keep-*` options

   Restic would previously forget snapshots that should have been kept when a
   negative value was passed to the `--keep-*` options. Negative values are now
   forbidden. To keep all snapshots, the special value `unlimited` is now
   supported. For example, `--keep-monthly unlimited` will keep all monthly
   snapshots.

   https://github.com/restic/restic/issues/2565
   https://github.com/restic/restic/pull/4234

 * Bugfix #3311: Support non-UTF8 paths as symlink target

   Earlier restic versions did not correctly `backup` and `restore` symlinks that
   contain a non-UTF8 target. Note that this only affected systems that still use a
   non-Unicode encoding for filesystem paths.

   The repository format is now extended to add support for such symlinks. Please
   note that snapshots must have been created with at least restic version 0.16.0
   for `restore` to correctly handle non-UTF8 symlink targets when restoring them.

   https://github.com/restic/restic/issues/3311
   https://github.com/restic/restic/pull/3802

 * Bugfix #4199: Avoid lock refresh issues on slow network connections

   On network connections with a low upload speed, backups and other operations
   could fail with the error message `Fatal: failed to refresh lock in time`.

   This has now been fixed by reworking the lock refresh handling.

   https://github.com/restic/restic/issues/4199
   https://github.com/restic/restic/pull/4304

 * Bugfix #4274: Improve lock refresh handling after standby

   If the restic process was stopped or the host running restic entered standby
   during a long running operation such as a backup, this previously resulted in
   the operation failing with `Fatal: failed to refresh lock in time`.

   This has now been fixed such that restic first checks whether it is safe to
   continue the current operation and only throws an error if not.

   https://github.com/restic/restic/issues/4274
   https://github.com/restic/restic/pull/4374

 * Bugfix #4319: Correctly clean up status bar output of the `backup` command

   Due to a regression in restic 0.15.2, the status bar of the `backup` command
   could leave some output behind. This happened if filenames were printed that are
   wider than the current terminal width. This has now been fixed.

   https://github.com/restic/restic/issues/4319
   https://github.com/restic/restic/pull/4318

 * Bugfix #4333: `generate` and `init` no longer silently ignore unexpected arguments

   https://github.com/restic/restic/pull/4333

 * Bugfix #4400: Ignore missing folders in `rest` backend

   If a repository accessed via the REST backend was missing folders, then restic
   would fail with an error while trying to list the data in the repository. This
   has been now fixed.

   https://github.com/restic/rest-server/issues/235
   https://github.com/restic/restic/pull/4400

 * Change #4176: Fix JSON message type of `scan_finished` for the `backup` command

   Restic incorrectly set the `message_type` of the `scan_finished` message to
   `status` instead of `verbose_status`. This has now been corrected so that the
   messages report the correct type.

   https://github.com/restic/restic/pull/4176

 * Change #4201: Require Go 1.20 for Solaris builds

   Building restic on Solaris now requires Go 1.20, as the library used to access
   Azure uses the mmap syscall, which is only available on Solaris starting from Go
   1.20. All other platforms however continue to build with Go 1.18.

   https://github.com/restic/restic/pull/4201

 * Enhancement #426: Show progress bar during restore

   The `restore` command now shows a progress report while restoring files.

   Example: `[0:42] 5.76% 23 files 12.98 MiB, total 3456 files 23.54 GiB`

   JSON output is now also supported.

   https://github.com/restic/restic/issues/426
   https://github.com/restic/restic/issues/3413
   https://github.com/restic/restic/issues/3627
   https://github.com/restic/restic/pull/3991
   https://github.com/restic/restic/pull/4314
   https://forum.restic.net/t/progress-bar-for-restore/5210

 * Enhancement #719: Add `--retry-lock` option

   This option allows specifying a duration for which restic will wait if the
   repository is already locked.

   https://github.com/restic/restic/issues/719
   https://github.com/restic/restic/pull/2214
   https://github.com/restic/restic/pull/4107

 * Enhancement #1495: Sort snapshots by timestamp in `restic find`

   The `find` command used to print snapshots in an arbitrary order. Restic now
   prints snapshots sorted by timestamp.

   https://github.com/restic/restic/issues/1495
   https://github.com/restic/restic/pull/4409

 * Enhancement #1759: Add `repair index` and `repair snapshots` commands

   The `rebuild-index` command has been renamed to `repair index`. The old name
   will still work, but is deprecated.

   When a snapshot was damaged, the only option up to now was to completely forget
   the snapshot, even if only some unimportant files in it were damaged and other
   files were still fine.

   Restic now has a `repair snapshots` command, which can salvage any non-damaged
   files and parts of files in the snapshots by removing damaged directories and
   missing file contents. Please note that the damaged data may still be lost and
   see the "Troubleshooting" section in the documentation for more details.

   https://github.com/restic/restic/issues/1759
   https://github.com/restic/restic/issues/1714
   https://github.com/restic/restic/issues/1798
   https://github.com/restic/restic/issues/2334
   https://github.com/restic/restic/pull/2876
   https://forum.restic.net/t/corrupted-repo-how-to-repair/799
   https://forum.restic.net/t/recovery-options-for-damaged-repositories/1571

 * Enhancement #1926: Allow certificate paths to be passed through environment variables

   Restic will now read paths to certificates from the environment variables
   `RESTIC_CACERT` or `RESTIC_TLS_CLIENT_CERT` if `--cacert` or `--tls-client-cert`
   are not specified.

   https://github.com/restic/restic/issues/1926
   https://github.com/restic/restic/pull/4384

 * Enhancement #2359: Provide multi-platform Docker images

   The official Docker images are now built for the architectures linux/386,
   linux/amd64, linux/arm and linux/arm64.

   As an alternative to the Docker Hub, the Docker images are also available on
   ghcr.io, the GitHub Container Registry.

   https://github.com/restic/restic/issues/2359
   https://github.com/restic/restic/issues/4269
   https://github.com/restic/restic/pull/4364

 * Enhancement #2468: Add support for non-global Azure clouds

   The `azure` backend previously only supported storages using the global domain
   `core.windows.net`. This meant that backups to other domains such as Azure China
   (`core.chinacloudapi.cn`) or Azure Germany (`core.cloudapi.de`) were not
   supported. Restic now allows overriding the global domain using the environment
   variable `AZURE_ENDPOINT_SUFFIX`.

   https://github.com/restic/restic/issues/2468
   https://github.com/restic/restic/pull/4387

 * Enhancement #2679: Reduce file fragmentation for local backend

   Before this change, local backend files could become fragmented. Now restic will
   try to preallocate space for pack files to avoid their fragmentation.

   https://github.com/restic/restic/issues/2679
   https://github.com/restic/restic/pull/3261

 * Enhancement #3328: Reduce memory usage by up to 25%

   The in-memory index has been optimized to be more garbage collection friendly.
   Restic now defaults to `GOGC=50` to run the Go garbage collector more
   frequently.

   https://github.com/restic/restic/issues/3328
   https://github.com/restic/restic/pull/4352
   https://github.com/restic/restic/pull/4353

 * Enhancement #3397: Improve accuracy of ETA displayed during backup

   Restic's `backup` command displayed an ETA that did not adapt when the rate of
   progress made during the backup changed during the course of the backup.

   Restic now uses recent progress when computing the ETA. It is important to
   realize that the estimate may still be wrong, because restic cannot predict the
   future, but the hope is that the ETA will be more accurate in most cases.

   https://github.com/restic/restic/issues/3397
   https://github.com/restic/restic/pull/3563

 * Enhancement #3624: Keep oldest snapshot when there are not enough snapshots

   The `forget` command now additionally preserves the oldest snapshot if fewer
   snapshots than allowed by the `--keep-*` parameters would otherwise be kept.
   This maximizes the amount of history kept within the specified limits.

   https://github.com/restic/restic/issues/3624
   https://github.com/restic/restic/pull/4366
   https://forum.restic.net/t/keeping-yearly-snapshots-policy-when-backup-began-during-the-year/4670/2

 * Enhancement #3698: Add support for Managed / Workload Identity to `azure` backend

   Restic now additionally supports authenticating to Azure using Workload Identity
   or Managed Identity credentials, which are automatically injected in several
   environments such as a managed Kubernetes cluster.

   https://github.com/restic/restic/issues/3698
   https://github.com/restic/restic/pull/4029

 * Enhancement #3871: Support `<snapshot>:<subfolder>` syntax to select subfolders

   Commands like `diff` or `restore` always worked with the full snapshot. This did
   not allow comparing only a specific subfolder or only restoring that folder
   (`restore --include subfolder` filters the restored files, but still creates the
   directories included in `subfolder`).

   The commands `diff`, `dump`, `ls` and `restore` now support the
   `<snapshot>:<subfolder>` syntax, where `snapshot` is the ID of a snapshot (or
   the string `latest`) and `subfolder` is a path within the snapshot. The commands
   will then only work with the specified path of the snapshot. The `subfolder`
   must be a path to a folder as returned by `ls`. Two examples:

   `restic restore -t target latest:/some/path` `restic diff 12345678:/some/path
   90abcef:/some/path`

   For debugging purposes, the `cat` command now supports `cat tree
   <snapshot>:<subfolder>` to return the directory metadata for the given
   subfolder.

   https://github.com/restic/restic/issues/3871
   https://github.com/restic/restic/pull/4334

 * Enhancement #3941: Support `--group-by` for backup parent selection

   Previously, the `backup` command by default selected the parent snapshot based
   on the hostname and the backup paths. When the backup path list changed, the
   `backup` command was unable to determine a suitable parent snapshot and had to
   read all files again.

   The new `--group-by` option for the `backup` command allows filtering snapshots
   for the parent selection by `host`, `paths` and `tags`. It defaults to
   `host,paths` which selects the latest snapshot with hostname and paths matching
   those of the backup run. This matches the behavior of prior restic versions.

   The new `--group-by` option should be set to the same value as passed to `forget
   --group-by`.

   https://github.com/restic/restic/issues/3941
   https://github.com/restic/restic/pull/4081

 * Enhancement #4130: Cancel current command if cache becomes unusable

   If the cache directory was removed or ran out of space while restic was running,
   this would previously cause further caching attempts to fail and thereby
   drastically slow down the command execution. Now, the currently running command
   is instead canceled.

   https://github.com/restic/restic/issues/4130
   https://github.com/restic/restic/pull/4166

 * Enhancement #4159: Add `--human-readable` option to `ls` and `find` commands

   Previously, when using the `-l` option with the `ls` and `find` commands, the
   displayed size was always in bytes, without an option for a more human readable
   format such as MiB or GiB.

   The new `--human-readable` option will convert longer size values into more
   human friendly values with an appropriate suffix depending on the output size.
   For example, a size of `14680064` will be shown as `14.000 MiB`.

   https://github.com/restic/restic/issues/4159
   https://github.com/restic/restic/pull/4351

 * Enhancement #4188: Include restic version in snapshot metadata

   The restic version used to backup a snapshot is now included in its metadata and
   shown when inspecting a snapshot using `restic cat snapshot <snapshotID>` or
   `restic snapshots --json`.

   https://github.com/restic/restic/issues/4188
   https://github.com/restic/restic/pull/4378

 * Enhancement #4220: Add `jq` binary to Docker image

   The Docker image now contains `jq`, which can be useful to process JSON data
   output by restic.

   https://github.com/restic/restic/pull/4220

 * Enhancement #4226: Allow specifying region of new buckets in the `gs` backend

   Previously, buckets used by the Google Cloud Storage backend would always get
   created in the "us" region. It is now possible to specify the region where a
   bucket should be created by using the `-o gs.region=us` option.

   https://github.com/restic/restic/pull/4226

 * Enhancement #4375: Add support for extended attributes on symlinks

   Restic now supports extended attributes on symlinks when backing up, restoring,
   or FUSE-mounting snapshots. This includes, for example, the `security.selinux`
   xattr on Linux distributions that use SELinux.

   https://github.com/restic/restic/issues/4375
   https://github.com/restic/restic/pull/4379


# Changelog for restic 0.15.2 (2023-04-24)
The following sections list the changes in restic 0.15.2 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Sec #4275: Update golang.org/x/net to address CVE-2022-41723
 * Fix #2260: Sanitize filenames printed by `backup` during processing
 * Fix #4211: Make `dump` interpret `--host` and `--path` correctly
 * Fix #4239: Correct number of blocks reported in mount point
 * Fix #4253: Minimize risk of spurious filesystem loops with `mount`
 * Enh #4180: Add release binaries for riscv64 architecture on Linux
 * Enh #4219: Upgrade Minio to version 7.0.49

## Details

 * Security #4275: Update golang.org/x/net to address CVE-2022-41723

   https://github.com/restic/restic/issues/4275
   https://github.com/restic/restic/pull/4213

 * Bugfix #2260: Sanitize filenames printed by `backup` during processing

   The `backup` command would previously not sanitize the filenames it printed
   during processing, potentially causing newlines or terminal control characters
   to mangle the status output or even change the state of a terminal.

   Filenames are now checked and quoted if they contain non-printable or
   non-Unicode characters.

   https://github.com/restic/restic/issues/2260
   https://github.com/restic/restic/issues/4191
   https://github.com/restic/restic/pull/4192

 * Bugfix #4211: Make `dump` interpret `--host` and `--path` correctly

   A regression in restic 0.15.0 caused `dump` to confuse its `--host=<host>` and
   `--path=<path>` options: it looked for snapshots with paths called `<host>` from
   hosts called `<path>`. It now treats the options as intended.

   https://github.com/restic/restic/issues/4211
   https://github.com/restic/restic/pull/4212

 * Bugfix #4239: Correct number of blocks reported in mount point

   Restic mount points reported an incorrect number of 512-byte (POSIX standard)
   blocks for files and links due to a rounding bug. In particular, empty files
   were reported as taking one block instead of zero.

   The rounding is now fixed: the number of blocks reported is the file size (or
   link target size) divided by 512 and rounded up to a whole number.

   https://github.com/restic/restic/issues/4239
   https://github.com/restic/restic/pull/4240

 * Bugfix #4253: Minimize risk of spurious filesystem loops with `mount`

   When a backup contains a directory that has the same name as its parent, say
   `a/b/b`, and the GNU `find` command was run on this backup in a restic mount,
   `find` would refuse to traverse the lowest `b` directory, instead printing `File
   system loop detected`. This was due to the way the restic mount command
   generates inode numbers for directories in the mount point.

   The rule for generating these inode numbers was changed in 0.15.0. It has now
   been changed again to avoid this issue. A perfect rule does not exist, but the
   probability of this behavior occurring is now extremely small.

   When it does occur, the mount point is not broken, and scripts that traverse the
   mount point should work as long as they don't rely on inode numbers for
   detecting filesystem loops.

   https://github.com/restic/restic/issues/4253
   https://github.com/restic/restic/pull/4255

 * Enhancement #4180: Add release binaries for riscv64 architecture on Linux

   Builds for the `riscv64` architecture on Linux are now included in the release
   binaries.

   https://github.com/restic/restic/pull/4180

 * Enhancement #4219: Upgrade Minio to version 7.0.49

   The upgraded version now allows use of the `ap-southeast-4` region (Melbourne).

   https://github.com/restic/restic/pull/4219


# Changelog for restic 0.15.1 (2023-01-30)
The following sections list the changes in restic 0.15.1 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #3750: Remove `b2_download_file_by_name: 404` warning from B2 backend
 * Fix #4147: Make `prune --quiet` not print progress bar
 * Fix #4163: Make `self-update --output` work with new filename on Windows
 * Fix #4167: Add missing ETA in `backup` progress bar
 * Enh #4143: Ignore empty lock files

## Details

 * Bugfix #3750: Remove `b2_download_file_by_name: 404` warning from B2 backend

   In some cases the B2 backend could print `b2_download_file_by_name: 404: :
   b2.b2err` warnings. These are only debug messages and can be safely ignored.

   Restic now uses an updated library for accessing B2, which removes the warning.

   https://github.com/restic/restic/issues/3750
   https://github.com/restic/restic/issues/4144
   https://github.com/restic/restic/pull/4146

 * Bugfix #4147: Make `prune --quiet` not print progress bar

   A regression in restic 0.15.0 caused `prune --quiet` to show a progress bar
   while deciding how to process each pack files. This has now been fixed.

   https://github.com/restic/restic/issues/4147
   https://github.com/restic/restic/pull/4153

 * Bugfix #4163: Make `self-update --output` work with new filename on Windows

   Since restic 0.14.0 the `self-update` command did not work when a custom output
   filename was specified via the `--output` option. This has now been fixed.

   As a workaround, either use an older restic version to run the self-update or
   create an empty file with the output filename before updating e.g. using CMD:

   `type nul > new-file.exe` `restic self-update --output new-file.exe`

   https://github.com/restic/restic/pull/4163
   https://forum.restic.net/t/self-update-windows-started-failing-after-release-of-0-15/5836

 * Bugfix #4167: Add missing ETA in `backup` progress bar

   A regression in restic 0.15.0 caused the ETA to be missing from the progress bar
   displayed by the `backup` command. This has now been fixed.

   https://github.com/restic/restic/pull/4167

 * Enhancement #4143: Ignore empty lock files

   With restic 0.15.0 the checks for stale locks became much stricter than before.
   In particular, empty or unreadable locks were no longer silently ignored. This
   made restic to complain with `Load(<lock/1234567812>, 0, 0) returned error,
   retrying after 552.330144ms: load(<lock/1234567812>): invalid data returned` and
   fail in the end.

   The error message is now clarified and the implementation changed to ignore
   empty lock files which are sometimes created as the result of a failed uploads
   on some backends.

   Please note that unreadable lock files still have to cleaned up manually. To do
   so, you can run `restic unlock --remove-all` which removes all existing lock
   files. But first make sure that no other restic process is currently using the
   repository.

   https://github.com/restic/restic/issues/4143
   https://github.com/restic/restic/pull/4152


# Changelog for restic 0.15.0 (2023-01-12)
The following sections list the changes in restic 0.15.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #2015: Make `mount` return exit code 0 after receiving Ctrl-C / SIGINT
 * Fix #2578: Make `restore` replace existing symlinks
 * Fix #2591: Don't read password from stdin for `backup --stdin`
 * Fix #3161: Delete files on Backblaze B2 more reliably
 * Fix #3336: Make SFTP backend report no space left on device
 * Fix #3567: Improve handling of interrupted syscalls in `mount` command
 * Fix #3897: Fix stuck `copy` command when `-o <backend>.connections=1`
 * Fix #3918: Correct prune statistics for partially compressed repositories
 * Fix #3951: Make `ls` return exit code 1 if snapshot cannot be loaded
 * Fix #4003: Make `backup` no longer hang on Solaris when seeing a FIFO file
 * Fix #4016: Support ExFAT-formatted local backends on macOS Ventura
 * Fix #4085: Make `init` ignore "Access Denied" errors when creating S3 buckets
 * Fix #4100: Make `self-update` enabled by default only in release builds
 * Fix #4103: Don't generate negative UIDs and GIDs in tar files from `dump`
 * Chg #2724: Include full snapshot ID in JSON output of `backup`
 * Chg #3929: Make `unlock` display message only when locks were actually removed
 * Chg #4033: Don't print skipped snapshots by default in `copy` command
 * Chg #4041: Update dependencies and require Go 1.18 or newer
 * Enh #14: Implement `rewrite` command
 * Enh #79: Restore files with long runs of zeros as sparse files
 * Enh #1078: Support restoring symbolic links on Windows
 * Enh #1734: Inform about successful retries after errors
 * Enh #1866: Improve handling of directories with duplicate entries
 * Enh #2134: Support B2 API keys restricted to hiding but not deleting files
 * Enh #2152: Make `init` open only one connection for the SFTP backend
 * Enh #2533: Handle cache corruption on disk and in downloads
 * Enh #2715: Stricter repository lock handling
 * Enh #2750: Make backup file read concurrency configurable
 * Enh #3029: Add support for `credential_process` to S3 backend
 * Enh #3096: Make `mount` command support macOS using macFUSE 4.x
 * Enh #3124: Support JSON output for the `init` command
 * Enh #3899: Optimize prune memory usage
 * Enh #3905: Improve speed of parent snapshot detection in `backup` command
 * Enh #3915: Add compression statistics to the `stats` command
 * Enh #3925: Provide command completion for PowerShell
 * Enh #3931: Allow `backup` file tree scanner to be disabled
 * Enh #3932: Improve handling of ErrDot errors in rclone and sftp backends
 * Enh #3943: Ignore additional/unknown files in repository
 * Enh #3955: Improve `backup` performance for small files

## Details

 * Bugfix #2015: Make `mount` return exit code 0 after receiving Ctrl-C / SIGINT

   To stop the `mount` command, a user has to press Ctrl-C or send a SIGINT signal
   to restic. This used to cause restic to exit with a non-zero exit code.

   The exit code has now been changed to zero as the above is the expected way to
   stop the `mount` command and should therefore be considered successful.

   https://github.com/restic/restic/issues/2015
   https://github.com/restic/restic/pull/3894

 * Bugfix #2578: Make `restore` replace existing symlinks

   When restoring a symlink, restic used to report an error if the target path
   already existed. This has now been fixed such that the potentially existing
   target path is first removed before the symlink is restored.

   https://github.com/restic/restic/issues/2578
   https://github.com/restic/restic/pull/3780

 * Bugfix #2591: Don't read password from stdin for `backup --stdin`

   The `backup` command when used with `--stdin` previously tried to read first the
   password, then the data to be backed up from standard input. This meant it would
   often confuse part of the data for the password.

   From now on, it will instead exit with the message `Fatal: cannot read both
   password and data from stdin` unless the password is passed in some other way
   (such as `--restic-password-file`, `RESTIC_PASSWORD`, etc).

   To enter the password interactively a password command has to be used. For
   example on Linux, `mysqldump somedatabase | restic backup --stdin
   --password-command='sh -c "systemd-ask-password < /dev/tty"'` securely reads the
   password from the terminal.

   https://github.com/restic/restic/issues/2591
   https://github.com/restic/restic/pull/4011

 * Bugfix #3161: Delete files on Backblaze B2 more reliably

   Restic used to only delete the latest version of files stored in B2. In most
   cases this worked well as there was only a single version of the file. However,
   due to retries while uploading it is possible for multiple file versions to be
   stored at B2. This could lead to various problems for files that should have
   been deleted but still existed.

   The implementation has now been changed to delete all versions of files, which
   doubles the amount of Class B transactions necessary to delete files, but
   assures that no file versions are left behind.

   https://github.com/restic/restic/issues/3161
   https://github.com/restic/restic/pull/3885

 * Bugfix #3336: Make SFTP backend report no space left on device

   Backing up to an SFTP backend would spew repeated SSH_FX_FAILURE messages when
   the remote disk was full. Restic now reports "sftp: no space left on device" and
   exits immediately when it detects this condition.

   A fix for this issue was implemented in restic 0.12.1, but unfortunately the fix
   itself contained a bug that prevented it from taking effect.

   https://github.com/restic/restic/issues/3336
   https://github.com/restic/restic/pull/3345
   https://github.com/restic/restic/pull/4075

 * Bugfix #3567: Improve handling of interrupted syscalls in `mount` command

   Accessing restic's FUSE mount could result in "input/output" errors when using
   programs in which syscalls can be interrupted. This is for example the case for
   Go programs. This has now been fixed by improved error handling of interrupted
   syscalls.

   https://github.com/restic/restic/issues/3567
   https://github.com/restic/restic/issues/3694
   https://github.com/restic/restic/pull/3875

 * Bugfix #3897: Fix stuck `copy` command when `-o <backend>.connections=1`

   When running the `copy` command with `-o <backend>.connections=1` the command
   would be infinitely stuck. This has now been fixed.

   https://github.com/restic/restic/issues/3897
   https://github.com/restic/restic/pull/3898

 * Bugfix #3918: Correct prune statistics for partially compressed repositories

   In a partially compressed repository, one data blob can exist both in an
   uncompressed and a compressed version. This caused the `prune` statistics to
   become inaccurate and e.g. report a too high value for the unused size, such as
   "unused size after prune: 16777215.991 TiB". This has now been fixed.

   https://github.com/restic/restic/issues/3918
   https://github.com/restic/restic/pull/3980

 * Bugfix #3951: Make `ls` return exit code 1 if snapshot cannot be loaded

   The `ls` command used to show a warning and return exit code 0 when failing to
   load a snapshot. This has now been fixed such that it instead returns exit code
   1 (still showing a warning).

   https://github.com/restic/restic/pull/3951

 * Bugfix #4003: Make `backup` no longer hang on Solaris when seeing a FIFO file

   The `backup` command used to hang on Solaris whenever it encountered a FIFO file
   (named pipe), due to a bug in the handling of extended attributes. This bug has
   now been fixed.

   https://github.com/restic/restic/issues/4003
   https://github.com/restic/restic/pull/4053

 * Bugfix #4016: Support ExFAT-formatted local backends on macOS Ventura

   ExFAT-formatted disks could not be used as local backends starting from macOS
   Ventura. Restic commands would fail with an "inappropriate ioctl for device"
   error. This has now been fixed.

   https://github.com/restic/restic/issues/4016
   https://github.com/restic/restic/pull/4021

 * Bugfix #4085: Make `init` ignore "Access Denied" errors when creating S3 buckets

   In restic 0.9.0 through 0.13.0, the `init` command ignored some permission
   errors from S3 backends when trying to check for bucket existence, so that
   manually created buckets with custom permissions could be used for backups.

   This feature became broken in 0.14.0, but has now been restored again.

   https://github.com/restic/restic/issues/4085
   https://github.com/restic/restic/pull/4086

 * Bugfix #4100: Make `self-update` enabled by default only in release builds

   The `self-update` command was previously included by default in all builds of
   restic as opposed to only in official release builds, even if the `selfupdate`
   tag was not explicitly enabled when building.

   This has now been corrected, and the `self-update` command is only available if
   restic was built with `-tags selfupdate` (as done for official release builds by
   `build.go`).

   https://github.com/restic/restic/pull/4100

 * Bugfix #4103: Don't generate negative UIDs and GIDs in tar files from `dump`

   When using a 32-bit build of restic, the `dump` command could in some cases
   create tar files containing negative UIDs and GIDs, which cannot be read by GNU
   tar. This corner case especially applies to backups from stdin on Windows.

   This is now fixed such that `dump` creates valid tar files in these cases too.

   https://github.com/restic/restic/issues/4103
   https://github.com/restic/restic/pull/4104

 * Change #2724: Include full snapshot ID in JSON output of `backup`

   We have changed the JSON output of the backup command to include the full
   snapshot ID instead of just a shortened version, as the latter can be ambiguous
   in some rare cases. To derive the short ID, please truncate the full ID down to
   eight characters.

   https://github.com/restic/restic/issues/2724
   https://github.com/restic/restic/pull/3993

 * Change #3929: Make `unlock` display message only when locks were actually removed

   The `unlock` command used to print the "successfully removed locks" message
   whenever it was run, regardless of lock files having being removed or not.

   This has now been changed such that it only prints the message if any lock files
   were actually removed. In addition, it also reports the number of removed lock
   files.

   https://github.com/restic/restic/issues/3929
   https://github.com/restic/restic/pull/3935

 * Change #4033: Don't print skipped snapshots by default in `copy` command

   The `copy` command used to print each snapshot that was skipped because it
   already existed in the target repository. The amount of this output could
   practically bury the list of snapshots that were actually copied.

   From now on, the skipped snapshots are by default not printed at all, but this
   can be re-enabled by increasing the verbosity level of the command.

   https://github.com/restic/restic/issues/4033
   https://github.com/restic/restic/pull/4066

 * Change #4041: Update dependencies and require Go 1.18 or newer

   Most dependencies have been updated. Since some libraries require newer language
   features, support for Go 1.15-1.17 has been dropped, which means that restic now
   requires at least Go 1.18 to build.

   https://github.com/restic/restic/pull/4041

 * Enhancement #14: Implement `rewrite` command

   Restic now has a `rewrite` command which allows to rewrite existing snapshots to
   remove unwanted files.

   https://github.com/restic/restic/issues/14
   https://github.com/restic/restic/pull/2731
   https://github.com/restic/restic/pull/4079

 * Enhancement #79: Restore files with long runs of zeros as sparse files

   When using `restore --sparse`, the restorer may now write files containing long
   runs of zeros as sparse files (also called files with holes), where the zeros
   are not actually written to disk.

   How much space is saved by writing sparse files depends on the operating system,
   file system and the distribution of zeros in the file.

   During backup restic still reads the whole file including sparse regions, but
   with optimized processing speed of sparse regions.

   https://github.com/restic/restic/issues/79
   https://github.com/restic/restic/issues/3903
   https://github.com/restic/restic/pull/2601
   https://github.com/restic/restic/pull/3854
   https://forum.restic.net/t/sparse-file-support/1264

 * Enhancement #1078: Support restoring symbolic links on Windows

   The `restore` command now supports restoring symbolic links on Windows. Because
   of Windows specific restrictions this is only possible when running restic with
   the `SeCreateSymbolicLinkPrivilege` privilege or as an administrator.

   https://github.com/restic/restic/issues/1078
   https://github.com/restic/restic/issues/2699
   https://github.com/restic/restic/pull/2875

 * Enhancement #1734: Inform about successful retries after errors

   When a recoverable error is encountered, restic shows a warning message saying
   that it's retrying, e.g.:

   `Save(<data/956b9ced99>) returned error, retrying after 357.131936ms: ...`

   This message can be confusing in that it never clearly states whether the retry
   is successful or not. This has now been fixed such that restic follows up with a
   message confirming a successful retry, e.g.:

   `Save(<data/956b9ced99>) operation successful after 1 retries`

   https://github.com/restic/restic/issues/1734
   https://github.com/restic/restic/pull/2661

 * Enhancement #1866: Improve handling of directories with duplicate entries

   If for some reason a directory contains a duplicate entry, the `backup` command
   would previously fail with a `node "path/to/file" already present` or `nodes are
   not ordered got "path/to/file", last "path/to/file"` error.

   The error handling has been improved to only report a warning in this case. Make
   sure to check that the filesystem in question is not damaged if you see this!

   https://github.com/restic/restic/issues/1866
   https://github.com/restic/restic/issues/3937
   https://github.com/restic/restic/pull/3880

 * Enhancement #2134: Support B2 API keys restricted to hiding but not deleting files

   When the B2 backend does not have the necessary permissions to permanently
   delete files, it now automatically falls back to hiding files. This allows using
   restic with an application key which is not allowed to delete files. This can
   prevent an attacker from deleting backups with such an API key.

   To use this feature create an application key without the `deleteFiles`
   capability. It is recommended to restrict the key to just one bucket. For
   example using the `b2` command line tool:

   `b2 create-key --bucket <bucketName> <keyName>
   listBuckets,readFiles,writeFiles,listFiles`

   Alternatively, you can use the S3 backend to access B2, as described in the
   documentation. In this mode, files are also only hidden instead of being deleted
   permanently.

   https://github.com/restic/restic/issues/2134
   https://github.com/restic/restic/pull/2398

 * Enhancement #2152: Make `init` open only one connection for the SFTP backend

   The `init` command using the SFTP backend used to connect twice to the
   repository. This could be inconvenient if the user must enter a password, or
   cause `init` to fail if the server does not correctly close the first SFTP
   connection.

   This has now been fixed by reusing the first/initial SFTP connection opened.

   https://github.com/restic/restic/issues/2152
   https://github.com/restic/restic/pull/3882

 * Enhancement #2533: Handle cache corruption on disk and in downloads

   In rare situations, like for example after a system crash, the data stored in
   the cache might be corrupted. This could cause restic to fail and required
   manually deleting the cache.

   Restic now automatically removes broken data from the cache, allowing it to
   recover from such a situation without user intervention. In addition, restic
   retries downloads which return corrupt data in order to also handle temporary
   download problems.

   https://github.com/restic/restic/issues/2533
   https://github.com/restic/restic/pull/3521

 * Enhancement #2715: Stricter repository lock handling

   Previously, restic commands kept running even if they failed to refresh their
   locks in time. This could be a problem e.g. in case the client system running a
   backup entered the standby power mode while the backup was still in progress
   (which would prevent the client from refreshing its lock), and after a short
   delay another host successfully runs `unlock` and `prune` on the repository,
   which would remove all data added by the in-progress backup. If the backup
   client later continues its backup, even though its lock had expired in the
   meantime, this would lead to an incomplete snapshot.

   To address this, lock handling is now much stricter. Commands requiring a lock
   are canceled if the lock is not refreshed successfully in time. In addition, if
   a lock file is not readable restic will not allow starting a command. It may be
   necessary to remove invalid lock files manually or use `unlock --remove-all`.
   Please make sure that no other restic processes are running concurrently before
   doing this, however.

   https://github.com/restic/restic/issues/2715
   https://github.com/restic/restic/pull/3569

 * Enhancement #2750: Make backup file read concurrency configurable

   The `backup` command now supports a `--read-concurrency` option which allows
   tuning restic for very fast storage like NVMe disks by controlling the number of
   concurrent file reads during the backup process.

   https://github.com/restic/restic/pull/2750

 * Enhancement #3029: Add support for `credential_process` to S3 backend

   Restic now uses a newer library for the S3 backend, which adds support for the
   `credential_process` option in the AWS credential configuration.

   https://github.com/restic/restic/issues/3029
   https://github.com/restic/restic/issues/4034
   https://github.com/restic/restic/pull/4025

 * Enhancement #3096: Make `mount` command support macOS using macFUSE 4.x

   Restic now uses a different FUSE library for mounting snapshots and making them
   available as a FUSE filesystem using the `mount` command. This adds support for
   macFUSE 4.x which can be used to make this work on recent macOS versions.

   https://github.com/restic/restic/issues/3096
   https://github.com/restic/restic/pull/4024

 * Enhancement #3124: Support JSON output for the `init` command

   The `init` command used to ignore the `--json` option, but now outputs a JSON
   message if the repository was created successfully.

   https://github.com/restic/restic/issues/3124
   https://github.com/restic/restic/pull/3132

 * Enhancement #3899: Optimize prune memory usage

   The `prune` command needs large amounts of memory in order to determine what to
   keep and what to remove. This is now optimized to use up to 30% less memory.

   https://github.com/restic/restic/pull/3899

 * Enhancement #3905: Improve speed of parent snapshot detection in `backup` command

   Backing up a large number of files using `--files-from-verbatim` or
   `--files-from-raw` options could require a long time to find the parent
   snapshot. This has been improved.

   https://github.com/restic/restic/pull/3905

 * Enhancement #3915: Add compression statistics to the `stats` command

   When executed with `--mode raw-data` on a repository that supports compression,
   the `stats` command now calculates and displays, for the selected repository or
   snapshots: the uncompressed size of the data; the compression progress
   (percentage of data that has been compressed); the compression ratio of the
   compressed data; the total space saving.

   It also takes into account both the compressed and uncompressed data if the
   repository is only partially compressed.

   https://github.com/restic/restic/pull/3915

 * Enhancement #3925: Provide command completion for PowerShell

   Restic already provided generation of completion files for bash, fish and zsh.
   Now powershell is supported, too.

   https://github.com/restic/restic/pull/3925/files

 * Enhancement #3931: Allow `backup` file tree scanner to be disabled

   The `backup` command walks the file tree in a separate scanner process to find
   the total size and file/directory count, and uses this to provide an ETA. This
   can slow down backups, especially of network filesystems.

   The command now has a new option `--no-scan` which can be used to disable this
   scanning in order to speed up backups when needed.

   https://github.com/restic/restic/pull/3931

 * Enhancement #3932: Improve handling of ErrDot errors in rclone and sftp backends

   Since Go 1.19, restic can no longer implicitly run relative executables which
   are found in the current directory (e.g. `rclone` if found in `.`). This is a
   security feature of Go to prevent against running unintended and possibly
   harmful executables.

   The error message for this was just "cannot run executable found relative to
   current directory". This has now been improved to yield a more specific error
   message, informing the user how to explicitly allow running the executable using
   the `-o rclone.program` and `-o sftp.command` extended options with `./`.

   https://github.com/restic/restic/issues/3932
   https://pkg.go.dev/os/exec#hdr-Executables_in_the_current_directory
   https://go.dev/blog/path-security

 * Enhancement #3943: Ignore additional/unknown files in repository

   If a restic repository had additional files in it (not created by restic),
   commands like `find` and `restore` could become confused and fail with an
   `multiple IDs with prefix "12345678" found` error. These commands now ignore
   such additional files.

   https://github.com/restic/restic/pull/3943
   https://forum.restic.net/t/which-protocol-should-i-choose-for-remote-linux-backups/5446/17

 * Enhancement #3955: Improve `backup` performance for small files

   When backing up small files restic was slower than it could be. In particular
   this affected backups using maximum compression.

   This has been fixed by reworking the internal parallelism of the backup command,
   making it back up small files around two times faster.

   https://github.com/restic/restic/pull/3955


# Changelog for restic 0.14.0 (2022-08-25)
The following sections list the changes in restic 0.14.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #2248: Support `self-update` on Windows
 * Fix #3428: List snapshots in backend at most once to resolve snapshot IDs
 * Fix #3432: Fix rare 'not found in repository' error for `copy` command
 * Fix #3681: Fix rclone (shimmed by Scoop) and sftp not working on Windows
 * Fix #3685: The `diff` command incorrectly listed some files as added
 * Fix #3716: Print "wrong password" to stderr instead of stdout
 * Fix #3720: Directory sync errors for repositories accessed via SMB
 * Fix #3736: The `stats` command miscalculated restore size for multiple snapshots
 * Fix #3772: Correctly rebuild index for legacy repositories
 * Fix #3776: Limit number of key files tested while opening a repository
 * Fix #3861: Yield error on invalid policy to `forget`
 * Chg #1842: Support debug log creation in release builds
 * Chg #3295: Deprecate `check --check-unused` and add further checks
 * Chg #3680: Update dependencies and require Go 1.15 or newer
 * Chg #3742: Replace `--repo2` option used by `init`/`copy` with `--from-repo`
 * Enh #21: Add compression support
 * Enh #1153: Support pruning even when the disk is full
 * Enh #2162: Adaptive IO concurrency based on backend connections
 * Enh #2291: Allow pack size customization
 * Enh #2295: Allow use of SAS token to authenticate to Azure
 * Enh #2351: Use config file permissions to control file group access
 * Enh #2696: Improve backup speed with many small files
 * Enh #2907: Make snapshot directory structure of `mount` command customizable
 * Enh #2923: Improve speed of `copy` command
 * Enh #3114: Optimize handling of duplicate blobs in `prune`
 * Enh #3465: Improve handling of temporary files on Windows
 * Enh #3475: Allow limiting IO concurrency for local and SFTP backend
 * Enh #3484: Stream data in `check` and `prune` commands
 * Enh #3709: Validate exclude patterns before backing up
 * Enh #3729: Display full IDs in `check` warnings
 * Enh #3773: Optimize memory usage for directories with many files
 * Enh #3819: Validate include/exclude patterns before restoring
 * Enh #3837: Improve SFTP repository initialization over slow links

## Details

 * Bugfix #2248: Support `self-update` on Windows

   Restic `self-update` would fail in situations where the operating system locks
   running binaries, including Windows. The new behavior works around this by
   renaming the running file and swapping the updated file in place.

   https://github.com/restic/restic/issues/2248
   https://github.com/restic/restic/pull/3675

 * Bugfix #3428: List snapshots in backend at most once to resolve snapshot IDs

   Many commands support specifying a list of snapshot IDs which are then used to
   determine the snapshots to be processed by the command. To resolve snapshot IDs
   or `latest`, and check that these exist, restic previously listed all snapshots
   stored in the repository. Depending on the backend this could be a slow and/or
   expensive operation.

   Restic now lists the snapshots only once and remembers the result in order to
   resolve all further snapshot IDs swiftly.

   https://github.com/restic/restic/issues/3428
   https://github.com/restic/restic/pull/3570
   https://github.com/restic/restic/pull/3395

 * Bugfix #3432: Fix rare 'not found in repository' error for `copy` command

   In rare cases `copy` (and other commands) would report that `LoadTree(...)`
   returned an `id [...] not found in repository` error. This could be caused by a
   backup or copy command running concurrently. The error was only temporary;
   running the failed restic command a second time as a workaround did resolve the
   error.

   This issue has now been fixed by correcting the order in which restic reads data
   from the repository. It is now guaranteed that restic only loads snapshots for
   which all necessary data is already available.

   https://github.com/restic/restic/issues/3432
   https://github.com/restic/restic/pull/3570

 * Bugfix #3681: Fix rclone (shimmed by Scoop) and sftp not working on Windows

   In #3602 a fix was introduced to address the problem of `rclone` prematurely
   exiting when Ctrl+C is pressed on Windows. The solution was to create the
   subprocess with its console detached from the restic console.

   However, this solution failed when using `rclone` installed by Scoop or using
   `sftp` with a passphrase-protected private key. We've now fixed this by using a
   different approach to prevent Ctrl-C from passing down too early.

   https://github.com/restic/restic/issues/3681
   https://github.com/restic/restic/issues/3692
   https://github.com/restic/restic/pull/3696

 * Bugfix #3685: The `diff` command incorrectly listed some files as added

   There was a bug in the `diff` command, causing it to always show files in a
   removed directory as added. This has now been fixed.

   https://github.com/restic/restic/issues/3685
   https://github.com/restic/restic/pull/3686

 * Bugfix #3716: Print "wrong password" to stderr instead of stdout

   If an invalid password was entered, the error message was printed on stdout and
   not on stderr as intended. This has now been fixed.

   https://github.com/restic/restic/pull/3716
   https://forum.restic.net/t/4965

 * Bugfix #3720: Directory sync errors for repositories accessed via SMB

   On Linux and macOS, accessing a repository via a SMB/CIFS mount resulted in
   restic failing to save the lock file, yielding the following errors:

   Save(<lock/071fe833f0>) returned error, retrying after 552.330144ms: sync
   /repo/locks: no such file or directory Save(<lock/bf789d7343>) returned error,
   retrying after 552.330144ms: sync /repo/locks: invalid argument

   This has now been fixed by ignoring the relevant error codes.

   https://github.com/restic/restic/issues/3720
   https://github.com/restic/restic/issues/3751
   https://github.com/restic/restic/pull/3752

 * Bugfix #3736: The `stats` command miscalculated restore size for multiple snapshots

   Since restic 0.10.0 the restore size calculated by the `stats` command for
   multiple snapshots was too low. The hardlink detection was accidentally applied
   across multiple snapshots and thus ignored many files. This has now been fixed.

   https://github.com/restic/restic/issues/3736
   https://github.com/restic/restic/pull/3740

 * Bugfix #3772: Correctly rebuild index for legacy repositories

   After running `rebuild-index` on a legacy repository containing mixed pack files
   (that is, pack files which store both metadata and file data), `check` printed
   warnings like `pack 12345678 contained in several indexes: ...`. This warning
   was not critical, but has now nonetheless been fixed by properly handling mixed
   pack files while rebuilding the index.

   Running `prune` for such legacy repositories will also fix the warning by
   reorganizing the pack files which caused it.

   https://github.com/restic/restic/pull/3772
   https://github.com/restic/restic/pull/3884
   https://forum.restic.net/t/5044/13

 * Bugfix #3776: Limit number of key files tested while opening a repository

   Previously, restic tested the password against every key in the repository when
   opening a repository. The more keys there were in the repository, the slower
   this operation became.

   Restic now tests the password against up to 20 key files in the repository.
   Alternatively, you can use the `--key-hint=<key ID>` option to specify a
   specific key file to use instead.

   https://github.com/restic/restic/pull/3776

 * Bugfix #3861: Yield error on invalid policy to `forget`

   The `forget` command previously silently ignored invalid/unsupported units in
   the duration options, such as e.g. `--keep-within-daily 2w`.

   Specifying an invalid/unsupported duration unit now results in an error.

   https://github.com/restic/restic/issues/3861
   https://github.com/restic/restic/pull/3862

 * Change #1842: Support debug log creation in release builds

   Creating a debug log was only possible in debug builds which required users to
   manually build restic. We changed the release builds to allow creating debug
   logs by simply setting the environment variable `DEBUG_LOG=logname.log`.

   https://github.com/restic/restic/issues/1842
   https://github.com/restic/restic/pull/3826

 * Change #3295: Deprecate `check --check-unused` and add further checks

   Since restic 0.12.0, it is expected to still have unused blobs after running
   `prune`. This made the `--check-unused` option of the `check` command rather
   useless and tended to confuse users. This option has been deprecated and is now
   ignored.

   The `check` command now also warns if a repository is using either the legacy S3
   layout or mixed pack files with both tree and data blobs. The latter is known to
   cause performance problems.

   https://github.com/restic/restic/issues/3295
   https://github.com/restic/restic/pull/3730

 * Change #3680: Update dependencies and require Go 1.15 or newer

   We've updated most dependencies. Since some libraries require newer language
   features we're dropping support for Go 1.14, which means that restic now
   requires at least Go 1.15 to build.

   https://github.com/restic/restic/issues/3680
   https://github.com/restic/restic/issues/3883

 * Change #3742: Replace `--repo2` option used by `init`/`copy` with `--from-repo`

   The `init` and `copy` commands can read data from another repository. However,
   confusingly `--repo2` referred to the repository *from* which the `init` command
   copies parameters, but for the `copy` command `--repo2` referred to the copy
   *destination*.

   We've introduced a new option, `--from-repo`, which always refers to the source
   repository for both commands. The old parameter names have been deprecated but
   still work. To create a new repository and copy all snapshots to it, the
   commands are now as follows:

   ```
   restic -r /srv/restic-repo-copy init --from-repo /srv/restic-repo --copy-chunker-params
   restic -r /srv/restic-repo-copy copy --from-repo /srv/restic-repo
   ```

   https://github.com/restic/restic/pull/3742
   https://forum.restic.net/t/5017

 * Enhancement #21: Add compression support

   We've added compression support to the restic repository format. To create a
   repository using the new format run `init --repository-version 2`. Please note
   that the repository cannot be read by restic versions prior to 0.14.0.

   You can configure whether data is compressed with the option `--compression`. It
   can be set to `auto` (the default, which will compress very fast), `max` (which
   will trade backup speed and CPU usage for better compression), or `off` (which
   disables compression). Each setting is only applied for the current run of
   restic and does *not* apply to future runs. The option can also be set via the
   environment variable `RESTIC_COMPRESSION`.

   To upgrade in place run `migrate upgrade_repo_v2` followed by `prune`. See the
   documentation for more details. The migration checks the repository integrity
   and upgrades the repository format, but will not change any data. Afterwards,
   prune will rewrite the metadata to make use of compression.

   As an alternative you can use the `copy` command to migrate snapshots; First
   create a new repository using `init --repository-version 2 --copy-chunker-params
   --repo2 path/to/old/repo`, and then use the `copy` command to copy all snapshots
   to the new repository.

   https://github.com/restic/restic/issues/21
   https://github.com/restic/restic/issues/3779
   https://github.com/restic/restic/pull/3666
   https://github.com/restic/restic/pull/3704
   https://github.com/restic/restic/pull/3733

 * Enhancement #1153: Support pruning even when the disk is full

   When running out of disk space it was no longer possible to add or remove data
   from a repository. To help with recovering from such a deadlock, the prune
   command now supports an `--unsafe-recover-no-free-space` option to recover from
   these situations. Make sure to read the documentation first!

   https://github.com/restic/restic/issues/1153
   https://github.com/restic/restic/pull/3481

 * Enhancement #2162: Adaptive IO concurrency based on backend connections

   Many commands used hard-coded limits for the number of concurrent operations.
   This prevented speed improvements by increasing the number of connections used
   by a backend.

   These limits have now been replaced by using the configured number of backend
   connections instead, which can be controlled using the `-o
   <backend-name>.connections=5` option. Commands will then automatically scale
   their parallelism accordingly.

   To limit the number of CPU cores used by restic, you can set the environment
   variable `GOMAXPROCS` accordingly. For example to use a single CPU core, use
   `GOMAXPROCS=1`.

   https://github.com/restic/restic/issues/2162
   https://github.com/restic/restic/issues/1467
   https://github.com/restic/restic/pull/3611

 * Enhancement #2291: Allow pack size customization

   Restic now uses a target pack size of 16 MiB by default. This can be customized
   using the `--pack-size size` option. Supported pack sizes range between 4 and
   128 MiB.

   It is possible to migrate an existing repository to _larger_ pack files using
   `prune --repack-small`. This will rewrite every pack file which is significantly
   smaller than the target size.

   https://github.com/restic/restic/issues/2291
   https://github.com/restic/restic/pull/3731

 * Enhancement #2295: Allow use of SAS token to authenticate to Azure

   Previously restic only supported AccountKeys to authenticate to Azure storage
   accounts, which necessitates giving a significant amount of access.

   We added support for Azure SAS tokens which are a more fine-grained and
   time-limited manner of granting access. Set the `AZURE_ACCOUNT_NAME` and
   `AZURE_ACCOUNT_SAS` environment variables to use a SAS token for authentication.
   Note that if `AZURE_ACCOUNT_KEY` is set, it will take precedence.

   https://github.com/restic/restic/issues/2295
   https://github.com/restic/restic/pull/3661

 * Enhancement #2351: Use config file permissions to control file group access

   Previously files in a local/SFTP repository would always end up with very
   restrictive access permissions, allowing access only to the owner. This
   prevented a number of valid use-cases involving groups and ACLs.

   We now use the permissions of the config file in the repository to decide
   whether group access should be given to newly created repository files or not.
   We arrange for repository files to be created group readable exactly when the
   repository config file is group readable.

   To opt-in to group readable repositories, a simple `chmod -R g+r` or equivalent
   on the config file can be used. For repositories that should be writable by
   group members a tad more setup is required, see the docs.

   Posix ACLs can also be used now that the group permissions being forced to zero
   no longer masks the effect of ACL entries.

   https://github.com/restic/restic/issues/2351
   https://github.com/restic/restic/pull/3419
   https://forum.restic.net/t/1391

 * Enhancement #2696: Improve backup speed with many small files

   We have restructured the backup pipeline to continue reading files while all
   upload connections are busy. This allows the backup to already prepare the next
   data file such that the upload can continue as soon as a connection becomes
   available. This can especially improve the backup performance for high latency
   backends.

   The upload concurrency is now controlled using the `-o
   <backend-name>.connections=5` option.

   https://github.com/restic/restic/issues/2696
   https://github.com/restic/restic/pull/3489

 * Enhancement #2907: Make snapshot directory structure of `mount` command customizable

   We've added the possibility to customize the snapshot directory structure of the
   `mount` command using templates passed to the `--snapshot-template` option. The
   formatting of snapshots' timestamps is now controlled using `--time-template`
   and supports subdirectories to for example group snapshots by year. Please see
   `restic help mount` for further details.

   Characters in tag names which are not allowed in a filename are replaced by
   underscores `_`. For example a tag `foo/bar` will result in a directory name of
   `foo_bar`.

   https://github.com/restic/restic/issues/2907
   https://github.com/restic/restic/pull/2913
   https://github.com/restic/restic/pull/3691

 * Enhancement #2923: Improve speed of `copy` command

   The `copy` command could require a long time to copy snapshots for non-local
   backends. This has been improved to provide a throughput comparable to the
   `restore` command.

   Additionally, `copy` now displays a progress bar.

   https://github.com/restic/restic/issues/2923
   https://github.com/restic/restic/pull/3513

 * Enhancement #3114: Optimize handling of duplicate blobs in `prune`

   Restic `prune` always used to repack all data files containing duplicate blobs.
   This effectively removed all duplicates during prune. However, as a consequence
   all these data files were repacked even if the unused repository space threshold
   could be reached with less work.

   This is now changed and `prune` works nice and fast even when there are lots of
   duplicate blobs.

   https://github.com/restic/restic/issues/3114
   https://github.com/restic/restic/pull/3290

 * Enhancement #3465: Improve handling of temporary files on Windows

   In some cases restic failed to delete temporary files, causing the current
   command to fail. This has now been fixed by ensuring that Windows automatically
   deletes the file. In addition, temporary files are only written to disk when
   necessary, reducing disk writes.

   https://github.com/restic/restic/issues/3465
   https://github.com/restic/restic/issues/1551
   https://github.com/restic/restic/pull/3610

 * Enhancement #3475: Allow limiting IO concurrency for local and SFTP backend

   Restic did not support limiting the IO concurrency / number of connections for
   accessing repositories stored using the local or SFTP backends. The number of
   connections is now limited as for other backends, and can be configured via the
   `-o local.connections=2` and `-o sftp.connections=5` options. This ensures that
   restic does not overwhelm the backend with concurrent IO operations.

   https://github.com/restic/restic/pull/3475

 * Enhancement #3484: Stream data in `check` and `prune` commands

   The commands `check --read-data` and `prune` previously downloaded data files
   into temporary files which could end up being written to disk. This could cause
   a large amount of data being written to disk.

   The pack files are now instead streamed, which removes the need for temporary
   files. Please note that *uploads* during `backup` and `prune` still require
   temporary files.

   https://github.com/restic/restic/issues/3710
   https://github.com/restic/restic/pull/3484
   https://github.com/restic/restic/pull/3717

 * Enhancement #3709: Validate exclude patterns before backing up

   Exclude patterns provided via `--exclude`, `--iexclude`, `--exclude-file` or
   `--iexclude-file` previously weren't validated. As a consequence, invalid
   patterns resulted in files that were meant to be excluded being backed up.

   Restic now validates all patterns before running the backup and aborts with a
   fatal error if an invalid pattern is detected.

   https://github.com/restic/restic/issues/3709
   https://github.com/restic/restic/pull/3734

 * Enhancement #3729: Display full IDs in `check` warnings

   When running commands to inspect or repair a damaged repository, it is often
   necessary to supply the full IDs of objects stored in the repository.

   The output of `check` now includes full IDs instead of their shortened variant.

   https://github.com/restic/restic/pull/3729

 * Enhancement #3773: Optimize memory usage for directories with many files

   Backing up a directory with hundreds of thousands or more files caused restic to
   require large amounts of memory. We've now optimized the `backup` command such
   that it requires up to 30% less memory.

   https://github.com/restic/restic/pull/3773

 * Enhancement #3819: Validate include/exclude patterns before restoring

   Patterns provided to `restore` via `--exclude`, `--iexclude`, `--include` and
   `--iinclude` weren't validated before running the restore. Invalid patterns
   would result in error messages being printed repeatedly, and possibly unwanted
   files being restored.

   Restic now validates all patterns before running the restore, and aborts with a
   fatal error if an invalid pattern is detected.

   https://github.com/restic/restic/pull/3819

 * Enhancement #3837: Improve SFTP repository initialization over slow links

   The `init` command, when used on an SFTP backend, now sends multiple `mkdir`
   commands to the backend concurrently. This reduces the waiting times when
   creating a repository over a very slow connection.

   https://github.com/restic/restic/issues/3837
   https://github.com/restic/restic/pull/3840


# Changelog for restic 0.13.0 (2022-03-26)
The following sections list the changes in restic 0.13.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1106: Never lock repository for `list locks`
 * Fix #2345: Make cache crash-resistant and usable by multiple concurrent processes
 * Fix #2452: Improve error handling of repository locking
 * Fix #2738: Don't print progress for `backup --json --quiet`
 * Fix #3382: Make `check` command honor `RESTIC_CACHE_DIR` environment variable
 * Fix #3488: `rebuild-index` failed if an index file was damaged
 * Fix #3518: Make `copy` command honor `--no-lock` for source repository
 * Fix #3556: Fix hang with Backblaze B2 on SSL certificate authority error
 * Fix #3591: Fix handling of `prune --max-repack-size=0`
 * Fix #3601: Fix rclone backend prematurely exiting when receiving SIGINT on Windows
 * Fix #3619: Avoid choosing parent snapshots newer than time of new snapshot
 * Fix #3667: The `mount` command now reports symlinks sizes
 * Chg #3519: Require Go 1.14 or newer
 * Chg #3641: Ignore parent snapshot for `backup --stdin`
 * Enh #233: Support negative include/exclude patterns
 * Enh #1542: Add `--dry-run`/`-n` option to `backup` command
 * Enh #2202: Add upload checksum for Azure, GS, S3 and Swift backends
 * Enh #2388: Add warning for S3 if partial credentials are provided
 * Enh #2508: Support JSON output and quiet mode for the `diff` command
 * Enh #2594: Speed up the `restore --verify` command
 * Enh #2656: Add flag to disable TLS verification for self-signed certificates
 * Enh #2816: The `backup` command no longer updates file access times on Linux
 * Enh #2880: Make `recover` collect only unreferenced trees
 * Enh #3003: Atomic uploads for the SFTP backend
 * Enh #3127: Add xattr (extended attributes) support for Solaris
 * Enh #3429: Verify that new or modified keys are stored correctly
 * Enh #3436: Improve local backend's resilience to (system) crashes
 * Enh #3464: Skip lock creation on `forget` if `--no-lock` and `--dry-run`
 * Enh #3490: Support random subset by size in `check --read-data-subset`
 * Enh #3508: Cache blobs read by the `dump` command
 * Enh #3511: Support configurable timeout for the rclone backend
 * Enh #3541: Improve handling of temporary B2 delete errors
 * Enh #3542: Add file mode in symbolic notation to `ls --json`
 * Enh #3593: Improve `copy` performance by parallelizing IO

## Details

 * Bugfix #1106: Never lock repository for `list locks`

   The `list locks` command previously locked to the repository by default. This
   had the problem that it wouldn't work for an exclusively locked repository and
   that the command would also display its own lock file which can be confusing.

   Now, the `list locks` command never locks the repository.

   https://github.com/restic/restic/issues/1106
   https://github.com/restic/restic/pull/3665

 * Bugfix #2345: Make cache crash-resistant and usable by multiple concurrent processes

   The restic cache directory (`RESTIC_CACHE_DIR`) could end up in a broken state
   in the event of restic (or the OS) crashing. This is now less likely to occur as
   files are downloaded to a temporary location before being moved to their proper
   location.

   This also allows multiple concurrent restic processes to operate on a single
   repository without conflicts. Previously, concurrent operations could cause
   segfaults because the processes saw each other's partially downloaded files.

   https://github.com/restic/restic/issues/2345
   https://github.com/restic/restic/pull/2838

 * Bugfix #2452: Improve error handling of repository locking

   Previously, when the lock refresh failed to delete the old lock file, it forgot
   about the newly created one. Instead it continued trying to delete the old
   (usually no longer existing) lock file and thus over time lots of lock files
   accumulated. This has now been fixed.

   https://github.com/restic/restic/issues/2452
   https://github.com/restic/restic/issues/2473
   https://github.com/restic/restic/issues/2562
   https://github.com/restic/restic/pull/3512

 * Bugfix #2738: Don't print progress for `backup --json --quiet`

   Unlike the text output, the `--json` output format still printed progress
   information even in `--quiet` mode. This has now been fixed by always disabling
   the progress output in quiet mode.

   https://github.com/restic/restic/issues/2738
   https://github.com/restic/restic/pull/3264

 * Bugfix #3382: Make `check` command honor `RESTIC_CACHE_DIR` environment variable

   Previously, the `check` command didn't honor the `RESTIC_CACHE_DIR` environment
   variable, which caused problems in certain system/usage configurations. This has
   now been fixed.

   https://github.com/restic/restic/issues/3382
   https://github.com/restic/restic/pull/3474

 * Bugfix #3488: `rebuild-index` failed if an index file was damaged

   Previously, the `rebuild-index` command would fail with an error if an index
   file was damaged or truncated. This has now been fixed.

   On older restic versions, a (slow) workaround is to use `rebuild-index
   --read-all-packs` or to manually delete the damaged index.

   https://github.com/restic/restic/pull/3488

 * Bugfix #3518: Make `copy` command honor `--no-lock` for source repository

   The `copy` command previously did not respect the `--no-lock` option for the
   source repository, causing failures with read-only storage backends. This has
   now been fixed such that the option is now respected.

   https://github.com/restic/restic/issues/3518
   https://github.com/restic/restic/pull/3589

 * Bugfix #3556: Fix hang with Backblaze B2 on SSL certificate authority error

   Previously, if a request failed with an SSL unknown certificate authority error,
   the B2 backend retried indefinitely and restic would appear to hang.

   This has now been fixed and restic instead fails with an error message.

   https://github.com/restic/restic/issues/3556
   https://github.com/restic/restic/issues/2355
   https://github.com/restic/restic/pull/3571

 * Bugfix #3591: Fix handling of `prune --max-repack-size=0`

   Restic ignored the `--max-repack-size` option when passing a value of 0. This
   has now been fixed.

   As a workaround, `--max-repack-size=1` can be used with older versions of
   restic.

   https://github.com/restic/restic/pull/3591

 * Bugfix #3601: Fix rclone backend prematurely exiting when receiving SIGINT on Windows

   Previously, pressing Ctrl+C in a Windows console where restic was running with
   rclone as the backend would cause rclone to exit prematurely due to getting a
   `SIGINT` signal at the same time as restic. Restic would then wait for a long
   time for time with "unexpected EOF" and "rclone stdio connection already closed"
   errors.

   This has now been fixed by restic starting the rclone process detached from the
   console restic runs in (similar to starting processes in a new process group on
   Linux), which enables restic to gracefully clean up rclone (which now never gets
   the `SIGINT`).

   https://github.com/restic/restic/issues/3601
   https://github.com/restic/restic/pull/3602

 * Bugfix #3619: Avoid choosing parent snapshots newer than time of new snapshot

   The `backup` command, when a `--parent` was not provided, previously chose the
   most recent matching snapshot as the parent snapshot. However, this didn't make
   sense when the user passed `--time` to create a new snapshot older than the most
   recent snapshot.

   Instead, `backup` now chooses the most recent snapshot which is not newer than
   the snapshot-being-created's timestamp, to avoid any time travel.

   https://github.com/restic/restic/pull/3619

 * Bugfix #3667: The `mount` command now reports symlinks sizes

   Symlinks used to have size zero in restic mountpoints, confusing some
   third-party tools. They now have a size equal to the byte length of their target
   path, as required by POSIX.

   https://github.com/restic/restic/issues/3667
   https://github.com/restic/restic/pull/3668

 * Change #3519: Require Go 1.14 or newer

   Restic now requires Go 1.14 to build. This allows it to use new standard library
   features instead of an external dependency.

   https://github.com/restic/restic/issues/3519

 * Change #3641: Ignore parent snapshot for `backup --stdin`

   Restic uses a parent snapshot to speed up directory scanning when performing
   backups, but this only wasted time and memory when the backup source is stdin
   (using the `--stdin` option of the `backup` command), since no directory
   scanning is performed in this case.

   Snapshots made with `backup --stdin` no longer have a parent snapshot, which
   allows restic to skip some startup operations and saves a bit of resources.

   The `--parent` option is still available for `backup --stdin`, but is now
   ignored.

   https://github.com/restic/restic/issues/3641
   https://github.com/restic/restic/pull/3645

 * Enhancement #233: Support negative include/exclude patterns

   If a pattern starts with an exclamation mark and it matches a file that was
   previously matched by a regular pattern, the match is cancelled. Notably, this
   can be used with `--exclude-file` to cancel the exclusion of some files.

   It works similarly to `.gitignore`, with the same limitation; Once a directory
   is excluded, it is not possible to include files inside the directory.

   Example of use as an exclude pattern for the `backup` command:

   $HOME/**/* !$HOME/Documents !$HOME/code !$HOME/.emacs.d !$HOME/games # [...]
   node_modules *~ *.o *.lo *.pyc # [...] $HOME/code/linux/* !$HOME/code/linux/.git
   # [...]

   https://github.com/restic/restic/issues/233
   https://github.com/restic/restic/pull/2311

 * Enhancement #1542: Add `--dry-run`/`-n` option to `backup` command

   Testing exclude filters and other configuration options was error prone as wrong
   filters could cause files to be uploaded unintentionally. It was also not
   possible to estimate beforehand how much data would be uploaded.

   The `backup` command now has a `--dry-run`/`-n` option, which performs all the
   normal steps of a backup without actually writing anything to the repository.

   Passing -vv will log information about files that would be added, allowing for
   verification of source and exclusion options before running the real backup.

   https://github.com/restic/restic/issues/1542
   https://github.com/restic/restic/pull/2308
   https://github.com/restic/restic/pull/3210
   https://github.com/restic/restic/pull/3300

 * Enhancement #2202: Add upload checksum for Azure, GS, S3 and Swift backends

   Previously only the B2 and partially the Swift backends verified the integrity
   of uploaded (encrypted) files. The verification works by informing the backend
   about the expected hash of the uploaded file. The backend then verifies the
   upload and thereby rules out any data corruption during upload.

   We have now added upload checksums for the Azure, GS, S3 and Swift backends,
   which besides integrity checking for uploads also means that restic can now be
   used to store backups in S3 buckets which have Object Lock enabled.

   https://github.com/restic/restic/issues/2202
   https://github.com/restic/restic/issues/2700
   https://github.com/restic/restic/issues/3023
   https://github.com/restic/restic/pull/3246

 * Enhancement #2388: Add warning for S3 if partial credentials are provided

   Previously restic did not notify about incomplete credentials when using the S3
   backend, instead just reporting access denied.

   Restic now checks that both the AWS key ID and secret environment variables are
   set before connecting to the remote server, and reports an error if not.

   https://github.com/restic/restic/issues/2388
   https://github.com/restic/restic/pull/3532

 * Enhancement #2508: Support JSON output and quiet mode for the `diff` command

   The `diff` command now supports outputting machine-readable output in JSON
   format. To enable this, pass the `--json` option to the command. To only print
   the summary and suppress detailed output, pass the `--quiet` option.

   https://github.com/restic/restic/issues/2508
   https://github.com/restic/restic/pull/3592

 * Enhancement #2594: Speed up the `restore --verify` command

   The `--verify` option lets the `restore` command verify the file content after
   it has restored a snapshot. The performance of this operation has now been
   improved by up to a factor of two.

   https://github.com/restic/restic/pull/2594

 * Enhancement #2656: Add flag to disable TLS verification for self-signed certificates

   There is now an `--insecure-tls` global option in restic, which disables TLS
   verification for self-signed certificates in order to support some development
   workflows.

   https://github.com/restic/restic/issues/2656
   https://github.com/restic/restic/pull/2657

 * Enhancement #2816: The `backup` command no longer updates file access times on Linux

   When reading files during backup, restic used to cause the operating system to
   update the files' access times. Note that this did not apply to filesystems with
   disabled file access times.

   Restic now instructs the operating system not to update the file access time, if
   the user running restic is the file owner or has root permissions.

   https://github.com/restic/restic/pull/2816

 * Enhancement #2880: Make `recover` collect only unreferenced trees

   Previously, the `recover` command used to generate a snapshot containing *all*
   root trees, even those which were already referenced by a snapshot.

   This has been improved such that it now only processes trees not already
   referenced by any snapshot.

   https://github.com/restic/restic/pull/2880

 * Enhancement #3003: Atomic uploads for the SFTP backend

   The SFTP backend did not upload files atomically. An interrupted upload could
   leave an incomplete file behind which could prevent restic from accessing the
   repository. This has now been fixed and uploads in the SFTP backend are done
   atomically.

   https://github.com/restic/restic/issues/3003
   https://github.com/restic/restic/pull/3524

 * Enhancement #3127: Add xattr (extended attributes) support for Solaris

   Restic now supports xattr for the Solaris operating system.

   https://github.com/restic/restic/issues/3127
   https://github.com/restic/restic/pull/3628

 * Enhancement #3429: Verify that new or modified keys are stored correctly

   When adding a new key or changing the password of a key, restic used to just
   create the new key (and remove the old one, when changing the password). There
   was no verification that the new key was stored correctly and works properly. As
   the repository cannot be decrypted without a valid key file, this could in rare
   cases cause the repository to become inaccessible.

   Restic now checks that new key files actually work before continuing. This can
   protect against some (rare) cases of hardware or storage problems.

   https://github.com/restic/restic/pull/3429

 * Enhancement #3436: Improve local backend's resilience to (system) crashes

   Restic now ensures that files stored using the `local` backend are created
   atomically (that is, files are either stored completely or not at all). This
   ensures that no incomplete files are left behind even if restic is terminated
   while writing a file.

   In addition, restic now tries to ensure that the directory in the repository
   which contains a newly uploaded file is also written to disk. This can prevent
   missing files if the system crashes or the disk is not properly unmounted.

   https://github.com/restic/restic/pull/3436

 * Enhancement #3464: Skip lock creation on `forget` if `--no-lock` and `--dry-run`

   Restic used to silently ignore the `--no-lock` option of the `forget` command.

   It now skips creation of lock file in case both `--dry-run` and `--no-lock` are
   specified. If `--no-lock` option is specified without `--dry-run`, restic prints
   a warning message to stderr.

   https://github.com/restic/restic/issues/3464
   https://github.com/restic/restic/pull/3623

 * Enhancement #3490: Support random subset by size in `check --read-data-subset`

   The `--read-data-subset` option of the `check` command now supports a third way
   of specifying the subset to check, namely `nS` where `n` is a size in bytes with
   suffix `S` as k/K, m/M, g/G or t/T.

   https://github.com/restic/restic/issues/3490
   https://github.com/restic/restic/pull/3548

 * Enhancement #3508: Cache blobs read by the `dump` command

   When dumping a file using the `dump` command, restic did not cache blobs in any
   way, so even consecutive runs of the same blob were loaded from the repository
   again and again, slowing down the dump.

   Now, the caching mechanism already used by the `fuse` command is also used by
   the `dump` command. This makes dumping much faster, especially for sparse files.

   https://github.com/restic/restic/pull/3508

 * Enhancement #3511: Support configurable timeout for the rclone backend

   A slow rclone backend could cause restic to time out while waiting for the
   repository to open. Restic now offers an `-o rclone.timeout` option to make this
   timeout configurable.

   https://github.com/restic/restic/issues/3511
   https://github.com/restic/restic/pull/3514

 * Enhancement #3541: Improve handling of temporary B2 delete errors

   Deleting files on B2 could sometimes fail temporarily, which required restic to
   retry the delete operation. In some cases the file was deleted nevertheless,
   causing the retries and ultimately the restic command to fail. This has now been
   fixed.

   https://github.com/restic/restic/issues/3541
   https://github.com/restic/restic/pull/3544

 * Enhancement #3542: Add file mode in symbolic notation to `ls --json`

   The `ls --json` command now provides the file mode in symbolic notation (using
   the `permissions` key), aligned with `find --json`.

   https://github.com/restic/restic/issues/3542
   https://github.com/restic/restic/pull/3573
   https://forum.restic.net/t/restic-ls-understanding-file-mode-with-json/4371

 * Enhancement #3593: Improve `copy` performance by parallelizing IO

   Restic copy previously only used a single thread for copying blobs between
   repositories, which resulted in limited performance when copying small blobs
   to/from a high latency backend (i.e. any remote backend, especially b2).

   Copying will now use 8 parallel threads to increase the throughput of the copy
   operation.

   https://github.com/restic/restic/pull/3593


# Changelog for restic 0.12.1 (2021-08-03)
The following sections list the changes in restic 0.12.1 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #2742: Improve error handling for rclone and REST backend over HTTP2
 * Fix #3111: Fix terminal output redirection for PowerShell
 * Fix #3184: `backup --quiet` no longer prints status information
 * Fix #3214: Treat an empty password as a fatal error for repository init
 * Fix #3267: `copy` failed to copy snapshots in rare cases
 * Fix #3296: Fix crash of `check --read-data-subset=x%` run for an empty repository
 * Fix #3302: Fix `fdopendir: not a directory` error for local backend
 * Fix #3305: Fix possibly missing backup summary of JSON output in case of error
 * Fix #3334: Print `created new cache` message only on a terminal
 * Fix #3380: Fix crash of `backup --exclude='**'`
 * Fix #3439: Correctly handle download errors during `restore`
 * Chg #3247: Empty files now have size of 0 in `ls --json` output
 * Enh #2780: Add release binaries for s390x architecture on Linux
 * Enh #3167: Allow specifying limit of `snapshots` list
 * Enh #3293: Add `--repository-file2` option to `init` and `copy` command
 * Enh #3312: Add auto-completion support for fish
 * Enh #3336: SFTP backend now checks for disk space
 * Enh #3377: Add release binaries for Apple Silicon
 * Enh #3414: Add `--keep-within-hourly` option to restic forget
 * Enh #3426: Optimize read performance of mount command
 * Enh #3427: `find --pack` fallback to index if data file is missing
 * Enh #3456: Support filtering and specifying untagged snapshots

## Details

 * Bugfix #2742: Improve error handling for rclone and REST backend over HTTP2

   When retrieving data from the rclone / REST backend while also using HTTP2
   restic did not detect when no data was returned at all. This could cause for
   example the `check` command to report the following error:

   Pack ID does not match, want [...], got e3b0c442

   This has been fixed by correctly detecting and retrying the incomplete download.

   https://github.com/restic/restic/issues/2742
   https://github.com/restic/restic/pull/3453
   https://forum.restic.net/t/http2-stream-closed-connection-reset-context-canceled/3743/10

 * Bugfix #3111: Fix terminal output redirection for PowerShell

   When redirecting the output of restic using PowerShell on Windows, the output
   contained terminal escape characters. This has been fixed by properly detecting
   the terminal type.

   In addition, the mintty terminal now shows progress output for the backup
   command.

   https://github.com/restic/restic/issues/3111
   https://github.com/restic/restic/pull/3325

 * Bugfix #3184: `backup --quiet` no longer prints status information

   A regression in the latest restic version caused the output of `backup --quiet`
   to contain large amounts of backup progress information when run using an
   interactive terminal. This is fixed now.

   A workaround for this bug is to run restic as follows: `restic backup --quiet
   [..] | cat -`.

   https://github.com/restic/restic/issues/3184
   https://github.com/restic/restic/pull/3186

 * Bugfix #3214: Treat an empty password as a fatal error for repository init

   When attempting to initialize a new repository, if an empty password was
   supplied, the repository would be created but the init command would return an
   error with a stack trace. Now, if an empty password is provided, it is treated
   as a fatal error, and no repository is created.

   https://github.com/restic/restic/issues/3214
   https://github.com/restic/restic/pull/3283

 * Bugfix #3267: `copy` failed to copy snapshots in rare cases

   The `copy` command could in rare cases fail with the error message
   `SaveTree(...) returned unexpected id ...`. This has been fixed.

   On Linux/BSDs, the error could be caused by backing up symlinks with non-UTF-8
   target paths. Note that, due to limitations in the repository format, these are
   not stored properly and should be avoided if possible.

   https://github.com/restic/restic/issues/3267
   https://github.com/restic/restic/pull/3310

 * Bugfix #3296: Fix crash of `check --read-data-subset=x%` run for an empty repository

   The command `restic check --read-data-subset=x%` crashed when run for an empty
   repository. This has been fixed.

   https://github.com/restic/restic/issues/3296
   https://github.com/restic/restic/pull/3309

 * Bugfix #3302: Fix `fdopendir: not a directory` error for local backend

   The `check`, `list packs`, `prune` and `rebuild-index` commands failed for the
   local backend when the `data` folder in the repository contained files. This has
   been fixed.

   https://github.com/restic/restic/issues/3302
   https://github.com/restic/restic/pull/3308

 * Bugfix #3305: Fix possibly missing backup summary of JSON output in case of error

   When using `--json` output it happened from time to time that the summary output
   was missing in case an error occurred. This has been fixed.

   https://github.com/restic/restic/pull/3305

 * Bugfix #3334: Print `created new cache` message only on a terminal

   The message `created new cache` was printed even when the output wasn't a
   terminal. That broke piping `restic dump` output to tar or zip if cache
   directory didn't exist. The message is now only printed on a terminal.

   https://github.com/restic/restic/issues/3334
   https://github.com/restic/restic/pull/3343

 * Bugfix #3380: Fix crash of `backup --exclude='**'`

   The exclude filter `**`, which excludes all files, caused restic to crash. This
   has been corrected.

   https://github.com/restic/restic/issues/3380
   https://github.com/restic/restic/pull/3393

 * Bugfix #3439: Correctly handle download errors during `restore`

   Due to a regression in restic 0.12.0, the `restore` command in some cases did
   not retry download errors and only printed a warning. This has been fixed by
   retrying incomplete data downloads.

   https://github.com/restic/restic/issues/3439
   https://github.com/restic/restic/pull/3449

 * Change #3247: Empty files now have size of 0 in `ls --json` output

   The `ls --json` command used to omit the sizes of empty files in its output. It
   now reports a size of zero explicitly for regular files, while omitting the size
   field for all other types.

   https://github.com/restic/restic/issues/3247
   https://github.com/restic/restic/pull/3257

 * Enhancement #2780: Add release binaries for s390x architecture on Linux

   We've added release binaries for Linux using the s390x architecture.

   https://github.com/restic/restic/issues/2780
   https://github.com/restic/restic/pull/3452

 * Enhancement #3167: Allow specifying limit of `snapshots` list

   The `--last` option allowed limiting the output of the `snapshots` command to
   the latest snapshot for each host. The new `--latest n` option allows limiting
   the output to the latest `n` snapshots.

   This change deprecates the option `--last` in favour of `--latest 1`.

   https://github.com/restic/restic/pull/3167

 * Enhancement #3293: Add `--repository-file2` option to `init` and `copy` command

   The `init` and `copy` command can now be used with the `--repository-file2`
   option or the `$RESTIC_REPOSITORY_FILE2` environment variable. These to options
   are in addition to the `--repo2` flag and allow you to read the destination
   repository from a file.

   Using both `--repository-file` and `--repo2` options resulted in an error for
   the `copy` or `init` command. The handling of this combination of options has
   been fixed. A workaround for this issue is to only use `--repo` or `-r` and
   `--repo2` for `init` or `copy`.

   https://github.com/restic/restic/issues/3293
   https://github.com/restic/restic/pull/3294

 * Enhancement #3312: Add auto-completion support for fish

   The `generate` command now supports fish auto completion.

   https://github.com/restic/restic/pull/3312

 * Enhancement #3336: SFTP backend now checks for disk space

   Backing up over SFTP previously spewed multiple generic "failure" messages when
   the remote disk was full. It now checks for disk space before writing a file and
   fails immediately with a "no space left on device" message.

   https://github.com/restic/restic/issues/3336
   https://github.com/restic/restic/pull/3345

 * Enhancement #3377: Add release binaries for Apple Silicon

   We've added release binaries for macOS on Apple Silicon (M1).

   https://github.com/restic/restic/issues/3377
   https://github.com/restic/restic/pull/3394

 * Enhancement #3414: Add `--keep-within-hourly` option to restic forget

   The `forget` command allowed keeping a given number of hourly backups or to keep
   all backups within a given interval, but it was not possible to specify keeping
   hourly backups within a given interval.

   The new `--keep-within-hourly` option now offers this functionality. Similar
   options for daily/weekly/monthly/yearly are also implemented, the new options
   are:

   --keep-within-hourly <1y2m3d4h> --keep-within-daily <1y2m3d4h>
   --keep-within-weekly <1y2m3d4h> --keep-within-monthly <1y2m3d4h>
   --keep-within-yearly <1y2m3d4h>

   https://github.com/restic/restic/issues/3414
   https://github.com/restic/restic/pull/3416
   https://forum.restic.net/t/forget-policy/4014/11

 * Enhancement #3426: Optimize read performance of mount command

   Reading large files in a mounted repository may be up to five times faster. This
   improvement primarily applies to repositories stored at a backend that can be
   accessed with low latency, like e.g. the local backend.

   https://github.com/restic/restic/pull/3426

 * Enhancement #3427: `find --pack` fallback to index if data file is missing

   When investigating a repository with missing data files, it might be useful to
   determine affected snapshots before running `rebuild-index`. Previously, `find
   --pack pack-id` returned no data as it required accessing the data file. Now, if
   the necessary data is still available in the repository index, it gets retrieved
   from there.

   The command now also supports looking up multiple pack files in a single `find`
   run.

   https://github.com/restic/restic/pull/3427
   https://forum.restic.net/t/missing-packs-not-found/2600

 * Enhancement #3456: Support filtering and specifying untagged snapshots

   It was previously not possible to specify an empty tag with the `--tag` and
   `--keep-tag` options. This has now been fixed, such that `--tag ''` and
   `--keep-tag ''` now matches snapshots without tags. This allows e.g. the
   `snapshots` and `forget` commands to only operate on untagged snapshots.

   https://github.com/restic/restic/issues/3456
   https://github.com/restic/restic/pull/3457


# Changelog for restic 0.12.0 (2021-02-14)
The following sections list the changes in restic 0.12.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1681: Make `mount` not create missing mount point directory
 * Fix #1800: Ignore `no data available` filesystem error during backup
 * Fix #2563: Report the correct owner of directories in FUSE mounts
 * Fix #2688: Make `backup` and `tag` commands separate tags by comma
 * Fix #2739: Make the `cat` command respect the `--no-lock` option
 * Fix #3014: Fix sporadic stream reset between rclone and restic
 * Fix #3087: The `--use-fs-snapshot` option now works on windows/386
 * Fix #3100: Do not require gs bucket permissions when running `init`
 * Fix #3111: Correctly detect output redirection for `backup` command on Windows
 * Fix #3151: Don't create invalid snapshots when `backup` is interrupted
 * Fix #3152: Do not hang until foregrounded when completed in background
 * Fix #3166: Improve error handling in the `restore` command
 * Fix #3232: Correct statistics for overlapping backup sources
 * Fix #3249: Improve error handling in `gs` backend
 * Chg #3095: Deleting files on Google Drive now moves them to the trash
 * Enh #909: Back up mountpoints as empty directories
 * Enh #2186: Allow specifying percentage in `check --read-data-subset`
 * Enh #2433: Make the `dump` command support `zip` format
 * Enh #2453: Report permanent/fatal backend errors earlier
 * Enh #2495: Add option to let `backup` trust mtime without checking ctime
 * Enh #2528: Add Alibaba/Aliyun OSS support in the `s3` backend
 * Enh #2706: Configurable progress reports for non-interactive terminals
 * Enh #2718: Improve `prune` performance and make it more customizable
 * Enh #2941: Speed up the repacking step of the `prune` command
 * Enh #2944: Add `backup` options `--files-from-{verbatim,raw}`
 * Enh #3006: Speed up the `rebuild-index` command
 * Enh #3048: Add more checks for index and pack files in the `check` command
 * Enh #3083: Allow usage of deprecated S3 `ListObjects` API
 * Enh #3099: Reduce memory usage of `check` command
 * Enh #3106: Parallelize scan of snapshot content in `copy` and `prune`
 * Enh #3130: Parallelize reading of locks and snapshots
 * Enh #3147: Support additional environment variables for Swift authentication
 * Enh #3191: Add release binaries for MIPS architectures
 * Enh #3250: Add several more error checks
 * Enh #3254: Enable HTTP/2 for backend connections

## Details

 * Bugfix #1681: Make `mount` not create missing mount point directory

   When specifying a non-existent directory as mount point for the `mount` command,
   restic used to create the specified directory automatically.

   This has now changed such that restic instead gives an error when the specified
   directory for the mount point does not exist.

   https://github.com/restic/restic/issues/1681
   https://github.com/restic/restic/pull/3008

 * Bugfix #1800: Ignore `no data available` filesystem error during backup

   Restic was unable to backup files on some filesystems, for example certain
   configurations of CIFS on Linux which return a `no data available` error when
   reading extended attributes. These errors are now ignored.

   https://github.com/restic/restic/issues/1800
   https://github.com/restic/restic/pull/3034

 * Bugfix #2563: Report the correct owner of directories in FUSE mounts

   Restic 0.10.0 changed the FUSE mount to always report the current user as the
   owner of directories within the FUSE mount, which is incorrect.

   This is now changed back to reporting the correct owner of a directory.

   https://github.com/restic/restic/issues/2563
   https://github.com/restic/restic/pull/3141

 * Bugfix #2688: Make `backup` and `tag` commands separate tags by comma

   Running `restic backup --tag foo,bar` previously created snapshots with one
   single tag containing a comma (`foo,bar`) instead of two tags (`foo`, `bar`).

   Similarly, the `tag` command's `--set`, `--add` and `--remove` options would
   treat `foo,bar` as one tag instead of two tags. This was inconsistent with other
   commands and often unexpected when one intended `foo,bar` to mean two tags.

   To be consistent in all commands, restic now interprets `foo,bar` to mean two
   separate tags (`foo` and `bar`) instead of one tag (`foo,bar`) everywhere,
   including in the `backup` and `tag` commands.

   NOTE: This change might result in unexpected behavior in cases where you use the
   `forget` command and filter on tags like `foo,bar`. Snapshots previously backed
   up with `--tag foo,bar` will still not match that filter, but snapshots saved
   from now on will match that filter.

   To replace `foo,bar` tags with `foo` and `bar` tags in old snapshots, you can
   first generate a list of the relevant snapshots using a command like:

   Restic snapshots --json --quiet | jq '.[] | select(contains({tags:
   ["foo,bar"]})) | .id'

   And then use `restic tag --set foo --set bar snapshotID [...]` to set the new
   tags. Please adjust the commands to include real tag names and any additional
   tags, as well as the list of snapshots to process.

   https://github.com/restic/restic/issues/2688
   https://github.com/restic/restic/pull/2690
   https://github.com/restic/restic/pull/3197

 * Bugfix #2739: Make the `cat` command respect the `--no-lock` option

   The `cat` command would not respect the `--no-lock` flag. This is now fixed.

   https://github.com/restic/restic/issues/2739

 * Bugfix #3014: Fix sporadic stream reset between rclone and restic

   Sometimes when using restic with the `rclone` backend, an error message similar
   to the following would be printed:

   Didn't finish writing GET request (wrote 0/xxx): http2: stream closed

   It was found that this was caused by restic closing the connection to rclone to
   soon when downloading data. A workaround has been added which waits for the end
   of the download before closing the connection.

   https://github.com/rclone/rclone/issues/2598
   https://github.com/restic/restic/pull/3014

 * Bugfix #3087: The `--use-fs-snapshot` option now works on windows/386

   Restic failed to create VSS snapshots on windows/386 with the following error:

   GetSnapshotProperties() failed: E_INVALIDARG (0x80070057)

   This is now fixed.

   https://github.com/restic/restic/issues/3087
   https://github.com/restic/restic/pull/3090

 * Bugfix #3100: Do not require gs bucket permissions when running `init`

   Restic used to require bucket level permissions for the `gs` backend in order to
   initialize a restic repository.

   It now allows a `gs` service account to initialize a repository if the bucket
   does exist and the service account has permissions to write/read to that bucket.

   https://github.com/restic/restic/issues/3100

 * Bugfix #3111: Correctly detect output redirection for `backup` command on Windows

   On Windows, since restic 0.10.0 the `backup` command did not properly detect
   when the output was redirected to a file. This caused restic to output terminal
   control characters. This has been fixed by correcting the terminal detection.

   https://github.com/restic/restic/issues/3111
   https://github.com/restic/restic/pull/3150

 * Bugfix #3151: Don't create invalid snapshots when `backup` is interrupted

   When canceling a backup run at a certain moment it was possible that restic
   created a snapshot with an invalid "null" tree. This caused `check` and other
   operations to fail. The `backup` command now properly handles interruptions and
   never saves a snapshot when interrupted.

   https://github.com/restic/restic/issues/3151
   https://github.com/restic/restic/pull/3164

 * Bugfix #3152: Do not hang until foregrounded when completed in background

   On Linux, when running in the background restic failed to stop the terminal
   output of the `backup` command after it had completed. This caused restic to
   hang until moved to the foreground. This has now been fixed.

   https://github.com/restic/restic/pull/3152
   https://forum.restic.net/t/restic-alpine-container-cron-hangs-epoll-pwait/3334

 * Bugfix #3166: Improve error handling in the `restore` command

   The `restore` command used to not print errors while downloading file contents
   from the repository. It also incorrectly exited with a zero error code even when
   there were errors during the restore process. This has all been fixed and
   `restore` now returns with a non-zero exit code when there's an error.

   https://github.com/restic/restic/issues/3166
   https://github.com/restic/restic/pull/3207

 * Bugfix #3232: Correct statistics for overlapping backup sources

   A user reported that restic's statistics and progress information during backup
   was not correctly calculated when the backup sources (files/dirs to save)
   overlap. For example, consider a directory `foo` which contains (among others) a
   file `foo/bar`. When `restic backup foo foo/bar` was run, restic counted the
   size of the file `foo/bar` twice, so the completeness percentage as well as the
   number of files was wrong. This is now corrected.

   https://github.com/restic/restic/issues/3232
   https://github.com/restic/restic/pull/3243

 * Bugfix #3249: Improve error handling in `gs` backend

   The `gs` backend did not notice when the last step of completing a file upload
   failed. Under rare circumstances, this could cause missing files in the backup
   repository. This has now been fixed.

   https://github.com/restic/restic/pull/3249

 * Change #3095: Deleting files on Google Drive now moves them to the trash

   When deleting files on Google Drive via the `rclone` backend, restic used to
   bypass the trash folder required that one used the `-o rclone.args` option to
   enable usage of the trash folder. This ensured that deleted files in Google
   Drive were not kept indefinitely in the trash folder. However, since Google
   Drive's trash retention policy changed to deleting trashed files after 30 days,
   this is no longer needed.

   Restic now leaves it up to rclone and its configuration to use or not use the
   trash folder when deleting files. The default is to use the trash folder, as of
   rclone 1.53.2. To re-enable the restic 0.11 behavior, set the
   `RCLONE_DRIVE_USE_TRASH` environment variable or change the rclone
   configuration. See the rclone documentation for more details.

   https://github.com/restic/restic/issues/3095
   https://github.com/restic/restic/pull/3102

 * Enhancement #909: Back up mountpoints as empty directories

   When the `--one-file-system` option is specified to `restic backup`, it ignores
   all file systems mounted below one of the target directories. This means that
   when a snapshot is restored, users needed to manually recreate the mountpoint
   directories.

   Restic now backs up mountpoints as empty directories and therefore implements
   the same approach as `tar`.

   https://github.com/restic/restic/issues/909
   https://github.com/restic/restic/pull/3119

 * Enhancement #2186: Allow specifying percentage in `check --read-data-subset`

   We've enhanced the `check` command's `--read-data-subset` option to also accept
   a percentage (e.g. `2.5%` or `10%`). This will check the given percentage of
   pack files (which are randomly selected on each run).

   https://github.com/restic/restic/issues/2186
   https://github.com/restic/restic/pull/3038

 * Enhancement #2433: Make the `dump` command support `zip` format

   Previously, restic could dump the contents of a whole folder structure only in
   the `tar` format. The `dump` command now has a new flag to change output format
   to `zip`. Just pass `--archive zip` as an option to `restic dump`.

   https://github.com/restic/restic/pull/2433
   https://github.com/restic/restic/pull/3081

 * Enhancement #2453: Report permanent/fatal backend errors earlier

   When encountering errors in reading from or writing to storage backends, restic
   retries the failing operation up to nine times (for a total of ten attempts). It
   used to retry all backend operations, but now detects some permanent error
   conditions so that it can report fatal errors earlier.

   Permanent failures include local disks being full, SSH connections dropping and
   permission errors.

   https://github.com/restic/restic/issues/2453
   https://github.com/restic/restic/issues/3180
   https://github.com/restic/restic/pull/3170
   https://github.com/restic/restic/pull/3181

 * Enhancement #2495: Add option to let `backup` trust mtime without checking ctime

   The `backup` command used to require that both `ctime` and `mtime` of a file
   matched with a previously backed up version to determine that the file was
   unchanged. In other words, if either `ctime` or `mtime` of the file had changed,
   it would be considered changed and restic would read the file's content again to
   back up the relevant (changed) parts of it.

   The new option `--ignore-ctime` makes restic look at `mtime` only, such that
   `ctime` changes for a file does not cause restic to read the file's contents
   again.

   The check for both `ctime` and `mtime` was introduced in restic 0.9.6 to make
   backups more reliable in the face of programs that reset `mtime` (some Unix
   archivers do that), but it turned out to often be expensive because it made
   restic read file contents even if only the metadata (owner, permissions) of a
   file had changed. The new `--ignore-ctime` option lets the user restore the
   0.9.5 behavior when needed. The existing `--ignore-inode` option already turned
   off this behavior, but also removed a different check.

   Please note that changes in files' metadata are still recorded, regardless of
   the command line options provided to the backup command.

   https://github.com/restic/restic/issues/2495
   https://github.com/restic/restic/issues/2558
   https://github.com/restic/restic/issues/2819
   https://github.com/restic/restic/pull/2823

 * Enhancement #2528: Add Alibaba/Aliyun OSS support in the `s3` backend

   A new extended option `s3.bucket-lookup` has been added to support
   Alibaba/Aliyun OSS in the `s3` backend. The option can be set to one of the
   following values:

   - `auto` - Existing behaviour - `dns` - Use DNS style bucket access - `path` -
   Use path style bucket access

   To make the `s3` backend work with Alibaba/Aliyun OSS you must set
   `s3.bucket-lookup` to `dns` and set the `s3.region` parameter. For example:

   Restic -o s3.bucket-lookup=dns -o s3.region=oss-eu-west-1 -r
   s3:https://oss-eu-west-1.aliyuncs.com/bucketname init

   Note that `s3.region` must be set, otherwise the MinIO SDK tries to look it up
   and it seems that Alibaba doesn't support that properly.

   https://github.com/restic/restic/issues/2528
   https://github.com/restic/restic/pull/2535

 * Enhancement #2706: Configurable progress reports for non-interactive terminals

   The `backup`, `check` and `prune` commands never printed any progress reports on
   non-interactive terminals. This behavior is now configurable using the
   `RESTIC_PROGRESS_FPS` environment variable. Use for example a value of `1` for
   an update every second, or `0.01666` for an update every minute.

   The `backup` command now also prints the current progress when restic receives a
   `SIGUSR1` signal.

   Setting the `RESTIC_PROGRESS_FPS` environment variable or sending a `SIGUSR1`
   signal prints a status report even when `--quiet` was specified.

   https://github.com/restic/restic/issues/2706
   https://github.com/restic/restic/issues/3194
   https://github.com/restic/restic/pull/3199

 * Enhancement #2718: Improve `prune` performance and make it more customizable

   The `prune` command is now much faster. This is especially the case for remote
   repositories or repositories with not much data to remove. Also the memory usage
   of the `prune` command is now reduced.

   Restic used to rebuild the index from scratch after pruning. This could lead to
   missing packs in the index in some cases for eventually consistent backends such
   as e.g. AWS S3. This behavior is now changed and the index rebuilding uses the
   information already known by `prune`.

   By default, the `prune` command no longer removes all unused data. This behavior
   can be fine-tuned by new options, like the acceptable amount of unused space or
   the maximum size of data to reorganize. For more details, please see
   https://restic.readthedocs.io/en/stable/060_forget.html .

   Moreover, `prune` now accepts the `--dry-run` option and also running `forget
   --dry-run --prune` will show what `prune` would do.

   This enhancement also fixes several open issues, e.g.: -
   https://github.com/restic/restic/issues/1140 -
   https://github.com/restic/restic/issues/1599 -
   https://github.com/restic/restic/issues/1985 -
   https://github.com/restic/restic/issues/2112 -
   https://github.com/restic/restic/issues/2227 -
   https://github.com/restic/restic/issues/2305

   https://github.com/restic/restic/pull/2718
   https://github.com/restic/restic/pull/2842

 * Enhancement #2941: Speed up the repacking step of the `prune` command

   The repack step of the `prune` command, which moves still used file parts into
   new pack files such that the old ones can be garbage collected later on, now
   processes multiple pack files in parallel. This is especially beneficial for
   high latency backends or when using a fast network connection.

   https://github.com/restic/restic/pull/2941

 * Enhancement #2944: Add `backup` options `--files-from-{verbatim,raw}`

   The new `backup` options `--files-from-verbatim` and `--files-from-raw` read a
   list of files to back up from a file. Unlike the existing `--files-from` option,
   these options do not interpret the listed filenames as glob patterns; instead,
   whitespace in filenames is preserved as-is and no pattern expansion is done.
   Please see the documentation for specifics.

   These new options are highly recommended over `--files-from`, when using a
   script to generate the list of files to back up.

   https://github.com/restic/restic/issues/2944
   https://github.com/restic/restic/issues/3013

 * Enhancement #3006: Speed up the `rebuild-index` command

   We've optimized the `rebuild-index` command. Now, existing index entries are
   used to minimize the number of pack files that must be read. This speeds up the
   index rebuild a lot.

   Additionally, the option `--read-all-packs` has been added, implementing the
   previous behavior.

   https://github.com/restic/restic/pull/3006
   https://github.com/restic/restic/issue/2547

 * Enhancement #3048: Add more checks for index and pack files in the `check` command

   The `check` command run with the `--read-data` or `--read-data-subset` options
   used to only verify only the pack file content - it did not check if the blobs
   within the pack are correctly contained in the index.

   A check for the latter is now in place, which can print the following error:

   Blob ID is not contained in index or position is incorrect

   Another test is also added, which compares pack file sizes computed from the
   index and the pack header with the actual file size. This test is able to detect
   truncated pack files.

   If the index is not correct, it can be rebuilt by using the `rebuild-index`
   command.

   Having added these tests, `restic check` is now able to detect non-existing
   blobs which are wrongly referenced in the index. This situation could have lead
   to missing data.

   https://github.com/restic/restic/pull/3048
   https://github.com/restic/restic/pull/3082

 * Enhancement #3083: Allow usage of deprecated S3 `ListObjects` API

   Some S3 API implementations, e.g. Ceph before version 14.2.5, have a broken
   `ListObjectsV2` implementation which causes problems for restic when using their
   API endpoints. When a broken server implementation is used, restic prints errors
   similar to the following:

   List() returned error: Truncated response should have continuation token set

   As a temporary workaround, restic now allows using the older `ListObjects`
   endpoint by setting the `s3.list-objects-v1` extended option, for instance:

   Restic -o s3.list-objects-v1=true snapshots

   Please note that this option may be removed in future versions of restic.

   https://github.com/restic/restic/issues/3083
   https://github.com/restic/restic/pull/3085

 * Enhancement #3099: Reduce memory usage of `check` command

   The `check` command now requires less memory if it is run without the
   `--check-unused` option.

   https://github.com/restic/restic/pull/3099

 * Enhancement #3106: Parallelize scan of snapshot content in `copy` and `prune`

   The `copy` and `prune` commands used to traverse the directories of snapshots
   one by one to find used data. This snapshot traversal is now parallelized which
   can speed up this step several times.

   In addition the `check` command now reports how many snapshots have already been
   processed.

   https://github.com/restic/restic/pull/3106

 * Enhancement #3130: Parallelize reading of locks and snapshots

   Restic used to read snapshots sequentially. For repositories containing many
   snapshots this slowed down commands which have to read all snapshots.

   Now the reading of snapshots is parallelized. This speeds up for example
   `prune`, `backup` and other commands that search for snapshots with certain
   properties or which have to find the `latest` snapshot.

   The speed up also applies to locks stored in the backup repository.

   https://github.com/restic/restic/pull/3130
   https://github.com/restic/restic/pull/3174

 * Enhancement #3147: Support additional environment variables for Swift authentication

   The `swift` backend now supports the following additional environment variables
   for passing authentication details to restic: `OS_USER_ID`, `OS_USER_DOMAIN_ID`,
   `OS_PROJECT_DOMAIN_ID` and `OS_TRUST_ID`

   Depending on the `openrc` configuration file these might be required when the
   user and project domains differ from one another.

   https://github.com/restic/restic/issues/3147
   https://github.com/restic/restic/pull/3158

 * Enhancement #3191: Add release binaries for MIPS architectures

   We've added a few new architectures for Linux to the release binaries: `mips`,
   `mipsle`, `mips64`, and `mip64le`. MIPS is mostly used for low-end embedded
   systems.

   https://github.com/restic/restic/issues/3191
   https://github.com/restic/restic/pull/3208

 * Enhancement #3250: Add several more error checks

   We've added a lot more error checks in places where errors were previously
   ignored (as hinted by the static analysis program `errcheck` via
   `golangci-lint`).

   https://github.com/restic/restic/pull/3250

 * Enhancement #3254: Enable HTTP/2 for backend connections

   Go's HTTP library usually automatically chooses between HTTP/1.x and HTTP/2
   depending on what the server supports. But for compatibility this mechanism is
   disabled if DialContext is used (which is the case for restic). This change
   allows restic's HTTP client to negotiate HTTP/2 if supported by the server.

   https://github.com/restic/restic/pull/3254


# Changelog for restic 0.11.0 (2020-11-05)
The following sections list the changes in restic 0.11.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1212: Restore timestamps and permissions on intermediate directories
 * Fix #1756: Mark repository files as read-only when using the local backend
 * Fix #2241: Hide password in REST backend repository URLs
 * Fix #2319: Correctly dump directories into tar files
 * Fix #2491: Don't require `self-update --output` placeholder file
 * Fix #2834: Fix rare cases of backup command hanging forever
 * Fix #2938: Fix manpage formatting
 * Fix #2942: Make --exclude-larger-than handle disappearing files
 * Fix #2951: Restic generate, help and self-update no longer check passwords
 * Fix #2979: Make snapshots --json output [] instead of null when no snapshots
 * Enh #340: Add support for Volume Shadow Copy Service (VSS) on Windows
 * Enh #1458: New option --repository-file
 * Enh #2849: Authenticate to Google Cloud Storage with access token
 * Enh #2969: Optimize check for unchanged files during backup
 * Enh #2978: Warn if parent snapshot cannot be loaded during backup

## Details

 * Bugfix #1212: Restore timestamps and permissions on intermediate directories

   When using the `--include` option of the restore command, restic restored
   timestamps and permissions only on directories selected by the include pattern.
   Intermediate directories, which are necessary to restore files located in sub-
   directories, were created with default permissions. We've fixed the restore
   command to restore timestamps and permissions for these directories as well.

   https://github.com/restic/restic/issues/1212
   https://github.com/restic/restic/issues/1402
   https://github.com/restic/restic/pull/2906

 * Bugfix #1756: Mark repository files as read-only when using the local backend

   Files stored in a local repository were marked as writable on the filesystem for
   non-Windows systems, which did not prevent accidental file modifications outside
   of restic. In addition, the local backend did not work with certain filesystems
   and network mounts which do not permit modifications of file permissions.

   Restic now marks files stored in a local repository as read-only on the
   filesystem on non-Windows systems. The error handling is improved to support
   more filesystems.

   https://github.com/restic/restic/issues/1756
   https://github.com/restic/restic/issues/2157
   https://github.com/restic/restic/pull/2989

 * Bugfix #2241: Hide password in REST backend repository URLs

   When using a password in the REST backend repository URL, the password could in
   some cases be included in the output from restic, e.g. when initializing a repo
   or during an error.

   The password is now replaced with "***" where applicable.

   https://github.com/restic/restic/issues/2241
   https://github.com/restic/restic/pull/2658

 * Bugfix #2319: Correctly dump directories into tar files

   The dump command previously wrote directories in a tar file in a way which can
   cause compatibility problems. This caused, for example, 7zip on Windows to not
   open tar files containing directories. In addition it was not possible to dump
   directories with extended attributes. These compatibility problems are now
   corrected.

   In addition, a tar file now includes the name of the owner and group of a file.

   https://github.com/restic/restic/issues/2319
   https://github.com/restic/restic/pull/3039

 * Bugfix #2491: Don't require `self-update --output` placeholder file

   `restic self-update --output /path/to/new-restic` used to require that
   new-restic was an existing file, to be overwritten. Now it's possible to
   download an updated restic binary to a new path, without first having to create
   a placeholder file.

   https://github.com/restic/restic/issues/2491
   https://github.com/restic/restic/pull/2937

 * Bugfix #2834: Fix rare cases of backup command hanging forever

   We've fixed an issue with the backup progress reporting which could cause restic
   to hang forever right before finishing a backup.

   https://github.com/restic/restic/issues/2834
   https://github.com/restic/restic/pull/2963

 * Bugfix #2938: Fix manpage formatting

   The manpage formatting in restic v0.10.0 was garbled, which is fixed now.

   https://github.com/restic/restic/issues/2938
   https://github.com/restic/restic/pull/2977

 * Bugfix #2942: Make --exclude-larger-than handle disappearing files

   There was a small bug in the backup command's --exclude-larger-than option where
   files that disappeared between scanning and actually backing them up to the
   repository caused a panic. This is now fixed.

   https://github.com/restic/restic/issues/2942

 * Bugfix #2951: Restic generate, help and self-update no longer check passwords

   The commands `restic cache`, `generate`, `help` and `self-update` don't need
   passwords, but they previously did run the RESTIC_PASSWORD_COMMAND (if set in
   the environment), prompting users to authenticate for no reason. They now skip
   running the password command.

   https://github.com/restic/restic/issues/2951
   https://github.com/restic/restic/pull/2987

 * Bugfix #2979: Make snapshots --json output [] instead of null when no snapshots

   Restic previously output `null` instead of `[]` for the `--json snapshots`
   command, when there were no snapshots in the repository. This caused some minor
   problems when parsing the output, but is now fixed such that `[]` is output when
   the list of snapshots is empty.

   https://github.com/restic/restic/issues/2979
   https://github.com/restic/restic/pull/2984

 * Enhancement #340: Add support for Volume Shadow Copy Service (VSS) on Windows

   Volume Shadow Copy Service allows read access to files that are locked by
   another process using an exclusive lock through a filesystem snapshot. Restic
   was unable to backup those files before. This update enables backing up these
   files.

   This needs to be enabled explicitly using the --use-fs-snapshot option of the
   backup command.

   https://github.com/restic/restic/issues/340
   https://github.com/restic/restic/pull/2274

 * Enhancement #1458: New option --repository-file

   We've added a new command-line option --repository-file as an alternative to -r.
   This allows to read the repository URL from a file in order to prevent certain
   types of information leaks, especially for URLs containing credentials.

   https://github.com/restic/restic/issues/1458
   https://github.com/restic/restic/issues/2900
   https://github.com/restic/restic/pull/2910

 * Enhancement #2849: Authenticate to Google Cloud Storage with access token

   When using the GCS backend, it is now possible to authenticate with OAuth2
   access tokens instead of a credentials file by setting the GOOGLE_ACCESS_TOKEN
   environment variable.

   https://github.com/restic/restic/pull/2849

 * Enhancement #2969: Optimize check for unchanged files during backup

   During a backup restic skips processing files which have not changed since the
   last backup run. Previously this required opening each file once which can be
   slow on network filesystems. The backup command now checks for file changes
   before opening a file. This considerably reduces the time to create a backup on
   network filesystems.

   https://github.com/restic/restic/issues/2969
   https://github.com/restic/restic/pull/2970

 * Enhancement #2978: Warn if parent snapshot cannot be loaded during backup

   During a backup restic uses the parent snapshot to check whether a file was
   changed and has to be backed up again. For this check the backup has to read the
   directories contained in the old snapshot. If a tree blob cannot be loaded,
   restic now warns about this problem with the backup repository.

   https://github.com/restic/restic/pull/2978


# Changelog for restic 0.10.0 (2020-09-19)
The following sections list the changes in restic 0.10.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1863: Report correct number of directories processed by backup
 * Fix #2254: Fix tar issues when dumping `/`
 * Fix #2281: Handle format verbs like '%' properly in `find` output
 * Fix #2298: Do not hang when run as a background job
 * Fix #2389: Fix mangled json output of backup command
 * Fix #2390: Refresh lock timestamp
 * Fix #2429: Backup --json reports total_bytes_processed as 0
 * Fix #2469: Fix incorrect bytes stats in `diff` command
 * Fix #2518: Do not crash with Synology NAS sftp server
 * Fix #2531: Fix incorrect size calculation in `stats --mode restore-size`
 * Fix #2537: Fix incorrect file counts in `stats --mode restore-size`
 * Fix #2592: SFTP backend supports IPv6 addresses
 * Fix #2607: Honor RESTIC_CACHE_DIR environment variable on Mac and Windows
 * Fix #2668: Don't abort the stats command when data blobs are missing
 * Fix #2674: Add stricter prune error checks
 * Fix #2899: Fix possible crash in the progress bar of check --read-data
 * Chg #1597: Honor the --no-lock flag in the mount command
 * Chg #2482: Remove vendored dependencies
 * Chg #2546: Return exit code 3 when failing to backup all source data
 * Chg #2600: Update dependencies, require Go >= 1.13
 * Enh #323: Add command for copying snapshots between repositories
 * Enh #551: Use optimized library for hash calculation of file chunks
 * Enh #1570: Support specifying multiple host flags for various commands
 * Enh #1680: Optimize `restic mount`
 * Enh #2072: Display snapshot date when using `restic find`
 * Enh #2175: Allow specifying user and host when creating keys
 * Enh #2195: Simplify and improve restore performance
 * Enh #2277: Add support for ppc64le
 * Enh #2328: Improve speed of check command
 * Enh #2395: Ignore sync errors when operation not supported by local filesystem
 * Enh #2423: Support user@domain parsing as user
 * Enh #2427: Add flag `--iexclude-file` to backup command
 * Enh #2569: Support excluding files by their size
 * Enh #2571: Self-heal missing file parts during backup of unchanged files
 * Enh #2576: Improve the chunking algorithm
 * Enh #2598: Improve speed of diff command
 * Enh #2599: Slightly reduce memory usage of prune and stats commands
 * Enh #2733: S3 backend: Add support for WebIdentityTokenFile
 * Enh #2773: Optimize handling of new index entries
 * Enh #2781: Reduce memory consumption of in-memory index
 * Enh #2786: Optimize `list blobs` command
 * Enh #2790: Optimized file access in restic mount
 * Enh #2840: Speed-up file deletion in forget, prune and rebuild-index
 * Enh #2858: Support filtering snapshots by tag and path in the stats command

## Details

 * Bugfix #1863: Report correct number of directories processed by backup

   The directory statistics calculation was fixed to report the actual number of
   processed directories instead of always zero.

   https://github.com/restic/restic/issues/1863

 * Bugfix #2254: Fix tar issues when dumping `/`

   We've fixed an issue with dumping either `/` or files on the first sublevel e.g.
   `/foo` to tar. This also fixes tar dumping issues on Windows where this issue
   could also happen.

   https://github.com/restic/restic/issues/2254
   https://github.com/restic/restic/issues/2357
   https://github.com/restic/restic/pull/2255

 * Bugfix #2281: Handle format verbs like '%' properly in `find` output

   The JSON or "normal" output of the `find` command can now deal with file names
   that contain substrings which the Golang `fmt` package considers "format verbs"
   like `%s`.

   https://github.com/restic/restic/issues/2281

 * Bugfix #2298: Do not hang when run as a background job

   Restic did hang on exit while restoring the terminal configuration when it was
   started as a background job, for example using `restic ... &`. This has been
   fixed by only restoring the terminal configuration when restic is interrupted
   while reading a password from the terminal.

   https://github.com/restic/restic/issues/2298

 * Bugfix #2389: Fix mangled json output of backup command

   We've fixed a race condition in the json output of the backup command that could
   cause multiple lines to get mixed up. We've also ensured that the backup summary
   is printed last.

   https://github.com/restic/restic/issues/2389
   https://github.com/restic/restic/pull/2545

 * Bugfix #2390: Refresh lock timestamp

   Long-running operations did not refresh lock timestamp, resulting in locks
   becoming stale. This is now fixed.

   https://github.com/restic/restic/issues/2390

 * Bugfix #2429: Backup --json reports total_bytes_processed as 0

   We've fixed the json output of total_bytes_processed. The non-json output was
   already fixed with pull request #2138 but left the json output untouched.

   https://github.com/restic/restic/issues/2429

 * Bugfix #2469: Fix incorrect bytes stats in `diff` command

   In some cases, the wrong number of bytes (e.g. 16777215.998 TiB) were reported
   by the `diff` command. This is now fixed.

   https://github.com/restic/restic/issues/2469

 * Bugfix #2518: Do not crash with Synology NAS sftp server

   It was found that when restic is used to store data on an sftp server on a
   Synology NAS with a relative path (one which does not start with a slash), it
   may go into an endless loop trying to create directories on the server. We've
   fixed this bug by using a function in the sftp library instead of our own
   implementation.

   The bug was discovered because the Synology sftp server behaves erratic with
   non-absolute path (e.g. `home/restic-repo`). This can be resolved by just using
   an absolute path instead (`/home/restic-repo`). We've also added a paragraph in
   the FAQ.

   https://github.com/restic/restic/issues/2518
   https://github.com/restic/restic/issues/2363
   https://github.com/restic/restic/pull/2530

 * Bugfix #2531: Fix incorrect size calculation in `stats --mode restore-size`

   The restore-size mode of stats was counting hard-linked files as if they were
   independent.

   https://github.com/restic/restic/issues/2531

 * Bugfix #2537: Fix incorrect file counts in `stats --mode restore-size`

   The restore-size mode of stats was failing to count empty directories and some
   files with hard links.

   https://github.com/restic/restic/issues/2537

 * Bugfix #2592: SFTP backend supports IPv6 addresses

   The SFTP backend now supports IPv6 addresses natively, without relying on
   aliases in the external SSH configuration.

   https://github.com/restic/restic/pull/2592

 * Bugfix #2607: Honor RESTIC_CACHE_DIR environment variable on Mac and Windows

   On Mac and Windows, the RESTIC_CACHE_DIR environment variable was ignored. This
   variable can now be used on all platforms to set the directory where restic
   stores caches.

   https://github.com/restic/restic/pull/2607

 * Bugfix #2668: Don't abort the stats command when data blobs are missing

   Running the stats command in the blobs-per-file mode on a repository with
   missing data blobs previously resulted in a crash.

   https://github.com/restic/restic/pull/2668

 * Bugfix #2674: Add stricter prune error checks

   Additional checks were added to the prune command in order to improve resiliency
   to backend, hardware and/or networking issues. The checks now detect a few more
   cases where such outside factors could potentially cause data loss.

   https://github.com/restic/restic/pull/2674

 * Bugfix #2899: Fix possible crash in the progress bar of check --read-data

   We've fixed a possible crash while displaying the progress bar for the check
   --read-data command. The crash occurred when the length of the progress bar
   status exceeded the terminal width, which only happened for very narrow terminal
   windows.

   https://github.com/restic/restic/pull/2899
   https://forum.restic.net/t/restic-rclone-pcloud-connection-issues/2963/15

 * Change #1597: Honor the --no-lock flag in the mount command

   The mount command now does not lock the repository if given the --no-lock flag.
   This allows to mount repositories which are archived on a read only
   backend/filesystem.

   https://github.com/restic/restic/issues/1597
   https://github.com/restic/restic/pull/2821

 * Change #2482: Remove vendored dependencies

   We've removed the vendored dependencies (in the subdir `vendor/`). When building
   restic, the Go compiler automatically fetches the dependencies. It will also
   cryptographically verify that the correct code has been fetched by using the
   hashes in `go.sum` (see the link to the documentation below).

   https://github.com/restic/restic/issues/2482
   https://golang.org/cmd/go/#hdr-Module_downloading_and_verification

 * Change #2546: Return exit code 3 when failing to backup all source data

   The backup command used to return a zero exit code as long as a snapshot could
   be created successfully, even if some of the source files could not be read (in
   which case the snapshot would contain the rest of the files).

   This made it hard for automation/scripts to detect failures/incomplete backups
   by looking at the exit code. Restic now returns the following exit codes for the
   backup command:

   - 0 when the command was successful - 1 when there was a fatal error (no
   snapshot created) - 3 when some source data could not be read (incomplete
   snapshot created)

   https://github.com/restic/restic/issues/956
   https://github.com/restic/restic/issues/2064
   https://github.com/restic/restic/issues/2526
   https://github.com/restic/restic/issues/2364
   https://github.com/restic/restic/pull/2546

 * Change #2600: Update dependencies, require Go >= 1.13

   Restic now requires Go to be at least 1.13. This allows simplifications in the
   build process and removing workarounds.

   This is also probably the last version of restic still supporting mounting
   repositories via fuse on macOS. The library we're using for fuse does not
   support macOS any more and osxfuse is not open source any more.

   https://github.com/bazil/fuse/issues/224
   https://github.com/osxfuse/osxfuse/issues/590
   https://github.com/restic/restic/pull/2600
   https://github.com/restic/restic/pull/2852
   https://github.com/restic/restic/pull/2927

 * Enhancement #323: Add command for copying snapshots between repositories

   We've added a copy command, allowing you to copy snapshots from one repository
   to another.

   Note that this process will have to read (download) and write (upload) the
   entire snapshot(s) due to the different encryption keys used on the source and
   destination repository. Also, the transferred files are not re-chunked, which
   may break deduplication between files already stored in the destination repo and
   files copied there using this command.

   To fully support deduplication between repositories when the copy command is
   used, the init command now supports the `--copy-chunker-params` option, which
   initializes the new repository with identical parameters for splitting files
   into chunks as an already existing repository. This allows copied snapshots to
   be equally deduplicated in both repositories.

   https://github.com/restic/restic/issues/323
   https://github.com/restic/restic/pull/2606
   https://github.com/restic/restic/pull/2928

 * Enhancement #551: Use optimized library for hash calculation of file chunks

   We've switched the library used to calculate the hashes of file chunks, which
   are used for deduplication, to the optimized Minio SHA-256 implementation.

   Depending on the CPU it improves the hashing throughput by 10-30%. Modern x86
   CPUs with the SHA Extension should be about two to three times faster.

   https://github.com/restic/restic/issues/551
   https://github.com/restic/restic/pull/2709

 * Enhancement #1570: Support specifying multiple host flags for various commands

   Previously commands didn't take more than one `--host` or `-H` argument into
   account, which could be limiting with e.g. the `forget` command.

   The `dump`, `find`, `forget`, `ls`, `mount`, `restore`, `snapshots`, `stats` and
   `tag` commands will now take into account multiple `--host` and `-H` flags.

   https://github.com/restic/restic/issues/1570

 * Enhancement #1680: Optimize `restic mount`

   We've optimized the FUSE implementation used within restic. `restic mount` is
   now more responsive and uses less memory.

   https://github.com/restic/restic/issues/1680
   https://github.com/restic/restic/pull/2587
   https://github.com/restic/restic/pull/2787

 * Enhancement #2072: Display snapshot date when using `restic find`

   Added the respective snapshot date to the output of `restic find`.

   https://github.com/restic/restic/issues/2072

 * Enhancement #2175: Allow specifying user and host when creating keys

   When adding a new key to the repository, the username and hostname for the new
   key can be specified on the command line. This allows overriding the defaults,
   for example if you would prefer to use the FQDN to identify the host or if you
   want to add keys for several different hosts without having to run the key add
   command on those hosts.

   https://github.com/restic/restic/issues/2175

 * Enhancement #2195: Simplify and improve restore performance

   Significantly improves restore performance of large files (i.e. 50M+):
   https://github.com/restic/restic/issues/2074
   https://forum.restic.net/t/restore-using-rclone-gdrive-backend-is-slow/1112/8
   https://forum.restic.net/t/degraded-restore-performance-s3-backend/1400

   Fixes "not enough cache capacity" error during restore:
   https://github.com/restic/restic/issues/2244

   NOTE: This new implementation does not guarantee order in which blobs are
   written to the target files and, for example, the last blob of a file can be
   written to the file before any of the preceding file blobs. It is therefore
   possible to have gaps in the data written to the target files if restore fails
   or interrupted by the user.

   The implementation will try to preallocate space for the restored files on the
   filesystem to prevent file fragmentation. This ensures good read performance for
   large files, like for example VM images. If preallocating space is not supported
   by the filesystem, then this step is silently skipped.

   https://github.com/restic/restic/pull/2195
   https://github.com/restic/restic/pull/2893

 * Enhancement #2277: Add support for ppc64le

   Adds support for ppc64le, the processor architecture from IBM.

   https://github.com/restic/restic/issues/2277

 * Enhancement #2328: Improve speed of check command

   We've improved the check command to traverse trees only once independent of
   whether they are contained in multiple snapshots. The check command is now much
   faster for repositories with a large number of snapshots.

   https://github.com/restic/restic/issues/2284
   https://github.com/restic/restic/pull/2328

 * Enhancement #2395: Ignore sync errors when operation not supported by local filesystem

   The local backend has been modified to work with filesystems which doesn't
   support the `sync` operation. This operation is normally used by restic to
   ensure that data files are fully written to disk before continuing.

   For these limited filesystems, saving a file in the backend would previously
   fail with an "operation not supported" error. This error is now ignored, which
   means that e.g. an SMB mount on macOS can now be used as storage location for a
   repository.

   https://github.com/restic/restic/issues/2395
   https://forum.restic.net/t/sync-errors-on-mac-over-smb/1859

 * Enhancement #2423: Support user@domain parsing as user

   Added the ability for user@domain-like users to be authenticated over SFTP
   servers.

   https://github.com/restic/restic/pull/2423

 * Enhancement #2427: Add flag `--iexclude-file` to backup command

   The backup command now supports the flag `--iexclude-file` which is a
   case-insensitive version of `--exclude-file`.

   https://github.com/restic/restic/issues/2427
   https://github.com/restic/restic/pull/2898

 * Enhancement #2569: Support excluding files by their size

   The `backup` command now supports the `--exclude-larger-than` option to exclude
   files which are larger than the specified maximum size. This can for example be
   useful to exclude unimportant files with a large file size.

   https://github.com/restic/restic/issues/2569
   https://github.com/restic/restic/pull/2914

 * Enhancement #2571: Self-heal missing file parts during backup of unchanged files

   We've improved the resilience of restic to certain types of repository
   corruption.

   For files that are unchanged since the parent snapshot, the backup command now
   verifies that all parts of the files still exist in the repository. Parts that
   are missing, e.g. from a damaged repository, are backed up again. This
   verification was already run for files that were modified since the parent
   snapshot, but is now also done for unchanged files.

   Note that restic will not backup file parts that are referenced in the index but
   where the actual data is not present on disk, as this situation can only be
   detected by restic check. Please ensure that you run `restic check` regularly.

   https://github.com/restic/restic/issues/2571
   https://github.com/restic/restic/pull/2827

 * Enhancement #2576: Improve the chunking algorithm

   We've updated the chunker library responsible for splitting files into smaller
   blocks. It should improve the chunking throughput by 5-15% depending on the CPU.

   https://github.com/restic/restic/issues/2820
   https://github.com/restic/restic/pull/2576
   https://github.com/restic/restic/pull/2845

 * Enhancement #2598: Improve speed of diff command

   We've improved the performance of the diff command when comparing snapshots with
   similar content. It should run up to twice as fast as before.

   https://github.com/restic/restic/pull/2598

 * Enhancement #2599: Slightly reduce memory usage of prune and stats commands

   The prune and the stats command kept directory identifiers in memory twice while
   searching for used blobs.

   https://github.com/restic/restic/pull/2599

 * Enhancement #2733: S3 backend: Add support for WebIdentityTokenFile

   We've added support for EKS IAM roles for service accounts feature to the S3
   backend.

   https://github.com/restic/restic/issues/2703
   https://github.com/restic/restic/pull/2733

 * Enhancement #2773: Optimize handling of new index entries

   Restic now uses less memory for backups which add a lot of data, e.g. large
   initial backups. In addition, we've improved the stability in some edge cases.

   https://github.com/restic/restic/pull/2773

 * Enhancement #2781: Reduce memory consumption of in-memory index

   We've improved how the index is stored in memory. This change can reduce memory
   usage for large repositories by up to 50% (depending on the operation).

   https://github.com/restic/restic/pull/2781
   https://github.com/restic/restic/pull/2812

 * Enhancement #2786: Optimize `list blobs` command

   We've changed the implementation of `list blobs` which should be now a bit
   faster and consume almost no memory even for large repositories.

   https://github.com/restic/restic/pull/2786

 * Enhancement #2790: Optimized file access in restic mount

   Reading large (> 100GiB) files from restic mountpoints is now faster, and the
   speedup is greater for larger files.

   https://github.com/restic/restic/pull/2790

 * Enhancement #2840: Speed-up file deletion in forget, prune and rebuild-index

   We've sped up the file deletion for the commands forget, prune and
   rebuild-index, especially for remote repositories. Deletion was sequential
   before and is now run in parallel.

   https://github.com/restic/restic/pull/2840

 * Enhancement #2858: Support filtering snapshots by tag and path in the stats command

   We've added filtering snapshots by `--tag tagList` and by `--path path` to the
   `stats` command. This includes filtering of only 'latest' snapshots or all
   snapshots in a repository.

   https://github.com/restic/restic/issues/2858
   https://github.com/restic/restic/pull/2859
   https://forum.restic.net/t/stats-for-a-host-and-filtered-snapshots/3020


# Changelog for restic 0.9.6 (2019-11-22)
The following sections list the changes in restic 0.9.6 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #2063: Allow absolute path for filename when backing up from stdin
 * Fix #2174: Save files with invalid timestamps
 * Fix #2249: Read fresh metadata for unmodified files
 * Fix #2301: Add upper bound for t in --read-data-subset=n/t
 * Fix #2321: Check errors when loading index files
 * Enh #2179: Use ctime when checking for file changes
 * Enh #2306: Allow multiple retries for interactive password input
 * Enh #2330: Make `--group-by` accept both singular and plural
 * Enh #2350: Add option to configure S3 region

## Details

 * Bugfix #2063: Allow absolute path for filename when backing up from stdin

   When backing up from stdin, handle directory path for `--stdin-filename`. This
   can be used to specify the full path for the backed-up file.

   https://github.com/restic/restic/issues/2063

 * Bugfix #2174: Save files with invalid timestamps

   When restic reads invalid timestamps (year is before 0000 or after 9999) it
   refused to read and archive the file. We've changed the behavior and will now
   save modified timestamps with the year set to either 0000 or 9999, the rest of
   the timestamp stays the same, so the file will be saved (albeit with a bogus
   timestamp).

   https://github.com/restic/restic/issues/2174
   https://github.com/restic/restic/issues/1173

 * Bugfix #2249: Read fresh metadata for unmodified files

   Restic took all metadata for files which were detected as unmodified, not taking
   into account changed metadata (ownership, mode). This is now corrected.

   https://github.com/restic/restic/issues/2249
   https://github.com/restic/restic/pull/2252

 * Bugfix #2301: Add upper bound for t in --read-data-subset=n/t

   256 is the effective maximum for t, but restic would allow larger values,
   leading to strange behavior.

   https://github.com/restic/restic/issues/2301
   https://github.com/restic/restic/pull/2304

 * Bugfix #2321: Check errors when loading index files

   Restic now checks and handles errors which occur when loading index files, the
   missing check leads to odd errors (and a stack trace printed to users) later.
   This was reported in the forum.

   https://github.com/restic/restic/pull/2321
   https://forum.restic.net/t/check-rebuild-index-prune/1848/13

 * Enhancement #2179: Use ctime when checking for file changes

   Previously, restic only checked a file's mtime (along with other non-timestamp
   metadata) to decide if a file has changed. This could cause restic to not notice
   that a file has changed (and therefore continue to store the old version, as
   opposed to the modified version) if something edits the file and then resets the
   timestamp. Restic now also checks the ctime of files, so any modifications to a
   file should be noticed, and the modified file will be backed up. The ctime check
   will be disabled if the --ignore-inode flag was given.

   If this change causes problems for you, please open an issue, and we can look in
   to adding a separate flag to disable just the ctime check.

   https://github.com/restic/restic/issues/2179
   https://github.com/restic/restic/pull/2212

 * Enhancement #2306: Allow multiple retries for interactive password input

   Restic used to quit if the repository password was typed incorrectly once.
   Restic will now ask the user again for the repository password if typed
   incorrectly. The user will now get three tries to input the correct password
   before restic quits.

   https://github.com/restic/restic/issues/2306

 * Enhancement #2330: Make `--group-by` accept both singular and plural

   One can now use the values `host`/`hosts`, `path`/`paths` and `tag` / `tags`
   interchangeably in the `--group-by` argument.

   https://github.com/restic/restic/issues/2330

 * Enhancement #2350: Add option to configure S3 region

   We've added a new option for setting the region when accessing an S3-compatible
   service. For some providers, it is required to set this to a valid value. You
   can do that either by setting the environment variable `AWS_DEFAULT_REGION` or
   using the option `s3.region`, e.g. like this: `-o s3.region="us-east-1"`.

   https://github.com/restic/restic/pull/2350


# Changelog for restic 0.9.5 (2019-04-23)
The following sections list the changes in restic 0.9.5 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #2135: Return error when no bytes could be read from stdin
 * Fix #2181: Don't cancel timeout after 30 seconds for self-update
 * Fix #2203: Fix reading passwords from stdin
 * Fix #2224: Don't abort the find command when a tree can't be loaded
 * Enh #1895: Add case insensitive include & exclude options
 * Enh #1937: Support streaming JSON output for backup
 * Enh #2037: Add group-by option to snapshots command
 * Enh #2124: Ability to dump folders to tar via stdout
 * Enh #2139: Return error if no bytes could be read for `backup --stdin`
 * Enh #2155: Add Openstack application credential auth for Swift
 * Enh #2184: Add --json support to forget command
 * Enh #2205: Add --ignore-inode option to backup cmd
 * Enh #2220: Add config option to set S3 storage class

## Details

 * Bugfix #2135: Return error when no bytes could be read from stdin

   We assume that users reading backup data from stdin want to know when no data
   could be read, so now restic returns an error when `backup --stdin` is called
   but no bytes could be read. Usually, this means that an earlier command in a
   pipe has failed. The documentation was amended and now recommends setting the
   `pipefail` option (`set -o pipefail`).

   https://github.com/restic/restic/pull/2135
   https://github.com/restic/restic/pull/2139

 * Bugfix #2181: Don't cancel timeout after 30 seconds for self-update

   https://github.com/restic/restic/issues/2181

 * Bugfix #2203: Fix reading passwords from stdin

   Passwords for the `init`, `key add`, and `key passwd` commands can now be read
   from non-terminal stdin.

   https://github.com/restic/restic/issues/2203

 * Bugfix #2224: Don't abort the find command when a tree can't be loaded

   Change the find command so that missing trees don't result in a crash. Instead,
   the error is logged to the debug log, and the tree ID is displayed along with
   the snapshot it belongs to. This makes it possible to recover repositories that
   are missing trees by forgetting the snapshots they are used in.

   https://github.com/restic/restic/issues/2224

 * Enhancement #1895: Add case insensitive include & exclude options

   The backup and restore commands now have --iexclude and --iinclude flags as case
   insensitive variants of --exclude and --include.

   https://github.com/restic/restic/issues/1895
   https://github.com/restic/restic/pull/2032

 * Enhancement #1937: Support streaming JSON output for backup

   We've added support for getting machine-readable status output during backup,
   just pass the flag `--json` for `restic backup` and restic will output a stream
   of JSON objects which contain the current progress.

   https://github.com/restic/restic/issues/1937
   https://github.com/restic/restic/pull/1944

 * Enhancement #2037: Add group-by option to snapshots command

   We have added an option to group the output of the snapshots command, similar to
   the output of the forget command. The option has been called "--group-by" and
   accepts any combination of the values "host", "paths" and "tags", separated by
   commas. Default behavior (not specifying --group-by) has not been changed. We
   have added support of the grouping to the JSON output.

   https://github.com/restic/restic/issues/2037
   https://github.com/restic/restic/pull/2087

 * Enhancement #2124: Ability to dump folders to tar via stdout

   We've added the ability to dump whole folders to stdout via the `dump` command.
   Restic now requires at least Go 1.10 due to a limitation of the standard library
   for Go <= 1.9.

   https://github.com/restic/restic/issues/2123
   https://github.com/restic/restic/pull/2124

 * Enhancement #2139: Return error if no bytes could be read for `backup --stdin`

   When restic is used to backup the output of a program, like `mysqldump | restic
   backup --stdin`, it now returns an error if no bytes could be read at all. This
   catches the failure case when `mysqldump` failed for some reason and did not
   output any data to stdout.

   https://github.com/restic/restic/pull/2139

 * Enhancement #2155: Add Openstack application credential auth for Swift

   Since Openstack Queens Identity (auth V3) service supports an application
   credential auth method. It allows to create a technical account with the limited
   roles. This commit adds an application credential authentication method for the
   Swift backend.

   https://github.com/restic/restic/issues/2155

 * Enhancement #2184: Add --json support to forget command

   The forget command now supports the --json argument, outputting the information
   about what is (or would-be) kept and removed from the repository.

   https://github.com/restic/restic/issues/2184
   https://github.com/restic/restic/pull/2185

 * Enhancement #2205: Add --ignore-inode option to backup cmd

   This option handles backup of virtual filesystems that do not keep fixed inodes
   for files, like Fuse-based, pCloud, etc. Ignoring inode changes allows to
   consider the file as unchanged if last modification date and size are unchanged.

   https://github.com/restic/restic/issues/1631
   https://github.com/restic/restic/pull/2205
   https://github.com/restic/restic/pull/2047

 * Enhancement #2220: Add config option to set S3 storage class

   The `s3.storage-class` option can be passed to restic (using `-o`) to specify
   the storage class to be used for S3 objects created by restic.

   The storage class is passed as-is to S3, so it needs to be understood by the
   API. On AWS, it can be one of `STANDARD`, `STANDARD_IA`, `ONEZONE_IA`,
   `INTELLIGENT_TIERING` and `REDUCED_REDUNDANCY`. If unspecified, the default
   storage class is used (`STANDARD` on AWS).

   You can mix storage classes in the same bucket, and the setting isn't stored in
   the restic repository, so be sure to specify it with each command that writes to
   S3.

   https://github.com/restic/restic/issues/706
   https://github.com/restic/restic/pull/2220


# Changelog for restic 0.9.4 (2019-01-06)
The following sections list the changes in restic 0.9.4 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1989: Google Cloud Storage: Respect bandwidth limit
 * Fix #2040: Add host name filter shorthand flag for `stats` command
 * Fix #2068: Correctly return error loading data
 * Fix #2095: Consistently use local time for snapshots times
 * Enh #1605: Concurrent restore
 * Enh #2017: Mount: Enforce FUSE Unix permissions with allow-other
 * Enh #2070: Make all commands display timestamps in local time
 * Enh #2085: Allow --files-from to be specified multiple times
 * Enh #2089: Increase granularity of the "keep within" retention policy
 * Enh #2094: Run command to get password
 * Enh #2097: Add key hinting

## Details

 * Bugfix #1989: Google Cloud Storage: Respect bandwidth limit

   The GCS backend did not respect the bandwidth limit configured, a previous
   commit accidentally removed support for it.

   https://github.com/restic/restic/issues/1989
   https://github.com/restic/restic/pull/2100

 * Bugfix #2040: Add host name filter shorthand flag for `stats` command

   The default value for `--host` flag was set to 'H' (the shorthand version of the
   flag), this caused the lookup for the latest snapshot to fail.

   Add shorthand flag `-H` for `--host` (with empty default so if these flags are
   not specified the latest snapshot will not filter by host name).

   Also add shorthand `-H` for `backup` command.

   https://github.com/restic/restic/issues/2040

 * Bugfix #2068: Correctly return error loading data

   In one case during `prune` and `check`, an error loading data from the backend
   is not returned properly. This is now corrected.

   https://github.com/restic/restic/issues/1999#issuecomment-433737921
   https://github.com/restic/restic/pull/2068

 * Bugfix #2095: Consistently use local time for snapshots times

   By default snapshots created with restic backup were set to local time, but when
   the --time flag was used the provided timestamp was parsed as UTC. With this
   change all snapshots times are set to local time.

   https://github.com/restic/restic/pull/2095

 * Enhancement #1605: Concurrent restore

   This change significantly improves restore performance, especially when using
   high-latency remote repositories like B2.

   The implementation now uses several concurrent threads to download and process
   multiple remote files concurrently. To further reduce restore time, each remote
   file is downloaded using a single repository request.

   https://github.com/restic/restic/issues/1605
   https://github.com/restic/restic/pull/1719

 * Enhancement #2017: Mount: Enforce FUSE Unix permissions with allow-other

   The fuse mount (`restic mount`) now lets the kernel check the permissions of the
   files within snapshots (this is done through the `DefaultPermissions` FUSE
   option) when the option `--allow-other` is specified.

   To restore the old behavior, we've added the `--no-default-permissions` option.
   This allows all users that have access to the mount point to access all files
   within the snapshots.

   https://github.com/restic/restic/pull/2017

 * Enhancement #2070: Make all commands display timestamps in local time

   Restic used to drop the timezone information from displayed timestamps, it now
   converts timestamps to local time before printing them so the times can be
   easily compared to.

   https://github.com/restic/restic/pull/2070

 * Enhancement #2085: Allow --files-from to be specified multiple times

   Before, restic took only the last file specified with `--files-from` into
   account, this is now corrected.

   https://github.com/restic/restic/issues/2085
   https://github.com/restic/restic/pull/2086

 * Enhancement #2089: Increase granularity of the "keep within" retention policy

   The `keep-within` option of the `forget` command now accepts time ranges with an
   hourly granularity. For example, running `restic forget --keep-within 3d12h`
   will keep all the snapshots made within three days and twelve hours from the
   time of the latest snapshot.

   https://github.com/restic/restic/issues/2089
   https://github.com/restic/restic/pull/2090

 * Enhancement #2094: Run command to get password

   We've added the `--password-command` option which allows specifying a command
   that restic runs every time the password for the repository is needed, so it can
   be integrated with a password manager or keyring. The option can also be set via
   the environment variable `$RESTIC_PASSWORD_COMMAND`.

   https://github.com/restic/restic/pull/2094

 * Enhancement #2097: Add key hinting

   Added a new option `--key-hint` and corresponding environment variable
   `RESTIC_KEY_HINT`. The key hint is a key ID to try decrypting first, before
   other keys in the repository.

   This change will benefit repositories with many keys; if the correct key hint is
   supplied then restic only needs to check one key. If the key hint is incorrect
   (the key does not exist, or the password is incorrect) then restic will check
   all keys, as usual.

   https://github.com/restic/restic/issues/2097


# Changelog for restic 0.9.3 (2018-10-13)
The following sections list the changes in restic 0.9.3 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1935: Remove truncated files from cache
 * Fix #1978: Do not return an error when the scanner is slower than backup
 * Enh #1766: Restore: suppress lchown errors when not running as root
 * Enh #1777: Improve the `find` command
 * Enh #1876: Display reason why forget keeps snapshots
 * Enh #1891: Accept glob in paths loaded via --files-from
 * Enh #1909: Reject files/dirs by name first
 * Enh #1920: Vendor dependencies with Go 1.11 Modules
 * Enh #1940: Add directory filter to ls command
 * Enh #1949: Add new command `self-update`
 * Enh #1953: Ls: Add JSON output support for restic ls cmd
 * Enh #1962: Stream JSON output for ls command
 * Enh #1967: Use `--host` everywhere
 * Enh #2028: Display size of cache directories

## Details

 * Bugfix #1935: Remove truncated files from cache

   When a file in the local cache is truncated, and restic tries to access data
   beyond the end of the (cached) file, it used to return an error "EOF". This is
   now fixed, such truncated files are removed and the data is fetched directly
   from the backend.

   https://github.com/restic/restic/issues/1935

 * Bugfix #1978: Do not return an error when the scanner is slower than backup

   When restic makes a backup, there's a background task called "scanner" which
   collects information on how many files and directories are to be saved, in order
   to display progress information to the user. When the backup finishes faster
   than the scanner, it is aborted because the result is not needed any more. This
   logic contained a bug, where quitting the scanner process was treated as an
   error, and caused restic to print an unhelpful error message ("context
   canceled").

   https://github.com/restic/restic/issues/1978
   https://github.com/restic/restic/pull/1991

 * Enhancement #1766: Restore: suppress lchown errors when not running as root

   Like "cp" and "rsync" do, restic now only reports errors for changing the
   ownership of files during restore if it is run ￼as root, on non-Windows
   operating systems. On Windows, the error is reported as usual.

   https://github.com/restic/restic/issues/1766

 * Enhancement #1777: Improve the `find` command

   We've updated the `find` command to support multiple patterns.

   `restic find` is now able to list the snapshots containing a specific tree or
   blob, or even the snapshots that contain blobs belonging to a given pack. A list
   of IDs can be given, as long as they all have the same type.

   The command `find` can also display the pack IDs the blobs belong to, if the
   `--show-pack-id` flag is provided.

   https://github.com/restic/restic/issues/1777
   https://github.com/restic/restic/pull/1780

 * Enhancement #1876: Display reason why forget keeps snapshots

   We've added a column to the list of snapshots `forget` keeps which details the
   reasons to keep a particular snapshot. This makes debugging policies for forget
   much easier. Please remember to always try things out with `--dry-run`!

   https://github.com/restic/restic/pull/1876

 * Enhancement #1891: Accept glob in paths loaded via --files-from

   Before that, behaviour was different if paths were appended to command line or
   from a file, because wild card characters were expanded by shell if appended to
   command line, but not expanded if loaded from file.

   https://github.com/restic/restic/issues/1891

 * Enhancement #1909: Reject files/dirs by name first

   The current scanner/archiver code had an architectural limitation: it always ran
   the `lstat()` system call on all files and directories before a decision to
   include/exclude the file/dir was made. This lead to a lot of unnecessary system
   calls for items that could have been rejected by their name or path only.

   We've changed the archiver/scanner implementation so that it now first rejects
   by name/path, and only runs the system call on the remaining items. This reduces
   the number of `lstat()` system calls a lot (depending on the exclude settings).

   https://github.com/restic/restic/issues/1909
   https://github.com/restic/restic/pull/1912

 * Enhancement #1920: Vendor dependencies with Go 1.11 Modules

   Until now, we've used `dep` for managing dependencies, we've now switch to using
   Go modules. For users this does not change much, only if you want to compile
   restic without downloading anything with Go 1.11, then you need to run: `go
   build -mod=vendor build.go`

   https://github.com/restic/restic/pull/1920

 * Enhancement #1940: Add directory filter to ls command

   The ls command can now be filtered by directories, so that only files in the
   given directories will be shown. If the --recursive flag is specified, then ls
   will traverse subfolders and list their files as well.

   It used to be possible to specify multiple snapshots, but that has been replaced
   by only one snapshot and the possibility of specifying multiple directories.

   Specifying directories constrains the walk, which can significantly speed up the
   listing.

   https://github.com/restic/restic/issues/1940
   https://github.com/restic/restic/pull/1941

 * Enhancement #1949: Add new command `self-update`

   We have added a new command called `self-update` which downloads the latest
   released version of restic from GitHub and replaces the current binary with it.
   It does not rely on any external program (so it'll work everywhere), but still
   verifies the GPG signature using the embedded GPG public key.

   By default, the `self-update` command is hidden behind the `selfupdate` built
   tag, which is only set when restic is built using `build.go` (including official
   releases). The reason for this is that downstream distributions will then not
   include the command by default, so users are encouraged to use the
   platform-specific distribution mechanism.

   https://github.com/restic/restic/pull/1949

 * Enhancement #1953: Ls: Add JSON output support for restic ls cmd

   We've implemented listing files in the repository with JSON as output, just pass
   `--json` as an option to `restic ls`. This makes the output of the command
   machine readable.

   https://github.com/restic/restic/pull/1953

 * Enhancement #1962: Stream JSON output for ls command

   The `ls` command now supports JSON output with the global `--json` flag, and
   this change streams out JSON messages one object at a time rather than en entire
   array buffered in memory before encoding. The advantage is it allows large
   listings to be handled efficiently.

   Two message types are printed: snapshots and nodes. A snapshot object will
   precede node objects which belong to that snapshot. The `struct_type` field can
   be used to determine which kind of message an object is.

   https://github.com/restic/restic/pull/1962

 * Enhancement #1967: Use `--host` everywhere

   We now use the flag `--host` for all commands which need a host name, using
   `--hostname` (e.g. for `restic backup`) still works, but will print a
   deprecation warning. Also, add the short option `-H` where possible.

   https://github.com/restic/restic/issues/1967

 * Enhancement #2028: Display size of cache directories

   The `cache` command now by default shows the size of the individual cache
   directories. It can be disabled with `--no-size`.

   https://github.com/restic/restic/issues/2028
   https://github.com/restic/restic/pull/2033


# Changelog for restic 0.9.2 (2018-08-06)
The following sections list the changes in restic 0.9.2 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1854: Allow saving files/dirs on different fs with `--one-file-system`
 * Fix #1861: Fix case-insensitive search with restic find
 * Fix #1870: Fix restore with --include
 * Fix #1880: Use `--cache-dir` argument for `check` command
 * Fix #1893: Return error when exclude file cannot be read
 * Enh #874: Add stats command to get information about a repository
 * Enh #1477: S3 backend: accept AWS_SESSION_TOKEN
 * Enh #1772: Add restore --verify to verify restored file content
 * Enh #1853: Add JSON output support to `restic key list`
 * Enh #1901: Update the Backblaze B2 library
 * Enh #1906: Add support for B2 application keys

## Details

 * Bugfix #1854: Allow saving files/dirs on different fs with `--one-file-system`

   Restic now allows saving files/dirs on a different file system in a subdir
   correctly even when `--one-file-system` is specified.

   The first thing the restic archiver code does is to build a tree of the target
   files/directories. If it detects that a parent directory is already included
   (e.g. `restic backup /foo /foo/bar/baz`), it'll ignore the latter argument.

   Without `--one-file-system`, that's perfectly valid: If `/foo` is to be
   archived, it will include `/foo/bar/baz`. But with `--one-file-system`,
   `/foo/bar/baz` may reside on a different file system, so it won't be included
   with `/foo`.

   https://github.com/restic/restic/issues/1854
   https://github.com/restic/restic/pull/1855

 * Bugfix #1861: Fix case-insensitive search with restic find

   We've fixed the behavior for `restic find -i PATTERN`, which was broken in
   v0.9.1.

   https://github.com/restic/restic/pull/1861

 * Bugfix #1870: Fix restore with --include

   We fixed a bug which prevented restic to restore files with an include filter.

   https://github.com/restic/restic/issues/1870
   https://github.com/restic/restic/pull/1900

 * Bugfix #1880: Use `--cache-dir` argument for `check` command

   `check` command now uses a temporary sub-directory of the specified directory if
   set using the `--cache-dir` argument. If not set, the cache directory is created
   in the default temporary directory as before. In either case a temporary cache
   is used to ensure the actual repository is checked (rather than a local copy).

   The `--cache-dir` argument was not used by the `check` command, instead a cache
   directory was created in the temporary directory.

   https://github.com/restic/restic/issues/1880

 * Bugfix #1893: Return error when exclude file cannot be read

   A bug was found: when multiple exclude files were passed to restic and one of
   them could not be read, an error was printed and restic continued, ignoring even
   the existing exclude files. Now, an error message is printed and restic aborts
   when an exclude file cannot be read.

   https://github.com/restic/restic/issues/1893

 * Enhancement #874: Add stats command to get information about a repository

   https://github.com/restic/restic/issues/874
   https://github.com/restic/restic/pull/1729

 * Enhancement #1477: S3 backend: accept AWS_SESSION_TOKEN

   Before, it was not possible to use s3 backend with AWS temporary security
   credentials(with AWS_SESSION_TOKEN). This change gives higher priority to
   credentials.EnvAWS credentials provider.

   https://github.com/restic/restic/issues/1477
   https://github.com/restic/restic/pull/1479
   https://github.com/restic/restic/pull/1647

 * Enhancement #1772: Add restore --verify to verify restored file content

   Restore will print error message if restored file content does not match
   expected SHA256 checksum

   https://github.com/restic/restic/pull/1772

 * Enhancement #1853: Add JSON output support to `restic key list`

   This PR enables users to get the output of `restic key list` in JSON in addition
   to the existing table format.

   https://github.com/restic/restic/pull/1853

 * Enhancement #1901: Update the Backblaze B2 library

   We've updated the library we're using for accessing the Backblaze B2 service to
   0.5.0 to include support for upcoming so-called "application keys". With this
   feature, you can create access credentials for B2 which are restricted to e.g. a
   single bucket or even a sub-directory of a bucket.

   https://github.com/restic/restic/pull/1901
   https://github.com/kurin/blazer

 * Enhancement #1906: Add support for B2 application keys

   Restic can now use so-called "application keys" which can be created in the B2
   dashboard and were only introduced recently. In contrast to the "master key",
   such keys can be restricted to a specific bucket and/or path.

   https://github.com/restic/restic/issues/1906
   https://github.com/restic/restic/pull/1914


# Changelog for restic 0.9.1 (2018-06-10)
The following sections list the changes in restic 0.9.1 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1801: Add limiting bandwidth to the rclone backend
 * Fix #1822: Allow uploading large files to MS Azure
 * Fix #1825: Correct `find` to not skip snapshots
 * Fix #1833: Fix caching files on error
 * Fix #1834: Resolve deadlock

## Details

 * Bugfix #1801: Add limiting bandwidth to the rclone backend

   The rclone backend did not respect `--limit-upload` or `--limit-download`.
   Oftentimes it's not necessary to use this, as the limiting in rclone itself
   should be used because it gives much better results, but in case a remote
   instance of rclone is used (e.g. called via ssh), it is still relevant to limit
   the bandwidth from restic to rclone.

   https://github.com/restic/restic/issues/1801

 * Bugfix #1822: Allow uploading large files to MS Azure

   Sometimes, restic creates files to be uploaded to the repository which are quite
   large, e.g. when saving directories with many entries or very large files. The
   MS Azure API does not allow uploading files larger that 256MiB directly, rather
   restic needs to upload them in blocks of 100MiB. This is now implemented.

   https://github.com/restic/restic/issues/1822

 * Bugfix #1825: Correct `find` to not skip snapshots

   Under certain circumstances, the `find` command was found to skip snapshots
   containing directories with files to look for when the directories haven't been
   modified at all, and were already printed as part of a different snapshot. This
   is now corrected.

   In addition, we've switched to our own matching/pattern implementation, so now
   things like `restic find "/home/user/foo/**/main.go"` are possible.

   https://github.com/restic/restic/issues/1825
   https://github.com/restic/restic/issues/1823

 * Bugfix #1833: Fix caching files on error

   During `check` it may happen that different threads access the same file in the
   backend, which is then downloaded into the cache only once. When that fails,
   only the thread which is responsible for downloading the file signals the
   correct error. The other threads just assume that the file has been downloaded
   successfully and then get an error when they try to access the cached file.

   https://github.com/restic/restic/issues/1833

 * Bugfix #1834: Resolve deadlock

   When the "scanning" process restic runs to find out how much data there is does
   not finish before the backup itself is done, restic stops doing anything. This
   is resolved now.

   https://github.com/restic/restic/issues/1834
   https://github.com/restic/restic/pull/1835


# Changelog for restic 0.9.0 (2018-05-21)
The following sections list the changes in restic 0.9.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1608: Respect time stamp for new backup when reading from stdin
 * Fix #1652: Ignore/remove invalid lock files
 * Fix #1684: Fix backend tests for rest-server
 * Fix #1730: Ignore sockets for restore
 * Fix #1745: Correctly parse the argument to --tls-client-cert
 * Enh #549: Rework archiver code
 * Enh #827: Add --new-password-file flag for non-interactive password changes
 * Enh #1433: Support UTF-16 encoding and process Byte Order Mark
 * Enh #1477: Accept AWS_SESSION_TOKEN for the s3 backend
 * Enh #1552: Use Google Application Default credentials
 * Enh #1561: Allow using rclone to access other services
 * Enh #1648: Ignore AWS permission denied error when creating a repository
 * Enh #1649: Add illumos/Solaris support
 * Enh #1665: Improve cache handling for `restic check`
 * Enh #1709: Improve messages `restic check` prints
 * Enh #1721: Add `cache` command to list cache dirs
 * Enh #1735: Allow keeping a time range of snapshots
 * Enh #1758: Allow saving OneDrive folders in Windows
 * Enh #1782: Use default AWS credentials chain for S3 backend

## Details

 * Bugfix #1608: Respect time stamp for new backup when reading from stdin

   When reading backups from stdin (via `restic backup --stdin`), restic now uses
   the time stamp for the new backup passed in `--time`.

   https://github.com/restic/restic/issues/1608
   https://github.com/restic/restic/pull/1703

 * Bugfix #1652: Ignore/remove invalid lock files

   This corrects a bug introduced recently: When an invalid lock file in the repo
   is encountered (e.g. if the file is empty), the code used to ignore that, but
   now returns the error. Now, invalid files are ignored for the normal lock check,
   and removed when `restic unlock --remove-all` is run.

   https://github.com/restic/restic/issues/1652
   https://github.com/restic/restic/pull/1653

 * Bugfix #1684: Fix backend tests for rest-server

   The REST server for restic now requires an explicit parameter (`--no-auth`) if
   no authentication should be allowed. This is fixed in the tests.

   https://github.com/restic/restic/pull/1684

 * Bugfix #1730: Ignore sockets for restore

   We've received a report and correct the behavior in which the restore code
   aborted restoring a directory when a socket was encountered. Unix domain socket
   files cannot be restored (they are created on the fly once a process starts
   listening). The error handling was corrected, and in addition we're now ignoring
   sockets during restore.

   https://github.com/restic/restic/issues/1730
   https://github.com/restic/restic/pull/1731

 * Bugfix #1745: Correctly parse the argument to --tls-client-cert

   Previously, the --tls-client-cert method attempt to read ARGV[1] (hardcoded)
   instead of the argument that was passed to it. This has been corrected.

   https://github.com/restic/restic/issues/1745
   https://github.com/restic/restic/pull/1746

 * Enhancement #549: Rework archiver code

   The core archiver code and the complementary code for the `backup` command was
   rewritten completely. This resolves very annoying issues such as 549. The first
   backup with this release of restic will likely result in all files being re-read
   locally, so it will take a lot longer. The next backup after that will be fast
   again.

   Basically, with the old code, restic took the last path component of each
   to-be-saved file or directory as the top-level file/directory within the
   snapshot. This meant that when called as `restic backup /home/user/foo`, the
   snapshot would contain the files in the directory `/home/user/foo` as `/foo`.

   This is not the case any more with the new archiver code. Now, restic works very
   similar to what `tar` does: When restic is called with an absolute path to save,
   then it'll preserve the directory structure within the snapshot. For the example
   above, the snapshot would contain the files in the directory within
   `/home/user/foo` in the snapshot. For relative directories, it only preserves
   the relative path components. So `restic backup user/foo` will save the files as
   `/user/foo` in the snapshot.

   While we were at it, the status display and notification system was completely
   rewritten. By default, restic now shows which files are currently read (unless
   `--quiet` is specified) in a multi-line status display.

   The `backup` command also gained a new option: `--verbose`. It can be specified
   once (which prints a bit more detail what restic is doing) or twice (which
   prints a line for each file/directory restic encountered, together with some
   statistics).

   Another issue that was resolved is the new code only reads two files at most.
   The old code would read way too many files in parallel, thereby slowing down the
   backup process on spinning discs a lot.

   https://github.com/restic/restic/issues/549
   https://github.com/restic/restic/issues/1286
   https://github.com/restic/restic/issues/446
   https://github.com/restic/restic/issues/1344
   https://github.com/restic/restic/issues/1416
   https://github.com/restic/restic/issues/1456
   https://github.com/restic/restic/issues/1145
   https://github.com/restic/restic/issues/1160
   https://github.com/restic/restic/pull/1494

 * Enhancement #827: Add --new-password-file flag for non-interactive password changes

   This makes it possible to change a repository password without being prompted.

   https://github.com/restic/restic/issues/827
   https://github.com/restic/restic/pull/1720
   https://forum.restic.net/t/changing-repo-password-without-prompt/591

 * Enhancement #1433: Support UTF-16 encoding and process Byte Order Mark

   On Windows, text editors commonly leave a Byte Order Mark at the beginning of
   the file to define which encoding is used (oftentimes UTF-16). We've added code
   to support processing the BOMs in text files, like the exclude files, the
   password file and the file passed via `--files-from`. This does not apply to any
   file being saved in a backup, those are not touched and archived as they are.

   https://github.com/restic/restic/issues/1433
   https://github.com/restic/restic/issues/1738
   https://github.com/restic/restic/pull/1748

 * Enhancement #1477: Accept AWS_SESSION_TOKEN for the s3 backend

   Before, it was not possible to use s3 backend with AWS temporary security
   credentials(with AWS_SESSION_TOKEN). This change gives higher priority to
   credentials.EnvAWS credentials provider.

   https://github.com/restic/restic/issues/1477
   https://github.com/restic/restic/pull/1479
   https://github.com/restic/restic/pull/1647

 * Enhancement #1552: Use Google Application Default credentials

   Google provide libraries to generate appropriate credentials with various
   fallback sources. This change uses the library to generate our GCS client, which
   allows us to make use of these extra methods.

   This should be backward compatible with previous restic behaviour while adding
   the additional capabilities to auth from Google's internal metadata endpoints.
   For users running restic in GCP this can make authentication far easier than it
   was before.

   https://github.com/restic/restic/pull/1552
   https://developers.google.com/identity/protocols/application-default-credentials

 * Enhancement #1561: Allow using rclone to access other services

   We've added the ability to use rclone to store backup data on all backends that
   it supports. This was done in collaboration with Nick, the author of rclone. You
   can now use it to first configure a service, then restic manages the rest
   (starting and stopping rclone). For details, please see the manual.

   https://github.com/restic/restic/issues/1561
   https://github.com/restic/restic/pull/1657
   https://rclone.org

 * Enhancement #1648: Ignore AWS permission denied error when creating a repository

   It's not possible to use s3 backend scoped to a subdirectory(with specific
   permissions). Restic doesn't try to create repository in a subdirectory, when
   'bucket exists' of parent directory check fails due to permission issues.

   https://github.com/restic/restic/pull/1648

 * Enhancement #1649: Add illumos/Solaris support

   https://github.com/restic/restic/pull/1649

 * Enhancement #1665: Improve cache handling for `restic check`

   For safety reasons, restic does not use a local metadata cache for the `restic
   check` command, so that data is loaded from the repository and restic can check
   it's in good condition. When the cache is disabled, restic will fetch each tiny
   blob needed for checking the integrity using a separate backend request. For
   non-local backends, that will take a long time, and depending on the backend
   (e.g. B2) may also be much more expensive.

   This PR adds a few commits which will change the behavior as follows:

   * When `restic check` is called without any additional parameters, it will build
   a new cache in a temporary directory, which is removed at the end of the check.
   This way, we'll get readahead for metadata files (so restic will fetch the whole
   file when the first blob from the file is requested), but all data is freshly
   fetched from the storage backend. This is the default behavior and will work for
   almost all users.

   * When `restic check` is called with `--with-cache`, the default on-disc cache
   is used. This behavior hasn't changed since the cache was introduced.

   * When `--no-cache` is specified, restic falls back to the old behavior, and
   read all tiny blobs in separate requests.

   https://github.com/restic/restic/issues/1665
   https://github.com/restic/restic/issues/1694
   https://github.com/restic/restic/pull/1696

 * Enhancement #1709: Improve messages `restic check` prints

   Some messages `restic check` prints are not really errors, so from now on restic
   does not treat them as errors any more and exits cleanly.

   https://github.com/restic/restic/pull/1709
   https://forum.restic.net/t/what-is-the-standard-procedure-to-follow-if-a-backup-or-restore-is-interrupted/571/2

 * Enhancement #1721: Add `cache` command to list cache dirs

   The command `cache` was added, it allows listing restic's cache directoriers
   together with the last usage. It also allows removing old cache dirs without
   having to access a repo, via `restic cache --cleanup`

   https://github.com/restic/restic/issues/1721
   https://github.com/restic/restic/pull/1749

 * Enhancement #1735: Allow keeping a time range of snapshots

   We've added the `--keep-within` option to the `forget` command. It instructs
   restic to keep all snapshots within the given duration since the newest
   snapshot. For example, running `restic forget --keep-within 5m7d` will keep all
   snapshots which have been made in the five months and seven days since the
   latest snapshot.

   https://github.com/restic/restic/pull/1735

 * Enhancement #1758: Allow saving OneDrive folders in Windows

   Restic now contains a bugfix to two libraries, which allows saving OneDrive
   folders in Windows. In order to use the newer versions of the libraries, the
   minimal version required to compile restic is now Go 1.9.

   https://github.com/restic/restic/issues/1758
   https://github.com/restic/restic/pull/1765

 * Enhancement #1782: Use default AWS credentials chain for S3 backend

   Adds support for file credentials to the S3 backend (e.g. ~/.aws/credentials),
   and reorders the credentials chain for the S3 backend to match AWS's standard,
   which is static credentials, env vars, credentials file, and finally remote.

   https://github.com/restic/restic/pull/1782


# Changelog for restic 0.8.3 (2018-02-26)
The following sections list the changes in restic 0.8.3 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1633: Fixed unexpected 'pack file cannot be listed' error
 * Fix #1638: Handle errors listing files in the backend
 * Fix #1641: Ignore files with invalid names in the repo
 * Enh #1497: Add --read-data-subset flag to check command
 * Enh #1560: Retry all repository file download errors
 * Enh #1623: Don't check for presence of files in the backend before writing
 * Enh #1634: Upgrade B2 client library, reduce HTTP requests

## Details

 * Bugfix #1633: Fixed unexpected 'pack file cannot be listed' error

   Due to a regression introduced in 0.8.2, the `rebuild-index` and `prune`
   commands failed to read pack files with size of 587, 588, 589 or 590 bytes.

   https://github.com/restic/restic/issues/1633
   https://github.com/restic/restic/pull/1635

 * Bugfix #1638: Handle errors listing files in the backend

   A user reported in the forum that restic completes a backup although a
   concurrent `prune` operation was running. A few error messages were printed, but
   the backup was attempted and completed successfully. No error code was returned.

   This should not happen: The repository is exclusively locked during `prune`, so
   when `restic backup` is run in parallel, it should abort and return an error
   code instead.

   It was found that the bug was in the code introduced only recently, which
   retries a List() operation on the backend should that fail. It is now corrected.

   https://github.com/restic/restic/pull/1638
   https://forum.restic.net/t/restic-backup-returns-0-exit-code-when-already-locked/484

 * Bugfix #1641: Ignore files with invalid names in the repo

   The release 0.8.2 introduced a bug: when restic encounters files in the repo
   which do not have a valid name, it tries to load a file with a name of lots of
   zeroes instead of ignoring it. This is now resolved, invalid file names are just
   ignored.

   https://github.com/restic/restic/issues/1641
   https://github.com/restic/restic/pull/1643
   https://forum.restic.net/t/help-fixing-repo-no-such-file/485/3

 * Enhancement #1497: Add --read-data-subset flag to check command

   This change introduces ability to check integrity of a subset of repository data
   packs. This can be used to spread integrity check of larger repositories over a
   period of time.

   https://github.com/restic/restic/issues/1497
   https://github.com/restic/restic/pull/1556

 * Enhancement #1560: Retry all repository file download errors

   Restic will now retry failed downloads, similar to other operations.

   https://github.com/restic/restic/pull/1560

 * Enhancement #1623: Don't check for presence of files in the backend before writing

   Before, all backend implementations were required to return an error if the file
   that is to be written already exists in the backend. For most backends, that
   means making a request (e.g. via HTTP) and returning an error when the file
   already exists.

   This is not accurate, the file could have been created between the HTTP request
   testing for it, and when writing starts, so we've relaxed this requirement,
   which saves one additional HTTP request per newly added file.

   https://github.com/restic/restic/pull/1623

 * Enhancement #1634: Upgrade B2 client library, reduce HTTP requests

   We've upgraded the B2 client library restic uses to access BackBlaze B2. This
   reduces the number of HTTP requests needed to upload a new file from two to one,
   which should improve throughput to B2.

   https://github.com/restic/restic/pull/1634


# Changelog for restic 0.8.2 (2018-02-17)
The following sections list the changes in restic 0.8.2 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1506: Limit bandwidth at the http.RoundTripper for HTTP based backends
 * Fix #1512: Restore directory permissions as the last step
 * Fix #1528: Correctly create missing subdirs in data/
 * Fix #1589: Complete intermediate index upload
 * Fix #1590: Strip spaces for lines read via --files-from
 * Fix #1594: Google Cloud Storage: Use generic HTTP transport
 * Fix #1595: Backup: Remove bandwidth display
 * Enh #1507: Only reload snapshots once per minute for fuse mount
 * Enh #1522: Add support for TLS client certificate authentication
 * Enh #1538: Reduce memory allocations for querying the index
 * Enh #1541: Reduce number of remote requests during repository check
 * Enh #1549: Speed up querying across indices and scanning existing files
 * Enh #1554: Fuse/mount: Correctly handle EOF, add template option
 * Enh #1564: Don't terminate ssh on SIGINT
 * Enh #1567: Reduce number of backend requests for rebuild-index and prune
 * Enh #1579: Retry Backend.List() in case of errors
 * Enh #1584: Limit index file size

## Details

 * Bugfix #1506: Limit bandwidth at the http.RoundTripper for HTTP based backends

   https://github.com/restic/restic/issues/1506
   https://github.com/restic/restic/pull/1511

 * Bugfix #1512: Restore directory permissions as the last step

   This change allows restoring into directories that were not writable during
   backup. Before, restic created the directory, set the read-only mode and then
   failed to create files in the directory. This change now restores the directory
   (with its permissions) as the very last step.

   https://github.com/restic/restic/issues/1512
   https://github.com/restic/restic/pull/1536

 * Bugfix #1528: Correctly create missing subdirs in data/

   https://github.com/restic/restic/issues/1528
   https://github.com/restic/restic/pull/1529

 * Bugfix #1589: Complete intermediate index upload

   After a user posted a comprehensive report of what he observed, we were able to
   find a bug and correct it: During backup, restic uploads so-called
   "intermediate" index files. When the backup finishes during a transfer of such
   an intermediate index, the upload is cancelled, but the backup is finished
   without an error. This leads to an inconsistent state, where the snapshot
   references data that is contained in the repo, but is not referenced in any
   index.

   The situation can be resolved by building a new index with `rebuild-index`, but
   looks very confusing at first. Since all the data got uploaded to the repo
   successfully, there was no risk of data loss, just minor inconvenience for our
   users.

   https://github.com/restic/restic/pull/1589
   https://forum.restic.net/t/error-loading-tree-check-prune-and-forget-gives-error-b2-backend/406

 * Bugfix #1590: Strip spaces for lines read via --files-from

   Leading and trailing spaces in lines read via `--files-from` are now stripped,
   so it behaves the same as with lines read via `--exclude-file`.

   https://github.com/restic/restic/issues/1590
   https://github.com/restic/restic/pull/1613

 * Bugfix #1594: Google Cloud Storage: Use generic HTTP transport

   It was discovered that the Google Cloud Storage backend did not use the generic
   HTTP transport, so things such as bandwidth limiting with `--limit-upload` did
   not work. This is resolved now.

   https://github.com/restic/restic/pull/1594

 * Bugfix #1595: Backup: Remove bandwidth display

   This commit removes the bandwidth displayed during backup process. It is
   misleading and seldom correct, because it's neither the "read bandwidth" (only
   for the very first backup) nor the "upload bandwidth". Many users are confused
   about (and rightly so), c.f. #1581, #1033, #1591

   We'll eventually replace this display with something more relevant when the new
   archiver code is ready.

   https://github.com/restic/restic/pull/1595

 * Enhancement #1507: Only reload snapshots once per minute for fuse mount

   https://github.com/restic/restic/pull/1507

 * Enhancement #1522: Add support for TLS client certificate authentication

   Support has been added for using a TLS client certificate for authentication to
   HTTP based backend. A file containing the PEM encoded private key and
   certificate can be set using the `--tls-client-cert` option.

   https://github.com/restic/restic/issues/1522
   https://github.com/restic/restic/pull/1524

 * Enhancement #1538: Reduce memory allocations for querying the index

   This change reduces the internal memory allocations when the index data
   structures in memory are queried if a blob (part of a file) already exists in
   the repo. It should speed up backup a bit, and maybe even reduce RAM usage.

   https://github.com/restic/restic/pull/1538

 * Enhancement #1541: Reduce number of remote requests during repository check

   This change eliminates redundant remote repository calls and significantly
   improves repository check time.

   https://github.com/restic/restic/issues/1541
   https://github.com/restic/restic/pull/1548

 * Enhancement #1549: Speed up querying across indices and scanning existing files

   This change increases the whenever a blob (part of a file) is searched for in a
   restic repository. This will reduce cpu usage some when backing up files already
   backed up by restic. Cpu usage is further decreased when scanning files.

   https://github.com/restic/restic/pull/1549

 * Enhancement #1554: Fuse/mount: Correctly handle EOF, add template option

   We've added the `--snapshot-template` string, which can be used to specify a
   template for a snapshot directory. In addition, accessing data after the end of
   a file via the fuse mount is now handled correctly.

   https://github.com/restic/restic/pull/1554

 * Enhancement #1564: Don't terminate ssh on SIGINT

   We've reworked the code which runs the `ssh` login for the sftp backend so that
   it can prompt for a password (if needed) but does not exit when the user presses
   CTRL+C (SIGINT) e.g. during backup. This allows restic to properly shut down
   when it receives SIGINT and remove the lock file from the repo, afterwards
   exiting the `ssh` process.

   https://github.com/restic/restic/pull/1564
   https://github.com/restic/restic/pull/1588

 * Enhancement #1567: Reduce number of backend requests for rebuild-index and prune

   We've found a way to reduce then number of backend requests for the
   `rebuild-index` and `prune` operations. This significantly speeds up the
   operations for high-latency backends.

   https://github.com/restic/restic/issues/1567
   https://github.com/restic/restic/pull/1574
   https://github.com/restic/restic/pull/1575

 * Enhancement #1579: Retry Backend.List() in case of errors

   https://github.com/restic/restic/pull/1579

 * Enhancement #1584: Limit index file size

   Before, restic would create a single new index file on `prune` or
   `rebuild-index`, this may lead to memory problems when this huge index is
   created and loaded again. We're now limiting the size of the index file, and
   split newly created index files into several smaller ones. This allows restic to
   be more memory-efficient.

   https://github.com/restic/restic/issues/1412
   https://github.com/restic/restic/issues/979
   https://github.com/restic/restic/issues/526
   https://github.com/restic/restic/pull/1584


# Changelog for restic 0.8.1 (2017-12-27)
The following sections list the changes in restic 0.8.1 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1454: Correct cache dir location for Windows and Darwin
 * Fix #1457: Improve s3 backend with DigitalOcean Spaces
 * Fix #1459: Disable handling SIGPIPE
 * Chg #1452: Do not save atime by default
 * Enh #11: Add the `diff` command
 * Enh #1436: Add code to detect old cache directories
 * Enh #1439: Improve cancellation logic

## Details

 * Bugfix #1454: Correct cache dir location for Windows and Darwin

   The cache directory on Windows and Darwin was not correct, instead the directory
   `.cache` was used.

   https://github.com/restic/restic/pull/1454

 * Bugfix #1457: Improve s3 backend with DigitalOcean Spaces

   https://github.com/restic/restic/issues/1457
   https://github.com/restic/restic/pull/1459

 * Bugfix #1459: Disable handling SIGPIPE

   We've disabled handling SIGPIPE again. Turns out, writing to broken TCP
   connections also raised SIGPIPE, so restic exits on the first write to a broken
   connection. Instead, restic should retry the request.

   https://github.com/restic/restic/issues/1457
   https://github.com/restic/restic/issues/1466
   https://github.com/restic/restic/pull/1459

 * Change #1452: Do not save atime by default

   By default, the access time for files and dirs is not saved any more. It is not
   possible to reliably disable updating the access time during a backup, so for
   the next backup the access time is different again. This means a lot of metadata
   is saved. If you want to save the access time anyway, pass `--with-atime` to the
   `backup` command.

   https://github.com/restic/restic/pull/1452

 * Enhancement #11: Add the `diff` command

   The command `diff` was added, it allows comparing two snapshots and listing all
   differences.

   https://github.com/restic/restic/issues/11
   https://github.com/restic/restic/issues/1460
   https://github.com/restic/restic/pull/1462

 * Enhancement #1436: Add code to detect old cache directories

   We've added code to detect old cache directories of repositories that haven't
   been used in a long time, restic now prints a note when it detects that such
   dirs exist. Also, the option `--cleanup-cache` was added to automatically remove
   such directories. That's not a problem because the cache will be rebuild once a
   repo is accessed again.

   https://github.com/restic/restic/pull/1436

 * Enhancement #1439: Improve cancellation logic

   The cancellation logic was improved, restic can now shut down cleanly when
   requested to do so (e.g. via ctrl+c).

   https://github.com/restic/restic/pull/1439


# Changelog for restic 0.8.0 (2017-11-26)
The following sections list the changes in restic 0.8.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Sec #1445: Prevent writing outside the target directory during restore
 * Fix #1256: Re-enable workaround for S3 backend
 * Fix #1291: Reuse backend TCP connections to BackBlaze B2
 * Fix #1317: Run prune when `forget --prune` is called with just snapshot IDs
 * Fix #1437: Remove implicit path `/restic` for the s3 backend
 * Enh #448: Sftp backend prompts for password
 * Enh #510: Add `dump` command
 * Enh #1040: Add local metadata cache
 * Enh #1102: Add subdirectory `ids` to fuse mount
 * Enh #1114: Add `--cacert` to specify TLS certificates to check against
 * Enh #1216: Add upload/download limiting
 * Enh #1249: Add `latest` symlink in fuse mount
 * Enh #1269: Add `--compact` to `forget` command
 * Enh #1271: Cache results for excludes for `backup`
 * Enh #1274: Add `generate` command, replaces `manpage` and `autocomplete`
 * Enh #1281: Google Cloud Storage backend needs less permissions
 * Enh #1319: Make `check` print `no errors found` explicitly
 * Enh #1353: Retry failed backend requests
 * Enh #1367: Allow comments in files read from via `--file-from`

## Details

 * Security #1445: Prevent writing outside the target directory during restore

   A vulnerability was found in the restic restorer, which allowed attackers in
   special circumstances to restore files to a location outside of the target
   directory. Due to the circumstances we estimate this to be a low-risk
   vulnerability, but urge all users to upgrade to the latest version of restic.

   Exploiting the vulnerability requires a Linux/Unix system which saves backups
   via restic and a Windows systems which restores files from the repo. In
   addition, the attackers need to be able to create files with arbitrary names
   which are then saved to the restic repo. For example, by creating a file named
   "..\test.txt" (which is a perfectly legal filename on Linux) and restoring a
   snapshot containing this file on Windows, it would be written to the parent of
   the target directory.

   We'd like to thank Tyler Spivey for reporting this responsibly!

   https://github.com/restic/restic/pull/1445

 * Bugfix #1256: Re-enable workaround for S3 backend

   We've re-enabled a workaround for `minio-go` (the library we're using to access
   s3 backends), this reduces memory usage.

   https://github.com/restic/restic/issues/1256
   https://github.com/restic/restic/pull/1267

 * Bugfix #1291: Reuse backend TCP connections to BackBlaze B2

   A bug was discovered in the library we're using to access Backblaze, it now
   reuses already established TCP connections which should be a lot faster and not
   cause network failures any more.

   https://github.com/restic/restic/issues/1291
   https://github.com/restic/restic/pull/1301

 * Bugfix #1317: Run prune when `forget --prune` is called with just snapshot IDs

   A bug in the `forget` command caused `prune` not to be run when `--prune` was
   specified without a policy, e.g. when only snapshot IDs that should be forgotten
   are listed manually.

   https://github.com/restic/restic/pull/1317

 * Bugfix #1437: Remove implicit path `/restic` for the s3 backend

   The s3 backend used the subdir `restic` within a bucket if no explicit path
   after the bucket name was specified. Since this version, restic does not use
   this default path any more. If you created a repo on s3 in a bucket without
   specifying a path within the bucket, you need to add `/restic` at the end of the
   repository specification to access your repo:
   `s3:s3.amazonaws.com/bucket/restic`

   https://github.com/restic/restic/issues/1292
   https://github.com/restic/restic/pull/1437

 * Enhancement #448: Sftp backend prompts for password

   The sftp backend now prompts for the password if a password is necessary for
   login.

   https://github.com/restic/restic/issues/448
   https://github.com/restic/restic/pull/1270

 * Enhancement #510: Add `dump` command

   We've added the `dump` command which prints a file from a snapshot to stdout.
   This can e.g. be used to restore files read with `backup --stdin`.

   https://github.com/restic/restic/issues/510
   https://github.com/restic/restic/pull/1346

 * Enhancement #1040: Add local metadata cache

   We've added a local cache for metadata so that restic doesn't need to load all
   metadata (snapshots, indexes, ...) from the repo each time it starts. By default
   the cache is active, but there's a new global option `--no-cache` that can be
   used to disable the cache. By default, the cache a standard cache folder for the
   OS, which can be overridden with `--cache-dir`. The cache will automatically
   populate, indexes and snapshots are saved as they are loaded. Cache directories
   for repos that haven't been used recently can automatically be removed by restic
   with the `--cleanup-cache` option.

   A related change was to by default create pack files in the repo that contain
   either data or metadata, not both mixed together. This allows easy caching of
   only the metadata files. The next run of `restic prune` will untangle mixed
   files automatically.

   https://github.com/restic/restic/issues/29
   https://github.com/restic/restic/issues/738
   https://github.com/restic/restic/issues/282
   https://github.com/restic/restic/pull/1040
   https://github.com/restic/restic/pull/1287
   https://github.com/restic/restic/pull/1436
   https://github.com/restic/restic/pull/1265

 * Enhancement #1102: Add subdirectory `ids` to fuse mount

   The fuse mount now has an `ids` subdirectory which contains the snapshots below
   their (short) IDs.

   https://github.com/restic/restic/issues/1102
   https://github.com/restic/restic/pull/1299
   https://github.com/restic/restic/pull/1320

 * Enhancement #1114: Add `--cacert` to specify TLS certificates to check against

   We've added the `--cacert` option which can be used to pass one (or more) CA
   certificates to restic. These are used in addition to the system CA certificates
   to verify HTTPS certificates (e.g. for the REST backend).

   https://github.com/restic/restic/issues/1114
   https://github.com/restic/restic/pull/1276

 * Enhancement #1216: Add upload/download limiting

   We've added support for rate limiting through `--limit-upload` and
   `--limit-download` flags.

   https://github.com/restic/restic/issues/1216
   https://github.com/restic/restic/pull/1336
   https://github.com/restic/restic/pull/1358

 * Enhancement #1249: Add `latest` symlink in fuse mount

   The directory structure in the fuse mount now exposes a symlink `latest` which
   points to the latest snapshot in that particular directory.

   https://github.com/restic/restic/pull/1249

 * Enhancement #1269: Add `--compact` to `forget` command

   The option `--compact` was added to the `forget` command to provide the same
   compact view as the `snapshots` command.

   https://github.com/restic/restic/pull/1269

 * Enhancement #1271: Cache results for excludes for `backup`

   The `backup` command now caches the result of excludes for a directory.

   https://github.com/restic/restic/issues/1271
   https://github.com/restic/restic/pull/1326

 * Enhancement #1274: Add `generate` command, replaces `manpage` and `autocomplete`

   The `generate` command has been added, which replaces the now removed commands
   `manpage` and `autocomplete`. This release of restic contains the most recent
   manpages in `doc/man` and the auto-completion files for bash and zsh in
   `doc/bash-completion.sh` and `doc/zsh-completion.zsh`

   https://github.com/restic/restic/issues/1274
   https://github.com/restic/restic/pull/1282

 * Enhancement #1281: Google Cloud Storage backend needs less permissions

   The Google Cloud Storage backend no longer requires the service account to have
   the `storage.buckets.get` permission ("Storage Admin" role) in `restic init` if
   the bucket already exists.

   https://github.com/restic/restic/pull/1281

 * Enhancement #1319: Make `check` print `no errors found` explicitly

   The `check` command now explicitly prints `No errors were found` when no errors
   could be found.

   https://github.com/restic/restic/issues/1303
   https://github.com/restic/restic/pull/1319

 * Enhancement #1353: Retry failed backend requests

   https://github.com/restic/restic/pull/1353

 * Enhancement #1367: Allow comments in files read from via `--file-from`

   When the list of files/dirs to be saved is read from a file with `--files-from`,
   comment lines (starting with `#`) are now ignored.

   https://github.com/restic/restic/issues/1367
   https://github.com/restic/restic/pull/1368


# Changelog for restic 0.7.3 (2017-09-20)
The following sections list the changes in restic 0.7.3 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1246: List all files stored in Google Cloud Storage

## Details

 * Bugfix #1246: List all files stored in Google Cloud Storage

   For large backups stored in Google Cloud Storage, the `prune` command fails
   because listing only returns the first 1000 files. This has been corrected, no
   data is lost in the process. In addition, a plausibility check was added to
   `prune`.

   https://github.com/restic/restic/issues/1246
   https://github.com/restic/restic/pull/1247


# Changelog for restic 0.7.2 (2017-09-13)
The following sections list the changes in restic 0.7.2 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1164: Make the `key remove` command behave as documented
 * Fix #1167: Do not create a local repo unless `init` is used
 * Fix #1191: Make sure to write profiling files on interrupt
 * Enh #317: Add `--exclude-caches` and `--exclude-if-present`
 * Enh #697: Automatically generate man pages for all restic commands
 * Enh #1044: Improve `restore`, do not traverse/load excluded directories
 * Enh #1061: Add Dockerfile and official Docker image
 * Enh #1126: Use the standard Go git repository layout, use `dep` for vendoring
 * Enh #1132: Make `key` command always prompt for a password
 * Enh #1134: Add support for storing backups on Google Cloud Storage
 * Enh #1144: Properly report errors when reading files with exclude patterns
 * Enh #1149: Add support for storing backups on Microsoft Azure Blob Storage
 * Enh #1179: Resolve name conflicts, append a counter
 * Enh #1196: Add `--group-by` to `forget` command for flexible grouping
 * Enh #1203: Print stats on all BSD systems when SIGINFO (ctrl+t) is received
 * Enh #1205: Allow specifying time/date for a backup with `--time`
 * Enh #1218: Add `--compact` to `snapshots` command

## Details

 * Bugfix #1164: Make the `key remove` command behave as documented

   https://github.com/restic/restic/pull/1164

 * Bugfix #1167: Do not create a local repo unless `init` is used

   When a restic command other than `init` is used with a local repository and the
   repository directory does not exist, restic creates the directory structure.
   That's an error, only the `init` command should create the dir.

   https://github.com/restic/restic/issues/1167
   https://github.com/restic/restic/pull/1182

 * Bugfix #1191: Make sure to write profiling files on interrupt

   Since a few releases restic had the ability to write profiling files for memory
   and CPU usage when `debug` is enabled. It was discovered that when restic is
   interrupted (ctrl+c is pressed), the proper shutdown hook is not run. This is
   now corrected.

   https://github.com/restic/restic/pull/1191

 * Enhancement #317: Add `--exclude-caches` and `--exclude-if-present`

   A new option `--exclude-caches` was added that allows excluding cache
   directories (that are tagged as such). This is a special case of a more generic
   option `--exclude-if-present` which excludes a directory if a file with a
   specific name (and contents) is present.

   https://github.com/restic/restic/issues/317
   https://github.com/restic/restic/pull/1170
   https://github.com/restic/restic/pull/1224

 * Enhancement #697: Automatically generate man pages for all restic commands

   https://github.com/restic/restic/issues/697
   https://github.com/restic/restic/pull/1147

 * Enhancement #1044: Improve `restore`, do not traverse/load excluded directories

   https://github.com/restic/restic/pull/1044

 * Enhancement #1061: Add Dockerfile and official Docker image

   https://github.com/restic/restic/pull/1061

 * Enhancement #1126: Use the standard Go git repository layout, use `dep` for vendoring

   The git repository layout was changed to resemble the layout typically used in
   Go projects, we're not using `gb` for building restic any more and vendoring the
   dependencies is now taken care of by `dep`.

   https://github.com/restic/restic/pull/1126

 * Enhancement #1132: Make `key` command always prompt for a password

   The `key` command now prompts for a password even if the original password to
   access a repo has been specified via the `RESTIC_PASSWORD` environment variable
   or a password file.

   https://github.com/restic/restic/issues/1132
   https://github.com/restic/restic/pull/1133

 * Enhancement #1134: Add support for storing backups on Google Cloud Storage

   https://github.com/restic/restic/issues/211
   https://github.com/restic/restic/pull/1134
   https://github.com/restic/restic/pull/1052

 * Enhancement #1144: Properly report errors when reading files with exclude patterns

   https://github.com/restic/restic/pull/1144

 * Enhancement #1149: Add support for storing backups on Microsoft Azure Blob Storage

   The library we're using to access the service requires Go 1.8, so restic now
   needs at least Go 1.8.

   https://github.com/restic/restic/issues/609
   https://github.com/restic/restic/pull/1149
   https://github.com/restic/restic/pull/1059

 * Enhancement #1179: Resolve name conflicts, append a counter

   https://github.com/restic/restic/issues/1179
   https://github.com/restic/restic/pull/1209

 * Enhancement #1196: Add `--group-by` to `forget` command for flexible grouping

   https://github.com/restic/restic/pull/1196

 * Enhancement #1203: Print stats on all BSD systems when SIGINFO (ctrl+t) is received

   https://github.com/restic/restic/pull/1203
   https://github.com/restic/restic/pull/1082#issuecomment-326279920

 * Enhancement #1205: Allow specifying time/date for a backup with `--time`

   https://github.com/restic/restic/pull/1205

 * Enhancement #1218: Add `--compact` to `snapshots` command

   The option `--compact` was added to the `snapshots` command to get a better
   overview of the snapshots in a repo. It limits each snapshot to a single line.

   https://github.com/restic/restic/issues/1218
   https://github.com/restic/restic/pull/1223


# Changelog for restic 0.7.1 (2017-07-22)
The following sections list the changes in restic 0.7.1 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #1115: Fix `prune`, only include existing files in indexes
 * Enh #1055: Create subdirs below `data/` for local/sftp backends
 * Enh #1067: Allow loading credentials for s3 from IAM
 * Enh #1073: Add `migrate` cmd to migrate from `s3legacy` to `default` layout
 * Enh #1080: Ignore chmod() errors on filesystems which do not support it
 * Enh #1081: Clarify semantic for `--tag` for the `forget` command
 * Enh #1082: Print stats on SIGINFO on Darwin and FreeBSD (ctrl+t)

## Details

 * Bugfix #1115: Fix `prune`, only include existing files in indexes

   A bug was found (and corrected) in the index rebuilding after prune, which led
   to indexes which include blobs that were not present in the repo any more. There
   were already checks in place which detected this situation and aborted with an
   error message. A new run of either `prune` or `rebuild-index` corrected the
   index files. This is now fixed and a test has been added to detect this.

   https://github.com/restic/restic/pull/1115

 * Enhancement #1055: Create subdirs below `data/` for local/sftp backends

   The local and sftp backends now create the subdirs below `data/` on open/init.
   This way, restic makes sure that they always exist. This is connected to an
   issue for the sftp server.

   https://github.com/restic/restic/issues/1055
   https://github.com/restic/rest-server/pull/11#issuecomment-309879710
   https://github.com/restic/restic/pull/1077
   https://github.com/restic/restic/pull/1105

 * Enhancement #1067: Allow loading credentials for s3 from IAM

   When no S3 credentials are specified in the environment variables, restic now
   tries to load credentials from an IAM instance profile when the s3 backend is
   used.

   https://github.com/restic/restic/issues/1067
   https://github.com/restic/restic/pull/1086

 * Enhancement #1073: Add `migrate` cmd to migrate from `s3legacy` to `default` layout

   The `migrate` command for changing the `s3legacy` layout to the `default` layout
   for s3 backends has been improved: It can now be restarted with `restic migrate
   --force s3_layout` and automatically retries operations on error.

   https://github.com/restic/restic/issues/1073
   https://github.com/restic/restic/pull/1075

 * Enhancement #1080: Ignore chmod() errors on filesystems which do not support it

   https://github.com/restic/restic/pull/1080
   https://github.com/restic/restic/pull/1112

 * Enhancement #1081: Clarify semantic for `--tag` for the `forget` command

   https://github.com/restic/restic/issues/1081
   https://github.com/restic/restic/pull/1090

 * Enhancement #1082: Print stats on SIGINFO on Darwin and FreeBSD (ctrl+t)

   https://github.com/restic/restic/pull/1082


# Changelog for restic 0.7.0 (2017-07-01)
The following sections list the changes in restic 0.7.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Fix #965: Switch to `default` repo layout for the s3 backend
 * Fix #1013: Switch back to using the high-level minio-go API for s3
 * Enh #512: Add Backblaze B2 backend
 * Enh #636: Add dirs `tags` and `hosts` to fuse mount
 * Enh #975: Add new backend for OpenStack Swift
 * Enh #989: Improve performance of the `find` command
 * Enh #998: Improve performance of the fuse mount
 * Enh #1021: Detect invalid backend name and print error
 * Enh #1029: Remove invalid pack files when `prune` is run

## Details

 * Bugfix #965: Switch to `default` repo layout for the s3 backend

   The default layout for the s3 backend is now `default` (instead of `s3legacy`).
   Also, there's a new `migrate` command to convert an existing repo, it can be run
   like this: `restic migrate s3_layout`

   https://github.com/restic/restic/issues/965
   https://github.com/restic/restic/pull/1004

 * Bugfix #1013: Switch back to using the high-level minio-go API for s3

   For the s3 backend we're back to using the high-level API the s3 client library
   for uploading data, a few users reported dropped connections (which the library
   will automatically retry now).

   https://github.com/restic/restic/issues/1013
   https://github.com/restic/restic/issues/1023
   https://github.com/restic/restic/pull/1025

 * Enhancement #512: Add Backblaze B2 backend

   https://github.com/restic/restic/issues/512
   https://github.com/restic/restic/pull/978

 * Enhancement #636: Add dirs `tags` and `hosts` to fuse mount

   The fuse mount now has two more directories: `tags` contains a subdir for each
   tag, which in turn contains only the snapshots that have this tag. The subdir
   `hosts` contains a subdir for each host that has a snapshot, and the subdir
   contains the snapshots for that host.

   https://github.com/restic/restic/issues/636
   https://github.com/restic/restic/pull/1050

 * Enhancement #975: Add new backend for OpenStack Swift

   https://github.com/restic/restic/pull/975
   https://github.com/restic/restic/pull/648

 * Enhancement #989: Improve performance of the `find` command

   Improved performance for the `find` command: Restic recognizes paths it has
   already checked for the files in question, so the number of backend requests is
   reduced a lot.

   https://github.com/restic/restic/issues/989
   https://github.com/restic/restic/pull/993

 * Enhancement #998: Improve performance of the fuse mount

   Listing directories which contain large files now is significantly faster.

   https://github.com/restic/restic/pull/998

 * Enhancement #1021: Detect invalid backend name and print error

   Restic now tries to detect when an invalid/unknown backend is used and returns
   an error message.

   https://github.com/restic/restic/issues/1021
   https://github.com/restic/restic/pull/1070

 * Enhancement #1029: Remove invalid pack files when `prune` is run

   The `prune` command has been improved and will now remove invalid pack files,
   for example files that have not been uploaded completely because a backup was
   interrupted.

   https://github.com/restic/restic/issues/1029
   https://github.com/restic/restic/pull/1036


# Changelog for restic 0.6.1 (2017-06-01)
The following sections list the changes in restic 0.6.1 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Enh #974: Remove regular status reports
 * Enh #981: Remove temporary path from binary in `build.go`
 * Enh #985: Allow multiple parallel idle HTTP connections

## Details

 * Enhancement #974: Remove regular status reports

   Regular status report: We've removed the status report that was printed every 10
   seconds when restic is run non-interactively. You can still force reporting the
   current status by sending a `USR1` signal to the process.

   https://github.com/restic/restic/pull/974

 * Enhancement #981: Remove temporary path from binary in `build.go`

   The `build.go` now strips the temporary directory used for compilation from the
   binary. This is the first step in enabling reproducible builds.

   https://github.com/restic/restic/pull/981

 * Enhancement #985: Allow multiple parallel idle HTTP connections

   Backends based on HTTP now allow several idle connections in parallel. This is
   especially important for the REST backend, which (when used with a local server)
   may create a lot connections and exhaust available ports quickly.

   https://github.com/restic/restic/issues/985
   https://github.com/restic/restic/pull/986


# Changelog for restic 0.6.0 (2017-05-29)
The following sections list the changes in restic 0.6.0 relevant to
restic users. The changes are ordered by importance.

## Summary

 * Enh #957: Make `forget` consistent
 * Enh #962: Improve memory and runtime for the s3 backend
 * Enh #966: Unify repository layout for all backends

## Details

 * Enhancement #957: Make `forget` consistent

   The `forget` command was corrected to be more consistent in which snapshots are
   to be forgotten. It is possible that the new code removes more snapshots than
   before, so please review what would be deleted by using the `--dry-run` option.

   https://github.com/restic/restic/issues/953
   https://github.com/restic/restic/pull/957

 * Enhancement #962: Improve memory and runtime for the s3 backend

   We've updated the library used for accessing s3, switched to using a lower level
   API and added caching for some requests. This lead to a decrease in memory usage
   and a great speedup. In addition, we added benchmark functions for all backends,
   so we can track improvements over time. The Continuous Integration test service
   we're using (Travis) now runs the s3 backend tests not only against a Minio
   server, but also against the Amazon s3 live service, so we should be notified of
   any regressions much sooner.

   https://github.com/restic/restic/pull/962
   https://github.com/restic/restic/pull/960
   https://github.com/restic/restic/pull/946
   https://github.com/restic/restic/pull/938
   https://github.com/restic/restic/pull/883

 * Enhancement #966: Unify repository layout for all backends

   Up to now the s3 backend used a special repository layout. We've decided to
   unify the repository layout and implemented the default layout also for the s3
   backend. For creating a new repository on s3 with the default layout, use
   `restic -o s3.layout=default init`. For further commands the option is not
   necessary any more, restic will automatically detect the correct layout to use.
   A future version will switch to the default layout for new repositories.

   https://github.com/restic/restic/issues/965
   https://github.com/restic/restic/pull/966
