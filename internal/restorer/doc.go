// Package restorer contains code to restore data from a repository.
//
// The Restorer tries to keep the number of backend requests minimal. It does
// this by downloading all required blobs of a pack file with a single backend
// request and avoiding repeated downloads of the same pack. In addition,
// several pack files are fetched concurrently.
//
// Here is high-level pseudo-code of the how the Restorer attempts to achieve
// these goals:
//
//   while there are packs to process
//     choose a pack to process                      [1]
//     get the pack from the backend or cache        [2]
//     write pack blobs to the files that need them  [3]
//     if not all pack blobs were used
//       cache the pack for future use               [4]
//
// Pack download and processing (steps [2] - [4]) runs on multiple concurrent
// Goroutines. The Restorer runs all steps [2]-[4] sequentially on the same
// Goroutine.
//
// Before a pack is downloaded (step [2]), the required space is "reserved" in
// the pack cache. Actual download uses single backend request to get all
// required pack blobs. This may download blobs that are not needed, but we
// assume it'll still be faster than getting individual blobs.
//
// Target files are written (step [3]) in the "right" order, first file blob
// first, then second, then third and so on. Blob write order implies that some
// pack blobs may not be immediately used, i.e. they are "out of order" for
// their respective target files. Packs with unused blobs are cached (step
// [4]). The cache has capacity limit and may purge packs before they are fully
// used, in which case the purged packs will need to be re-downloaded.
package restorer
