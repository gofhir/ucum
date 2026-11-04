# 016 — un prefijo o una unidad de valor 0 dividen por cero

**Estado:** paso 9 (cierre), sin commit. Plan aprobado por el usuario el 2026-11-03; code review convergió en la ronda 2 (§10). `make check`, `make check-packages`, `make compare-java` y las mutaciones completas en verde el 2026-11-03 ([evidencia](evidence/016-2026-11-03-mutations.txt)).

## 1. Revisión previa

**La especificación.** El valor de un prefijo es «the scalar value by which
the unit atom is multiplied if combined with the prefix» (§27 ■2), y el de una
unidad es un «real number scalar» (§18 ■5): ninguno excluye el 0. Una unidad
escalada especial es x = f⁻¹(α x') y su inversa x' = f(x)/α (§22 ■4): con
α = 0 la primera vale f⁻¹(0) para todo x', y la segunda no existe. El XML
oficial no tiene ningún valor 0: el caso solo aparece con `NewFromReader`.

**El defecto.** Medido sobre `main` (v4.4.7) con un prefijo propio `O` = 0:

| Entrada | `main` |
|---|---|
| `Convert(1, "K", "OCel")` | **pánico** «division by zero» |
| `Convert(1, "Np", "ONp")`, `Convert(1, "B", "OB")` | **pánico** |
| `Canonical(1, "Om-1")` | **pánico** |
| `Convert(1, "m", "Om")`, `Canonical(1, "/Om")`, `Divide(1 m, 2 Om)` | `ErrDivisionByZero` |
| `Convert(1, "Om", "m")`, `Canonical(1, "Om")`, `Multiply` | 0 |
| `Convert(1, "OCel", "K")` | 273.15 (f⁻¹(0·1), lo que da §22 ■4) |
| `Convert(1, "ONp", "Np")` | 0 |
| `ConversionFactor("m", "Om")`, `ConvertRat(1, "m", "Om")` | `ErrDivisionByZero` |

La ronda adversarial (§9) barrió todas las entradas públicas sobre todas las
unidades del XML con y sin `O` (exponentes -2…2): unos 22 mil pánicos y 117
resultados NaN o ±Inf **sin error**, p. ej. `Convert(1, "1", "ONp")` = NaN,
`Convert(1, "%", "OB")` = -Inf, `Convert(1, "%-1", "ONp")` = +Inf, y lo mismo
en el servicio insensible a mayúsculas. También caen `Multiply(1 Om, 2 m-2)`,
`Divide(1 Om, 2 m2)` y `Divide(1 Om0, 2 m)`: `productResult` funde en el
prefijo menor y arma `Om-1`; y `fhir.ConvertDecimal`.

Tres sitios de pánico, todos «division by zero»:

1. **Hacia una escala especial con α = 0** se divide por α sin chequeo:
   `special.Use.FromCanonicalRat` (`Quo` por `Alpha`, vía exacta) y
   `special.LogToLog` (`Quo` por `dst.Alpha`); en la vía float64,
   `FromCanonical` divide por 0.0 y da NaN o ±Inf sin error.
2. **Una potencia negativa de un valor 0**: `decimal.Decimal.Pow` invierte el
   0. `canonicalizeSymbol` la llama en cuatro sitios: unidad base (`OK-1`,
   `Om-1`), arbitraria (`O[IU]-1`), adimensional (`Obit-1`) y con valor
   (`[zero]-1`, `O[zero]-2`).

Una unidad especial con referencia 0 (`[zcel]` = cel(0 K)) no cae: como
destino ya da `ErrDivisionByZero` por su factor 0.

**Referencias.** Ucum-java no se carga con un XML propio; `gofhir/server`
usa esta librería.

## 2. Cambio

> Reescrito tras la ronda adversarial (§9).

Las dos causas pasan a `ErrDivisionByZero`, el error que la librería ya da
para un divisor que vale 0 (`convert.go`: «A zero destination factor … would
divide by zero»):

1. **Destino especial con α = 0** (`Convert` y `ConvertRat`): el engine lo
   decide junto al chequeo de `dstCan.Value.IsZero()`, antes de la vía no
   finita (#55) y de la exacta, con `ucumerr.Conversion(from, to,
   ErrDivisionByZero)`. Es una propiedad de la unidad, no del valor: va antes
   que el dominio (`Convert(-1, "1", "ONp")` es `ErrDivisionByZero`, no
   `ErrOutOfDomain`), y antes que una entrada ±Inf o NaN. Un origen con
   α = 0 no cambia: f⁻¹(0) existe en todo handler (Cel 273.15, log 1, tan 0,
   sqrt 0).
2. **Potencia negativa de 0** en la canonicalización: un solo helper antes de
   `Pow`, usado en los cuatro sitios de `canonicalizeSymbol`, da
   `ErrDivisionByZero` para un valor 0 con exponente negativo. El godoc de
   `decimal.Pow` pasa a decir que no acepta 0 con exponente negativo.
3. **`productResult`** ya cae a la vía canónica cuando su forma con prefijos
   da error: con la guarda 2, `Multiply(1 Om, 2 m-2)` y `Divide(1 Om, 2 m2)`
   dan un valor (0 m-1, 0 /m), no un error.

Queda igual: el XML oficial; todo lo que hoy da un valor o un error con un
valor 0.

## 3. Criterio de salida

- Ninguna entrada pública cae con un prefijo o una unidad de valor 0, y
  ninguna devuelve NaN o ±Inf sin error salvo que la entrada lo sea (#55):
  cada una da un valor finito o `ErrDivisionByZero` (`errors.Is`), en el
  servicio float64, el exacto, el insensible a mayúsculas, `Multiply`/`Divide`
  y `fhir`.
- Lo que hoy da un valor finito o un error sigue igual.
- `make check`, `make check-packages`, `make compare-java` y las mutaciones
  completas.

## 4. Tests (primero)

En `zeroprefix_test.go` y `fhir/zeroprefix_test.go`, con un XML propio: el
embebido más el prefijo `O` = 0, `[zero]` = 0 m, y una copia métrica
(`isMetric="yes"`) de cada función especial que nombra el XML (leídas del XML:
cel, degf, degre, lg, ln, ld, lgTimes2, pH, hpX, hpC, sqrt, tan…), para que
cada handler tenga un destino con prefijo `O`. Los esperados salen de §22 ■4 y
de la aritmética exacta, no de la librería.

1. Los pánicos dan `ErrDivisionByZero`: `K` → `OCel`, `Np` → `ONp`,
   `B` → `OB`, `[hp'_X]` → `ONp`, `[zcel]`-like hacia `OCel` por
   `ConvertRat`, `Canonical(1, "Om-1")`, `OK-1`, `O[IU]-1`, `Obit-1`,
   `[zero]-1`, `O[zero]-2`, `/[zero]2`, `m/Om2`.
2. Todo destino especial con prefijo `O` (las del XML con `isMetric="yes"` y
   las copias métricas), desde orígenes `1`, `%`, `10*`, la misma familia y
   otra logarítmica, con valores 0, ±1, 2.5, 1e300, 5e-324, ±Inf y NaN:
   `ErrDivisionByZero` en `Convert` y `ConvertRat`, antes que el dominio.
3. Lo que no cambia: `OCel` → K es 273.15; `ONp` → Np es 0; `Om` → m es 0;
   `m` → `Om` y `/Om` siguen en `ErrDivisionByZero`.
4. `Multiply(1 Om, 2 m-2)` = 0 m-1, `Divide(1 Om, 2 m2)` = 0 /m,
   `Divide(1 Om0, 2 m)`: un valor; `fhir` (`CanonicalKey`, `Compare`,
   `ConvertDecimal`) con `Om-1`: error, sin pánico; con `OCel` como origen,
   273.15 K (f⁻¹(0), corregido al escribir los tests).
5. Un barrido acotado: todas las unidades del XML con y sin `O`, exponentes
   -2…2, como origen y destino de `Convert` y `Canonical`, en el servicio
   sensible e insensible a mayúsculas: ni pánico ni NaN/±Inf sin error. Se
   mide su duración y se acota (la sonda completa de la ronda tardó 48 s:
   el test usa un subconjunto representativo por clase).

## 5. Mutaciones previstas

- Sin el chequeo de α = 0 hacia una escala especial, en `Convert` y en
  `ConvertRat` (1, 2).
- El chequeo de α después de la vía no finita (2, ±Inf).
- Sin el helper de potencia negativa de 0, en cada uno de los cuatro sitios
  (1).
- Mutaciones completas.

## 6. Fuera de alcance

- El signo del cero.
- `Validate("Om-1")` da nil: valida la sintaxis, no el valor.

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.8. Un pánico pasa a ser `ErrDivisionByZero`.

## 9. Ronda adversarial del plan (2026-11-03)

Dos Opus independientes. Los dos barrieron todas las entradas públicas sobre
`main` con un XML propio (prefijo 0, unidad 0, referencias 0, copias métricas
de cada handler), y uno prototipó el arreglo: 0 pánicos y 0 NaN/±Inf en todo
el barrido, la suite en verde.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| NaN/±Inf sin error (`Convert(1, "1", "ONp")` = NaN, `"%"` → `OB` = -Inf), fuera del plan | a, b (alta) | **Aceptado:** §1, §3 «finito o error», tests 2 y 5 |
| `Pow` se llama en cuatro sitios, no en dos; `Om-1` pasa por la rama base | a (alta), b (media) | **Aceptado:** un helper en los cuatro (§2.2) y una mutación por sitio |
| `Multiply`/`Divide` caen por `productResult` (`Om-1`) | a, b (media) | **Aceptado:** con la guarda dan un valor (§2.3, test 4) |
| §4.2 solo usa las métricas oficiales (Cel, lg, ln, lgTimes2) | a (media) | **Aceptado:** copias métricas de cada función del XML |
| La guarda de α va junto al factor 0, antes de la vía no finita | a (media) | **Aceptado:** §2.1, test con ±Inf y NaN, mutación |
| `ErrDivisionByZero` va antes que el dominio | a (confirmado) | **Aceptado:** §2.1 |
| §2 afirmaba que el godoc de `Pow` ya lo decía | a (baja) | **Aceptado:** se agrega al godoc |
| Insensible a mayúsculas y `fhir.ConvertDecimal` sin test | b (baja) | **Aceptado:** tests 4 y 5 |

## 10. Code review

### Ronda 1 (2026-11-03)

Dos Opus independientes sobre copias del repo. Los dos barrieron todas las
entradas públicas con XML propios (prefijo 0, unidades 0, definidas a partir
de ellas, referencias 0 o con prefijo `O`, copias métricas de cada función,
19 códigos compuestos; 167 mil pares, valores finitos y no finitos): 0
pánicos y 0 NaN/±Inf sin error desde una entrada finita. Sin regresiones en
el XML oficial. K203-K209 mueren cada una por la razón que dice.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El comentario de `[zcel]` decía que vale f⁻¹(0) = 273.15 K: el 0 multiplica por fuera de la función, vale 0 K | a (media) | **Confirmado** (`Canonical(v, "[zcel]")` = 0 K): comentario corregido y casos `K` → `[zcel]` = `ErrDivisionByZero` |
| `Compare(5 OCel, 273.15 K)` aceptaba un error | a (baja) | **Confirmado:** el test exige (0, nil) |
| §4.2 sin `ConvertRat` en el servicio insensible | a (baja) | **Confirmado:** agregado |
| `zeroAlphaDestination` construía un `special.Use`: +4 asignaciones por conversión hacia una escala especial (173 → 177 en `BenchmarkConvertSpecial`) | b (baja) | **Confirmado:** la guarda mira el prefijo del símbolo; vuelve a 173 y 203 (`ConvertTan`); el tiempo, en corridas intercaladas, queda dentro del ruido |

La corrida completa de mutaciones previa a la ronda dio 226 de 227 `ok`: J1
tenía el patrón viejo (`prefixVal.Pow` pasó a `baseCanonical`); corregido y
muerto solo.

### Ronda 2 (2026-11-03): converge

Dos Opus sobre el diff incremental. Ninguno deja hallazgos de severidad media
o mayor. La guarda en línea es equivalente a `specialUseForTerm(t) != nil &&
α = 0` para todo término (paréntesis, anotaciones, `OCel1`, sin prefijo, sin
handler); un barrido de destinos con prefijo `O` en esas formas, en los dos
servicios y en `fhir.ConvertDecimal` (630 casos), da `ErrDivisionByZero` sin
pánico. Asignaciones como antes del cambio (173 en `ConvertSpecial`, 166 en
`ConvertRat` Cel → [degF]).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| La guarda copia las condiciones de `specialUseForTerm`: pueden separarse sin que un test lo note (cuatro mutantes de esas condiciones sobreviven, atajados antes por otras guardas) | a, b (baja) | **Aceptado:** helper común `loneSpecial` sin asignaciones, usado por los dos; 173 y 203 asignaciones |
