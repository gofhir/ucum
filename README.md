# ucum

A [UCUM](https://ucum.org/) (Unified Code for Units of Measure) implementation in Go: validation, conversion, canonical forms and dimensional analysis, with no dependencies outside the standard library.

The UCUM definitions (`ucum-essence.xml`) are embedded, so there is nothing to fetch at runtime.

```
go get github.com/gofhir/ucum/v2
```

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

## Conformance

The official HL7 test suite (`UcumFunctionalTests.xml`, the one used by `FHIR/Ucum-java`) runs as part of `go test`. All **573 cases pass with zero skips**: 529 validation, 30 conversion, 9 display-name generation, 3 division, 2 multiplication.

Be aware of what that suite does *not* cover, since a green run is easy to over-read: its conversion cases are compared with `1e-6` relative tolerance after significant-figure rounding, none of its 30 conversion cases involves a special unit, none of its 529 validation cases contains a zero divisor, and it has no canonicalization section at all. The tests in this repository go beyond it deliberately, asserting exactness as a property rather than comparing against tolerances.

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

## Known limitations

- **Nothing here is FHIR-specific.** This is a UCUM engine. A FHIR consumer additionally needs the FHIR value-set subsets (`ucum-common` and friends), and must know that FHIRPath treats calendar durations as distinct from the UCUM quantities `a` and `mo` — this library converts them per UCUM (`1 a = 365.25 d`), which is correct for UCUM and wrong for calendar arithmetic. Tracked in [#12](https://github.com/gofhir/ucum/issues/12).
- **Some logarithmic and trigonometric handlers are unverified** against the spec beyond the cases fixed so far. The bel family and the tangent scales have each already needed a correction; treat unusual ones with suspicion and check against `ucum-essence.xml`.
- **`UnitValue.Value` and `Prefix.Value` are exported fields of an unexported type**, so they can be read but barely used. Use `ExactService` for exact numbers rather than reaching into the model. Tracked in [#18](https://github.com/gofhir/ucum/issues/18).
- **Significant figures are not modelled.** Results are correctly rounded, which is a different guarantee from carrying the precision of the input.

## Credit

The lexer and expression parser are ports of `Lexer.java` and `ExpressionParser.java` from [FHIR/Ucum-java](https://github.com/FHIR/Ucum-java). `ucum-essence.xml` is published by the UCUM organisation.
