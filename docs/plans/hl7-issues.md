# Observaciones a la especificación FHIR

Problemas de los artefactos de HL7 (ValueSets, ejemplos) que encuentran los
incrementos. No se publican (decisión del usuario, 2026-10-28): quedan aquí
como registro, redactados en inglés por si algún día se reportan. Los de
Ucum-java están en [002-java-issues.md](002-java-issues.md).

---

## Observation 1 — ucum-common lists codes that are not valid UCUM

**Found in:** plan [004](004-annotation-space.md), 2026-10-28.
**Packages:** `hl7.fhir.r4.examples` 4.0.1, `hl7.fhir.r4b.examples` 4.3.0,
`hl7.fhir.r5.examples` 5.0.0 (SHA-256 in `scripts/packages.sha256`).

UCUM v2.2 §6■1 allows only characters 33–126 inside curly braces; the space
(32) is not among them. Eleven concepts of
`http://hl7.org/fhir/ValueSet/ucum-common` in R4 and R4B contain one:
`{# of calculi}`, `{# of donor informative markers}`, `{# of fetuses}`,
`{# of informative markers}`, `{2 or 3 times}/d`, `{3 times}/d`,
`{4 times}/d`, `{5 times}/d`, `{clock time}`, `{P2Y12 Reaction Units}`,
`%{Negative Control}`. R4 repeats them in `Bundle-valueset-expansions.json`.

R5 no longer lists them, but its `Ingredient-example` uses `{delivered dose}`
in `strength.presentationRatio.denominator.code`.

Ucum-java accepts these codes because it checks for printable ASCII, not for
§6■1 (issue 5 of 002-java-issues.md). ucum-lhc 8.0.1 rejects them, as its
source reads (`source/unitString.js:69`, not run).

**Suggested fix:** replace the space with an underscore, as Ucum-java's own
tests already write `{clock_time}` (`UcumCommonUnitsTest.java:270`).

**Verified by:** `TestCommonUnitsValueSetVersions` and `TestOfficialExamples`
(`fhir/examples_test.go`), with `UCUM_PACKAGES` set.

---

## Observation 2 — the R5 decimal regex requires a literal `}` after an exponent

**Found in:** plan [008](008-nonfinite-key.md), 2026-10-28.
**Package:** `hl7.fhir.r5.examples` 5.0.0, `package/StructureDefinition-decimal.json`
(version 5.0.0), element `decimal.value`, extension
`http://hl7.org/fhir/StructureDefinition/regex`.

The R5 pattern is:

    -?(0|[1-9][0-9]{0,17})(\.[0-9]{1,17})?([eE][+-]?[0-9]{1,9}})?

The exponent group ends in `{1,9}}`: the second `}` is a literal, so the
pattern rejects `1e5` and `1.5E-3`, and accepts `1e5}`. R4 4.0.1 and R4B
4.3.0 have `([eE][+-]?[0-9]+)?`, which accepts both. Measured with Python's
`re.fullmatch` on each pattern as written in the package.

The R5 decimal datatype allows an exponent, so the pattern contradicts the
specification it describes. Probable fix: `([eE][+-]?[0-9]{1,9})?`.

Not affected here: this library parses decimals with its own reader
(`fhir.ParseDecimal`), not with the regex. Found while checking that FHIR's
decimal excludes NaN and INF, which all three patterns do.
