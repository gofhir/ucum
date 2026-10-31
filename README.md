# ucum

A [UCUM](https://ucum.org/) (Unified Code for Units of Measure) implementation in Go: validation, conversion, canonical forms and dimensional analysis, with no dependencies outside the standard library.

The UCUM definitions (`ucum-essence.xml`) are embedded, so there is nothing to fetch at runtime.

```
go get github.com/gofhir/ucum/v4
```

## Scope

This library is a UCUM engine and the FHIR-facing pieces built on it. It is deliberately **not** a terminology server, and it does not carry a browsable catalogue of units.

That division follows from how UCUM works. Its FHIR CodeSystem is declared `content: not-present` — UCUM is a *grammar*, not a list, and it generates arbitrarily many valid codes (`mg/dL`, `mg/dL/h`, `10*3/uL`, …). There is no complete set to enumerate. So the operations split cleanly:

| concern | belongs to |
|---|---|
| `$expand`, `$validate-code`, `$lookup` | a terminology server |
| the grammar, conversion, canonical forms, exactness | this package |
| the FHIR value set, calendar durations, Quantity comparison | `ucum/fhir` |

A server implementing `$validate-code` over UCUM calls `Validate`, precisely because a `not-present` CodeSystem cannot be checked against a table. One implementing `$expand` over `ucum-common` reads `fhir.CommonCodes()`. This package supplies what those operations cannot invent; it does not perform them.

The consequence for callers is that "which units…" questions are answered by combining the value set with the engine, not by a catalogue API:

```go
// Units in the value set that a value in mg/dL can be converted to.
for _, code := range fhir.CommonCodes() {
    if ok, err := svc.IsComparable("mg/dL", code); err == nil && ok {
        // 21 of them: g/L, g/dL, mg/L, ng/mL, ...
    }
}
```

Search by display name and filtering by property work the same way, over `fhir.CommonDisplay` and `ValidateInProperty`.

The definitions themselves are internal, as is everything else. The root package is two files — the interfaces, the constructors and the error types — and the implementation sits under `internal/`, split by layer:

```
internal/ucumerr   the error types and sentinels
internal/decimal   exact rational arithmetic
internal/model     the definitions in memory, with their indexes
internal/essence   ucum-essence.xml and its reader
internal/expr      the grammar: lexer, parser, AST, composer
internal/special   the conversions for the non-ratio scales
internal/engine    canonicalization and conversion
```

The public surface is `Service`, `ExactService`, `Identified`, `Pair`, `RatPair`, `Definitions`, the two error types and the sentinels below. `Model` and its friends used to be exported — a leftover of the port from Java — but nothing could be done with them, and they are gone.

## Quickstart

```go
svc, err := ucum.New()
if err != nil {
    return err
}

svc.Validate("mg/dL")                          // nil
svc.Convert(1, "mg/dL", "g/L")                 // 0.01
svc.Canonical(1, "mg/dL")                      // {10, "g.m-3"}
svc.IsComparable("mg/dL", "g/L")               // true
svc.IsComparable("cm2", "cm")                  // false
svc.Analyze("kg.m/s2")                         // "(kilogram) * (meter) / (second ^ 2)"
svc.Multiply(ucum.Pair{Value: 2, Code: "m"}, ucum.Pair{Value: 3, Code: "m"})   // {6, "m2"}
svc.Divide(ucum.Pair{Value: 1.5, Code: "g"}, ucum.Pair{Value: 2, Code: "m"}) // {0.75, "g.m-1"}
svc.ValidateInProperty("N", "force")           // nil
```

`Multiply` and `Divide` write the result in the operands' own units, not in base units, as FHIRPath does: `12 cm * 3 cm` is `{36, "cm2"}`, not `{0.0036, "m2"}`. The same atom adds its exponents, and where it carries two prefixes both are read in the smaller one, so `2 cm * 2 m` is `{400, "cm2"}` in either order and `1 km / 1 m` is `{1000, "1"}`. Different atoms stay apart even when they share a dimension (`L * dm3` is `"L.dm3"`), annotations are dropped, and the result is always commensurable with, and equal in value to, the canonical one. An operand with a special unit (`Cel`, `[pH]`, `Cel/min`) or an arbitrary unit (`[IU]`), or a value that is not finite, keeps the canonical result.

A `Service` is safe for concurrent use and caches parsed expressions internally, so build one and share it.

To report which UCUM release you are running against — a FHIR server declaring the version of the code system it supports, for instance:

```go
defs := svc.(ucum.Identified).Definitions()
defs.Version       // "2.2"
defs.RevisionDate  // "2024-06-17"
```

## The two vocabularies

UCUM defines a case-insensitive variant of every symbol, "to be used when there is a risk of upper and lower case to be confused", and states that the two are **incompatible**. They are parallel vocabularies, chosen at construction and never mixed:

```go
cs, _ := ucum.New()                  // case-sensitive: what FHIR uses
ci, _ := ucum.NewCaseInsensitive()   // case-insensitive

cs.Canonical(1, "G")   // Gauss
ci.Canonical(1, "G")   // gram
```

That is why they cannot be mixed: `G` is *giga* or *Gauss* case-sensitively and *gram* case-insensitively, and the specification devotes a section, *Summary of Conflicts*, to such collisions. Within the case-insensitive variant case carries no meaning, so `MG/DL`, `mg/dl` and `Mg/Dl` are one code.

Use it for data from a system that cannot preserve case. **FHIR uses the case-sensitive form**, so `New` is the right choice there.

Two details worth knowing:

- **Canonical forms are always reported in case-sensitive codes**, in both variants, so that a canonical form is a stable comparison key no matter which vocabulary produced it.
- **The case-insensitive variant cannot distinguish two pairs** that the case-sensitive one can: `l`/`L` (both the liter, so nothing is lost) and `[iU]`/`[IU]` (distinct arbitrary units, so one is unreachable there). That follows from it being, as the specification puts it, the greatest common denominator.

## Conformance

The official HL7 test suite (`UcumFunctionalTests.xml`, the one used by `FHIR/Ucum-java`) runs as part of `go test`. All **573 cases pass with zero skips**: 529 validation, 30 conversion, 9 display-name generation, 3 division, 2 multiplication.

The grammar rules of §7 to §13 are checked individually too — integer factors, exponents, nested terms, the solidus, annotations — and there is one place where the specification's prose and its own conformance suite disagree. The prose of §9 reads as though an integer factor could be raised to a power ("2+10 means 2^10 = 1024"), while case 1-108 marks `10+3/ul` invalid with the reason "10 is not a valid unit", and case 1-107 shows the correct form, `10*+3/ul`. The suite governs: an integer is a number, not a unit atom. `TestIntegerFactorTakesNoExponent` pins that, because implementing the prose reading breaks 1-108 — which is how the disagreement was found.

Inside an annotation only the characters 33–126 are allowed (§6), so a space makes the code invalid: `{delivered dose}` is rejected, `{delivered_dose}` is not. Ucum-java accepts the space, and the R4 and R4B versions of the FHIR `ucum-common` value set list eleven codes with one, such as `{# of fetuses}`; R5 no longer does. Curly braces must not be nested either (§6), so `m{a{b}` is invalid; Ucum-java accepts it.

All 21 special units are additionally checked against the specification's normative definition tables — `TestAllSpecialHandlersAgainstSpec` pins each one to an independently verifiable reference point, such as pH 7 being 1e-7 mol/L or a 100% slope being a 45 degree incline.

Two fuzz targets run over the same corpus. `FuzzValidate` states that a code either parses or returns a `*ValidationError` whose offset lies inside it, and that whatever parses survives canonicalization, is comparable with itself and converts to itself as the identity. `FuzzComposeRoundTrip` states that rendering an AST and re-parsing it yields the same unit — which is how the dropped parentheses in `mL/(kg.min)` were found, in the first second of the first run.

Be aware of what the official suite does *not* cover, since a green run is easy to over-read: its conversion cases are compared with `1e-6` relative tolerance after significant-figure rounding, none of its 30 conversion cases involves a special unit, none of its 529 validation cases contains a zero divisor, and it has no canonicalization section at all. The tests in this repository go beyond it deliberately, asserting exactness as a property rather than comparing against tolerances.

## Exact arithmetic

The definitions are exact internally — a `big.Rat` under the hood — but `float64` results are rounded. Where that matters, `ExactService` exposes the exact values:

```go
ex, err := ucum.NewExact()

ex.ConversionFactor("L", "mL")                       // 1000, exactly
ex.ConvertRat(big.NewRat(100, 1), "[degF]", "Cel")   // 340/9, exactly
ex.CanonicalRat(big.NewRat(1, 1), "Cel")             // {5483/20, "K"}
```

`ConversionFactor` returns a factor you can cache and compose. The API is additive: `Service` behaves the same as before, and the value returned by `New` also satisfies `ExactService`, so a type assertion works on a `Service` you already hold:

```go
exact := svc.(ucum.ExactService)
```

`*big.Rat` rather than a decimal library keeps the dependency set empty; build whatever decimal type you need on top of it.

### Three classes of unit, not two

Which exact operation applies depends on the scale a unit lives on:

| class | example | `ConversionFactor` | `ConvertRat` |
|---|---|---|---|
| ratio scale | `L`, `mg/dL`, `mm[Hg]` | exact factor | exact |
| affine, rational | `Cel`, `[degF]`, `[degRe]` | `ErrNotLinear` | exact |
| not rational | `[pH]`, `B`, `B[V]`, `[p'diop]` | `ErrNotLinear` | `ErrNotRational` |

Between `Cel` and `[degF]` the relation is affine — `Cel = ([degF] − 32) × 5/9` — so no single multiplicative factor describes it, and `ConversionFactor` refuses rather than returning a misleading number. `ConvertRat` handles it exactly, because those constants are rational.

Logarithmic and trigonometric scales have no exact rational form at all: `10^-pH` and `atan` are irrational in general. `ConvertRat` returns `ErrNotRational` instead of a rounded value dressed up as a `*big.Rat`. Use the `float64` API for those:

```go
svc.Convert(7, "[pH]", "mol/L")   // 1e-07
```

Both errors are comparable with `errors.Is`.

## Special units

The 21 special units (temperature, pH, bel, prism diopter, homeopathic potencies) sit on non-ratio scales, and both `Convert` and `Canonical` apply their handler:

```go
svc.Canonical(100, "[degF]")   // {310.9277777777778, "K"}
svc.Canonical(50, "Cel")       // {323.15, "K"}
```

That matters if you normalize before comparing, which is the natural thing to do: 100 °F is *colder* than 50 °C, and the canonical values say so.

**A prefix scales the argument of the conversion, not its result.** UCUM §22.3 allows the prefix and §22.4 says where it applies: a scaled special unit is `s = (u, f_s, f_s-1, α)` with `x = f_s-1(α x')`. Scaling the result instead would move the origin of the scale along with the prefix.

```go
svc.Canonical(0, "mCel")      // {273.15, "K"}  — 0 mCel is 0 Cel
svc.Canonical(1, "kCel")      // {1273.15, "K"}
svc.Convert(1, "dB", "1")     // 1.2589…  — a gain of 1 dB is 10^0.1
svc.Convert(20, "dB", "1")    // 100
```

**In an algebraic term a special unit denotes a difference**, so the offset cancels and only the scale remains: `Convert(1, "Cel/min", "K/min")` is 1. That reading is refused where a difference has no meaning — `[pH]/min` and `B/s` are errors, since `10^-pH` does not decompose into a factor times a value.

Note that a *quantity* is not a unit. `Divide(Pair{1, "Cel"}, Pair{1, "min"})` divides a temperature, which is a point on its scale, by a time, and gives 4.569 K/s. Both readings are right for the question asked; `TestGradientReadingDependsOnTheQuestion` pins them side by side.

Parentheses are not an operation, so `(Cel)` is the same code as `Cel` and means the same thing.

## Arbitrary units

UCUM makes arbitrary units (`[IU]`, `[iU]`, `[arb'U]`, `[PFU]`, …) commensurable with nothing — not even with a different arbitrary unit, since the ratio between them depends on a biological assay. Each one is therefore its own dimension:

```go
svc.Convert(1, "[arb'U]", "[IU]")      // error: not comparable
svc.IsComparable("[IU]", "mol")        // false
svc.Convert(5, "[IU]/L", "[IU]/mL")    // 0.005 — only the volume rescales
```

## Properties

`ValidateInProperty` checks that a unit measures a named quantity. An atomic unit is judged by the property UCUM declares for it, strictly. A compound expression has no declared property, so it is judged dimensionally against the units that do declare one:

```go
svc.ValidateInProperty("N", "force")            // nil
svc.ValidateInProperty("g/L", "mass concentration")  // nil
svc.ValidateInProperty("m", "mass")             // error
svc.ValidateInProperty("mol", "fraction")       // error — mol declares "amount of substance"
```

`Properties()` enumerates the 101 the definitions declare, which is what makes the check usable: the names come from `ucum-essence.xml`, so nothing otherwise tells a caller that the spelling is `mass concentration` and not `concentration`, and a wrong guess is reported as an unknown property rather than as a unit that does not measure it.

```go
props := svc.(ucum.PropertyLister).Properties()  // ["(unclassified)", "acceleration", "acidity", …]
```

The same question asked dimensionally is `ValidateCanonicalUnits`, a free function rather than a method, since it needs nothing the interface does not already expose:

```go
ucum.ValidateCanonicalUnits(svc, "N", "g.m.s-2")   // nil
ucum.ValidateCanonicalUnits(svc, "kg", "kg")       // error: the canonical mass unit is the gram
```

The dimensional half of `ValidateInProperty` is not airtight, and cannot be: UCUM gives 15 canonical forms to more than one property. `"1"` is claimed by 11 of them, so for a compound dimensionless expression the check cannot tell `amount of substance` from `fraction`. Atomic codes never take that path.

## Errors

Every failure carries a typed error, and the sentinel behind it survives the wrapping. A bad code is a `*ValidationError`; a failed conversion between two codes is a `*ConversionError`.

```go
var ve *ucum.ValidationError
if errors.As(err, &ve) {
    // ve.Code, ve.Message, and ve.Offset: the byte position in the code, or -1
    // when the failure has no single position.
}

var ce *ucum.ConversionError
if errors.As(err, &ce) { /* incommensurable units, or a zero divisor */ }

if errors.Is(err, ucum.ErrDivisionByZero) { /* "m/0", or a zero divisor value */ }
```

```
Validate("m/")        invalid UCUM code "m/" at position 2: parse "m/": unexpected end of expression
Validate("nope")      invalid UCUM code "nope" at position 0: parse "nope": unknown unit "nope"
Convert(1, "m", "s")  cannot convert "m" to "s": units are not comparable: m vs s
Canonical(1, "m/0")   invalid UCUM code "m/0": division by zero in unit expression
```

A zero divisor is well-formed UCUM, so `Validate("m/0")` accepts it and canonicalization is where it fails — with an error, not a panic. Because it surfaces there, it can come out of `Canonical`, `Convert`, `IsComparable`, `Multiply`, `Divide`, `ValidateInProperty` and all three exact methods; `errors.Is` matches it on every one.

### Bounds

UCUM states no limit on the size of a code, because it describes a notation rather than an implementation. One that takes codes from the network needs limits anyway: without them a short string exhausts the process. `"m2000000000"` spent minutes building an integer of billions of digits, and two hundred nested parentheses crashed canonicalization with a Go stack overflow, which `recover` cannot catch.

| constant | value | error |
|---|---|---|
| `MaxCodeLength` | 1024 bytes | `ErrCodeTooLong` |
| `MaxNestingDepth` | 100 | `ErrCodeTooComplex` |
| `MaxExponent` | ±1000 | `ErrExponentTooLarge` |

They sit far above anything real: the longest code in the official suite is 17 bytes, the longest in `ucum-common` is 21, the deepest nesting in either is one, and no definition uses an exponent outside [-4, 4].

`MaxCacheEntries` (4096 per generation) bounds the parse cache for the same reason — an annotation is free text, so `mg/dL{lot17}` is a valid code and there are unboundedly many of them. Eviction costs a reparse, not an error, and a code that stays in use stays cached.

## Performance

Measured on an M-series laptop, 2000 iterations:

| operation | ns/op |
|---|---|
| `Validate` | 11–16 |
| `IsComparable` | 385 |
| `ConvertSimple` | 798 |
| `Canonical` | 1443 |
| `ConvertSpecial` | 3029 |
| `New` | 1702658 |

`New` parses the embedded definitions, so call it once. Everything else is cheap and the parse cache makes repeated codes cheaper: a cache hit is a `sync.Map` load, with no lock and no write, which is why `Validate` stays in the tens of nanoseconds even under a flood of distinct codes.

## Custom definitions

```go
svc, err := ucum.NewFromReader(myDefinitions)   // same schema as ucum-essence.xml
exact := svc.(ucum.ExactService)
```

## FHIR

The root package implements UCUM and nothing else. The FHIR-specific rules that UCUM cannot infer live in a subpackage:

```
go get github.com/gofhir/ucum/v4/fhir
```

**The ucum-common value set**, embedded from the published FHIR resource (version 5.0.0, 840 distinct codes):

```go
fhir.InCommonUnits("mg/dL")     // true
fhir.InCommonUnits("kOhm")      // false — valid UCUM, just not in the subset
fhir.CommonDisplay("%[slope]")  // "percent of slope", true
```

The value set is extensible in FHIR, so a code outside it is not invalid. Use `Validate` for validity and this for "is it a code FHIR expects commonly". Every one of the 840 codes is checked against the engine by a test.

**Calendar durations.** FHIRPath layers calendar duration keywords over the UCUM time units, and the relationship is equivalence rather than equality for everything above seconds — a UCUM year is exactly 365.25 days, a calendar year is 365 or 366:

```go
fhir.CalendarEquivalentOf("a")            // {Keyword: "year", Equal: false}
fhir.CalendarEquivalentOf("s")            // {Keyword: "second", Equal: true}
fhir.AllowedInDateTimeArithmetic("a")     // false — a calendar year varies in length
fhir.AllowedInDateTimeArithmetic("wk")    // true  — a week is seven days either way
```

Those two answer different questions. The equivalence table says how a UCUM quantity relates to a calendar keyword; the arithmetic rule says whether it can be added to a date. A week is only *equivalent* to a calendar week and is still valid in arithmetic, because both are exactly seven days. Only the year and the month are barred, being the two whose calendar length varies.

That follows the conformance suite rather than the prose: FHIRPath says an error is signalled for "a definite-quantity duration above seconds", but `r4/fhirpath/tests-fhir-r4.xml` evaluates `+ 1 'd'`, `+ 1 'wk'`, `+ 1 'h'` and `+ 1 'min'` without error and marks only `'a'` and `'mo'` invalid.

**Quantity comparison**, which is what a server needs for a quantity search with `gt`/`lt`, and which is exact wherever the units allow it:

```go
c, _ := fhir.NewComparator()
c.Compare(fhir.Quantity{Value: 100, Code: "[degF]"},
          fhir.Quantity{Value: 50, Code: "Cel"})   // -1: 100 °F is colder
c.CanonicalKey(fhir.Quantity{Value: 1, Code: "L"}) // "m3", 0.001 — an index key
```

**Quantity arithmetic**, which FHIRPath defines over quantities and requires to follow UCUM. The right operand is converted to the unit of the left one, exactly:

```go
c.Add(fhir.Quantity{Value: 1, Code: "m"},
      fhir.Quantity{Value: 50, Code: "cm"})        // 3/2 m, exactly
c.Add(fhir.Quantity{Value: 1, Code: "mg/dL"},
      fhir.Quantity{Value: 1, Code: "g/L"})        // 101 mg/dL, exactly
```

Both `Add` and `Sub` refuse a non-ratio scale with `ucum.ErrNotLinear`, because there is no answer to give: 20 Cel + 20 Cel is 40 Cel, while the same sum carried out in kelvin is 313.15 Cel, and neither is more correct. Where a rate or a gradient is meant, name the compound unit instead — `Convert(1, "Cel/min", "K/min")` — which is the reading in which the offset cancels.

A FHIR `decimal` arrives as text, and the `float64` nearest `0.01` is not `1/100`, so an exact comparison of a `float64` reports differences the source data does not have. Carry the decimal instead:

```go
v, _ := new(big.Rat).SetString("0.01")
c.Compare(fhir.Quantity{Value: 1, Code: "mg/dL"},
          fhir.Quantity{Exact: v, Code: "g/L"})    // 0: equal, as the data says
```

**Precision.** FHIR requires that implementations preserve the precision a decimal was written with — `0.010` is a different value from `0.01`, even though they are the same number. `fhir.Decimal` carries both:

```go
d, _ := fhir.ParseDecimal("0.010")
d.SignificantFigures()   // 2
d.String()               // "0.010", not "0.01"

out, _ := c.ConvertDecimal(d, "L", "mL")
out.String()             // "10", still 2 significant figures
out.Rat()                // exact underneath, whatever the rendering
```

A unit conversion multiplies by an exactly known factor, so it neither adds nor removes significant figures — the result carries the input's precision. `ConvertDecimal` refuses the non-rational scales (`[pH]`, bel, prism diopter), where the result is an approximation and claiming the input's precision for it would be false.

Arithmetic propagates precision by the ordinary rules of measurement, which differ between the two families of operation:

```go
a, _ := fhir.ParseDecimal("1.23")
b, _ := fhir.ParseDecimal("4.5")

a.Mul(b).String()   // "5.5"  — significant figures: the smaller count wins
a.Add(b).String()   // "5.7"  — decimal places: the coarser addend wins
```

Conflating the two is a common mistake: a sum is only as resolved as its coarsest addend, so `1.23 + 4.5` is `5.7` and not `5.73`. The value underneath stays exact regardless — `1.0 / 3.0` holds `1/3`, and only the rendering is rounded.

When the precision is coarser than the integer part, plain notation cannot express it, so the value renders in scientific notation: 150 to two significant figures is `"1.5e2"`, not `"150"`, which would claim three. Re-parsing recovers both the value and the precision.

Every rule in the subpackage cites the document it comes from, and where FHIR and UCUM disagree the disagreement is documented rather than silently resolved — FHIR R5's prose says UCUM defines a month as 30 days, while UCUM defines it as 30.4375, and this library follows UCUM.

## Known limitations

**The property of a compound expression can be ambiguous.** `ValidateInProperty` judges an atomic unit by the property UCUM declares for it, which is exact. A compound expression has no declared property, so it is judged dimensionally — and UCUM gives 15 canonical forms to more than one property. `"1"` is claimed by 11 of them, so a dimensionless compound cannot be distinguished as `amount of substance` rather than `fraction`. No amount of implementation fixes that; the reference implementation resolves it with a hardcoded special case for `concentration`.

**A result outside the range of float64 is an error.** `Canonical(1000, "B")` is 10^1000 and `Canonical(1e-300, "ym")` is 10^-324 m: neither has a float64 representation, so `Canonical`, `Convert`, `Multiply`, `Divide`, `fhir.Comparator.CanonicalKey` and `Compare` return an error wrapping `ErrOutOfRange` rather than `+Inf` or a 0 that looks like a value. A value with no image on the destination scale — 0 or a negative number into `B` or `[pH]`, a negative number into a square-root unit — wraps `ErrOutOfDomain`. A subnormal result is returned as a value, not an error (on a linear scale, the correctly rounded one), and an input that is already NaN or ±Inf is passed through. The `fhir` package does not pass it: FHIR's `decimal` has no such values, so `Comparator.CanonicalKey`, `Compare`, `Add` and `Sub` return `fhir.ErrNotFinite` for a `Value` of NaN or ±Inf with no `Exact`. On a scale compared in float64 (`[pH]`, `B`), `CanonicalKey` and `Compare` read an `Exact` that does not fit in a float64 as the nearest finite, non-zero one, so the range rule applies to the result: `1e400 B` is `ErrOutOfRange` rather than `+Inf`, while `1e400 [p'diop]` is π/2. There `Compare` rounds each operand as `CanonicalKey` does, so it returns 0 exactly when the two keys are equal; a linear operand with an `Exact` is rounded once from its exact value. The value of a logarithmic scale is not always correctly rounded: `1 [pH]` is computed from the float64 nearest 10^-1, which is not 1/10, so `Compare(1 [pH], Exact 0.1 mol/L)` is +1 by 1 ulp, while against a `Value` of `0.1` it is 0. Only the final result has to fit: `Canonical(618, "B[SPL]")` passes through 10^309 and is 2×10^307, and `Convert(1000, "B", "dB")` is 10000. A conversion between two logarithmic units is rounded once, so between scales of base 10, 100 or 1000 it is correctly rounded (`60 dB[mV]` in `B[V]` is exactly 0); where a base is irrational, as for `Np` or `bit_s`, it can be 1 ulp off, since the base's logarithm is itself a rounded float64. On a scale of integer base (`B`, `[pH]`, `[hp'_C]`, `bit_s`; not `Np`), a value whose exponent is an integer is mapped through the exact power, rounded once, before the unit's factor is applied: `Canonical(-300, "B")` is `1e-300` and `Canonical(-2900, "dB")` is `1e-290`. The exponent is the float64 one that `math.Pow` would receive, so two values with the same float64 exponent give the same result and `Canonical` stays monotonic (`0.3 kB` is `1e300`, like `300 B`); where a prefix makes that product inexact (`n`, `f`, `a`, `z`), the exact exponent decides. Where the value would otherwise be rebuilt from its logarithm — a power outside the normal range, as in `618 B[SPL]`, which passes through 10^309, or a product of two values whose result is not normal — the exact power times the factor is rounded once: `Convert(618, "B[SPL]", "Pa")` is 2×10^304 and `-308 B` is `1e-308`. There, a non-integer exponent e = n + f, with n the nearest integer, is split into the exact power base^n times `math.Pow(base, f)`. The result is then a few ulps from base^e (up to 3 for bases 2 and 10, up to 6 for 100, 1000 and 50000), and still monotonic; e is the float64 exponent, so a prefix whose factor is not a float64 (`zB`) adds the rounding of e, as on the direct path. Elsewhere a non-integer exponent goes through `math.Pow`, which is not correctly rounded (#75); in a product, its float64 enters as its exact binary value, as any linear operand does. On a square-root scale such as `[m/s2/Hz^(1/2)]`, a square outside the normal range is exact, rounded once. For linear units the exact API (`NewExact`) gives the value as a `*big.Rat`; it refuses the logarithmic scales with `ErrNotRational`, since their results are irrational.

**A numeric factor multiplying a special unit is read as a difference.** UCUM §22.3 says a special unit may be scaled "trough a prefix *or an arbitrary numeric factor*", so `2.Cel` should denote a scale with α = 2. A prefix is handled that way; a numeric factor is not, and `1.Cel` takes the algebraic reading instead, where the offset cancels. `Cel` and `(Cel)` are unaffected, and no code in the official suite or the FHIR value set has this shape.

## Differences from the Java reference

The lexer and parser are ports of [FHIR/Ucum-java](https://github.com/FHIR/Ucum-java), and the shape of the service follows it, but the behaviour has diverged. The table is backed by an executable comparison against Ucum-java 1.0.10: 11,235 cases drawn from `ucum-essence.xml`, the official suite, the `ucum-common` value set, Java's own `UcumCommonUnitsTest` and the UCUM quantities of the R4, R4B and R5 examples. Java's answers are committed, so `go test` replays them without Java; every difference falls in exactly one class, each class names the evidence for both sides. One class is a deliberate difference on this side: a special-unit value outside the range of `float64`, as in `canonical 1000 B` (10^1000) and `canonical 1000 [pH]` (about 6×10^-974 m-3), is `ErrOutOfRange` here, where Java returns a finite value that is wrong because it treats the unit as linear (class `ours-out-of-range`, 9 cases; see Known limitations). The [latest report](docs/plans/evidence/002-2026-10-27-compare-java.md) has the counts and examples; `make compare-java` regenerates it, and `make java-dump` re-runs Java (plan: [docs/plans/002-compare-java.md](docs/plans/002-compare-java.md)).

| | Ucum-java | here | class (cases) |
|---|---|---|---|
| Special-unit conversion | `HoldingHandler` holds the code and units without converting, so `[pH]`, `B`, `Np`, `bit_s`, `[p'diop]` and the homeopathic potencies reduce to base units with a factor of 1: 1 `[pH]` is 1 mol/l, where the specification gives 0.1 mol/l | all 21 convert, each checked against the function the specification defines | `java-special-as-linear` (172) |
| Temperature | `Converter` raises `Not handled yet (special unit with offset from 0 at intersect)`; its comment notes that fixing it *"requires a total rework of the architecture"* | works, including `[degRe]` | `java-offset-unsupported` (59) |
| Reading the definitions, registered handlers | `XmlDefinitionsParser` does not read the `<function>` element, so the handlers are a hardcoded list. Missing `[degRe]`, `B[10.nV]`, `[hp'_M]` and `[hp'_Q]`, which raise `Not handled yet (special unit)`, and registering `[hp_X]`/`[hp_C]` (arbitrary-unit codes) for the special units | built from the function each definition declares; an unknown function name is a construction error | `java-special-unregistered` (99) |
| Arbitrary units | `isArbitrary` is not in its model: they drop out of canonical forms, so `[hp_X]` converts to `[hp_C]` and `/[IU]` canonicalizes to the empty unit | each is its own dimension, per UCUM §24 | `java-arbitrary-dimensionless` (726) |
| `ValidateInProperty` | requires the canonical form to be a single base unit, plus a hardcoded case for `concentration`, so `J` is not an energy and `l` not a volume | declared property for atoms, dimensional for compounds | `java-inproperty-base-only` (212) |
| Decimal precision | propagated through every operation, in a bespoke numeric type: 1 `[qt_us]` is 0.000946 m3 (exactly 0.000946352946), and 2.5 m / 4 s is 0.63 m/s | exact `big.Rat` core, precision in `fhir.Decimal` — better for conversion, weaker for arithmetic | `java-decimal-precision` (435) |
| Case-insensitive codes | kept as data (`CODE`, so `M` for metre), but the parser does not accept them | resolved through `NewCaseInsensitive` — see [The two vocabularies](#the-two-vocabularies) | `java-no-case-insensitive` (251) |
| Square-bracket atoms with a solidus | the lexer ends `[m/s2/Hz^(1/2)]` at the first `/` and reports the unknown unit `[m` | read as the atom `ucum-essence.xml` defines | `java-lexer-bracket` (15) |
| Syntax | accepts `""`, `m//s`, `m)` and `{a}{b}`, which the grammar of UCUM (§7, §10) and the official suite (case 1-103) reject | rejected | `java-accepts-invalid-syntax` (4) |
| Division | `convert(2.5, [bu_us], [pk_us])` returns 1.00; Java's own values for the two units give 9.9997 | 10, exactly | `java-decimal-shift` (1) |

Where Java is ahead:

- It has always enforced UCUM §11 (prefixes only on metric atoms), which this package only started doing in v3.0.0.
- Precision propagates through its arithmetic; here it is a layer in `fhir.Decimal` (see Known limitations).

`TestFunctionalSpecialUnitsJavaFails` documents the conversions that raise in Java and work here.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The short version: every claim about UCUM cites the section of the specification that makes it, no figure the definitions already carry is restated in Go, and `make all` runs what CI runs. Vulnerability reports go through [SECURITY.md](SECURITY.md), which explains why a panic on a parsed code is treated as one.

## License

BSD 3-Clause; see [LICENSE](LICENSE).

Third-party material travels with its own terms, recorded in [NOTICE](NOTICE):

- **`lexer.go` and `parser.go`** are ports of `Lexer.java` and `ExpressionParser.java` from [FHIR/Ucum-java](https://github.com/FHIR/Ucum-java), BSD 3-Clause, © Health Intersections Pty Ltd. Everything else — the canonicalizer, the exact arithmetic, the special-unit handlers, the FHIR subpackage — is original.
- **`ucum-essence.xml`** is published by Regenstrief Institute and the UCUM Organization under the [UCUM 1.0 License](https://ucum.org/license), which **forbids modifying it**. It is embedded verbatim and never rewritten; updating to a newer UCUM release means replacing the file, not editing it.
- **`fhir/ucum-common.tsv`** is extracted from the FHIR `ucum-common` ValueSet, which HL7 dedicates to the public domain under CC0 1.0.
