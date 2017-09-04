This file describes changes relevant to all users that are made in each
released version of restic from the perspective of the user.

Important Changes in 0.X.Y
==========================

 * We've added an official docker image and a Dockerfile to build this image in
   `docker/`.
   https://github.com/restic/restic/pull/1061

 * The git repository layout was changed to resemble the layout typically used
   in Go projects, we're not using `gb` for building restic any more and
   vendoring the dependencies is now taken care of by `dep`.
   https://github.com/restic/restic/pull/1126

 * We now support saving backups on Google Cloud Storage.
   https://github.com/restic/restic/pull/1134
   https://github.com/restic/restic/pull/1052
   https://github.com/restic/restic/issues/211

 * We've added support for Microsoft Azure Blob Storage as a restic backend.
   https://github.com/restic/restic/pull/1149
   https://github.com/restic/restic/pull/1059
   https://github.com/restic/restic/issues/609

 * In the course of supporting Microsoft Azure Blobe Storage Go 1.8 is now a
   requirement to build restic.

 * The `restore` command has been improved: When dirs are excluded (or not
   included) in a restore, they are not loaded from the repo any more.
   https://github.com/restic/restic/pull/1044


Small changes
-------------

 * The `key` command now prompts for a password even if the original password
   to access a repo has been specified via the `RESTIC_PASSWORD` environment
   variable or a password file.
   https://github.com/restic/restic/issues/1132
   https://github.com/restic/restic/pull/1133

 * Properly report errors when reading files with exclude patterns.
   https://github.com/restic/restic/pull/1144

 * We now automatically generate man pages for all restic commands, see the
   subdir `doc/man`.
   https://github.com/restic/restic/issues/697
   https://github.com/restic/restic/pull/1147

 * The `key remove` command was corrected and now works as documented.
   https://github.com/restic/restic/pull/1164

 * When a restic command other than `init` is used with a local repository and
   the repository directory does not exist, restic creates the directory
   structure. That's an error, only the `init` command should create the dir.
   https://github.com/restic/restic/issues/1167
   https://github.com/restic/restic/pull/1182

 * Restic now prints stats on all BSD systems (not only on darwin) when SIGINFO
   is received (usually when ctrl+t is pressed).
   https://github.com/restic/restic/pull/1203
   https://github.com/restic/restic/pull/1082#issuecomment-326279920

 * Since a few releases restic had the ability to write profiling files for
   memory and CPU usage when `debug` is enabled. It was discovered that when
   restic is interrupted (ctrl+c is pressed), the proper shutdown hook is not
   run. This is now corrected.
   https://github.com/restic/restic/pull/1191

Important Changes in 0.7.1
==========================

 * The `migrate` command for chaning the `s3legacy` layout to the `default`
   layout for s3 backends has been improved: It can now be restarted with
   `restic migrate --force s3_layout` and automatically retries operations on
   error.
   https://github.com/restic/restic/issues/1073
   https://github.com/restic/restic/pull/1075

Small changes
-------------

 * The local and sftp backends now create the subdirs below `data/` on
   open/init. This way, restic makes sure that they always exist. This is
   connected to an issue for the sftp server:
   https://github.com/restic/rest-server/pull/11#issuecomment-309879710
   https://github.com/restic/restic/issues/1055
   https://github.com/restic/restic/pull/1077
   https://github.com/restic/restic/pull/1105

 * When no S3 credentials are specified in the environment variables, restic
   now tries to load credentials from an IAM instance profile when the s3
   backend is used.
   https://github.com/restic/restic/issues/1067
   https://github.com/restic/restic/pull/1086

 * On Darwin and FreeBSD, restic now prints stats when SIGINFO is received
   (usually when ctrl+t is pressed).
   https://github.com/restic/restic/pull/1082

 * The dependencies have been updated.
   https://github.com/restic/restic/pull/1108
   https://github.com/restic/restic/pull/1124

 * A bug was found (and corrected) in the index rebuilding after prune, which
   led to indexes which include blobs that were not present in the repo any
   more. There were already checks in place which detected this situation and
   aborted with an error message. A new run of either `prune` or
   `rebuild-index` corrected the index files. This is now fixed and a test has
   been added to detect this.
   https://github.com/restic/restic/pull/1115

 * Errors for chmod() on Unix for filesystems which do not support it (e.g. smb
   mounted via gvfs) are now ignored.
   https://github.com/restic/restic/pull/1080
   https://github.com/restic/restic/pull/1112

 * The semantic for the `--tags` option to `forget` and `snapshots` was
   clarified:
   https://github.com/restic/restic/issues/1081
   https://github.com/restic/restic/pull/1090

Important Changes in 0.7.0
==========================

 * New "swift" backend: A new backend for the OpenStack Swift cloud storage
   protocol has been added, https://wiki.openstack.org/wiki/Swift
   https://github.com/restic/restic/pull/975
   https://github.com/restic/restic/pull/648

 * New "b2" backend: A new backend for Backblaze B2 cloud storage
   service has been added, https://www.backblaze.com
   https://github.com/restic/restic/issues/512
   https://github.com/restic/restic/pull/978

 * Improved performance for the `find` command: Restic recognizes paths it has
   already checked for the files in question, so the number of backend requests
   is reduced a lot.
   https://github.com/restic/restic/issues/989
   https://github.com/restic/restic/pull/993

 * Improved performance for the fuse mount: Listing directories which contain
   large files now is significantly faster.
   https://github.com/restic/restic/pull/998

 * The default layout for the s3 backend is now `default` (instead of
   `s3legacy`). Also, there's a new `migrate` command to convert an existing
   repo, it can be run like this: `restic migrate s3_layout`
   https://github.com/restic/restic/issues/965
   https://github.com/restic/restic/pull/1004

 * The fuse mount now has two more directories: `tags` contains a subdir for
   each tag, which in turn contains only the snapshots that have this tag. The
   subdir `hosts` contains a subdir for each host that has a snapshot, and the
   subdir contains the snapshots for that host.
   https://github.com/restic/restic/issues/636
   https://github.com/restic/restic/pull/1050

Small changes
-------------

 * For the s3 backend we're back to using the high-level API the s3 client
   library for uploading data, a few users reported dropped connections (which
   the library will automatically retry now).
   https://github.com/restic/restic/issues/1013
   https://github.com/restic/restic/issues/1023
   https://github.com/restic/restic/pull/1025

 * The `prune` command has been improved and will now remove invalid pack
   files, for example files that have not been uploaded completely because a
   backup was interrupted.
   https://github.com/restic/restic/issues/1029
   https://github.com/restic/restic/pull/1036

 * restic now tries to detect when an invalid/unknown backend is used and
   returns an error message.
   https://github.com/restic/restic/issues/1021
   https://github.com/restic/restic/pull/1070

Important Changes in 0.6.1
==========================

This is mostly a bugfix release and only contains small changes:

 * We've fixed a bug where `rebuild-index` would corrupt the index when used
   with the s3 backend together with the `default` layout. This is not the
   default setting.

 * Backends based on HTTP now allow several idle connections in parallel. This
   is especially important for the REST backend, which (when used with a local
   server) may create a lot connections and exhaust available ports quickly.
   https://github.com/restic/restic/issues/985
   https://github.com/restic/restic/pull/986

 * Regular status report: We've removed the status report that was printed
   every 10 seconds when restic is run non-interactively. You can still force
   reporting the current status by sending a `USR1` signal to the process.
   https://github.com/restic/restic/pull/974

 * The `build.go` now strips the temporary directory used for compilation from
   the binary. This is the first step in enabling reproducible builds.
   https://github.com/restic/restic/pull/981

Important Changes in 0.6.0
==========================

Consistent forget policy
------------------------

The `forget` command was corrected to be more consistent in which snapshots are
to be forgotten. It is possible that the new code removes more snapshots than
before, so please review what would be deleted by using the `--dry-run` option.

https://github.com/restic/restic/pull/957
https://github.com/restic/restic/issues/953

Unified repository layout
-------------------------

Up to now the s3 backend used a special repository layout. We've decided to
unify the repository layout and implemented the default layout also for the s3
backend. For creating a new repository on s3 with the default layout, use
`restic -o s3.layout=default init`. For further commands the option is not
necessary any more, restic will automatically detect the correct layout to use.
A future version will switch to the default layout for new repositories.

https://github.com/restic/restic/pull/966
https://github.com/restic/restic/issues/965

Memory and time improvements for the s3 backend
-----------------------------------------------

We've updated the library used for accessing s3, switched to using a lower
level API and added caching for some requests. This lead to a decrease in
memory usage and a great speedup. In addition, we added benchmark functions for
all backends, so we can track improvements over time. The Continuous
Integration test service we're using (Travis) now runs the s3 backend tests not
only against a Minio server, but also against the Amazon s3 live service, so we
should be notified of any regressions much sooner.

https://github.com/restic/restic/pull/962
https://github.com/restic/restic/pull/960
https://github.com/restic/restic/pull/946
https://github.com/restic/restic/pull/938
https://github.com/restic/restic/pull/883
