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

The definitions themselves are internal. `Model` and its friends used to be exported — a leftover of the port from Java — but nothing could be done with them: no exported function accepted one, and the service's own model was private. The public surface is now six types: `Service`, `ExactService`, `Pair`, `RatPair` and the two error types.

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

All 21 special units are additionally checked against the specification's normative definition tables — `TestAllSpecialHandlersAgainstSpec` pins each one to an independently verifiable reference point, such as pH 7 being 1e-7 mol/L or a 100% slope being a 45 degree incline.

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

That matters if you normalise before comparing, which is the natural thing to do: 100 °F is *colder* than 50 °C, and the canonical values say so.

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

The dimensional half is not airtight, and cannot be: UCUM gives 15 canonical forms to more than one property. `"1"` is claimed by 11 of them, so for a compound dimensionless expression the check cannot tell `amount of substance` from `fraction`. Atomic codes never take that path.

## Errors

```go
var ve *ucum.ValidationError
if errors.As(err, &ve) { /* invalid code */ }

var ce *ucum.ConversionError
if errors.As(err, &ce) { /* incommensurable units, or a zero divisor */ }
```

```
Validate("m/")        invalid UCUM code "m/": parse "m/": unexpected end of expression
Convert(1, "m", "s")  cannot convert "m" to "s": units are not comparable: m vs s
Canonical(1, "m/0")   division by zero in unit expression
```

A zero divisor is well-formed UCUM, so `Validate("m/0")` accepts it and canonicalization is where it fails — with an error, not a panic.

## Performance

Measured on an M-series laptop, 400 iterations × 3 runs:

| operation | ns/op |
|---|---|
| `Validate` | 11–19 |
| `IsComparable` | 376 |
| `ConvertSimple` | 850 |
| `Canonical` | 1226 |
| `ConvertSpecial` | 1613 |
| `New` | 1581173 |

`New` parses the embedded definitions, so call it once. Everything else is cheap and the parse cache makes repeated codes cheaper.

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

One, and it comes from the definitions rather than from this implementation.

**The property of a compound expression can be ambiguous.** `ValidateInProperty` judges an atomic unit by the property UCUM declares for it, which is exact. A compound expression has no declared property, so it is judged dimensionally — and UCUM gives 15 canonical forms to more than one property. `"1"` is claimed by 11 of them, so a dimensionless compound cannot be distinguished as `amount of substance` rather than `fraction`. No amount of implementation fixes that; the reference implementation resolves it with a hardcoded special case for `concentration`.

## Differences from the Java reference

The lexer and parser are ports of [FHIR/Ucum-java](https://github.com/FHIR/Ucum-java), and the shape of the service follows it, but the behaviour has diverged. Verified by reading its source, not from memory:

| | Ucum-java | here |
|---|---|---|
| Special-unit conversion | `HoldingHandler` holds the code and units without converting, so `[pH]`, `B`, `Np`, `bit_s`, `[p'diop]` and the homeopathic potencies reduce to base units with a factor of 1 | all 21 convert, each pinned to the specification's normative tables |
| Temperature | `Converter` raises `Not handled yet (special unit with offset from 0 at intersect)`; its comment notes that fixing it *"requires a total rework of the architecture"* | works, including `[degRe]` |
| Reading the definitions | `XmlDefinitionsParser` does not read the `<function>` element, so for `Cel` it keeps `Unit="cel(1 K)"` — a string its own expression parser cannot parse | the `<function>` is parsed, giving the reference quantity of every special unit |
| Registered handlers | a hardcoded list — the consequence of the row above, since without `<function>` there are no parameters to read. Missing `[degRe]`, `B[10.nV]`, `[m/s2/Hz^(1/2)]`, `[hp'_M]` and `[hp'_Q]`, and registering `[hp_X]`/`[hp_C]` (arbitrary-unit codes) for the special units | built from the function each definition declares; an unknown function name is a construction error |
| Arbitrary units | `isArbitrary` is not in its model, so they cannot be told apart | each is its own dimension, per UCUM §24 |
| `ValidateInProperty` | requires the canonical form to be a single base unit, plus a hardcoded case for `concentration` | declared property for atoms, dimensional for compounds |
| Decimal precision | propagated through every operation, in a bespoke numeric type | exact `big.Rat` core, precision in `fhir.Decimal` — better for conversion, weaker for arithmetic |

Where Java is ahead:

- It has always enforced UCUM §11 (prefixes only on metric atoms), which this package only started doing in v3.0.0.
- It parses and keeps UCUM's case-insensitive codes (`CODE`, so `M` for metre), which this package discards. Neither library's expression parser accepts them, but Java holds the data if it ever wants to.
- Precision propagates through its arithmetic; here it is a layer in `fhir.Decimal` (see Known limitations).

`TestFunctionalSpecialUnitsJavaFails` documents the conversions that raise in Java and work here.

## License

BSD 3-Clause; see [LICENSE](LICENSE).

Third-party material travels with its own terms, recorded in [NOTICE](NOTICE):

- **`lexer.go` and `parser.go`** are ports of `Lexer.java` and `ExpressionParser.java` from [FHIR/Ucum-java](https://github.com/FHIR/Ucum-java), BSD 3-Clause, © Health Intersections Pty Ltd. Everything else — the canonicalizer, the exact arithmetic, the special-unit handlers, the FHIR subpackage — is original.
- **`ucum-essence.xml`** is published by Regenstrief Institute and the UCUM Organization under the [UCUM 1.0 License](https://ucum.org/license), which **forbids modifying it**. It is embedded verbatim and never rewritten; updating to a newer UCUM release means replacing the file, not editing it.
- **`fhir/ucum-common.tsv`** is extracted from the FHIR `ucum-common` ValueSet, which HL7 dedicates to the public domain under CC0 1.0.
