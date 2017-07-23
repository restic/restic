[![GoDoc](https://godoc.org/github.com/restic/chunker?status.svg)](http://godoc.org/github.com/restic/chunker)
[![Build Status](https://travis-ci.org/restic/chunker.svg?branch=master)](https://travis-ci.org/restic/chunker)

The package `chunker` implements content-defined-chunking (CDC) based on a
rolling Rabin Hash. The library is part of the [restic backup
program](https://github.com/restic/restic).

An introduction to Content Defined Chunking can be found in the restic blog
post [Foundation - Introducing Content Defined Chunking (CDC)](https://restic.github.io/blog/2015-09-12/restic-foundation1-cdc).

You can find the API documentation at
https://godoc.org/github.com/restic/chunker
