# Contributing

## What this package is

A UCUM engine, plus the FHIR-facing rules built on it. It is deliberately not a
terminology server and carries no browsable catalogue of units — see [Scope](README.md#scope)
before proposing an API that enumerates or searches units.

## Ground rules

**Every claim about UCUM cites its source.** Not "this is how temperature
works", but the section of the specification that says so, quoted at the
declaration it justifies. The specification is at <https://ucum.org/ucum>, and
the same applies to FHIR and FHIRPath rules in `fhir/`. Where two documents
disagree, document the disagreement rather than resolving it silently: there are
several examples in the code already, including one where the UCUM prose and its
own conformance suite contradict each other.

**Read the definitions, do not restate them.** Every number UCUM publishes —
conversion factors, reference quantities, which units are metric, which are
arbitrary, which function a special unit performs — is read from
`ucum-essence.xml` at load time. A constant in Go that duplicates a figure from
that file is a bug waiting to diverge; four of them were. The only figures that
live in code are the ones the specification states in prose and the XML does not
carry, such as the origin of each temperature scale, and those are keyed by
function name so a new unit reusing an existing function needs no code at all.

**The definitions file is never edited.** Its licence forbids it. Updating to a
newer UCUM release means replacing the file wholesale and fixing whatever the
tests then say.

## Before you open a pull request

```
go test ./...             # all of it, including the official HL7 suite
go test -race ./...
golangci-lint run ./...   # the config is strict on purpose; zero issues
gofmt -l .                # empty
```

`make check` runs the formatting check, `go vet`, the linter and the tests in
one go. Two more targets cover what is too slow or too large for every run:
`make check-packages` downloads HL7's official R4, R4B and R5 example packages
(about 54 MB, SHA-256 pinned in `scripts/packages.sha256`) and checks every UCUM
quantity in them; `make mutations` injects known defects into a copy of the
repository and confirms that each one makes its test fail.

New behaviour needs a test that would have failed before it. A bug fix needs a
test that reproduces the bug, and the commit message should say what the wrong
answer was — that is what makes a regression recognisable a year later.

If you touch the lexer, the parser or the canonicalizer, run the fuzzers for
longer than CI does:

```
go test -run XXX -fuzz FuzzValidate ./internal/engine/
go test -run XXX -fuzz FuzzComposeRoundTrip ./internal/engine/
```

Both have found real bugs in code that passed the whole suite.

## Layout

The root package is the public API and nothing else. Everything under
`internal/` is free to change without a major version:

```
internal/ucumerr   error types and sentinels (a leaf, so any layer can build one)
internal/decimal   exact rational arithmetic
internal/model     the definitions in memory
internal/essence   ucum-essence.xml and its reader
internal/expr      lexer, parser, AST, composer
internal/special   the non-ratio scales
internal/engine    canonicalization and conversion
```

The dependency graph is acyclic and stays that way: `decimal ← model ←
{essence, expr, special} ← engine ← root`.

Adding a method to `Service` or `ExactService` breaks every implementation of
those interfaces and forces a major version. Add a small interface instead —
`Identified` and `PropertyLister` are the precedent — or, when the operation
needs nothing the interface does not already expose, a free function, as
`ValidateCanonicalUnits` does.

## Commits

[Conventional Commits](https://www.conventionalcommits.org/), because
release-please reads them to decide the version and write the changelog. `feat`
for anything that adds public surface, `fix` for a behaviour correction, and `!`
only for a genuine break: a changed signature or a removed export. A result that
changes because it was previously wrong is a `fix`.

## Developer Certificate of Origin

The project is licensed under the [BSD 3-Clause License](LICENSE), and every
contribution is accepted under the same licence. Each commit in a pull request
must be signed off to certify the Developer Certificate of Origin below, with a
line like this at the end of the commit message:

```
Signed-off-by: Your Name <you@example.com>
```

`git commit -s` adds it from the `user.name` and `user.email` in your Git
configuration. Use your real name: the sign-off is a public, permanent record.
To sign off the commits already in a branch, run `git rebase --signoff main`
and force-push it.

A pull request whose commits are not all signed off is not merged. Commits by
the copyright holder named in [LICENSE](LICENSE) do not need a sign-off: the
certificate is how a contributor licenses their work to the project, and the
holder's work is already theirs. Contributors keep the copyright on what they
write; the Git history records who wrote it.

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```
