The main goal of this POC is two fold

1. Reduce number of backend request
  * download each pack with single backend request
  * avoid repeated pack downloads when possible
2. Download multiple pack files

Here is high-level pseudo-code of the how the POC attempts to achieve these goals

```
while there are packs to process
  choose a pack to process                      [1]
  get the pack from the backend or cache        [2]
  write pack blobs to the files that need them  [3]
  if not all pack blobs were used
    put cache the pack cache                    [4]
```

Pack download and processing (steps [2] - [4]) runs on multiple concurrent goroutings. The POC runs all steps [2]-[4] sequentially on the same gorouting, but it is possible to split the work differently. For example, one pool of workers can handle download (step [2]) while the other pool can handle pack processing (steps [3] and [4]).

Before a pack is downloaded (step [2]), the required space is "reserved" in the pack cache, which may purge some cached packed to make room for the reservation (well, that's the plan but purging isn't implemented in the POC). Actual download uses single backend request to get all required pack blobs. This may download blobs that are not needed, but I assume it'll still be faster than getting individual blobs. We should be able to optimize this further by changing `Backend.Load(...)` to support multiple byte ranges, for example. 

Target files are written (step [3]) in the "right" order, first file blob first, then second, then third and so on. Blob write order implies that some pack blobs may not be immediately used, i.e. they are "out of order" for their respective target files. Packs with unused blobs are cached (step [4]). The cache has capacity limit and may purge packs before they are fully used, in which case the purged packs will need to be redownloaded.

Choosing which pack to process next (step [1]) is little convoluted in the POC. The code avoids processing of any given pack and any given target file by multiple workers concurrently. It also tries to reduce likelihook a pack will be purged from the cache by counting how many other packs will need to be processed before the pack is fully used up. Packs that need fewer other packs are processed first, everything else being equal.

----
Other ideas to consider

* Allow out-of-order target file writes. Inprogress restore will be somewhat confusing to observe, but this will eliminate the need to cache packs and should simplify implimentation. On the other hand, crashed/killed restore will be harder to recover.
