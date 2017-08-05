# simple-scrypt
[![GoDoc](https://godoc.org/github.com/elithrar/simple-scrypt?status.svg)](https://godoc.org/github.com/elithrar/simple-scrypt) [![Build Status](https://travis-ci.org/elithrar/simple-scrypt.svg?branch=master)](https://travis-ci.org/elithrar/simple-scrypt)

simple-scrypt provides a convenience wrapper around Go's existing
[scrypt](http://golang.org/x/crypto/scrypt) package that makes it easier to
securely derive strong keys ("hash user passwords"). This library allows you to:

* Generate a scrypt derived key with a crytographically secure salt and sane
  default parameters for N, r and p.
* Upgrade the parameters used to generate keys as hardware improves by storing
  them with the derived key (the scrypt spec. doesn't allow for this by
  default).
* Provide your own parameters (if you wish to).

The API closely mirrors Go's [bcrypt](https://golang.org/x/crypto/bcrypt)
library in an effort to make it easy to migrate—and because it's an easy to grok
API.

## Installation

With a [working Go toolchain](https://golang.org/doc/code.html):

```sh
go get -u github.com/elithrar/simple-scrypt
```

## Example

simple-scrypt doesn't try to re-invent the wheel or do anything "special". It
wraps the `scrypt.Key` function as thinly as possible, generates a
crytographically secure salt for you using Go's `crypto/rand` package, and
returns the derived key with the parameters prepended:

```go
package main

import(
    "fmt"
    "log"

    "github.com/elithrar/simple-scrypt"
)

func main() {
    // e.g. r.PostFormValue("password")
    passwordFromForm := "prew8fid9hick6c"

    // Generates a derived key of the form "N$r$p$salt$dk" where N, r and p are defined as per
    // Colin Percival's scrypt paper: http://www.tarsnap.com/scrypt/scrypt.pdf
    // scrypt.Defaults (N=16384, r=8, p=1) makes it easy to provide these parameters, and
    // (should you wish) provide your own values via the scrypt.Params type.
    hash, err := scrypt.GenerateFromPassword([]byte(passwordFromForm), scrypt.DefaultParams)
    if err != nil {
        log.Fatal(err)
    }

    // Print the derived key with its parameters prepended.
    fmt.Printf("%s\n", hash)

    // Uses the parameters from the existing derived key. Return an error if they don't match.
    err := scrypt.CompareHashAndPassword(hash, []byte(passwordFromForm))
    if err != nil {
        log.Fatal(err)
    }
}
```

## Upgrading Parameters

Upgrading derived keys from a set of parameters to a "stronger" set of parameters
as hardware improves, or as you scale (and move your auth process to separate
hardware), can be pretty useful. Here's how to do it with simple-scrypt:

```go
func main() {
    // SCENE: We've successfully authenticated a user, compared their submitted
    // (cleartext) password against the derived key stored in our database, and
    // now want to upgrade the parameters (more rounds, more parallelism) to
    // reflect some shiny new hardware we just purchased. As the user is logging
    // in, we can retrieve the parameters used to generate their key, and if
    // they don't match our "new" parameters, we can re-generate the key while
    // we still have the cleartext password in memory
    // (e.g. before the HTTP request ends).
    current, err := scrypt.Cost(hash)
    if err != nil {
        log.Fatal(err)
    }

    // Now to check them against our own Params struct (e.g. using reflect.DeepEquals)
    // and determine whether we want to generate a new key with our "upgraded" parameters.
    slower := scrypt.Params{
        N: 32768,
        R: 8,
        P: 2,
        SaltLen: 16,
        DKLen: 32,
    }

    if !reflect.DeepEqual(current, slower) {
        // Re-generate the key with the slower parameters
        // here using scrypt.GenerateFromPassword
    }
}
```

## Automatically Determining Parameters

Thanks to the work by [tgulacsi](https://github.com/tgulacsi), you can have simple-scrypt
automatically determine the optimal parameters for you (time vs. memory). You should run this once
on program startup, as calibrating parameters can be an expensive operation.

```go
var params scrypt.Params

func main() {
    var err error
    // 500ms, 64MB of RAM per hash.
    params, err = scrypt.Calibrate(500*time.Millisecond, 64, Params{})
    if err != nil {
        return nil, err
    }

    ...
}

func RegisterUserHandler(w http.ResponseWriter, r *http.Request) {
    err := r.ParseForm()
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Make sure you validate: not empty, not too long, etc.
    email := r.PostFormValue("email")
    pass := r.PostFormValue("password")

    // Use our calibrated parameters
    hash, err := scrypt.GenerateFromPassword([]byte(pass), params)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Save to DB, etc.
}
```

Be aware that increasing these, whilst making it harder to brute-force the resulting hash, also
increases the risk of a denial-of-service attack against your server. A surge in authenticate
attempts (even if legitimate!) could consume all available resources.

## License

MIT Licensed. See LICENSE file for details.

