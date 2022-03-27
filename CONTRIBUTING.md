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
as well as the [forum](https://forum.restic.net/) and the `#restic` channel
on [irc.libera.chat](https://kiwiirc.com/nextclient/#ircs://irc.libera.chat:6697/#restic).

If you want to find an area that currently needs improving have a look at the
open issues listed at the
[issues page](https://github.com/restic/restic/issues). This is also the place
for discussing enhancement to the restic tools.

If you are unsure what to do, please have a look at the issues, especially
those tagged
[minor complexity](https://github.com/restic/restic/labels/help%3A%20minor%20complexity)
or [good first issue](https://github.com/restic/restic/labels/help%3A%20good%20first%20issue).
If you are already a bit experienced with the restic internals, take a look
at the issues tagged as [help wanted](https://github.com/restic/restic/labels/help%3A%20wanted).


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
`go run build.go -tags debug` and instructing it to create a debug
log by setting the environment variable `DEBUG_LOG` to a file, e.g. like this:

    $ export DEBUG_LOG=/tmp/restic-debug.log
    $ restic backup ~/work

Please be aware that the debug log file will contain potentially sensitive
things like file and directory names, so please either redact it before
uploading it somewhere or post only the parts that are really relevant.


Development Environment
=======================

The repository contains the code written for restic in the directories
`cmd/` and `internal/`.

Make sure you have the minimum required Go version installed. Clone the repo
(without having `$GOPATH` set) and `cd` into the directory:

    $ unset GOPATH
    $ git clone https://github.com/restic/restic
    $ cd restic

Then use the `go` tool to build restic:

    $ go build ./cmd/restic
    $ ./restic version
    restic 0.10.0-dev (compiled manually) compiled with go1.15.2 on linux/amd64

You can run all tests with the following command:

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

 1. Next, fork our project on GitHub if you haven't done so already.

 2. Clone your fork of the repository locally and **create a new branch** for
    your changes. If you are working on the code itself, please set up the
    development environment as described in the previous section.

 3. Commit your changes to the new branch as fine grained as possible, as
    smaller patches, for individual changes, are easier to discuss and merge.

 4. Push the new branch with your changes to your fork of the repository.

 5. Create a pull request by visiting the GitHub website, it will guide you
    through the process. Please [allow edits from maintainers](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/allowing-changes-to-a-pull-request-branch-created-from-a-fork).

 6. You will receive comments on your code and the feature or bug that they
    address. Maybe you need to rework some minor things, in this case push new
    commits to the branch you created for the pull request (or amend the
    existing commit, use common sense to decide which is better), they will be
    automatically added to the pull request.

 7. If your pull request changes anything that users should be aware of
    (a bugfix, a new feature, ...) please add an entry as a new file in
    `changelog/unreleased` including the issue number in the filename (e.g.
    `issue-8756`). Use the template in `changelog/TEMPLATE` for the content.
    It will be used in the announcement of the next stable release. While
    writing, ask yourself: If I were the user, what would I need to be aware
    of with this change?

 8. Do not edit the man pages under `doc/man` or `doc/manual_rest.rst` -
    these are autogenerated before new releases.

 9. Once your code looks good and passes all the tests, we'll merge it. Thanks
    a lot for your contribution!

Please provide the patches for each bug or feature in a separate branch and
open up a pull request for each, as this simplifies discussion and merging.

The restic project uses the `gofmt` tool for Go source indentation, so please
run

    gofmt -w **/*.go

in the project root directory before committing. For each Pull Request, the
formatting is tested with `gofmt` for the latest stable version of Go.
Installing the script `fmt-check` from https://github.com/edsrzf/gofmt-git-hook
locally as a pre-commit hook checks formatting before committing automatically,
just copy this script to `.git/hooks/pre-commit`.

The project is using the program
[`golangci-lint`](https://github.com/golangci/golangci-lint) to run a list of
linters and checkers. It will be run on the code when you submit a PR. In order
to check your code beforehand, you can run `golangci-lint run` manually.
Eventually, we will enable `golangci-lint` for the whole code base. For now,
you can ignore warnings printed for lines you did not modify, those will be
ignored by the CI.

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
