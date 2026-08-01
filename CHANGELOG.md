# Changelog

## [3.0.0](https://github.com/gofhir/ucum/compare/v2.2.0...v3.0.0) (2026-08-01)


### Migration

The import path changes, as it does for every Go major:

```go
import "github.com/gofhir/ucum/v3"
import "github.com/gofhir/ucum/v3/fhir"
```

Then, in order of how likely they are to affect you:

1. **Codes that were accepted and should not have been now fail.** A prefix on a non-metric unit (`k[ft_i]`, `m[lb_av]`, `c[pi]`) is invalid per UCUM §11 ■1 and is rejected by `Validate`, `Convert` and `Canonical`. A special unit with an exponent (`Cel2`) or on a non-linear scale inside an algebraic term (`[pH]/L`, `B.m`) now returns an error rather than a number.

2. **Temperature gradients change value, because they were wrong.** `Convert(1, "[degF]/min", "K/min")` returned `1` and now returns `5/9`; `[degRe]` gradients change likewise. A rate of change in Fahrenheit was being converted as though a Fahrenheit degree were a kelvin. If you have stored results computed with the old behaviour, they are wrong by a factor of 1.8 (or 1.25 for Réaumur).

3. **The definitions model is no longer public.** `Model`, `Unit`, `BaseUnit`, `DefinedUnit`, `Prefix`, `UnitValue` and `FunctionDef` are internal, along with `UnitValue.Rat` and `Prefix.Rat`. Nothing could be built with them — no exported function accepted a `Model` — so the likely impact is nil. For exact numbers use `ExactService`, which resolves the whole definition chain.

Everything else is unchanged: `Service`, `ExactService`, `Pair`, `RatPair`, the error types and the constructors keep their shape.

### ⚠ BREAKING CHANGES

* Model, Unit, BaseUnit, DefinedUnit, Prefix, UnitValue and FunctionDef are no longer exported, along with UnitValue.Rat and Prefix.Rat.
* **special:** gradients in [degF] and [degRe] change value — they were wrong. Cel2, [degF]2, [pH]/L, B.m and similar now return an error.
* **parser:** codes combining a prefix with a non-metric atom no longer validate, and no longer convert or canonicalize. They were never valid UCUM.

### Bug Fixes

* move the module path to /v3 ahead of the major release ([#32](https://github.com/gofhir/ucum/issues/32)) ([df90ce9](https://github.com/gofhir/ucum/commit/df90ce9a23e114faf8d83992aeee50bd235ca6fb))
* **parser:** enforce UCUM §11, only metric atoms take a prefix ([#28](https://github.com/gofhir/ucum/issues/28)) ([80128f9](https://github.com/gofhir/ucum/commit/80128f906f1a6a052a7e07a94a78d9b3c7d58ef7))
* **special:** read every special-unit parameter from the definitions ([#30](https://github.com/gofhir/ucum/issues/30)) ([966c748](https://github.com/gofhir/ucum/commit/966c748de7424af2187e37d872345462df2537c7))


### Code Refactoring

* unexport the definitions model ([#31](https://github.com/gofhir/ucum/issues/31)) ([abd6d33](https://github.com/gofhir/ucum/commit/abd6d339ef6708f5e4ba2ea3d5bf851eecee165d))

## [2.2.0](https://github.com/gofhir/ucum/compare/v2.1.0...v2.2.0) (2026-08-01)


### Features

* add exact accessors for the model's numeric values ([#23](https://github.com/gofhir/ucum/issues/23)) ([c51681e](https://github.com/gofhir/ucum/commit/c51681e803b65a12ce9eea5476261b0b765f25bf))

  `UnitValue.Value` and `Prefix.Value` are exported fields typed with the unexported type `decimal`, so they could be read but not used — their only exported method formats to ten decimal places, rendering the milli prefix as `"0.0010000000"`. `UnitValue.Rat()` and `Prefix.Rat()` return the same numbers as exact `*big.Rat`, with a defensive copy and a nil-safe receiver.

  The fields are now marked `Deprecated` and will be unexported in the next major version, along with the rest of `Model`: no exported function accepts a `Model`, so it is inert and exists only to be inspected.


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
