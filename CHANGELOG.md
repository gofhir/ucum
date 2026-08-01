# Changelog

## [2.1.0](https://github.com/gofhir/ucum/compare/v2.0.2...v2.1.0) (2026-08-01)


### Features

* **fhir:** add a FHIR subpackage ([#20](https://github.com/gofhir/ucum/issues/20)) ([9ce2ef4](https://github.com/gofhir/ucum/commit/9ce2ef41b83b1d24586c4268ed87ad4005b77f4f))

  `github.com/gofhir/ucum/v2/fhir` carries the FHIR-specific rules a UCUM engine cannot infer. The root package is unchanged.

  - **ucum-common value set**, embedded from the published resource (version 5.0.0): `InCommonUnits`, `CommonDisplay`, `CommonCodes`. All 840 distinct codes are checked against the engine by a conformance test. The published resource lists 848 concepts but repeats eight codes.
  - **Calendar durations**: `CalendarEquivalentOf`, `MappedByFHIRQuantityConversion`, `AllowedInDateTimeArithmetic`, `IsDefiniteDuration`. FHIRPath pairs eight UCUM time units with calendar keywords and distinguishes equality from equivalence — only `s` and `ms` are equal, since a UCUM year is exactly 365.25 days. FHIR R5's implicit `Quantity` mapping is a narrower six codes, kept as a separate table.
  - **Quantity comparison**: `Comparator.Compare`, `Comparable` and `CanonicalKey`, for quantity search. Exact wherever the units allow, with a `float64` fallback for the non-rational scales. `Quantity.Exact` carries a FHIR decimal without float64 rounding, which matters because the float64 nearest `0.01` is not `1/100`.

  Every rule cites the specification it comes from. Where FHIR and UCUM disagree the disagreement is documented rather than resolved: FHIR R5's prose says UCUM defines a month as 30 days, while UCUM defines `mo` as a twelfth of a Julian year (30.4375 days), and this library follows UCUM.

### Documentation

* the repository now has a README, covering the exact API, the three classes of unit scale, the FHIR subpackage and the known limitations. Every snippet in it was compiled and run against the published module.

## [2.0.2](https://github.com/gofhir/ucum/compare/v2.0.1...v2.0.2) (2026-08-01)


### Bug Fixes

* **make ValidateInProperty resolve real properties** ([#15](https://github.com/gofhir/ucum/issues/15)) ([dda7f54](https://github.com/gofhir/ucum/commit/dda7f5482ca59b9f43db2d3a76f34e4650f13e0c))

  The method only worked for units canonicalizing to a single base unit with exponent 1. Everything else compared a canonical unit string against a property name, so it could never match: `ValidateInProperty("N", "force")` reported `has property "g.m.s-2"`, and `("L", "volume")`, `("mm[Hg]", "pressure")` and `("mol", "amount of substance")` failed the same way. It was exported with 0% test coverage.

  Behaviour changes in three ways. Units that genuinely have the requested property are now accepted — atomic units against the property UCUM declares for them, compound expressions dimensionally against the units that declare it (`m/s2` as acceleration, `kg.m/s2` as force, `g/L` as mass concentration). An atomic unit is judged *strictly* by its declared property, so `mol` is no longer accepted as a `fraction` even though both are dimensionless in UCUM. And an unknown property name is now an error rather than a silent mismatch.

  Properties cannot be resolved dimensionally alone: UCUM gives 15 canonical forms to more than one property — `"1"` to 11 of them, `"m"` to 4 — so the residual ambiguity for compound expressions is documented on the method.

## [2.0.1](https://github.com/gofhir/ucum/compare/v2.0.0...v2.0.1) (2026-08-01)


### Bug Fixes

* add the /v2 major version suffix to the module path ([#13](https://github.com/gofhir/ucum/issues/13)) ([442e6f4](https://github.com/gofhir/ucum/commit/442e6f443d1dc052b8d676a4feef7c8f8fc106a7))

## [2.0.0](https://github.com/gofhir/ucum/compare/v1.0.1...v2.0.0) (2026-08-01)


### ⚠ BREAKING CHANGES

* **Analyze** returns a different string for every input. It now renders the display format of the official HL7 test suite: `Analyze("m3.kg-1.s-2")` was `"meter3.kilogram-1.second-2"` and is now `"(meter ^ 3) * (kilogram ^ -1) * (second ^ -2)"`. It is a human-readable description rather than a parseable code, but anything asserting on or displaying it will see the change. `Analyze("")` also returns `"(unity)"` instead of an error, as the suite requires; `Validate("")` still rejects it.
* **Divide** is added to the `Service` interface, next to `Multiply`. Code that calls the interface is unaffected; code that *implements* `ucum.Service` — a test double, for instance — must add the method.
* **Arbitrary units** ([IU], [iU], [arb'U], ...) are now each their own dimension, as UCUM requires. Conversions and comparisons between different arbitrary units, or between an arbitrary unit and a dimensionless one, now fail instead of returning a meaningless number: `Convert(1, "[arb'U]", "[IU]")` was `1` and is now an error, and `IsComparable("[IU]", "mol")` was `true` and is now `false`. `Canonical` reports the arbitrary unit as its own canonical code rather than `"1"`. Conversions that only rescale the rest of the expression still work, such as `Convert(5, "[IU]/L", "[IU]/mL")`.
* **Numeric results change in the last bits** wherever a conversion is now exact. `Convert(1, "L", "mL")` was `1000.0000000000001137` and is `1000`; `Convert(100, "[degF]", "Cel")` was `37.777777777777828` and is `37.777777777777778`. Any test asserting on the old inexact values will fail.
* **Canonical applies the special-unit handler**, which it previously skipped. `Canonical(1, "Cel")` was `{1 K}` and is `{274.15 K}`. Code that compared canonical forms of special units was silently comparing raw scale values.
* **Conversions involving a zero divisor now return an error** instead of panicking or returning `+Inf`. `Canonical(1, "m/0")` panicked; `Convert(1, "1", "0")` returned `+Inf`.

### Features

* **exact rational API.** `ExactService` adds `ConversionFactor(from, to) (*big.Rat, error)`, `ConvertRat(value *big.Rat, from, to)` and `CanonicalRat(value *big.Rat, code)`, for callers that need results free of `float64` rounding. Additive and behind its own interface, so `Service` keeps its shape; the value returned by `New` also satisfies it. `big.Rat` is stdlib, so the dependency set stays empty. Errors distinguish the three classes a unit can fall into: `ErrNotLinear` for the affine scales, where no single factor exists but `ConvertRat` is still exact, and `ErrNotRational` for the logarithmic and trigonometric ones, where no exact rational result exists at all ([#3](https://github.com/gofhir/ucum/issues/3))
* **`Divide`** on `Service`, mirroring `Multiply` including the exact path ([#3](https://github.com/gofhir/ucum/issues/3))
* the full official HL7 test suite now runs: all 573 cases across validation, conversion, multiplication, division and display-name generation. The `<division>` and `<displayNameGeneration>` sections, 12 cases, were previously skipped in silence because the XML structs did not map them ([#3](https://github.com/gofhir/ucum/issues/3))

### Bug Fixes

* exact arithmetic, special-unit canonicalization, a zero-divisor panic, and full official-suite coverage ([#3](https://github.com/gofhir/ucum/issues/3)) ([ba5cac4](https://github.com/gofhir/ucum/commit/ba5cac48de2abbe8c60c84bf47f0eed8392a35a8))
* **special:** `[degRe]` used the Celsius offset, putting `0 °Ré` at `341.4375 K` instead of `273.15 K` ([#3](https://github.com/gofhir/ucum/issues/3))
* **special:** the five units defined with `lgTimes2` computed `10^(2v)` instead of `10^(v/2)` ([#4](https://github.com/gofhir/ucum/issues/4))
* **special:** `B[SPL]` lost the `2` in its `2×10⁻⁵ Pa` reference level ([#5](https://github.com/gofhir/ucum/issues/5))
* **special:** `%[slope]` returned radians labelled as degrees, off by `180/π` ([#6](https://github.com/gofhir/ucum/issues/6))
* **special:** restore the 10 in the B[10.nV] reference level ([#9](https://github.com/gofhir/ucum/issues/9)) ([c87c946](https://github.com/gofhir/ucum/commit/c87c946f2dcc64c931fce04720001fd045e3e296))

## [1.0.1](https://github.com/gofhir/ucum/compare/v1.0.0...v1.0.1) (2026-03-29)


### Bug Fixes

* resolve all 52 golangci-lint issues with full config ([19c2f9c](https://github.com/gofhir/ucum/commit/19c2f9cc4ca2714b0ca6b5bc7ae0a31aed727071))

## 1.0.0 (2026-03-29)


### Features

* add converter and full service implementation with special unit support ([69db35f](https://github.com/gofhir/ucum/commit/69db35f7fbe61755aa6b4cee2c0cea69b212d493))
* add expression parser, composer, and canonical types ([f13bf9b](https://github.com/gofhir/ucum/commit/f13bf9ba229a554da33df5e48894d5333a2de4b9))
* initial implementation - scaffolding, decimal, AST, model, lexer, special handlers ([f17d310](https://github.com/gofhir/ucum/commit/f17d310f9da9a30c480c0249538bc6eeb0611b0d))


### Bug Fixes

* resolve all 19 skipped functional tests ([a079bc1](https://github.com/gofhir/ucum/commit/a079bc142b5f03c6ab6ffbaddf797cab694352a1))
* resolve lint issues (errcheck, unused, empty branch) ([e65fdfb](https://github.com/gofhir/ucum/commit/e65fdfb46b23ff385942510059be013124ca98eb))
