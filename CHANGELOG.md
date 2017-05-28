This file describes changes relevant to all users that are made in each
released version of restic from the perspective of the user.

Important Changes in 0.X.Y
==========================

 * New "b2" backend: A new backend for Backblaze B2 cloud storage
   service has been added, https://www.backblaze.com
   https://github.com/restic/restic/issues/512
   https://github.com/restic/restic/pull/978

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
