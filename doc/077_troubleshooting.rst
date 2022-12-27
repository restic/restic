..
  Normally, there are no heading levels assigned to certain characters as the structure is
  determined from the succession of headings. However, this convention is used in Python’s
  Style Guide for documenting which you may follow:

  # with overline, for parts
  * for chapters
  = for sections
  - for subsections
  ^ for subsubsections
  " for paragraphs

#########################
Troubleshooting
#########################

Being a backup software, the repository format ensures that the data saved in the repository
is verifiable and error-restistant. Restic even implements some self-healing functionalities.

However, situations might occur where your repository gets in an incorrect state and measurements
need to be done to get you out of this situation. These situations might be due to hardware failure,
accidentially removing files directly from the repository or bugs in the restic implementation.

This document is meant to give you some hints about how to recover from such situations.

1. Stay calm and don't over-react
********************************************

The most important thing if you find yourself in the situation of a damaged repository is to
stay calm and don't do anything you might regret later.

The following point should be always considered:

- Make a copy of you repository and try to recover from that copy. If you suspect a storage failure,
  it may be even better, to make *two* copies: one to get all data out of the possibly failing storage
  and another one to try the recovery process.
- Pause your regular operations on the repository or let them run on a copy. You will especially make
  sure that no `forget` or `prune` is run as these command are supposed to remove data and may result
  in data loss.
- Search if your issue is already known and solved. Good starting points are the restic forum and the
  github issues.
- Get you some help if you are unsure what to do. Find a colleage or friend to discuss what should be done.
  Also feel free to consult the restic forum.
- When using the commands below, make sure you read and understand the documentation. Some of the commands
  may not be your every-day commands, so make sure you really understand what they are doing. 


2. `check` is your friend
********************************************

Run `restic check` to find out what type of error you have. The results may be technical but can give you
a good hint what's really wrong. 

Moreover, you can always run a `check` to ensure that your repair really was sucessful and your repository
is in a sane state again.
But make sure that your needed data is also still contained in your repository ;-)
 
Note that `check` also prints out warning in some cases. These warnings point out that the repo may be 
optimized but is still in perfect shape and does not need any troubleshooting. 

3. Index trouble -> `repair index`
********************************************

A common problem with broken repostories is that the index does no longer correctly represent the contents
of your pack files. This is especially the case if some pack files got lost.
`repair index` recovers this situation and ensures that the index exactly represents the pack files.

You might even need to manually remove corrupted pack files. In this case make sure, you run 
`restic repair index` after.

Also if you encounter problems with the index files itselves, `repair index` will solve these problems
immediately.

However, rebuilding the index does not solve every problem, e.g. lost pack files.

4. Delete unneeded defect snapshots -> `forget`
********************************************

If you encounter defect snapshots but realize you can spare them, it is often a good idea to simply
delete them using `forget`. In case that your repository remains with just sane snapshots (including
all trees and files) the next `prune` run will put your repository in a sane state.

This can be also used if you manage to create new snapshots which can replace the defect ones, see
below.

5. No fear to `backup` again
********************************************

There are quite some self-healing mechanisms withing the `backup` command. So it is always a good idea to
backup again and check if this did heal your repository.
If you realize that a specific file is broken in your repository and you have this file, any run of
`backup` which includes that file will be able to heal the situation.

Note that `backup` relies on a correct index state, so make sure your index is fine or run `repair index`
before running `backup`.

6. Unreferenced tree -> `recover`
********************************************

If for some reason you have unreferenced trees in your repository but you actually need them, run
`recover` it will generate a new snapshot which allows access to all trees that you have in your
repository. 

Note that `recover` relies on a correct index state, so make sure your index is fine or run `repair index`
before running `recover`.

7. Repair defect snapshots using `repair`
********************************************

If all other things did not help, you can repair defect snapshots with `repair`. Note that the repaired
snapshots will miss data which was referenced in the defect snapshot.
