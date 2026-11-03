# 015 — un factor canónico negativo en la vía log10

**Estado:** cerrado e integrado el 2026-11-03 (#95).

## 1. Revisión previa

**La especificación.** UCUM no restringe el signo del valor de una definición
(verificado en ucum.org/ucum el 2026-11-03):

- §18 ■5: «The product u' = r u of a real number scalar with the unit u is
  also a unit».
- §19 ■3: «where r is called the relative magnitude of u regarding v», sin
  signo ni rango.
- §27 ■2: el valor de un prefijo es «the scalar value by which the unit atom is
  multiplied», sin signo.
- §22 ■4: x = f⁻¹(α x'); una referencia negativa da un valor canónico negativo.

El XML oficial no tiene ningún valor negativo: el caso solo aparece con un XML
propio (`NewFromReader`).

**El defecto.** `log10Times` (`internal/engine/range.go`) devuelve el log10 de
|x·factor| con el signo de x solo: `decimal.Log10Rat` toma el valor absoluto
del factor, y el comentario dice «Todo factor canónico es positivo». Medido
sobre `main` (v4.4.6), con `[tiny]` = 1e-310 rad, `[nlnB]` = ln(-1 `[tiny]`),
`[neg]` = -1 m y `[nlg]` = lg(-1 m):

| Caso | `main` | Esperado |
|---|---|---|
| `700 [nlnB]` → rad (vía directa) | -1.014e-6 | -1.014e-6 |
| `714.25 [nlnB]` → rad (vía log10) | **+1.566** | -1.566 |
| `-1 rad` → `[nlnB]` | **`ErrOutOfDomain`** | ln(1e310) ≈ 713.8 |
| `-5 m` → `[nlg]` | 0.699 | lg 5 = 0.699 |
| `1 [nlg]` → m | -10 | -10 |
| `1e300 [neg]` → km | -1e297 | -1e297 |

Una misma conversión cambia de signo según el valor caiga en la vía directa o
en la log10: una inversión de monotonía. La ronda adversarial (§9) midió más
casos con el mismo origen:

- `Canonical(714.25 [nlnB])` y `CanonicalKey` dan +1.566 rad;
  `Compare(714.25 [nlnB], 0 rad)` da 1 (debe ser -1), y con 700 da -1: el
  orden se invierte;
- `Multiply(714.25 [nlnB], 1)` y `Divide(1, 714.25 [nlnB])` llevan el signo
  equivocado;
- hacia un destino, un intermedio subnormal entra a `log10Times` por la rama
  de `x.v`: `-1e-310 m` → `[nlg]`, → `[nld]`, `-1e-310 m2` → `[nsq]` y
  `-1e-320 mol/l` → `[npH]` dan `ErrOutOfDomain` y el valor existe;
- `LogToLog` da un número entre escalas de referencia de signo opuesto:
  `[nBW]` (lg(-1 W)) → `B[W]` da 1, → `dB[W]` da 10; `[npH]` → `[pH]` da 1.

Un origen `lg` o `ld` sale bien: pasa por la vía racional exacta
(`-310.5 [nlg]` → m es -3.16e-311). El defecto de origen solo se ve con `ln`.

**Referencias.** Ucum-java no es comparable aquí (no se carga con un XML
propio en `make java-dump`). `gofhir/server` usa esta librería.

## 2. Cambio

> Reescrito tras la ronda adversarial (§9).

1. **`log10Times`**: el signo es el de x por el del factor (`factor.Sign()`),
   en las dos ramas (`x.log` y `x.v`); el log10 sigue siendo el de
   |x·factor|. Se quita el comentario «Todo factor canónico es positivo».
2. **Todo lo que usa `log10Times`** hereda el signo: el dominio de un destino
   logarítmico o de raíz (`dst.InDomain(sign)`), el ε con signo de
   `toLinearScale`, la materialización (`fromLog10`), `combineScalars` y, por
   ellos, `Canonical`, `Multiply`, `Divide` y la vía float64 de `fhir`.
3. **`LogToLog`** (de una escala logarítmica a otra, sin pasar por la
   referencia): con un factor negativo el valor canónico del origen y el del
   destino tienen signos opuestos respecto de sus referencias, y el logaritmo
   del destino no existe. El engine lo decide antes de llamarla
   (`ErrOutOfDomain`); `LogToLog` no cambia.
4. **Cada comentario o godoc** que diga que un factor es positivo se corrige.

Queda igual: todo con factores positivos (el XML oficial: `factor.Sign()` es
1 y el chequeo de `LogToLog` solo rechaza signos opuestos); las vías exactas
(`big.Rat`, que ya llevan el signo: `ConvertRat`, `ConversionFactor`, la
temperatura); un prefijo negativo, que ya funciona (`1 QB` → B es -1000).

Cambia, con el XML oficial y la unidad `"0"` (factor cero; ronda 1 del code
review): `Multiply(-800 Np, 1 "0")` pasa de `ErrOutOfRange` a 0 de unidad 1, y
`Convert(1e-310, "0", "Np")` y `Convert(1e300, "0", "Np")` pasan de
`ErrOutOfRange` a `ErrOutOfDomain` (medido contra `main`). Es lo coherente: 0
por cualquier cosa es 0, y el lg y el ln de 0 quedan fuera del dominio, como ya
daba `Convert(-800, "0", "Np")`; `Multiply(1e-200 m, 1e-200 "0")` ya daba 0.

Regla general (ronda 2): un factor canónico cero hace que el intermedio sea
exactamente 0. Los productos y las conversiones dan 0 en vez de
`ErrOutOfRange`, y un destino logarítmico da `ErrOutOfDomain` para todo valor.
Ejemplos: `Divide(1 "0", v Np)` y `Convert(5e-324, "0", "Np")`. Con un origen
logarítmico y un factor cero el intermedio es el racional 0 (vía exacta, sin
ε), de modo que `-800 [zlnK]` → `JCel` redondea una vez, al par. El resultado
no depende de la vía (ronda 3): `linearTimes` y `toDestination` no leen como
subdesborde el cero de un factor cero (`factor.Sign() != 0`, `sign != 0`), así
que `Convert(v, "[zln]", "m")` da 0 para todo v, a ambos lados de 709, y
también hacia tan y raíz, en `Canonical`, `CanonicalKey` y `Compare`.

## 3. Criterio de salida

- Con un factor negativo, cada conversión da el mismo signo y el mismo valor
  (dentro del error ya admitido de cada vía) en la vía directa y en la log10:
  `714.25 [nlnB]` → rad es -1.566; `-1 rad` → `[nlnB]` es ≈ 713.8.
- ln(m/u), lg(m/u) y √(m/u) existen solo si m y u tienen el mismo signo: un
  valor canónico del mismo signo que la referencia da un valor, uno de signo
  opuesto da `ErrOutOfDomain`, en todas las vías (directa, log10, `LogToLog`).
- `Canonical`, `CanonicalKey` y `Compare` llevan el signo: el orden de
  `Compare` no se invierte entre la vía directa y la log10.
- Con factores positivos ningún resultado cambia: la suite actual, los
  oráculos y `make compare-java` sin cambios.
- `make check`, `make check-packages`, `make compare-java` y las mutaciones
  completas.

## 4. Tests (primero)

En `negfactor_test.go` y `fhir/negfactor_test.go`. El XML propio se arma sobre
el embebido: `[tiny]` = 1e-310 rad; lineal `[neg]` = -1 m; `ln` `[nlnB]` =
ln(-1 `[tiny]`) y `[plnB]` = ln(1 `[tiny]`); `lg` `[nlg]` = lg(-1 m), `[nlg2]`
= lg(-2 m), `[plg]` = lg(1 m), `[nBW]` = lg(-1 W), `[nBV]` = 2lg(-1 V); `ld`
`[nld]` = ld(-1 m); `sqrt` `[nsq]` = sqrt(-1 m2); `[npH]` = pH(-1 mol/l); y un
prefijo propio negativo (`Q` = -1000; el lector lo admite). Los esperados
salen de `big.Rat` y `big.Float` (exp y log por serie), nunca de la librería.

1. Desde la escala: para `[nlnB]`, una rejilla que cruza el borde entre la vía
   directa y la log10 (700 … 714.3), con signo y valor contra el oráculo y sin
   cambio de signo entre vecinos; para `lg`/`ld`, la misma rejilla como guarda
   (pasan por la vía exacta).
2. Hacia la escala: para `[nlnB]`, `[nlg]`, `[nld]`, `[nsq]` y `[npH]`, un
   valor canónico del mismo signo que la referencia da el valor del oráculo y
   uno de signo opuesto da `ErrOutOfDomain`, en la vía directa y en la log10:
   `-1 rad` → `[nlnB]`, y una rejilla de `-1e-300` a `-1e-320` (subnormales,
   rama `x.v`).
3. `LogToLog`: signos opuestos dan `ErrOutOfDomain`: `[nBW]` → `B[W]`,
   `[nBW]` → `dB[W]`, `B[W]` → `[nBW]`, `[nBV]` → `B[V]`, `[npH]` → `[pH]`,
   `[nlg]` → `[plg]`, `[nlnB]` → `[plnB]`. Los signos se cancelan y hay
   valor: `[nBW]` → `[nBW]`, `[nlg]` → `[nlg2]`, `[nlg]` → `[nld]`.
4. `Canonical`, `Multiply(714.25 [nlnB], 1)` y `Divide(1, 714.25 [nlnB])`.
5. `fhir`: `CanonicalKey` y `Compare(714.25 [nlnB], 0 rad)`,
   `Compare(714.25 [nlnB], 700 [nlnB])`, con `NewComparatorWith`.
6. Hacia la temperatura desde `[nlnB]` (la vía exacta de 012 con signo).
7. No cambia: los mismos casos con la referencia positiva dan lo de `main`;
   el prefijo negativo (`1 QB` → B, `-1 QCel` → K); la tangente ignora el
   valor de su referencia (ver §6).
8. El error admitido de la vía log10 queda escrito por caso (p. ej.
   `-1e-310 rad` → `[nlnB]` da -3.1e-15 donde el valor es 0: precisión de la
   vía log10, no el signo).

## 5. Mutaciones previstas

- `log10Times` sin el signo del factor, en cada rama (1, 2, 4, 5).
- Sin el chequeo de signo antes de `LogToLog` (3).
- El ε con signo de `toLinearScale`: posible mutante equivalente (ningún
  prefijo decimal pone f(0)/α en un punto medio); se anota si sobrevive.
- Mutaciones completas.

## 6. Fuera de alcance

- Un prefijo propio de valor 0 (013 §6), y el signo del cero.
- La tangente toma su argumento en radianes (`nativeUnit`, ADR 0001) y no usa
  el valor de su referencia: `1 [ntan]` (100tan(-1 rad)) → rad da +0.00999967
  y `[2tan]` (100tan(2 rad)) da lo mismo que `[p'diop]`. Conservar el valor
  rompería `%[slope]`, cuya función nombra `1 deg`. Queda con un test (§4.7).
- `Multiply(1 mm, 1 Qm)` elige el prefijo «menor» con signo (`addSymbol`):
  da `-0.001 Qm2` y no `mm2`. El valor es correcto. Pendiente propio.

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.7. Una conversión que daba el signo equivocado, o un error de
dominio donde el valor existe, pasa a dar el valor. Con el factor cero (la unidad `"0"`) un `ErrOutOfRange`
pasa a 0 (`Multiply(-800 Np, 1 "0")`) o a `ErrOutOfDomain` (`Convert(1e300, "0",
"Np")`), ambos coherentes con lo que ya daba el factor cero en otros caminos. Con un
XML propio, algunos valores equivocados pasan a `ErrOutOfDomain` (`2 [nld]` → B
daba 0.602), y el pánico con una referencia de valor 0 (`Convert(1, "[zln]",
"[mln]")`) pasa a `ErrOutOfDomain`. Con una referencia de valor 0, `Convert(v,
"[zln]", "m")` pasa de `ErrOutOfRange` a 0 (lg, 2lg, pH, hpX, tan y raíz
igual, y `Canonical`, `CanonicalKey` y `Compare`).

## 9. Ronda adversarial del plan (2026-11-03)

Dos Opus independientes (Codex sin cuota hasta el 2026-11-04). Los dos
prototiparon el arreglo: corrige la tabla del §1 y la suite completa pasa sin
cambios con factores positivos.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El test 3 usaba pares no comparables (`[nlg]` → `B`: «m vs 1») | a, b (alta) | **Aceptado:** pares comparables (§4.3) |
| El test 2 tenía el dominio al revés | b (alta) | **Confirmado:** ln(m/u) existe si m y u tienen el mismo signo; §3 y §4.2 corregidos |
| La tangente ignora el valor de su referencia | a, b (media) | **Fuera de alcance** (§6): es `nativeUnit` (ADR 0001); un test lo documenta |
| `Canonical`, `CanonicalKey`, `Compare` (orden invertido), `Multiply`, `Divide` sin test | a, b (media) | **Aceptado:** §4.4 y §4.5 |
| Destinos con intermedio subnormal (rama `x.v`): `[nlg]`, `[nld]`, `[nsq]`, `[npH]` | a, b (media) | **Aceptado:** §4.2 |
| Un origen `lg`/`ld` va por la vía exacta: la rejilla no mata el mutante | b (media) | **Aceptado:** el mutante se mata con `ln` y los destinos (§5) |
| `addSymbol` elige el prefijo con signo | a (baja) | **Fuera de alcance** (§6) |
| El ε con signo, posible equivalente | a (baja) | **Aceptado:** §5 |
| El prefijo negativo ya funciona | a, b (baja) | **Aceptado:** obligatorio en los tests (§4.7) |
| El error de la vía log10 cerca de 0 | b (baja) | **Aceptado:** §4.8 |

## 10. Code review

### Ronda 1 (2026-11-03, dos Opus independientes)

Ninguno encontró un resultado incorrecto del cambio: todas las sondas (ln, lg,
ld, 2lg, sqrt, pH, hpX, hpC, Cel, degF y tan sobre referencias negativas, el
prefijo `Q`, las dos referencias negativas) dan el signo y el dominio
correctos en las vías directa, log10 y exacta; `CanonicalKey` sobre `[nlnB]`
de 690 a 716 es monótona. `go test ./...` en verde, nada cambia con
referencias positivas.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| K197 no es equivalente: con un prefijo propio `X` = 2.2913482752e-14 (273.15/5²³, 11 cifras) f(0)/α de `XCel` es -5²³, un punto medio entre dos float64; `-800 [nlnK]` → `XCel` da -11920928955078126 y con K197, -…124. El comentario de `toLinearScale` (de 012) que pide «cientos de cifras» es falso | a (alta) | **Confirmado** con la aritmética exacta (5²³ impar y > 2⁵³); test, K197 a `kill` y el comentario corregido |
| Sobrevive quitar `srcUse.IsLogarithmic()` del chequeo: `-300 Cel` → `[nlnK]`, `-100 [p'diop]` → `[nlnB]` tienen valor y nadie lo prueba | b (media) | **Aceptado:** tests y K198 |
| Con un factor cero (`"0"` como unidad) el XML oficial cambia: `Multiply(-800 Np, 1 "0")` de `ErrOutOfRange` a 0; `Convert(1e-310, "0", "Np")` de `ErrOutOfRange` a `ErrOutOfDomain` | a (media) | **Aceptado:** los resultados nuevos son los coherentes (`Multiply(1e-200 m, 1e-200 "0")` ya daba 0); se registra en §2 y §8 y se prueba |
| `nfCan` lee los valores triviales con `CanonicalRat` | a (baja) | **Aceptado:** literales |
| El comentario del chequeo nombra solo «log a Cel» | b (baja) | **Aceptado** |

### Ronda 2 (2026-11-03, los mismos dos Opus)

Los dos confirmaron cerradas las correcciones de la ronda 1 (K197 y K198
muertos, los oráculos nuevos independientes, el comentario de `toLinearScale`
exacto: ningún prefijo del XML oficial cae en un punto medio). Sin signo ni
dominio equivocado en todas las clases de escala con referencia negativa y
con prefijos de punto medio en Cel, degF y degRe.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Una referencia de valor 0 (`[zln]` = ln(1 `[zero]`), `[zero]` = 0 m) hace caer `LogToLog` con «nil pointer dereference» (`Log10Rat(0)` es -Inf y `SetFloat64(-Inf)` da nil); sobrevive `< 0` → `<= 0` | a, b (media) | **Confirmado** en `main` con una sonda en un worktree aparte: es anterior a 015, pero está en la línea que el cambio protege. El chequeo pasa a `factor.Sign() <= 0` (el valor canónico es 0 y su logaritmo no existe); test y K199 |
| Con factor cero el ε de `toLinearScale` sale con signo +, y el intermedio es exactamente 0: `-800 [zlnK]` → `JCel` da …874 y el más cercano (empate al par) es …876 | b (baja) | **Aceptado:** con factor 0 el intermedio es el racional 0 (vía exacta), sin ε |
| El §2 da ejemplos del cambio con factor cero y no la regla general; el §8 no dice que con un XML propio algunos valores (equivocados) pasan a `ErrOutOfDomain` | b (baja) | **Aceptado** |
| El ε `0x1p-1074` → `0x1p-1000` sobrevive | a (baja) | **Fuera de alcance:** la línea es de 012 y no cambia aquí |

### Ronda 3 (2026-11-03, los mismos dos Opus): no converge

Los dos confirmaron cerradas las correcciones de la ronda 2 (K199 y K200
muertos; ninguna otra vía llega a `Log10Rat(0)` = -Inf en `SetFloat64`). El
revisor b barrió unas 113 mil sondas con un XML propio (referencias 0 y -1 en
todas las clases, prefijos 0, -1000 y de punto medio) y 36 mil con el XML
oficial, contra el `range.go` de `main`: sin pánico, sin cuelgue, sin signo
equivocado; todo cambio con el XML oficial involucra la unidad `"0"`. `go test
-race ./internal/engine/` en verde.

| Hallazgo | Revisor (severidad) | Estado |
|---|---|---|
| Con una referencia de valor 0 el resultado depende de la vía: `Convert(-800, "[zln]", "m")` da 0 y `Convert(1, "[zln]", "m")` da `ErrOutOfRange` (en `main`, los dos `ErrOutOfRange`); igual con lg, 2lg, pH, hpX, tan y sqrt, y en `Canonical`, `CanonicalKey` y `Compare`. Contradice la regla del §2. Causa: `linearTimes` lee todo cero como un desborde por abajo (`out == 0 && x.v != 0`), y `toDestination` igual (`r == 0 && !dst.IsLogarithmic()`) | a, b (media) | **Corregido:** `&& factor.Sign() != 0` y `&& sign != 0`; tests a ambos lados de 709 (1, -800, 800, 1e300) y K201 y K202, muertos |
| K196 y K198 apuntan a `factor.Sign() < 0`, que ya es `<= 0`: la corrida completa daría «pattern not found» | b (media) | **Corregido:** patrones en `<= 0`; K194-K202 aplican y mueren |
| Un prefijo de valor 0 ya da pánico en `main` (`division by zero`), no «NaN o +Inf» como dice el README | b (baja) | **Corregido** en el README |
| Las líneas de estado dicen «ronda 1» | b (baja) | **Rechazado:** la copia del revisor era anterior a la actualización |

La corrida completa de mutaciones previa a esta ronda se cortó por una señal
(`Terminated: 15`) después de K46, sin fallos hasta ahí: hay que repetirla.

Tope de 3 rondas alcanzado. El usuario dijo «continúa» (2026-11-03): se corrige y se cierra sin cuarta ronda, como en 014, con mutaciones completas.

Cierre: la corrida completa encontró K23, K24 y K184 con patrones viejos (el
`&& sign != 0` de `toDestination` y el `r.Mul` que pasó a `exactTimes`):
corregidos y muertos uno por uno. K156 (`mapRat` manda un cero exacto por la
forma log10) sobrevive a `./`, `./internal/engine/` y `./fhir/` enteros: con
el cambio, un cero de signo 0 da 0 en `linearTimes`, `toDestination` y
`combineScalars`, `ErrOutOfDomain` hacia un logaritmo y `ErrDivisionByZero`
como divisor, igual que `scalar{}`. Queda como equivalente; el atajo de
`mapRat` sigue como vía barata.
