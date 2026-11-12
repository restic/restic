This document describes how to report security issues and bugs, and how to provide
patches for restic.

Ways to Help Out
================

Thank you for your contribution! Always check if there is an existing issue and
add a comment there or otherwise **open an issue first** if you plan to work
on any larger amount of code or add a new significant feature.
This way, duplicate work is prevented and we can discuss your ideas and
design first. Small bugfixes are an exception to this rule, just open a
pull request in this case.

Contributions are welcome in general, but expect long review times due to
a general overload of the maintainers.

The restic project uses the GitHub infrastructure (see the
[project page](https://github.com/restic/restic)) for all related discussions
as well as the [forum](https://forum.restic.net/) and the `#restic` channel
on [irc.libera.chat](https://web.libera.chat/#restic).

If you want to find an area that currently needs improving, have a look at the
open issues listed at the [issues page](https://github.com/restic/restic/issues).
This is also the place for discussing enhancements to the restic tools.

If you are unsure what to do, please have a look at the issues, especially
those tagged
[minor complexity](https://github.com/restic/restic/labels/help%3A%20minor%20complexity)
or [good first issue](https://github.com/restic/restic/labels/help%3A%20good%20first%20issue).
If you are already a bit experienced with the restic internals, take a look
at the issues tagged as [help wanted](https://github.com/restic/restic/labels/help%3A%20wanted).


Reporting Bugs
==============

You've found a bug? Thanks for letting us know so we can fix it! Please fill out
the [Bug issue template](https://github.com/restic/restic/blob/master/.github/ISSUE_TEMPLATE/Bug.md?plain=1),
but refer to [Security Policy](./SECURITY.md) for critical security issues.
We need the information to reproduce and investigate bugs. Remember, the easier
it is for us to reproduce the bug, the earlier it will be corrected!

In addition, you can instruct restic to create a debug log by setting the
environment variable `DEBUG_LOG` to a file, e.g. like this:

    $ export DEBUG_LOG=/tmp/restic-debug.log
    $ restic backup ~/work

Please be aware that the debug log file will contain potentially sensitive
things like file and directory names, so please either redact it before
uploading it somewhere or post only the parts that are really relevant.

If restic gets stuck, please also include a stacktrace in the description.
On non-Windows systems, you can send a SIGQUIT signal to restic or press
`Ctrl-\` to achieve the same result. This causes restic to print a stacktrace
and then exit immediately. This will not damage your repository, however,
it might be necessary to manually clean up stale lock files using
`restic unlock`.

On Windows, please run restic with the environment variable
`RESTIC_DEBUG_STACKTRACE_SIGINT` set to `true` and press `Ctrl-C` to
create a stacktrace.

If you think restic uses too much memory or the cache directory is too large,
then please include the output of `restic stats --mode debug`.


Restic Development
==================

The [development environment](doc/090_participating.rst#development-environment)
section describes how to build restic and run the tests. The documentation also
contains guidance on debugging and writing tests.


Providing Patches
=================

You have fixed an annoying bug or have added a new feature? Very cool! Let's
get it into the project! The workflow we're using is also described on the
[GitHub Flow](https://guides.github.com/introduction/flow/) website, it boils
down to the following steps:

 0. If you want to work on something, please add a comment to the issue on
    GitHub. For a new feature, please add an issue before starting to work on
    it, so that duplicate work is prevented. Small bugfixes are an exception
    to this rule, just open a pull request in this case.

 1. Next, fork our project on GitHub if you haven't done so already.

 2. Clone your fork of the repository locally and **create a new branch** for
    your changes. If you are working on the code itself, please set up the
    development environment as described in the previous section.

 3. Commit in small, cohesive pieces. Smaller patches are easier to discuss and merge.

 4. Push the new branch with your changes to your fork of the repository.

 5. Create a pull request by visiting the GitHub website. It will guide you
    through the process. You **must** fill out the [Pull Request template](https://github.com/restic/restic/blob/master/.github/PULL_REQUEST_TEMPLATE.md?plain=1).
    Please also [allow edits from maintainers](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/allowing-changes-to-a-pull-request-branch-created-from-a-fork).

 6. You will receive comments on your commits and the feature or bug that they
    address. Maybe you need to rework some minor things, in this case push new
    commits to the branch you created for the pull request (or amend the
    existing commit; use common sense to decide which is better), they will be
    automatically added to the pull request. Do not include merge commits.

    Please note that it may take a while until a maintainer finds time to review
    your pull request. Please refrain from pinging maintainers for a review after
    a few days, unless the fix is critical.

    Maintainers may assign themselves to your pull request to simplify tracking
    of pull requests to review, but you are still responsible for addressing review
    comments.

 7. If your pull request changes anything that users should be aware of
    (a bugfix, a new feature, ...) please add an entry as a new file in
    `changelog/unreleased` including the issue number in the filename (e.g.
    `pull-1234` or `issue-8756`). Use the template in `changelog/TEMPLATE` for the content.
    It will be used in the announcement of the next stable release. While
    writing, ask yourself: If I were the user, what would I need to be aware
    of with this change? Documentation-only changes do not get a changelog entry.

 8. Do not edit the man pages under `doc/man` or the generated completion files
    at `doc/*-completion.*` - these are autogenerated before new releases.

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

For each pull request, several different systems run the integration tests on
Linux, macOS and Windows. We won't merge any code that does not pass all tests
for all systems, so when a test fails, try to find out what's wrong and fix
it. If you need help on this, please leave a comment in the pull request, and
we'll be glad to assist. Having a PR with failing integration tests is nothing
to be ashamed of. That happens regularly for all of us. That's what the tests
are there for.

More details of how to structure tests can be found here at
[writing tests](https://restic.readthedocs.io/en/stable/090_participating.html#writing-tests).

Git Commits
-----------

It would be good if you could follow the same general style regarding Git
commits as the rest of the project, this makes reviewing code, browsing the
history and triaging bugs much easier.

Git commit messages have a very terse summary in the first line of the commit
message, followed by an empty line, followed by a more verbose description or a
list of changed things. For examples, please refer to the excellent [How to
Write a Git Commit Message](https://chris.beams.io/posts/git-commit/).

If you change/add multiple different things that aren't related at all, try to
make several smaller commits. This is much easier to review. Using `git add -p`
allows staging and committing only some changes.

Do not add merge commits in pull requests. Instead rebase your pull request
onto the master branch if there are merge conflicts.

Code Review
===========

The restic project encourages actively reviewing the code, as it will store
your precious data, so it's common practice to receive comments on provided
patches.

If you are reviewing another contributor's code please consider the following
when reviewing:

* Be nice. Please make the review comment as constructive as possible so all
  participants will learn something from your review.

As a contributor you might be asked to rewrite portions of your code to make it
fit better into the upstream sources.
