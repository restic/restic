This document describes the way you can contribute to the restic project.

Ways to Help Out
================

Thank you for your contribution! Please **open an issue first** (or add a
comment to an existing issue) if you plan to work on any code or add a new
feature. This way, duplicate work is prevented and we can discuss your ideas
and design first.

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


Reporting Bugs
==============

You've found a bug? Thanks for letting us know so we can fix it! It is a good
idea to describe in detail how to reproduce the bug (when you know how), what
environment was used and so on. Please tell us at least the following things:

 * What's the version of restic you used? Please include the output of
   `restic version` in your bug report.
 * What commands did you execute to get to where the bug occurred?
 * What did you expect?
 * What happened instead?
 * Are you aware of a way to reproduce the bug?

Remember, the easier it is for us to reproduce the bug, the earlier it will be
corrected!

In addition, you can compile restic with debug support by running
`go run -mod=vendor build.go -tags debug` and instructing it to create a debug
log by setting the environment variable `DEBUG_LOG` to a file, e.g. like this:

    $ export DEBUG_LOG=/tmp/restic-debug.log
    $ restic backup ~/work

For Go < 1.11, you need to remove the `-mod=vendor` option from the build
command.

Please be aware that the debug log file will contain potentially sensitive
things like file and directory names, so please either redact it before
uploading it somewhere or post only the parts that are really relevant.


Development Environment
=======================

The repository contains several sets of directories with code: `cmd/` and
`internal/` contain the code written for restic, whereas `vendor/` contains
copies of libraries restic depends on. The libraries are managed with the
command `go mod vendor`.

Go >= 1.11
----------

For Go version 1.11 or later, you should clone the repo (without having
`$GOPATH` set) and `cd` into the directory:

    $ unset GOPATH
    $ git clone https://github.com/restic/restic
    $ cd restic

Then use the `go` tool to build restic:

    $ go build ./cmd/restic
    $ ./restic version
    restic 0.9.2-dev (compiled manually) compiled with go1.11 on linux/amd64

You can run all tests with the following command:

    $ go test ./...

Go < 1.11
---------

In order to compile restic with Go before 1.11, it needs to be checked out at
the right path within a `GOPATH`. The concept of a `GOPATH` is explained in
["How to write Go code"](https://golang.org/doc/code.html).

If you do not have a directory with Go code yet, executing the following
instructions in your shell will create one for you and check out the restic
repo:

    $ export GOPATH="$HOME/go"
    $ mkdir -p "$GOPATH/src/github.com/restic"
    $ cd "$GOPATH/src/github.com/restic"
    $ git clone https://github.com/restic/restic
    $ cd restic

You can then build restic as follows:

    $ go build ./cmd/restic
    $ ./restic version
    restic compiled manually
    compiled with go1.8.3 on linux/amd64

The following commands can be used to run all the tests:

    $ go test ./...

Providing Patches
=================

You have fixed an annoying bug or have added a new feature? Very cool! Let's
get it into the project! The workflow we're using is also described on the
[GitHub Flow](https://guides.github.com/introduction/flow/) website, it boils
down to the following steps:

 0. If you want to work on something, please add a comment to the issue on
    GitHub. For a new feature, please add an issue before starting to work on
    it, so that duplicate work is prevented.

 1. First we would kindly ask you to fork our project on GitHub if you haven't
    done so already.

 2. Clone the repository locally and create a new branch. If you are working on
    the code itself, please set up the development environment as described in
    the previous section. Especially take care to place your forked repository
    at the correct path (`src/github.com/restic/restic`) within your `GOPATH`.

 3. Then commit your changes as fine grained as possible, as smaller patches,
    that handle one and only one issue are easier to discuss and merge.

 4. Push the new branch with your changes to your fork of the repository.

 5. Create a pull request by visiting the GitHub website, it will guide you
    through the process.

 6. You will receive comments on your code and the feature or bug that they
    address. Maybe you need to rework some minor things, in this case push new
    commits to the branch you created for the pull request (or amend the
    existing commit, use common sense to decide which is better), they will be
    automatically added to the pull request.

 7. If your pull request changes anything that users should be aware of (a
    bugfix, a new feature, ...) please add an entry to the file
    ['CHANGELOG.md'](CHANGELOG.md). It will be used in the announcement of the
    next stable release. While writing, ask yourself: If I were the user, what
    would I need to be aware of with this change.

 8. Once your code looks good and passes all the tests, we'll merge it. Thanks
    a lot for your contribution!

Please provide the patches for each bug or feature in a separate branch and
open up a pull request for each.

The restic project uses the `gofmt` tool for Go source indentation, so please
run

    gofmt -w **/*.go

in the project root directory before committing. For each Pull Request, the
formatting is tested with `gofmt` for the latest stable version of Go.
Installing the script `fmt-check` from https://github.com/edsrzf/gofmt-git-hook
locally as a pre-commit hook checks formatting before committing automatically,
just copy this script to `.git/hooks/pre-commit`.

For each pull request, several different systems run the integration tests on
Linux, macOS and Windows. We won't merge any code that does not pass all tests
for all systems, so when a tests fails, try to find out what's wrong and fix
it. If you need help on this, please leave a comment in the pull request, and
we'll be glad to assist. Having a PR with failing integration tests is nothing
to be ashamed of. In contrast, that happens regularly for all of us. That's
what the tests are there for.

Git Commits
-----------

It would be good if you could follow the same general style regarding Git
commits as the rest of the project, this makes reviewing code, browsing the
history and triaging bugs much easier.

Git commit messages have a very terse summary in the first line of the commit
message, followed by an empty line, followed by a more verbose description or a
List of changed things. For examples, please refer to the excellent [How to
Write a Git Commit Message](https://chris.beams.io/posts/git-commit/).

If you change/add multiple different things that aren't related at all, try to
make several smaller commits. This is much easier to review. Using `git add -p`
allows staging and committing only some changes.

Code Review
===========

The restic project encourages actively reviewing the code, as it will store
your precious data, so it's common practice to receive comments on provided
patches.

If you are reviewing other contributor's code please consider the following
when reviewing:

* Be nice. Please make the review comment as constructive as possible so all
  participants will learn something from your review.

As a contributor you might be asked to rewrite portions of your code to make it
fit better into the upstream sources.
