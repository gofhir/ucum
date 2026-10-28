# Defectos de Ucum-java 1.0.10

Encontrados por la comparación ejecutable del plan [002](002-compare-java.md),
verificados el 2026-10-28 con el jar 1.0.10 y contra el código de `master`. No
se publican en FHIR/Ucum-java (decisión del usuario): quedan aquí como
registro, redactados en inglés por si algún día se reportan.

---

## Issue 1 — Decimal.divide loses a power of ten when rounding carries

**Version:** org.fhir.ucum 1.0.10 (latest release); code unchanged on `master`.

```java
new Decimal("0.099997").divide(new Decimal("0.01"))   // "1.0"   — expected about 10 (exact: 9.9997)
new Decimal("0.08809").divide(new Decimal("0.00881")) // "1.000" — expected about 9.999
new Decimal("0.0881").divide(new Decimal("0.00881"))  // "10"    — correct
```

It reaches conversions: `convert(new Decimal("2.5"), "[bu_us]", "[pk_us]")` returns `1.00`, while Ucum-java's own definitions (1 [bu_us] = 0.03523907016688 m3, 1 [pk_us] = 0.00881 m3) give 9.9997.

The quotient is 9.99…, and when it is rounded to the precision of the result the carry turns it into 10.0, but the exponent is not adjusted, so it comes out as 1.0. This is not #58: there the precision is too low, here the result is wrong by a factor of ten.

**Probable cause:** `Decimal.divide` (Decimal.java:627-767) builds the digits and the decimal position separately, and the rounding step does not move the decimal point when it adds a digit.

**Suggested fix:** after rounding, if the digit count grew, shift the decimal position by one, or compute the quotient with `BigDecimal.divide(divisor, MathContext)`.

---

## Issue 2 — Lexer cannot read `[m/s2/Hz^(1/2)]`, an atom of ucum-essence.xml

**Version:** 1.0.10; unchanged on `master`.

```java
service.validate("[m/s2/Hz^(1/2)]")
// "Error processing unit '[m/s2/Hz^(1/2)]': The unit '[m' is unknown' at position 0"
```

`ucum-essence.xml` (version 2.2) defines `[m/s2/Hz^(1/2)]` (line 1991). Inside square brackets UCUM allows any printable character except the brackets themselves (§5, §14), so the solidus, the parentheses and the caret belong to the atom.

**Cause:** `Lexer.isValidSymbolChar` (Lexer.java:136-140) admits only letters, digits and `[ ] % * ^ ' " _` (plus `.` inside brackets), so the symbol ends at the first `/`.

**Suggested fix:** inside brackets, accept every printable ASCII character except `[` and `]`.

Note that once it lexes, the unit is special (`sqrt`), and the registry has no handler for that function, so canonicalization would then fail with "Not handled yet (special unit)".

---

## Issue 3 — The parser accepts expressions the UCUM grammar rejects

**Version:** 1.0.10; unchanged on `master`.

`validate` returns `null`, that is, valid, for all four:

| Code | Rule |
|---|---|
| `""` | UcumFunctionalTests.xml, case 1-103: "empty string is not a valid term" |
| `m//s` | §7■3 and the BNF of §10 Exhibit 1: a unary solidus only starts a term |
| `m)` | §10 Exhibit 1: unbalanced parenthesis |
| `{a}{b}` | §10 Exhibit 1: an annotation follows a symbol or stands alone, never another annotation |

**Causes in ExpressionParser.java (`master`):**

- `parseTerm` returns `Factor(1)` when the first token is NONE (line 58), so the empty string is accepted.
- It accepts a SOLIDUS at the start of any term, not only the main one (line 60), so `m//s` is read as `m/(/s)`.
- It stops at CLOSE without checking that a parenthesis was open (line 70), and `parse` only checks `lexer.finished()` (line 51), which is already true after the stray `)`.
- An ANNOTATION is taken as an implicit multiplication after any component (line 78), annotations included.

**Suggested fix:**
- reject NONE as the first token;
- accept a leading solidus only in the main term;
- track the parenthesis depth;
- allow an annotation only after a symbol or as a whole component.

---

## Issue 4 — Arbitrary units are dimensionless and commensurable with each other

**Version:** 1.0.10; unchanged on `master`.

```java
service.getCanonicalUnits("[hp_X]")             // ""  (dimensionless)
service.isComparable("[hp_X]", "[hp_C]")         // true
service.convert(new Decimal(1), "[IU]", "[arb'U]") // 1.0
```

UCUM §24■3: an arbitrary unit "is not commensurable with any other unit", and its canonical form keeps it (§25■1). `ucum-essence.xml` marks the 41 arbitrary units with `isArbitrary="yes"`, but defines them as `1` of unit `1` (for example `[hp_X]`, line 1546).

**Cause:** `XmlDefinitionsParser.parseUnit` (XmlDefinitionsParser.java:109-120) never reads `isArbitrary`. The unit is therefore expanded through its definition, 1 of unit 1, and becomes dimensionless.

**Suggested fix:** read `isArbitrary`, and make an arbitrary unit a base dimension of its own in canonicalization, so that it is kept in the canonical form and is comparable only with itself.
