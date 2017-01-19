FAQ
===

This is the list of Frequently Asked Questions for restic.

`restic check` reports packs that aren't referenced in any index, is my repository broken?
------------------------------------------------------------------------------------------

When `restic check` reports that there are pack files in the repository that are not referenced in any index, that's (in contrast to what restic reports at the moment) not a source for concern. The output looks like this:

    $ restic check
    Create exclusive lock for repository
    Load indexes
    Check all packs
    pack 819a9a52e4f51230afa89aefbf90df37fb70996337ae57e6f7a822959206a85e: not referenced in any index
    pack de299e69fb075354a3775b6b045d152387201f1cdc229c31d1caa34c3b340141: not referenced in any index
    Check snapshots, trees and blobs
    Fatal: repository contains errors

The message means that there is more data stored in the repo than strictly necessary. With high probability this is duplicate data. In order to clean it up, the command `restic prune` can be used. The source of this additional data is not yet known.
