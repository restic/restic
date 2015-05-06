This document describes the way you can contribute to the restic project.

Ways to Help Out
================

Thank you for your contribution!

There are several ways you can help us out. First of all code contributions and
bug fixes are most welcome. However even "minor" details as fixing spelling
errors, improving documentation or pointing out usability issues are a great
help also.


The restic project uses the GitHub infrastructure (see the
[project page](https://github.com/restic/restic)) for all related discussions
as well as the `#restic` channel on `irc.freenode.net`.

If you want to find an area that currently needs improving have a look at the
open issues listed at the
[issues page](https://github.com/restic/restic/issues). This is also the place
for discussing enhancement to the restic tools.

If you are unsure what to do, please have a look at the issues, especially
those tagged
[minor complexity](https://github.com/restic/restic/labels/minor%20complexity).

Providing Patches
=================

You have fixed an annoying bug or have added a new feature? Very cool! Let's
get it into the project! First we would kindly ask you to fork our project on
GitHub if you haven't done so already.

The restic project uses the `gofmt` tool for go source indentation, so please
run

    gofmt -w **/*.go

in the project root directory before committing. Installing the script
`fmt-check` from https://github.com/edsrzf/gofmt-git-hook locally as a
pre-commit hook checks formatting before committing automatically, just copy
this script to `.git/hooks/pre-commit`.

Then commit your changes as fine grained as possible, as smaller patches, that
handle one and only one issue are easier to discuss and merge.

Please provide the patches for each bug or feature in a separate branch and
open up a pull request for each.

Code Review
===========

The restic project encourages actively reviewing the code, as it will store
your precious data, so it's not uncommon to receive comments on provided
patches.

If you are reviewing other contributor's code please consider the following
when reviewing:

* Be nice. Please make the review comment as constructive as possible so all
  participants will learn something from your review.

As a contributor you might be asked to rewrite portions of your code to make it
fit better into the upstream sources.
