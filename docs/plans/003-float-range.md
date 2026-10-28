# 003 — Resultados fuera de `float64` (issue #59)

**Estado:** paso 8 (code review, ronda 1). **Fecha:** 2026-10-27. **Versión base:** `5f216e9` (v4.3.1 más #60).

## Problema

Con una entrada finita, `Canonical`, `Convert`, `Multiply` y `Divide`
devuelven un valor no finito, o un 0 que no es el resultado, con error nil.
#59 lo reporta para las unidades logarítmicas. La sonda del paso 1 muestra que
el defecto es más amplio: también ocurre con unidades lineales y en el dominio
de las funciones especiales.

## 1. Revisión previa

### Medición (sonda en el scratchpad, `newTestService`, `5f216e9`)

| Llamada | Resultado actual | Qué es |
| --- | --- | --- |
| `Canonical(1000, "B")` | `+Inf "1"`, nil | desborde (10^1000, §46■3) |
| `Canonical(-1000, "B")` | `0 "1"`, nil | subdesborde (10^-1000) |
| `Canonical(-320, "B")` | `1e-320`, nil | subnormal: correcto, redondeado una vez |
| `Canonical(1e300, "Ym")` | `+Inf "m"`, nil | desborde lineal (10^324 m) |
| `Canonical(1e-300, "ym")` | `0 "m"`, nil | subdesborde lineal (10^-324 m) |
| `Convert(1e300, "Ym", "fm")` | `+Inf`, nil | desborde lineal |
| `Multiply(1e300 Ym, 1e300 Ym)` | `+Inf m2`, nil | desborde lineal |
| `Multiply(1000 B, 1 1)` | `+Inf 1`, nil | desborde especial |
| `Convert(1000, "B", "dB")` | `+Inf`, nil | desborde **intermedio**: el resultado verdadero, 10 000 dB, cabe |
| `Convert(0, "1", "B")` | `-Inf`, nil | fuera del dominio: lg 0 no existe (§46) |
| `Convert(-1, "1", "B")` | `NaN`, nil | fuera del dominio: lg de un negativo |

Una entrada no finita (NaN, ±Inf) da hoy un resultado no finito, y así está
documentado en `ucum.go:29`. Este incremento no cambia eso: lo trata #55.

### Dónde se redondea (código de producción)

- **Vía racional**, unidades lineales y temperaturas: `convert.go:42`
  (`canonicalScalar`), `convert.go:137` (`Convert`), `convert.go:243`
  (`combine`). `big.Rat.Float64` da ±Inf cuando no cabe y 0 o un subnormal
  cuando es muy chico.
- **Vía `float64`**, escalas sin forma racional (log, tan, sqrt):
  `convert.go:50`, `convert.go:150-158` y `convert.go:250-258`, con
  `special.Use.ToCanonical` y `FromCanonical` (`special.go:93-101`) y
  `decimal.MulExact` (`decimal.go:127`).
- `productResult` (`product.go:195`) ya rechaza un desborde o un subnormal en
  su forma, pero vuelve a la canónica, que tampoco se verifica.
- La API exacta (`exact.go`) no redondea: devuelve `big.Rat`, o
  `ErrNotRational` para las escalas no racionales. No está afectada.
- `fhir.CanonicalKey` hereda el defecto por la vía de `Value`. Su vía de
  `Exact` (`fhir/quantity.go:304`) también desborda, pero eso ya lo cubre #55.

### `gofhir/server` (principio 9)

Usa `ucum/v4` v4.2.0 solo en `internal/terminology/ucum` (validación y
lookup). No canonicaliza valores ni verifica finitud. **Nada que tomar.**

### Especificación

- UCUM §22■4: r = f(m/u)/α y m = f⁻¹(αr)·u.
- f⁻¹ de cada función: §44 (hpX, hpC… y tan), §45■3 (pH, 10^-x), §46■3 (lg,
  ln, 2lg), §47 (ld), §48 (sqrt).
- f (log) está definida solo para argumentos positivos. Un 0 o un negativo en
  la escala canónica no tiene imagen en una escala logarítmica, y la raíz
  cuadrada no tiene imagen real para un negativo.
- UCUM no habla de representación en coma flotante. El límite es de la API
  (`float64`), no de la especificación.

### Otras implementaciones

- **Ucum-java 1.0.10** usa `Decimal` de precisión arbitraria: no desborda.
  Para las unidades especiales da un valor finito pero equivocado, porque las
  trata como lineales (clase `java-special-as-linear` de 002).
- Nuestra API exacta (`NewExact`, `CanonicalRat`) da el valor exacto para las
  unidades lineales. El error puede apuntar a ella.

### Decisiones de la revisión

- **Mejorar:** el engine. Ante una entrada finita, se devuelve un error en vez
  de un resultado no finito o de un 0 que no corresponde. Las unidades con
  forma logarítmica se calculan además por log10, sin materializar el valor
  intermedio (sección 2, decisión de la ronda adversarial).
- **Descartar:** cualquier saturación (`MaxFloat64`) o tope silencioso:
  esconde el problema igual que `+Inf`.

## 2. Plan (versión 2, después de la ronda adversarial)

### Alcance

Para toda entrada **finita**, `Canonical`, `Convert`, `Multiply` y `Divide`
(y lo que se apoya en ellas: `CanonicalKey`, `Compare`,
`ValidateCanonicalUnits`) cumplen esto:

1. **Valor correcto cuando el resultado cabe,** aunque un valor intermedio
   no quepa. `618 B[SPL]` da 2·10^307 g.m-1.s-2, `324 [pH]` da
   6.02·10^-298 m-3 y `1000 B` → `dB` da 10 000. Además, `Convert(-320, "B",
   "B")` da -320 y no -320.0000048.
2. **Desborde:** si el resultado final no cabe en `float64` (|v| >
   `MaxFloat64` después de redondear), la llamada devuelve un error que
   envuelve `ErrOutOfRange`.
3. **Subdesborde:** si el resultado final verdadero no es cero y se redondea
   a 0 (|v| ≤ 2^-1075, porque el empate redondea a cero), la llamada
   devuelve `ErrOutOfRange`. Un resultado final subnormal distinto de cero
   se acepta: es el valor correctamente redondeado.
4. **Dominio:** si el valor en la escala canónica no tiene imagen en la
   escala especial de destino, la llamada devuelve un error que envuelve
   `ErrOutOfDomain`. Pasa con log de 0 o de un negativo, y con la raíz de un
   negativo.

Una entrada no finita (NaN, ±Inf) no pasa por ninguna verificación y sigue
dando lo de hoy. Eso queda para #55.

### Diseño: la vía log10

El defecto de fondo es que la vía `float64` materializa f⁻¹(αx) antes de
multiplicar por el factor de referencia y antes de aplicar f del destino. Un
valor intermedio fuera del rango normal desborda, se va a cero o pierde
cifras. Para todos los casos de la sonda, la corrección es la misma: llevar
el **log10 del valor canónico** en vez del valor, y materializarlo una sola
vez al final.

En `internal/special`, una interfaz nueva, `Log10Handler`. La implementan las
funciones cuya inversa es una potencia (§44-48): lg, 2lg, ln, ld, pH y hp*
(f⁻¹ exponencial), y sqrt (f⁻¹(v) = v²). Tiene dos métodos:

- `toCanonicalLog10(v) float64`: el log10 de f⁻¹(v), siempre positivo. Para
  una exponencial es ±v/d·log10(b); para sqrt, 2·log10|v|.
- `fromCanonicalLog10(l) float64`: f(10^l). Para una logarítmica es lineal en
  l, y no se materializa nada; para sqrt es 10^(l/2).

`Use` agrega `ToCanonicalLog10` y `FromCanonicalLog10`, con α en el argumento
(§22■4), como `ToCanonical` y `FromCanonical`. Las bases y divisores son los
que ya están en `functions`; no se agrega ningún número.

En `internal/decimal`, `Log10Rat(r)` calcula el log10 de un racional
positivo sin pasar por `float64`: por la mantisa y el exponente de
`big.Float`, así que da el log10 de `Ym13` (10^312). Sirve para los factores
canónicos.

En el engine (vía `float64` de `canonicalScalar`, `Convert` y `combine`):

- **R1. Destino logarítmico o sqrt:** siempre por log10. Se calcula
  L = log10 |valor canónico| sin materializarlo: desde un origen
  exponencial, con `ToCanonicalLog10` más `Log10Rat(factor)`; desde un
  origen lineal, `log10|x| + Log10Rat(factor)`. Un valor canónico ≤ 0 da
  `ErrOutOfDomain`. El resultado es `FromCanonicalLog10(L - Log10Rat(factor
  del destino))`. Pierde lo mismo que hoy cuando todo cabe (el error absoluto
  en L es del orden de 1 ulp) y ya no pierde nada cuando el intermedio no cabe.
- **R2. Destino lineal (`Canonical`, `Convert` a lineal, `Multiply` y
  `Divide`):** primero la vía directa de hoy. Si el intermedio especial queda
  fuera del rango normal (±Inf, 0 o subnormal), se repite por log10 y se
  materializa una vez: signo·10^L, con `ErrOutOfRange` si L >
  log10(`MaxFloat64`) o si L ≤ -1075·log10(2). Solo se cae a log10 fuera del
  rango normal porque 10^L pierde unas |L|·2^-52·ln 10 cifras relativas
  (hasta 2·10^-13 para |L| ≈ 330), y la vía directa da 1-2 ulp cuando cabe.
  Así, lo que hoy da bien no cambia.
- **R3. Handlers sin forma log10** (tan y temperaturas): vía directa, con las
  verificaciones de la regla de alcance. tan(x)/100 o atan de un argumento
  distinto de cero que da 0 se reporta como `ErrOutOfRange` (§44: f⁻¹ es
  inyectiva y vale 0 solo en 0).
- **R4. Vía racional:** una sola función, `roundRat(exact) (float64,
  error)`. Da `ErrOutOfRange` para ±Inf, y también para 0 con `exact` ≠ 0.
  Reemplaza los tres `Float64()` de `convert.go`. Las temperaturas y las
  lineales pasan por aquí.
- **`Multiply` y `Divide` por la vía `float64`:** si un operando está en
  forma log10, se combinan los log10 (suma o resta), los signos aparte, y se
  materializa una vez. `Divide(1 "1", -1000 B)` da `ErrOutOfRange`, porque
  10^1000 no cabe, y no «el divisor vale cero». El orden es este: primero el
  rango y después el cero, porque una exponencial nunca vale cero.
- `productResult` no cambia: su vuelta a la canónica pasa por `roundRat`.

Errores nuevos en la API pública (`errors.go`), con alias en
`internal/ucumerr`, como `ErrDivisionByZero`:

- `ErrOutOfRange`, «value is out of the range of float64». Para las unidades
  lineales, el mensaje sugiere la API exacta (`NewExact`).
- `ErrOutOfDomain`, «value is outside the domain of the special unit».

Dos sentinels, porque responden dos preguntas distintas: en un caso el valor
existe pero no cabe; en el otro, el valor no existe. Se envuelven en los tipos
de siempre: `ConversionError` para `Convert` y `ValidationError` para el
resto.

### Consumidores dentro del módulo

- **`ValidateCanonicalUnits`** (`ucum.go:99`) usa `Canonical(1, code)` para
  una pregunta de dimensión. Con `Ym13`, hoy funciona y daría
  `ErrOutOfRange`. Cambia así: si `Canonical(1, code)` da `ErrOutOfRange`,
  el código canónico se compara con `IsComparable(code, canonical)` y con
  `Canonical(1, canonical)`. Esta segunda llamada verifica que `canonical`
  esté escrito en forma canónica: su código vuelve igual y su valor es 1.
  Así no hace falta un método nuevo en la interfaz `Service`.
- **`fhir.Comparator.Compare`**, vía `float64` (`quantity.go:118-122`): hoy
  `Compare(1000 B, 400 B)` da 0, porque Inf == Inf. Pasa a dar el error, y se
  lista como cambio visible.
- **`fhir.CanonicalKey`, vía `Exact`** (`quantity.go:304`): pasa por la misma
  regla (`ErrOutOfRange` si ±Inf, o si da 0 desde un `Exact` distinto de
  cero). Si no, `Value: 1e-300 ym` daría error y `Exact: 1e-300 ym` daría 0
  en la misma versión. `TestCanonicalKeyEdges` (Exact -1e-400 → +0) se
  actualiza a propósito. #55 queda solo con las entradas no finitas, y se
  comenta en #55.

### Cambios que un consumidor ve (van al cuerpo del commit)

- Con una entrada finita, `Canonical`, `Convert`, `Multiply`, `Divide`,
  `CanonicalKey` y `Compare` devuelven un error que envuelve
  `ErrOutOfRange` o `ErrOutOfDomain` donde hoy devuelven ±Inf, NaN o un 0
  espurio con error nil.
- Las conversiones entre escalas logarítmicas dan el valor correcto donde hoy
  desbordan o pierden cifras.
- `errors.go`: dos sentinels nuevos.
- Commit `fix:`, sin `BREAKING CHANGE`: un error donde había un valor
  inválido corrige el defecto, como en 001. El README lo dice.

### Fuera de alcance

- Las entradas no finitas (#55).
- Precisión de tan en argumentos subnormales: se reporta `ErrOutOfRange`
  cuando da 0, y no se intenta más.

### Criterio de salida

1. Ninguna llamada pública con entrada finita devuelve un valor no finito,
   ni un 0 cuyo valor verdadero no es cero, con error nil. Lo verifica el
   barrido B contra un oráculo independiente.
2. Cuando el resultado final cabe, el valor coincide con el del oráculo, con
   tolerancia relativa max(1e-13, |L|·2^-50). Eso incluye los casos de
   intermedio fuera de rango (`618 B[SPL]`, `324 [pH]`, `1000 B`→`dB`,
   `-320 B`→`B`).
3. `TestCompareJava`: desaparece la clase `ours-float-overflow`; sus 9 casos
   pasan a `ours-out-of-range` (deliberada). Esa clase exige el error
   `ErrOutOfRange` (el comparador conserva la causa) y que el oráculo, con el
   umbral corregido, diga que el resultado final no cabe.
4. Todas las mutaciones de la sección 7 fallan. J1–J16 siguen fallando.
5. `make check` en verde. El costo del barrido se mide (tiempo y RSS) y se
   registra.

### Tests (paso 5; ningún esperado sale de correr el código propio)

**A. Casos escritos a mano** (`internal/engine/range_test.go`). Cada caso
cita su sección. El esperado sale de la fórmula, con el factor del XML
calculado aparte, y de los límites IEEE 754: `MaxFloat64`, 2^-1074 y la
mitad de 2^-1074.

- **Factor 1**, §46■3 y §47: `308 B` → 1e308 y `309 B` → `ErrOutOfRange`;
  `-323 B` → 1e-323 y `-323.6 B` → 5e-324, porque 10^-323.6 > 2^-1075;
  `-324 B` → `ErrOutOfRange`; `709 Np` cabe y `710 Np` no; `1023 bit_s` cabe
  y `1024 bit_s` no; `3080 dB` cabe y `3090 dB` no, con α en el argumento
  (§22■4).
- **Factor ≠ 1, con límites derivados del XML:**
  - `B[V]` (V = 1000 g.m2.s-2.C-1): `610 B[V]` cabe y `613 B[V]` no.
  - `B[SPL]` (ref 2·10^-5 Pa): `618 B[SPL]` da 2·10^307 y `620 B[SPL]` no
    cabe.
  - `[pH]` (§45■3, factor 6.02214076·10^26): `324 [pH]` da 6.02·10^-298;
    `-281 [pH]` cabe y `-282 [pH]` no; `350 [pH]` cabe y `351 [pH]` no.
  En el test, los límites se calculan con `Log10Rat` aplicado al factor que
  lee `readEssence`, y no se escriben a mano.
- **Escala log a escala log** (R1): `Convert(1000, "B", "dB")` → 10 000;
  `Convert(-320, "B", "B")` → -320; `Convert(1e-17, "B", "B")` → 1e-17;
  `Convert(-323.5, "B", "dB")` → -3235; `Convert(1000, "B", "Np")` →
  1000·ln 10.
- **Dominio**, §46 y §48: `Convert(0, "1", "B")`, `Convert(-1, "1", "B")`,
  `Convert(-1, "1", "Np")` y `Convert(0, "mol/l", "[pH]")` dan
  `ErrOutOfDomain`. Un negativo hacia `[m/s2/Hz^(1/2)]` también. En cambio,
  `Convert(1e-300, "1", "B")` da -300, y `Convert(1e300, "Ym", "B")` da 324
  (el origen lineal desborda en `float64`, pero L cabe).
- **Ceros de funciones inyectivas** (R3): `Canonical(5e-324, "[p'diop]")` →
  `ErrOutOfRange`. `Canonical(1e-200, "[m/s2/Hz^(1/2)]")` → `ErrOutOfRange`,
  porque 10^-400 no cabe ni por log10. `Canonical(1e-160,
  "[m/s2/Hz^(1/2)]")` → 1e-320.
- **Lineales** (R4): `Canonical(1e300, "Ym")`, `Canonical(1e-300, "ym")`,
  `Convert(1e300, "Ym", "fm")`, `Multiply(1e300 m, 1e300 m)` y
  `Divide(1e-300 m, 1e300 m)` → `ErrOutOfRange`. `Canonical(1e-310, "m")` →
  1e-310.
- **Temperaturas**, §21-22: `Canonical(1e308, "[degF]")` cabe;
  `Convert(-1e308, "K", "Cel")` cabe; `Canonical(1.7e308, "MCel")` y
  `Convert(MaxFloat64, "K", "[degF]")` (factor 9/5) → `ErrOutOfRange`.
- **Multiply y Divide por la vía `float64`:** `Multiply(200 B, 200 B)` →
  `ErrOutOfRange` (10^400) y `Multiply(-200 B, -200 B)` → `ErrOutOfRange`
  (10^-400). En cambio, `Multiply(200 B, -200 B)` → 1 y `Divide(200 B,
  200 B)` → 1, porque los intermedios no caben pero el resultado sí.
  `Divide(1 "1", -1000 B)` → `ErrOutOfRange`, y no división por cero.
- **Consumidores:** `ValidateCanonicalUnits("Ym13", "m13")` → nil, y un
  canónico mal escrito sigue fallando. `Compare(1000 B, 400 B)` →
  `ErrOutOfRange`. `CanonicalKey` con `Value` y con `Exact` de `1e-300 ym`
  → el mismo error.
- **Errores:** `errors.Is` con el sentinel, el tipo (`ConversionError` o
  `ValidationError`) y el código en el mensaje.
- **Frontera con #55:** `Canonical(±Inf, "B")`, `Canonical(NaN, "m")`,
  `Canonical(+Inf, "[pH]")` y un operando no finito en `Multiply` siguen como
  hoy.

**B. Barrido con oráculo independiente** (`TestRangeSweep`):

- **Oráculo:** un evaluador nuevo en el test.
  - Valor canónico exacto de cada átomo, recursivo sobre las definiciones que
    lee `readEssence` (`big.Rat`). Las definiciones se leen con la gramática
    de `internal/expr`.
  - Para las especiales, el log10 del valor final por la fórmula de la
    especificación: L = ±αv/d·log10(b) + log10(factor), con
    `big.Float` de 200 bits para log10(factor).
  - No llama a nada de `*Service` salvo `parser` y `defParser`, ni a
    `internal/special` ni a `internal/decimal`. Se verifica con grep en el
    test (K12).
  - Toda forma que no soporta falla explícitamente; no se salta.
- **Códigos:** todas las especiales del XML, solas y con cada prefijo del
  XML, más todas las unidades de `ucum-common` con `Y` y `y` cuando el
  código los admite. Son varios cientos.
- **Valores:** grilla alrededor del límite superior e inferior de cada código
  (L = log10(`MaxFloat64`) y L = -1075·log10(2), ±0.01, ±1, ±10), más 0,
  ±1, ±1e-300 y ±1e300.
- **Operaciones:** `Canonical`; `Convert` a la canónica; `Convert` entre dos
  unidades especiales de la misma dimensión (R1); y `Convert(x, u, u)` en la
  banda subnormal (ida y vuelta).
- **Propiedad:** si el oráculo dice que el final cabe, el valor coincide con
  la tolerancia del criterio 2. Si dice que no cabe, el error es
  `ErrOutOfRange`; si dice fuera de dominio, `ErrOutOfDomain`. Se saltan solo
  los puntos a menos de 1e-9 en L de un límite, que deciden el redondeo y no
  la regla, y se cuentan.

**C. Metamórfico:** `Canonical(v, u)` da error si y solo si `Convert(v, u,
canónica)` da error, con el mismo sentinel. Se descarta «`Multiply(a, 1)` ⇔
`Canonical(a)`», porque `productResult` conserva la unidad del operando
(`Multiply(1e300 Ym, 1)` da 1e300 Ym y está bien).

**D. Comparación con Java:** `ourOut` (`javacompare_test.go:201`) conserva
el error, y el oráculo distingue el error esperado de los demás antes de
clasificar. Clase `ours-out-of-range` como dice el criterio 3.

**E. Fuzz:** el fuzz existente de la API agrega la propiedad «entrada finita
⇒ valor finito o error con un sentinel conocido».

### Excepciones a la regla 5

Ninguna nueva. La forma log10 de cada función va en su handler, junto a la
función (§44-48), con las bases que ya están ahí. Los límites salen de
`math.MaxFloat64` e IEEE 754.

### Dependencias nuevas

Ninguna.

### 7. Mutaciones previstas (en una copia del repo)

| # | Mutación | Debe fallar |
| --- | --- | --- |
| K1 | `roundRat` no verifica ±Inf | A (lineales), B |
| K2 | `roundRat` no verifica el 0 espurio | A (`1e-300 ym`), B |
| K3 | `roundRat` rechaza los subnormales | A (`1e-310 m`), B |
| K4 | R2 sin caída a log10 (vuelve a la vía directa) | A (`618 B[SPL]`, `324 [pH]`), B |
| K5 | R1 materializa el intermedio (vuelve a la vía directa) | A (`-320 B`→`B`, `1000 B`→`dB`), B |
| K6 | Sin verificación de dominio | A (dominio) |
| K7 | Dominio de log ≥ 0 en vez de > 0 | A (`Convert(0, "1", "B")`) |
| K8 | `ErrOutOfDomain` y `ErrOutOfRange` intercambiados | A, B |
| K9 | `combine` con log10 sin verificar el resultado | A (`Multiply(200 B, 200 B)`), B |
| K10 | Umbral de subdesborde en 2^-1074 en vez de 2^-1075 | A (`-323.6 B`), B |
| K11 | α aplicado al resultado y no al argumento en la forma log10 | A (`3080 dB`), B |
| K12 | El oráculo llama a `Canonical` del servicio | el grep del test |
| K13 | `ours-out-of-range` sin la condición del oráculo | D |
| K14 | R3 no verifica el 0 de tan o sqrt | A (`5e-324 [p'diop]`) |
| K15 | `Divide` verifica el cero antes que el rango | A (`Divide(1, -1000 B)`) |
| K16 | `Log10Rat` vía `float64` (desborda con `Ym13`) | A (`Convert(1e300, "Ym", "B")`), B |
| K17 | `CanonicalKey` por `Exact` sin la regla | A (consumidores) |

## 3. Ronda adversarial (2026-10-27: Codex y Fable, en paralelo)

Cada número que citan los revisores se verificó con una sonda propia sobre
`5f216e9` antes de aceptarlo.

| # | Hallazgo | Revisor | Resolución |
| --- | --- | --- | --- |
| 1 | `616 B[V]` no cabe: V vale 1000 en unidades base; medido +Inf. El límite es ≈ 610.5 | Codex 3, Fable 1 | **Aceptado.** Los límites con factor ≠ 1 se derivan del XML en el test |
| 2 | La verificación del intermedio rechaza resultados que caben (`618 B[SPL]`, `324 [pH]`, medidos +Inf y 0) | Codex 6, Fable 2 | **Aceptado.** Cambia el diseño: vía log10 (R1, R2). El desborde intermedio sale de «fuera de alcance» |
| 3 | El oráculo usa 2^-1074 como umbral de cero; el correcto es 2^-1075. Medido: `-323.5 B` = 5e-324 | Codex 5, Fable 3 | **Aceptado.** Umbral -1075·log10(2), caso `-323.6 B` y mutación K10 |
| 4 | Intermedio subnormal o redondeado a 1: `Convert(-320, "B", "B")` = -320.0000048 y `Convert(1e-17, "B", "B")` = 0 (medidos) | Codex 2, Fable 4 | **Aceptado.** R1: un destino log nunca materializa el intermedio |
| 5 | tan y sqrt dan 0 con argumento distinto de cero: `5e-324 [p'diop]` y `1e-200 [m/s2/Hz^(1/2)]` (medidos) | Codex 1, Fable 5 | **Aceptado.** R3, sqrt con forma log10, y K14 |
| 6 | El oráculo de 002 (`xmlAtomValue`) ejecuta `ourCanonical`; no es independiente | Codex 4 | **Aceptado.** Evaluador nuevo. Comparte solo la gramática (`internal/expr`), porque el parser no es lo que se prueba; K12 por grep |
| 7 | Entradas no finitas: sin una regla explícita, `-Inf B` (hoy 0) caería en «0 de una exponencial» | Fable 6, Codex 9 | **Aceptado.** No finita ⇒ sin verificaciones; más casos de frontera |
| 8 | `Divide(1, -1000 B)` dice «divisor cero» siendo 10^-1000 | Fable 7 | **Aceptado.** Rango antes que cero, y K15 |
| 9 | `ValidateCanonicalUnits`, `Compare` y la vía `Exact` de `CanonicalKey` cambian o quedan inconsistentes | Fable 8 | **Aceptado.** Sección «Consumidores» |
| 10 | El metamórfico `Multiply(a, 1)` ⇔ `Canonical(a)` es falso por `productResult` | Codex 7 | **Aceptado.** Se descarta, con motivo en C |
| 11 | K9 no aísla el guard (falla antes en `ToCanonical`) | Codex 8 | **Aceptado.** K9 usa `200 B × 200 B` |
| 12 | `ourOut` pierde el error; la clase no puede verificar `errors.Is` | Codex 9 | **Aceptado.** D |
| 13 | `Q` y `q` no están en el XML | Fable 9 | **Aceptado.** Solo `Y` e `y` |
| 14 | K12 no se puede verificar como estaba escrita | Fable 10 | **Aceptado.** Reescrita como grep |
| 15 | `Convert(MaxFloat64, "K", "[degF]")` desborda (factor 9/5) | Fable 11 | **Aceptado.** Caso agregado |

## Decisiones del usuario

- 2026-10-27: plan aprobado (versión 2) sin cambios.

## Registro

### Paso 5: tests primero (2026-10-27)

- `internal/engine/range_test.go`: casos escritos a mano (A), barrido (B),
  metamórfico (C), independencia del oráculo (K12) y `FuzzFiniteResults`
  (E, agregado a `make fuzz`).
- `internal/engine/range_oracle_test.go`: el oráculo. Valor de cada átomo por
  inducción sobre las definiciones de `readEssence`, sin `xmlAtomValue`
  (que ejecuta el motor, hallazgo 6). Las fórmulas son las de §21-22 y
  §44-48, y los log10 van en `big.Float` cuando αv no cabe en float64.
- `range_test.go` en la raíz (sentinels públicos, `ValidateCanonicalUnits`)
  y `fhir/range_test.go` (`CanonicalKey` por `Value` y por `Exact`,
  `Compare`). `TestCanonicalKeyEdges` se actualizó a propósito: `-1e-400 m`
  por `Exact` pasa a dar `ErrOutOfRange`.
- Antes de implementar, todos fallaban por el motivo esperado, y el barrido
  no tuvo falsos positivos: cada falla correspondía a un defecto real del
  motor.

Desviaciones del plan:

- El barrido de lineales usa **todos los átomos lineales del XML** con `Y`
  e `y`, en vez de `ucum-common`. El paquete `engine` no puede importar
  `fhir`, porque sería un ciclo, y los átomos del XML ya son más (418
  códigos).
- La tolerancia del criterio 2 queda en max(1e-13, |L|·2^-49), no 2^-50: el
  propio oráculo calcula 10^L con un error del mismo orden.
- La ida y vuelta de ±`MaxFloat64` se excluye: cae en el límite por
  construcción.

### Paso 6: implementación (2026-10-27)

- `internal/special`: `Log10Handler` (logarítmicas y sqrt), dominio por
  handler (`inDomain`) y `LogToLog`.
- `internal/decimal.Log10Rat`.
- `internal/engine/range.go`: `scalar`, `mapScalar`, `toDestination`,
  `combineScalars`, `roundRat`, `convertFloat` y `combineFloat`.
  `convert.go` las llama; la vía no finita queda como estaba.
- `ucum.ValidateCanonicalUnits`: si `Canonical(1, code)` da
  `ErrOutOfRange`, compara por dimensión.
- `fhir.CanonicalKey`: la vía `Exact` aplica la misma regla.

Desviación del plan, encontrada con el oráculo: **entre dos unidades
logarítmicas, la conversión es afín** (`special.LogToLog`):

r = v·(α_s/α_d)·(k_s/k_d) + log10(factor)/(k_d·α_d)

Ni siquiera pasa por log10 del valor canónico. R1 tal como estaba escrita
(log10 de αv) perdía `5e-324 Np → Np` (αv·log10 e redondea a 0) y
`1e-300 pNp → pNp` (αv subnormal), y fallaba con `1e300 YNp → YNp`
(αv = 10^324). Con la forma afín, el cociente α_s/α_d va en racionales.

Por eso `mapScalar` ya no necesita `preferLog`: toda exponencial → logarítmica
pasa por `LogToLog`, y el parámetro quedó sin uso, así que se quitó.

Otro efecto: `FuzzProductUnits` admitía solo `ErrNotRational`. Ahora admite
también `ErrOutOfRange`, pero solo si el resultado exacto en la unidad
canónica (de `CanonicalRat`) no cabe en float64.

Comparación con Java (D):

- `ourOut` guarda el error.
- La clase `ours-float-overflow` (bug-ours, #59) se reemplaza por
  `ours-out-of-range` (deliberada). Sus 9 casos son los mismos.
- El oráculo exige `ErrOutOfRange` cuando `specialLog10` dice que el
  resultado no cabe (`outOfRange`), con el umbral corregido a 2^-1075.

### Mutaciones (paso 7)

La tabla de la sección 7 cambia así:

- K9 muta `fromLog10`.
- K10 es «combine multiplica directo aunque el producto no sea normal», con
  el caso `310 B[SPL] × 310 B[SPL]` agregado.
- K11 invierte α_s/α_d en `LogToLog`. La mutación original (α en el
  resultado y no en el argumento) es equivalente para las formas lineales en
  log10 y no se puede matar.
- K13 combina la clase sin su condición con un motor que da `ErrOutOfRange`
  para todo `B`.
- K15 toma un divisor subdesbordado por cero.
- K16 usa el caso `1e-300 Ym13/m13 → B`.
- Se agregan K18-K21: el fallback de `ValidateCanonicalUnits`, su
  verificación de forma canónica, el chequeo de `LogToLog` y un error
  relativo de 1e-10.
- M16 de 001 queda obsoleta: el código que mutaba (-0 → +0 en la vía
  `Exact`) lo reemplaza la regla de rango, y K17 la cubre.

Resultado: [`evidence/003-2026-10-27-mutations.txt`](evidence/003-2026-10-27-mutations.txt),
generado con `python3 -I scripts/mutations.py`: la línea base y las 58
mutaciones (001, 002 y 003) dan ok. Tres mutaciones de la primera corrida
(K17-K19) no compilaban porque dejaban sin uso un import o una variable; se
reescribieron para que compilen y el test sea el que las mate.

Costo de los tests nuevos (`engine.test -test.run TestRange`, tres
corridas): 0.6 s y 24 MB de RSS, con 20 205 verificaciones en el barrido.

## 8. Code review

### Ronda 1 (2026-10-27: Opus y Fable, en paralelo)

Codex no corrió: su sandbox no podía escribir en la copia del scratchpad. Lo
reemplazó un Opus `general-purpose`. Cada hallazgo se confirmó con un caso que
fallaba antes de corregirlo, o se descartó con evidencia.

**Opus**

| # | Hallazgo | Resolución |
|---|---|---|
| O1 | Un destino logarítmico siempre pasaba por log10 y perdía cifras: `1000 W → B[kW]` daba -1.8e-16 | Aceptado. `toDestination` va directo mientras x·factor es normal; casos `1000 W→B[kW]`, `1000 mW→B[W]` y `1000 uV→B[mV]`, que dan 0 exacto |
| O2 | `1e306 kB` daba NaN (Inf − Inf) | Aceptado. `mapScalar` da `ErrOutOfRange` si el log10 es Inf; tres casos `1e306 kB` |
| O3 | Los chequeos de rango de la raíz no tenían tests | Aceptado. Casos `4 m2/s3 → √` = 2, y `1e∓300` con prefijos extremos, que quedan fuera de rango |
| O4 | Ramas muertas: `IsNaN` en `mapScalar`, Inf/NaN después de `FromCanonical`, `factor.Sign`, `div && x2.v == 0` temprano, `sign == 0` en la vía log | Aceptado. Se eliminaron |

**Fable**

| # | Hallazgo | Resolución |
|---|---|---|
| F1 | Entre unidades logarítmicas se perdían cifras: `60 dB[mV] → B[V]` daba 3.3e-16, `3 dB → B` 0.30000000000000004 | Aceptado. `LogToLog` multiplica v por α_s/α_d exacto (`MulExact`) y aplica la razón de pendientes solo si difieren. Casos exactos para esos dos y `30 dB → Np` = 3·ln 10 (tol13). Se probó además contar aparte las potencias de 10 en `Log10Rat`, pero la mutación K29 de esa versión sobrevivía: `TestLog10RatPowersOfTen` muestra que log10 por mantisa y exponente ya es exacto de 10^-1000 a 10^1000. Se revirtió, y ahora K29 muta log10(2) |
| F2 | Un producto subnormal con factor > 1 no tenía caso (`-300 B × -30 B[V]`); K10 era débil | Aceptado. Caso = 1e-312, y K10 cambia a «directo si es finito y no subdesborda» |
| F3 | La raíz de un intermedio subnormal no tenía caso | Aceptado. Caso `1e-300 um2/s3 → √` = 1e-156; K27 lo toma directo |
| F4 | El chequeo de divisor cero en la vía log no tenía caso | Aceptado. `Divide(-400 B, 0 m)` da `ErrDivisionByZero`; K28 lo quita |
| F5 | `ValidateCanonicalUnits("Ym13", "Ym13")` daba `ErrOutOfRange` y no «does not reduce» | Aceptado. `validateCanonicalByComparison` responde «does not reduce» si la forma dada no cabe; K31 |
| F6 | Los comentarios de los sentinels están en inglés | Rechazado. Todo símbolo exportado de `ucumerr` y `errors.go` (p. ej. `ErrDivisionByZero`) se documenta en inglés, porque es el godoc público; los comentarios internos van en español |

Mutaciones de la corrida completa antes de la ronda 2:

- K6 y K8 (el chequeo de dominio de la vía log) habían quedado vivas porque O1
  desvió los valores normales. Se agregaron negativos no normales: `-1e-300 fW`
  y `-1e300 YW → B[W]`, y `-1e-300 um2/s3 → √`, que dan `ErrOutOfDomain`.
- K16 sobrevivía por lo mismo. Se agregó `-500 B → ym13/m13` = 1e-188, donde el
  valor va por log10 y el factor 10^312 no cabe en float64.
- K16 y K18 se reanclaron al código nuevo.

Resultado: [`evidence/003-2026-10-27-mutations.txt`](evidence/003-2026-10-27-mutations.txt),
la línea base y todas las mutaciones dan ok (`python3 -I scripts/mutations.py`,
salida 0). `make check` en verde.

### Ronda 2 (2026-10-27: Opus y Fable, en paralelo)

Recibieron solo los hallazgos de la ronda 1, los cambios y el diff. Los dos
verificaron con llamadas que las resoluciones de la ronda 1 se sostienen.

| # | Hallazgo | Resolución |
|---|---|---|
| R2-1 (medio, los dos) | `LogToLog` dejaba que un resultado distinto de cero redondeara a 0 sin error: `1e-300 yB → B`, `5e-324 dB → B`, `5e-324 Np → B` | Aceptado. `LogToLog` devuelve la suma afín exacta (`*big.Rat`) y `convertFloat` la redondea una vez con `roundRat`, que da `ErrOutOfRange` para Inf y para un 0 que no sale de 0. Siete casos a mano (incluye `1e-320 B[W] → YB[kW]` = -3e-24, donde el redondeo es legítimo); K20 y K32 |
| R2-2 (bajo, Opus) | El término de referencia de `LogToLog` perdía hasta 8 ulps por cancelación: `5.780447444613257 B[uV] → dB[mV]` daba -2.1955255538674265 | Aceptado. La misma suma exacta lo resuelve: el esperado es 10v - 60 (lgTimes2 en las dos), redondeado una vez = -2.19552555386743; K35 redondea cada término y muere. K33 (dividir el término por α_d en float64) se declaró equivalente y se quitó; la ronda 3 mostró que no lo es (ver R3-1) |
| R2-3 (bajo, los dos) | `ValidateCanonicalUnits("Ym13", "m13x")` daba el error de análisis de `m13x`, mientras la vía normal dice «does not reduce» | Aceptado. Todo error de `Canonical(1, canonical)` o de `IsComparable` es «does not reduce». Casos `m13x` y `]`; K19 y K31 |
| R2-4 (bajo, Opus) | Guardas que ninguna mutación mata: `own.Value != 1`, `if hs.slope() != hd.slope()`, y `log` en lugar de `log10` en la vía directa sin un caso exacto | Aceptado. Las dos primeras se quitaron (una forma que vuelve con el mismo código no lleva prefijo; x·1 = x). Casos exactos `1000 W → dB[W]` = 30 y `0.001 mol/l → [pH]` = 3; K34 usa ln/ln 10 y muere. `r == 0 && !dst.IsLogarithmic()` en la vía log queda: es la regla del dominio, aunque hoy ningún logaritmo llega ahí con l = 0 |
| R2-5 (bajo, los dos) | Godoc en español en símbolos exportados de `internal/`, junto a godoc en inglés en el mismo archivo | Aceptado. En `main` todo símbolo exportado, también en `internal/`, se documenta en inglés; se tradujeron `Log10Handler`, `InDomain`, `ToCanonicalLog10`, `FromCanonicalLog10`, `IsLogarithmic`, `LogToLog` y `Log10Rat`, y se corrigió f_s⁻¹ |
| R2-6 (bajo, Fable) | `dst.FromCanonicalLog10(0)` como sonda de capacidad | Aceptado. `Use.HasLog10()` |

Resultado: [`evidence/003-2026-10-27-mutations.txt`](evidence/003-2026-10-27-mutations.txt),
la línea base y todas las mutaciones dan ok (salida 0). `make check` en verde.
Como R2-1 es de severidad media, la ronda no converge: va la ronda 3.

### Ronda 3 (2026-10-27: Opus y Fable, en paralelo)

Los dos verificaron con llamadas que las seis resoluciones de la ronda 2 se
sostienen. Opus corrió 389 000 conversiones log→log contra un oráculo exacto en
Python (`Fraction`, y `Decimal` a 400 dígitos para las bases irracionales):
todo par de base 10 queda redondeado correctamente, subnormales incluidos, con
la clase de error correcta.

| # | Hallazgo | Resolución |
|---|---|---|
| R3-1 (medio Opus, bajo Fable) | K33 no es equivalente: la sonda de la ronda 2 solo probó α_d = 1/10. Con α_d = 10^-24, l/(k_d·α_d) en float64 da 3.0000000000000005e24 en `0 B[kW] → yB[W]` | Aceptado. K33 restaurada; casos exactos `0 B[kW] → yB[W]` = 3e24, `0 B[W] → zB[kW]` = -3e21 y `0 B[10.nV] → aB[uV]` = -4e18 |
| R3-2 (bajo, Opus) | k_s/k_d redondeado en float64: entre B y [hp'_M] es -1/3, y `82.3 B → [hp'_M]` daba -27.43333333333333 | Aceptado. Las pendientes (±1, ±1/2, ±2, ±3, exactas en float64) se dividen en racionales, igual que el término de referencia. Tres casos con esperados de `Fraction`; K36. Entre bases irracionales (Np, bit_s, [hp'_Q]) queda 1 ulp, propio de las constantes en float64 y anterior a este cambio |
| R3-3 (informativo, Fable) | Costo: `LogToLog` pasa de 554 ns y 23 allocs a 1312 ns y 38 allocs (benchstat, n = 10); `Convert(3, "dB", "B")` sube un 15-20 %, y las vías lineales no cambian | Aceptado como está: solo afecta a la conversión log→log, que ya pasaba por `big.Rat` |

Resultado: [`evidence/003-2026-10-27-mutations.txt`](evidence/003-2026-10-27-mutations.txt),
la línea base y todas las mutaciones dan ok (salida 0). `make check` en verde.

R3-1 tuvo severidad media para un revisor, así que la ronda 3 no cumple el
criterio de convergencia, y es el tope. Según §10 se consulta al usuario.
El usuario pidió una ronda 4 acotada al diff de la ronda 3.

### Ronda 4 (2026-10-27: Opus y Fable, en paralelo): converge

Ninguno deja hallazgos de severidad media o mayor.

- Fable rehízo K33 y K36 a mano: cada una rompe exactamente los casos nuevos.
  Derivó los esperados del XML y de §44 con `Fraction`, sin correr el código.
  Además corrió 212 544 conversiones log→log contra un modelo `big.Rat`
  independiente: 0 diferencias de valor, signo o clase de error.
- Opus corrió 326 700 conversiones (130 unidades logarítmicas, todos los
  prefijos, valores cerca del cero de cada par) contra su oráculo exacto. Los
  102 348 pares de base 10, 100 y 1000 quedan redondeados correctamente, con la
  clase de error correcta. Entre bases irracionales, 1557 de 17 952 quedan a
  1 ulp, el límite ya registrado en R3-2. De 13 mutaciones propias de
  `LogToLog` mueren 12. La que sobrevive (l/k_d en float64) es equivalente con
  los datos de UCUM: toda unidad con pendiente no entera tiene referencia 1,
  así que l = 0, y su barrido sale idéntico byte a byte.
- Bajo (Fable): los casos de [hp'_M] citaban §47, y los de la raíz cuadrada
  §47■4. Según la revisión previa (§1), hpX, hpC y hpM están en §44, `ld` en
  §47 y `sqrt` en §48. Se corrigieron las citas.
