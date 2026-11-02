# 012 — Las escalas de temperatura en la vía float64

**Estado:** paso 9 (cierre). Plan aprobado el 2026-11-02; review convergido en la ronda 3 (§10). Evidencia: `evidence/012-2026-11-02-mutations.txt` (`make mutations`, todas con el resultado esperado) y `evidence/002-2026-11-02-compare-java.md` (sin cambios frente al 2026-10-31). Falta commit y PR.

## 1. Revisión previa

### El defecto

Anotado en 010, §10, como `Multiply(-320 B, -273.15 Cel)` = `ErrOutOfRange`.
Medido contra `main` `d37ef13`, la nota estaba mal planteada y el defecto es
otro, más amplio:

- `Canonical(-273.15, "Cel")` no es 0: es `2.2737367544323207e-14 K`. El
  float64 -273.15 es -273.149999999999977262…, y `Canonical` va por la vía
  racional (`toCanonicalRat`: binario exacto más 273.15 exacto).
- Por eso `Multiply(-320 B, -273.15 Cel)` sí está fuera de rango: es
  10^-320 · 2.27e-14 ≈ 2.3e-334, bajo el menor subnormal.
- Pero `Multiply(3 B, -273.15 Cel)` también da `ErrOutOfRange`, con
  cualquier valor de `B`, y el exacto es `2.2737367544323206e-11 K`.

**Causa.** Una escala de temperatura solo pasa por la vía float64 en un
producto o una conversión con una escala no racional (`B`, `[pH]`, `Np`,
`tan`, `sqrt`); si no, todo es racional. Ahí `mapSpecial`
(`internal/engine/range.go`) usa `Use.ToCanonical`, que suma el offset
**como float64** (`offsetHandler.toCanonical`: `v + h.offset`):

- `-273.15 + 273.15` da 0 exacto en float64, y `mapSpecial` lo toma como un
  subdesborde (`m == 0 && value != 0` → `ErrOutOfRange`);
- cerca del cero absoluto, la suma cancela casi todas las cifras;
- lejos de él, el resultado se redondea dos veces: la suma y después el
  producto. Desde 011, `rat()` toma el binario de esa suma ya redondeada.

Sonda contra `main` (`Multiply(3 B, v escala)`, esperado
`nearest(10^3 · (bin(v) + offset) · tamaño del grado)` en `big.Rat`; v: el
origen de la escala, sus vecinos, valores típicos y 2.000 al azar en
[-offset, offset]):

| Escala | Puntos | `ErrOutOfRange` | Fuera del más cercano |
|---|---|---|---|
| `Cel` | 2.009 | 1 | 1.302 |
| `[degF]` | 2.009 | 1 | 922 |
| `[degRe]` | 2.009 | 1 | 1.052 |

Ejemplo de la cancelación: `Multiply(3 B, -273.1499999999999 Cel)` da
`5.68e-11`; el exacto es `7.96e-11`.

### `gofhir/server`

No tiene código de unidades propio (009, §1). Nada que tomar.

### Especificación

UCUM §21-22: la escala especial (r = f(m/u)/α) y las escalas de
temperatura (Celsius, Fahrenheit y Réaumur). El origen de cada una está en
prosa y vive en la tabla de `special.go` con su cita; el tamaño del grado es
la referencia de su definición en el XML. La regla de
rango de #59 (plan 003, §2): un resultado finito que cabe es un valor,
redondeado una vez. Sin constantes nuevas.

## 2. Cambio

> Reescrito tras la ronda adversarial (§9): la versión anterior no llegaba a
> `exactDirect` y suponía que un cero exacto no existía.

Una escala **con forma racional** (`RatHandler`: hoy solo `offsetHandler`)
entra a la vía float64 con su valor exacto, `ToCanonicalRat(bin(v))` (que ya
aplica α), y se redondea una vez:

1. **`Use.ToCanonicalExact`** devuelve ese racional también para un
   `RatHandler` (hoy solo para la potencia exacta de una escala
   logarítmica). Es el único lugar que alimentan `rat()`, `exactPower`,
   `directTimes` y `exactDirect`, así que la vía directa de un producto
   (011) y el tramo no normal (010) redondean una vez sin más cambios.
   `ToCanonicalNear` (el tramo guardado por log10) también lo devuelve.
2. **`mapSpecial`** para un `RatHandler`: con r el exacto,
   - r = 0 → `scalar{v: 0}` (un cero exacto es posible: `-273150 mCel`,
     `-27315 cCel` y `-2731.5 dCel` son 0 K);
   - `round(r)` normal → `scalar{v: round(r)}`;
   - si no (solo con un prefijo grande: `1.7e308 MCel` es 1.7e314 K) →
     la forma log10 (`sign`, `Log10Rat(r)`), como en 010, y `rat()` da r.
   Ya no hay `ErrOutOfRange` porque la suma float64 dé 0.
3. **`toDestination` hacia un `RatHandler`** (`Cel` como destino desde una
   escala no racional; solo con `NewFromReader`): si `rat()` da el exacto de
   `x`, `FromCanonicalRat(rat · factor)` redondeado una vez, sin el tope de
   330 (el offset devuelve al rango un intermedio que no cabe:
   `-400 [lgK]` → `Cel` es -273.15). Si `rat()` no lo da (bajo 10^-5000,
   §6), como hoy.
4. Un divisor exactamente cero (`-273150 mCel`) llega a
   `ErrDivisionByZero` por la vía directa: `x2.v` es 0. El comentario de
   `convert.go` que dice que float64 no puede representar el origen se
   corrige (sí puede, con prefijo).

Queda igual: `Canonical` y `Convert` entre escalas racionales (ya exactos),
`tan`, las entradas no finitas (#55, `mapFloat` y `Use.ToCanonical`), y la
API exacta.

**Cambia también lo que fija un test de 010:** `-308 B · 1 Cel`
(`nonnormal_test.go`) esperaba `10^-308 · bin(1 + 273.15)` = 2.7414999999999997e-306;
pasa a `nearest(10^-308 · 274.15)` = 2.7415e-306.

**Clase de error.** El exacto en el origen de `[degF]` (-459.67) es
negativo; hacia una escala logarítmica (con `NewFromReader`) deja de ser
`ErrOutOfRange` y pasa a ser `ErrOutOfDomain`, que es lo correcto.

## 3. Criterio de salida

- `Multiply` y `Divide` de una escala de temperatura (con y sin prefijo) por
  una escala no racional dan `nearest(exacto)`, con el exacto en `big.Rat`
  desde UCUM §21-22 y el XML: el origen no da error, su vecino no pierde
  cifras, y el cero exacto da 0 o `ErrDivisionByZero`.
- `ErrOutOfRange` solo donde el exacto no cabe (`-320 B` por `-273.15 Cel`;
  `-1 B · 1.7e308 MCel`), no donde sí cabe (`-320 B · 1.7e308 MCel` =
  1.7e-6 K).
- Monotonía en v alrededor del origen de cada escala, con el cruce de signo.
- `make check`, `make check-packages`, `make compare-java`, mutaciones
  completas. Costo medido: el exacto de la temperatura se calcula una vez
  por operando (no en `mapSpecial` y otra vez en `exactDirect`), y un
  producto sin temperatura (`B · m`) no cambia.

## 4. Tests (primero)

Los esperados salen de `big.Rat`: `bin(v)·α + origen` (origen de
`specTempOffset`, `internal/engine/range_oracle_test.go`, con su cita de
UCUM §21-22), por el tamaño del grado leído del XML (Cel 1 K, `[degF]` 5/9 K,
`[degRe]` 5/4 K; solo `Cel` es métrica y lleva prefijos).

1. Rejilla: `Cel`, `mCel`, `cCel`, `kCel`, `MCel`, `[degF]` y `[degRe]`, por
   `B`, `[pH]`, `Np`, `B[V]` y `B[kW]` (factor ≠ 1), `[p'diop]` (tan) y
   `[m/s2/Hz^(1/2)]` (sqrt), con exponentes enteros y no enteros, en los dos
   órdenes y en `Divide` con la temperatura de cada lado. El otro operando
   aporta su valor por la regla de 010/011 (potencia exacta, o el binario de
   su float64).
2. El origen y sus vecinos: `-273.15 Cel`, `-459.67 [degF]`, `-218.52 [degRe]`,
   `-0.27315 kCel` y ±1, ±2 ulp, contra el exacto; el signo cambia en el
   cruce exacto.
3. Cero exacto: `-273150 mCel`, `-27315 cCel`, `-2731.5 dCel`: `Multiply` da 0
   en los dos órdenes, `Divide(x, cero)` da `ErrDivisionByZero` y
   `Divide(cero, x)` da 0.
4. Rango: `Multiply(-320 B, -273.15 Cel)` = `ErrOutOfRange` (2.3e-334);
   `Multiply(-300 B, -273.15 Cel)` = `nearest(10^-300 · 2.27e-14)`
   (subnormal aceptado); `Multiply(-320 B, 1.7e308 MCel)` = 1.7e-6 K;
   `Multiply(-1 B, 1.7e308 MCel)` = `ErrOutOfRange`.
5. Divisor: `Divide(3 B, -273.15 Cel)` = `nearest(10^3 / 2.27e-14)`.
6. Definiciones propias (`NewFromReader`, una unidad `lg` de 1 K,
   `[lgK]`): `2 [lgK]` → `Cel` = -173.15, → `mCel` = -173150;
   `-400 [lgK]` → `Cel` = -273.15, → `[degF]` = -459.67; `400 [lgK]` → `Cel` =
   `ErrOutOfRange`; `-273.15 Cel` → `[lgK]` = `nearest(log10(2.27e-14))`;
   `-173.15 Cel` → `[lgK]` = 2; `-459.67 [degF]` → `[lgK]` = `ErrOutOfDomain`.
7. No cambia: `Canonical` y `Convert` de las escalas de temperatura (vía
   racional) y un producto `B` por `m`, idénticos a `main`.
8. El test de 010 (`-308 B · 1 Cel`) pasa a la regla nueva.

## 5. Mutaciones previstas

- `ToCanonicalExact` sin la rama `RatHandler` (1, 2).
- `exactDirect` con el binario de `x.v` para la temperatura (1).
- `mapSpecial` vuelve a `Use.ToCanonical` (2, 4).
- Un cero exacto vuelve a ser `ErrOutOfRange` (3).
- Sin la forma log10 para un `MCel` que no cabe (4).
- `toDestination` sin la forma racional, o con el tope de 330 (6).
- El origen del oráculo como float64 (`SetFloat64(273.15)`) debe hacer
  fallar los tests: prueba que el oráculo no copia al código.
- Mutaciones K1-K146 completas.

## 6. Fuera de alcance

- `sqrt` acepta valores negativos y el signo del cero (los otros pendientes).
- `Use.ToCanonical` en float64 para entradas no finitas (#55).
- Solo con definiciones propias (el XML no tiene una escala de temperatura
  `ln`): `720 [lnK]` → `YCel` da `ErrOutOfRange` como en `main` (el intermedio
  sin exacto no cabe aunque el resultado sí), y un prefijo propio de cientos
  de cifras puede dejar el origen tan cerca de un punto medio que el
  desempate con ε' falle (ronda 3).

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.4. Cambia resultados en 1 ulp o más (cerca del origen, mucho
más) en productos de temperatura por escalas no racionales, y el origen de
cada escala deja de ser `ErrOutOfRange`.

## 9. Ronda adversarial del plan (2026-11-02)

Codex sin cuota hasta el 2026-11-04; el usuario autorizó dos revisores Opus
independientes hasta que vuelva. Los dos midieron con sondas propias y
`big.Rat`, y confirmaron la tabla de §1 (Cel 1.257 y 1.287, `[degF]` 916 y
907, `[degRe]` 1.048 y 1.033, con otras muestras).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| §2.2 no llega a `exactDirect`: usa `exactPower` y el binario de `x.v`, no `rat()`; con §2.1 quedan 393-571 fallos de 2.006 por escala | a, b (alta) | **Confirmado** leyendo el código; §2.1: `ToCanonicalExact` da el racional de un `RatHandler`. El prototipo de b deja 0 fallos en 6 escalas |
| Un cero exacto existe con prefijo (`-273150 mCel`): `Canonical` da 0 K, pero `Multiply(3 B, ·)` da -5.68e-11 y `Divide(3 B, ·)` da -1.76e16 | a, b (alta) | **Confirmado** con sonda; §2.2 y §2.4, test 3; se corrige el comentario de `convert.go` |
| La rejilla no incluye tan, sqrt, log con factor ni `kCel` | a (media) | **Aceptado:** test 1 y 2 |
| `toDestination` hacia `Cel`: dos redondeos más y el tope de 330 (`-400 [lgK]` → `Cel` da `ErrOutOfRange`; el exacto es -273.15) | a, b (media) | **Aceptado:** §2.3 sin el tope, con el mismo racional que `rat()`; test 6 |
| Un test de 010 fija el defecto (`-308 B · 1 Cel` con `bin(1+273.15)`) | b (media) | **Aceptado:** §2 y test 8 |
| El origen de `[degF]` hacia una escala logarítmica pasa de `ErrOutOfRange` a `ErrOutOfDomain` | b (media) | **Aceptado:** §2 «Clase de error», test 6 |
| Un operando que no cabe aunque el resultado sí: `-320 B · 1.7e308 MCel` | b (media) | **Aceptado:** §2.2, forma log10; test 4 |
| Oráculo: el origen y el tamaño del grado no deben salir de `special.go` | a (baja) | **Aceptado:** `specTempOffset` y el XML; mutación del oráculo |
| Costo: el exacto se calcularía dos veces | a (baja) | **Aceptado:** criterio de §3 |
| Cambiar `offsetHandler.toCanonical` o `Use.ToCanonical` en vez de esto | b (baja) | **Rechazado:** α·v ya viene redondeado, no llega a `exactDirect`, y `Use.ToCanonical` también sirve a #55 |
| Bajo 10^-5000 sigue el `ErrOutOfRange` hacia `Cel` | b (baja) | **Aceptado como límite:** §6 |

## 10. Code review

### Ronda 1 (2026-11-02, dos Opus independientes)

Sin hallazgos en el cambio principal: el revisor b hizo un fuzz de 198.578
casos contra `big.Rat` (0 fuera del más cercano, 0 signos o clases de error
equivocados).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Hacia una escala de temperatura, un intermedio sin exacto bajo 10^-330 da `ErrOutOfRange` (`-5001 [lgK]`, `-1e300 [lgK]`, `-800 [lnK]` → `Cel`); el exacto, -273.15 + ε, cabe. Ya pasaba en `main` | a, b (media) | **Confirmado** con sonda. `toLinearScale` (extraída de `toDestination`, que superaba `gocognit`) da f(0)/α: ε queda bajo medio ulp del origen. Tests en `TestOffsetCustomLogScale`; mutación K155 |
| Sin la guarda del cero exacto en `mapRat`, los tests pasan; `-273150 mCel` → `[sqK]` daría `ErrOutOfRange` | b (baja) | **Confirmado:** test con `[sqK]` = sqrt(1 K); mutación K156 |
| `case RatHandler` de `ToCanonicalNear` es inalcanzable; `exact` repite `ToCanonicalExact` | a, b (baja) | **Aceptado:** se quita el `case`; `exact` queda documentado como caché |
| Costo: 155 → 175 asignaciones en `Multiply(3 B, 20 Cel)` | a (baja) | **Aceptado en parte:** `ToCanonicalExact` arma la temperatura sobre un solo `big.Rat`. Medido después: 175, `Multiply(3 B, 2 m)` 113 y `Canonical(20, "Cel")` 83, sin cambio. El exacto de la temperatura es el costo del arreglo |
| El oráculo de exponente no entero, tan, Np y sqrt repite la regla de 010/011 (el binario de `math.Pow`) | a (baja) | **Rechazado como defecto:** es la regla del plan (§4.1); esos casos prueban un redondeo, no la potencia verdadera |
| `NewExact().Multiply(1 Cel, 1 B)` no da error | b (nota) | **Rechazado:** `ExactService` también tiene la API float64; `ErrNotRational` es de los métodos `*Rat` (`ucum.go:196`, README) |

### Ronda 2 (2026-11-02, dos Opus independientes)

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El arreglo de la ronda 1 corta en 10^-330: un `[lnK]` entre e^-746 y e^-759 hacia `Cel`, `[degF]` o `mCel` sigue dando `ErrOutOfRange`, entre valores que sí dan el origen | a, b (media) | **Confirmado.** El umbral pasa al menor float64 normal (`smallestNormalLog10`): bajo él, el aporte no mueve el redondeo. Tests en -709, -746, -750, -759, -760 y -800 `[lnK]` hacia `Cel`, `[degF]`, `mCel` y `nCel`; mutación K157 (volver a -330) |
| Un origen justo en un punto medio entre dos float64 (prefijo propio de 2.359296e-14: 273.15/α = 607·5^19) se redondea al par, no al lado de ε: 1 ulp corrido | a (baja) | **Confirmado** con test (sin el signo da -1.1577606201171876e16; el exacto es -…874). Se redondea f(ε')/α con ε' el menor subnormal con el signo de ε |
| La rama de f(0)/α con α = 1 no distingue dividir en float64 | b (baja) | **Confirmado:** caso `nCel`; mutación K158 |
| `-700 [lnK]` → `Cel` da -273.14999999999998 | a (nota) | **Rechazado** con sonda: da -273.15, el float64 de -273.15; las 17 cifras son el mismo valor |
| Los mutantes `log10Times(1)` y sin `if x.log` sobreviven | b (baja) | El revisor los marca como equivalentes; no se verificó aparte y no entran al script |

### Ronda 3 (2026-11-02, dos Opus independientes): converge

Sin hallazgos de severidad media o mayor. El revisor b barrió 20.000
productos y cocientes al azar contra `big.Rat` (0 fallos) y la monotonía a
los dos lados del umbral (sin inversiones).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Una unidad `ln` con factor > 1 (`[lnhK]` = ln(100 K)) hacia `nCel` redondea dos veces: el intermedio es normal, sin exacto, y pasa por `(m - 273.15)/α` en float64 | a (baja) | **Confirmado** y corregido: el intermedio entra como el binario de su float64 y se redondea una vez. Test con -709, -710 y -712 `[lnhK]`; mutación K160 |
| Con un prefijo propio de unas 420 cifras, el desempate con ε' puede caer del lado equivocado | b (baja) | **Aceptado como límite** (§6): con los prefijos del XML no ocurre; el comentario ya no dice «con cualquier prefijo» |
| `720 [lnK]` → `YCel` da `ErrOutOfRange` (ya en `main`) | b (baja) | **Aceptado como límite** (§6): solo con una escala de temperatura `ln` propia |
