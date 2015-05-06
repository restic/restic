This document describes the way you can contribute to the restic
project.

Ways to help out
================

Thank you for your contribution!

There are several ways you can help us out. First of all code
contributions and bugfixes are most welcome. However even "minor"
details as fixing spelling errors, improving documentation or pointing
out usability issues are a great help also.

The restic project uses the github infrastructure ([project page][1])
for all related discussions as well as the '#restic' channel on
irc.freenode.net.

If you want to find an area that currently needs improving have a look
at the open issues listed at the [issues page][2]. This is also the
place for discussing enhancement to the restic tools.

Providing patches
=================

You have fixed an annoying bug or have added a new feature? Very cool!
Let's get it into the project! First we would kindly ask you to fork
our project on github if you haven't done so already.

The restic project uses the *gofmt* tool for go source indentation, so
please run

    gofmt -w **/*.go

in the project root directory before commiting.

Then commit your changes as fine grained as possible, as smaller
patches, that handle one and only one issue are easier to discuss and
merge.

Please provide the patches for each bug or feature in a separate
branch and open up a pull request for each.

Code review
===========

The restic project encourages actively reviewing the code, as it will
store your precious data, so it's not uncommon to recieve comments on
provided patches.

If you are reviewing other contributor's code please consider the
following when reviewing:

* Be nice.
* Please make the review comment as constructive as possible so all
  paticipants will learn something from your review.

As a contributor you might be asked to rewrite portions of your code
to make it fit better into the upstream sources.

[1]: https://github.com/restic/restic/
[2]: https://github.com/restic/restic/issues
