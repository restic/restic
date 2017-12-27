Changelog for restic 0.7.1 (2017-07-22)
=======================================

The following sections list the changes in restic 0.7.1 relevant to
restic users. The changes are ordered by importance.

Summary
-------

 * Fix #1115: Fix `prune`, only include existing files in indexes
 * Enh #1055: Create subdirs below `data/` for local/sftp backends
 * Enh #1067: Allow loading credentials for s3 from IAM
 * Enh #1073: Add `migrate` cmd to migrate from `s3legacy` to `default` layout
 * Enh #1081: Clarify semantic for `--tasg` for the `forget` command
 * Enh #1080: Ignore chmod() errors on filesystems which do not support it
 * Enh #1082: Print stats on SIGINFO on Darwin and FreeBSD (ctrl+t)

Details
-------

 * Bugfix #1115: Fix `prune`, only include existing files in indexes

   A bug was found (and corrected) in the index rebuilding after prune, which led to indexes which
   include blobs that were not present in the repo any more. There were already checks in place
   which detected this situation and aborted with an error message. A new run of either `prune` or
   `rebuild-index` corrected the index files. This is now fixed and a test has been added to detect
   this.

   https://github.com/restic/restic/pull/1115

 * Enhancement #1055: Create subdirs below `data/` for local/sftp backends

   The local and sftp backends now create the subdirs below `data/` on open/init. This way, restic
   makes sure that they always exist. This is connected to an issue for the sftp server:

   https://github.com/restic/restic/issues/1055
   https://github.com/restic/restic/pull/1077
   https://github.com/restic/restic/pull/1105
   https://github.com/restic/rest-server/pull/11#issuecomment-309879710

 * Enhancement #1067: Allow loading credentials for s3 from IAM

   When no S3 credentials are specified in the environment variables, restic now tries to load
   credentials from an IAM instance profile when the s3 backend is used.

   https://github.com/restic/restic/issues/1067
   https://github.com/restic/restic/pull/1086

 * Enhancement #1073: Add `migrate` cmd to migrate from `s3legacy` to `default` layout

   The `migrate` command for chaning the `s3legacy` layout to the `default` layout for s3
   backends has been improved: It can now be restarted with `restic migrate --force s3_layout`
   and automatically retries operations on error.

   https://github.com/restic/restic/issues/1073
   https://github.com/restic/restic/pull/1075

 * Enhancement #1081: Clarify semantic for `--tasg` for the `forget` command

   https://github.com/restic/restic/issues/1081
   https://github.com/restic/restic/pull/1090

 * Enhancement #1080: Ignore chmod() errors on filesystems which do not support it

   https://github.com/restic/restic/pull/1080
   https://github.com/restic/restic/pull/1112

 * Enhancement #1082: Print stats on SIGINFO on Darwin and FreeBSD (ctrl+t)

   https://github.com/restic/restic/pull/1082


Changelog for restic 0.7.0 (2017-07-01)
=======================================

The following sections list the changes in restic 0.7.0 relevant to
restic users. The changes are ordered by importance.

Summary
-------

 * Fix #1013: Switch back to using the high-level minio-go API for s3
 * Fix #965: Switch to `default` repo layout for the s3 backend
 * Enh #1021: Detect invalid backend name and print error
 * Enh #1029: Remove invalid pack files when `prune` is run
 * Enh #512: Add Backblaze B2 backend
 * Enh #636: Add dirs `tags` and `hosts` to fuse mount
 * Enh #989: Improve performance of the `find` command
 * Enh #975: Add new backend for OpenStack Swift
 * Enh #998: Improve performance of the fuse mount

Details
-------

 * Bugfix #1013: Switch back to using the high-level minio-go API for s3

   For the s3 backend we're back to using the high-level API the s3 client library for uploading
   data, a few users reported dropped connections (which the library will automatically retry
   now).

   https://github.com/restic/restic/issues/1013
   https://github.com/restic/restic/issues/1023
   https://github.com/restic/restic/pull/1025

 * Bugfix #965: Switch to `default` repo layout for the s3 backend

   The default layout for the s3 backend is now `default` (instead of `s3legacy`). Also, there's a
   new `migrate` command to convert an existing repo, it can be run like this: `restic migrate
   s3_layout`

   https://github.com/restic/restic/issues/965
   https://github.com/restic/restic/pull/1004

 * Enhancement #1021: Detect invalid backend name and print error

   Restic now tries to detect when an invalid/unknown backend is used and returns an error
   message.

   https://github.com/restic/restic/issues/1021
   https://github.com/restic/restic/pull/1070

 * Enhancement #1029: Remove invalid pack files when `prune` is run

   The `prune` command has been improved and will now remove invalid pack files, for example files
   that have not been uploaded completely because a backup was interrupted.

   https://github.com/restic/restic/issues/1029
   https://github.com/restic/restic/pull/1036

 * Enhancement #512: Add Backblaze B2 backend

   https://github.com/restic/restic/issues/512
   https://github.com/restic/restic/pull/978

 * Enhancement #636: Add dirs `tags` and `hosts` to fuse mount

   The fuse mount now has two more directories: `tags` contains a subdir for each tag, which in turn
   contains only the snapshots that have this tag. The subdir `hosts` contains a subdir for each
   host that has a snapshot, and the subdir contains the snapshots for that host.

   https://github.com/restic/restic/issues/636
   https://github.com/restic/restic/pull/1050

 * Enhancement #989: Improve performance of the `find` command

   Improved performance for the `find` command: Restic recognizes paths it has already checked
   for the files in question, so the number of backend requests is reduced a lot.

   https://github.com/restic/restic/issues/989
   https://github.com/restic/restic/pull/993

 * Enhancement #975: Add new backend for OpenStack Swift

   https://github.com/restic/restic/pull/975
   https://github.com/restic/restic/pull/648

 * Enhancement #998: Improve performance of the fuse mount

   Listing directories which contain large files now is significantly faster.

   https://github.com/restic/restic/pull/998


Changelog for restic 0.6.1 (2017-06-01)
=======================================

The following sections list the changes in restic 0.6.1 relevant to
restic users. The changes are ordered by importance.

Summary
-------

 * Enh #985: Allow multiple parallel idle HTTP connections
 * Enh #981: Remove temporary path from binary in `build.go`
 * Enh #974: Remove regular status reports

Details
-------

 * Enhancement #985: Allow multiple parallel idle HTTP connections

   Backends based on HTTP now allow several idle connections in parallel. This is especially
   important for the REST backend, which (when used with a local server) may create a lot
   connections and exhaust available ports quickly.

   https://github.com/restic/restic/issues/985
   https://github.com/restic/restic/pull/986

 * Enhancement #981: Remove temporary path from binary in `build.go`

   The `build.go` now strips the temporary directory used for compilation from the binary. This
   is the first step in enabling reproducible builds.

   https://github.com/restic/restic/pull/981

 * Enhancement #974: Remove regular status reports

   Regular status report: We've removed the status report that was printed every 10 seconds when
   restic is run non-interactively. You can still force reporting the current status by sending a
   `USR1` signal to the process.

   https://github.com/restic/restic/pull/974


Changelog for restic 0.6.0 (2017-05-29)
=======================================

The following sections list the changes in restic 0.6.0 relevant to
restic users. The changes are ordered by importance.

Summary
-------

 * Enh #953: Make `forget` consistent
 * Enh #965: Unify repository layout for all backends
 * Enh #962: Improve memory and runtime for the s3 backend

Details
-------

 * Enhancement #953: Make `forget` consistent

   The `forget` command was corrected to be more consistent in which snapshots are to be
   forgotten. It is possible that the new code removes more snapshots than before, so please
   review what would be deleted by using the `--dry-run` option.

   https://github.com/restic/restic/issues/953
   https://github.com/restic/restic/pull/957

 * Enhancement #965: Unify repository layout for all backends

   Up to now the s3 backend used a special repository layout. We've decided to unify the repository
   layout and implemented the default layout also for the s3 backend. For creating a new
   repository on s3 with the default layout, use `restic -o s3.layout=default init`. For further
   commands the option is not necessary any more, restic will automatically detect the correct
   layout to use. A future version will switch to the default layout for new repositories.

   https://github.com/restic/restic/issues/965
   https://github.com/restic/restic/pull/966

 * Enhancement #962: Improve memory and runtime for the s3 backend

   We've updated the library used for accessing s3, switched to using a lower level API and added
   caching for some requests. This lead to a decrease in memory usage and a great speedup. In
   addition, we added benchmark functions for all backends, so we can track improvements over
   time. The Continuous Integration test service we're using (Travis) now runs the s3 backend
   tests not only against a Minio server, but also against the Amazon s3 live service, so we should
   be notified of any regressions much sooner.

   https://github.com/restic/restic/pull/962
   https://github.com/restic/restic/pull/960
   https://github.com/restic/restic/pull/946
   https://github.com/restic/restic/pull/938
   https://github.com/restic/restic/pull/883


