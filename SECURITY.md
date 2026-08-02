# Security

## Reporting

Report a vulnerability through GitHub's private advisory form on this repository
(Security → Report a vulnerability), not as a public issue.

## What counts as a vulnerability here

This package parses strings that arrive from outside — a UCUM code in a FHIR
resource is attacker-controlled input — and it has no dependencies, so its
attack surface is the parser, the canonicalizer and the standard library. The
following are treated as vulnerabilities rather than as ordinary bugs:

- **Any panic or crash from a code string.** A Go stack overflow in particular,
  since it is a fatal error that `recover` cannot catch: it takes the process
  down, not the request. Two hundred nested parentheses used to do exactly that.
- **Unbounded time or memory for a bounded input.** `"m2000000000"` is eleven
  characters and used to spend minutes building an integer of billions of
  digits.
- **Unbounded growth over many requests.** An annotation is free text, so
  `mg/dL{lot17}` is a valid code and there are unboundedly many of them; the
  parse cache used to keep every one.

The bounds that close those off are public and documented — `MaxCodeLength`,
`MaxNestingDepth`, `MaxExponent` and `MaxCacheEntries` — and they are set far
above anything a real code needs. A code that legitimately exceeds one of them
is a bug report, not a security issue.

A *wrong conversion* is a correctness bug, and an important one in a clinical
context, but it is reported as an ordinary issue.

## Supported versions

The latest minor release. Fixes are not backported to earlier majors.
