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


Code of Conduct
===============

The following text is derived from the [Django Project's Code of
Conduct](https://www.djangoproject.com/conduct/), which is licensed as
[Creative Commons Attribution](https://creativecommons.org/licenses/by/3.0/).

Like the technical community as a whole, the restic team and community is made
up of a mixture of professionals and volunteers from all over the world,
working on every aspect of the mission - including mentorship, teaching, and
connecting people.

Diversity is one of our huge strengths, but it can also lead to communication
issues and unhappiness. To that end, we have a few ground rules that we ask
people to adhere to. This code applies equally to founders, mentors and those
seeking help and guidance.

This isn't an exhaustive list of things that you can't do. Rather, take it in
the spirit in which it's intended - a guide to make it easier to enrich all of
us and the technical communities in which we participate.

This code of conduct applies to all spaces managed by the restic project. This
includes IRC, the issue tracker, the forum, and any other forums created by the
project team which the community uses for communication. In addition,
violations of this code outside these spaces may affect a person's ability to
participate within them.

If you believe someone is violating the code of conduct, we ask that you report
it by emailing conduct AT restic.net.

 * Be friendly and patient.

 * Be welcoming. We strive to be a community that welcomes and supports people
   of all backgrounds and identities. This includes, but is not limited to
   members of any race, ethnicity, culture, national origin, colour,
   immigration status, social and economic class, educational level, sex,
   sexual orientation, gender identity and expression, age, size, family
   status, political belief, religion, and mental and physical ability.

 * Be considerate. Your work will be used by other people, and you in turn will
   depend on the work of others. Any decision you take will affect users and
   colleagues, and you should take those consequences into account when making
   decisions. Remember that we're a world-wide community, so you might not be
   communicating in someone else's primary language.

 * Be respectful. Not all of us will agree all the time, but disagreement is no
   excuse for poor behavior and poor manners. We might all experience some
   frustration now and then, but we cannot allow that frustration to turn into
   a personal attack. It's important to remember that a community where people
   feel uncomfortable or threatened is not a productive one. Members of the
   restic community should be respectful when dealing with other members as
   well as with people outside the restic community.

 * Be careful in the words that you choose. We are a community of
   professionals, and we conduct ourselves professionally. Be kind to others.
   Do not insult or put down other participants. Harassment and other
   exclusionary behavior aren't acceptable. This includes, but is not limited
   to:

    - Violent threats or language directed against another person.

    - Discriminatory jokes and language.

    - Posting sexually explicit or violent material.

    - Posting (or threatening to post) other people's personally identifying information ("doxing").

    - Personal insults, especially those using racist or sexist terms.

    - Unwelcome sexual attention.

    - Advocating for, or encouraging, any of the above behavior.

    - Repeated harassment of others. In general, if someone asks you to stop, then stop.

 * When we disagree, try to understand why. Disagreements, both social and
   technical, happen all the time and restic is no exception. It is important
   that we resolve disagreements and differing views constructively. Remember
   that we're different. The strength of restic comes from its varied
   community, people from a wide range of backgrounds. Different people have
   different perspectives on issues. Being unable to understand why someone
   holds a viewpoint doesn't mean that they're wrong. Don't forget that it is
   human to err and blaming each other doesn't get us anywhere. Instead, focus
   on helping to resolve issues and learning from mistakes.

Original text courtesy of the [Speak Up! project](http://web.archive.org/web/20141109123859/http://speakup.io/coc.html).

Questions?
----------

If you have questions, please see [the
FAQ](https://www.djangoproject.com/conduct/faq/). If that doesn't answer your
questions, feel free to contact us.

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
`go run build.go -tags debug` and instructing it to create a debug log by
setting the environment variable `DEBUG_LOG` to a file, e.g. like this:

    $ export DEBUG_LOG=/tmp/restic-debug.log
    $ restic backup ~/work

Please be aware that the debug log file will contain potentially sensitive
things like file and directory names, so please either redact it before
uploading it somewhere or post only the parts that are really relevant.


Development Environment
=======================

In order to compile restic with the `go` tool directly, it needs to be checked
out at the right path within a `GOPATH`. The concept of a `GOPATH` is explained
in ["How to write Go code"](https://golang.org/doc/code.html).

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

    $ go test ./cmd/... ./internal/...

The repository contains two sets of directories with code: `cmd/` and
`internal/` contain the code written for restic, whereas `vendor/` contains
copies of libraries restic depends on. The libraries are managed with the
[`dep`](https://github.com/golang/dep) tool.

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

in the project root directory before committing. Installing the script
`fmt-check` from https://github.com/edsrzf/gofmt-git-hook locally as a
pre-commit hook checks formatting before committing automatically, just copy
this script to `.git/hooks/pre-commit`.

For each pull request, several different systems run the integration tests on
Linux, OS X and Windows. We won't merge any code that does not pass all tests
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
Write a Git Commit Message](http://chris.beams.io/posts/git-commit/).

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
