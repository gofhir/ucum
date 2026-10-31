# 010 — Potencia exacta cuando el intermedio no es normal (#75)

**Estado:** paso 9 (cierre). Rediseño aprobado el 2026-10-30; el code review convergió en la ronda 3. Plan aprobado el 2026-10-30.

## 1. Revisión previa

### El defecto

`mapScalar` (`internal/engine/range.go:50`) lleva el valor de una escala
especial a la de referencia. Si `f⁻¹(αv)` no es un float64 normal (subnormal,
cero por redondeo o desborde), guarda su signo y su log10, y `linearTimes`
lo materializa una vez con `fromLog10`, es decir `math.Pow(10, l)`, con
`l = log10 f⁻¹(αv) + log10(factor)`. Eso evita perder el valor cuando el
factor de la unidad lo devuelve al rango (plan 003, #59), pero `math.Pow` y
la suma de dos log10 redondeados pierden cifras.

009 hizo exacta la potencia de exponente entero solo cuando el resultado es
normal (§3 de su plan: «subnormales, excluidos»). Medido en `main`
`43744ed` contra la potencia exacta por el factor, redondeada una vez
(`big.Rat`):

| Caso | Fuera del float64 más cercano |
|---|---|
| `Canonical(n, "B")`, n entero en [-323, -308] (subnormal) | 1 de 16: `-308 B` da `9.999999999999994e-309` |
| `Canonical(n, "[pH]")`, n en [332, 349] (resultado subnormal) | 5 de 18 |
| `Canonical(618, "B[SPL]")` (intermedio 10^309, resultado 2·10^307) | `1.9999999999998707e+307`, unos 650 ulp |
| `Multiply(-308 B, 1 "1")` | igual que `Canonical`: `combineScalars` también va por log10 |
| `Canonical(n, "[pH]")`, n en [308, 331] (intermedio no normal, resultado normal; Opus) | 24 de 24: `308 [pH]` da `6.0221407599998306e-282` |
| Todas las unidades logarítmicas, puntos de intermedio no normal (Opus) | 56 de 182 ([pH] 29/43, B[kW] 7/22, B[10.nV] 5/16) |
| `Convert(-600, "B", "10*-300")`, `Multiply(-600 B, 1e300 "1")` (Opus) | `9.999999999999994e-301` |
| `Multiply(170 [pH], 150 [pH])`: operandos normales, producto no normal (Opus) | unos 250 ulp |
| `Divide(1 "1", 308 B)` (Fable) | `9.999999999999994e-309`; el más cercano es `1e-308` |

### Cambio propuesto (rediseñado en la ronda adversarial, §6)

**Regla:** donde la vía float64 hoy **materializa por log10** (`linearTimes`
de un `scalar` log, y la rama log10 de `combineScalars`), si todo operando
tiene valor exacto, el resultado es ese valor exacto por el factor,
redondeado una vez. Donde hoy no se pasa por log10, nada cambia.

Valor exacto de un operando:
- de una escala especial con exponente entero (la regla de 009: el exponente
  float64, o el exacto con prefijos n, f, a, z), la potencia exacta, aunque
  su intermedio sea normal;
- de un float64 finito sin escala especial (o de una escala lineal como
  `Cel`), su binario exacto, como en el resto de la librería;
- un exponente no entero con intermedio normal aporta el binario de su
  float64 (el de `math.Pow`), como `Cel` (ronda 1 del code review, §8);
- un exponente no entero fuera del rango normal (guardado solo por su
  log10), o un operando NaN o ±Inf, no tiene valor exacto: se queda la vía
  log10 de hoy (#55 deja pasar los no finitos).

La potencia se calcula solo al materializar, no en `mapScalar`: el caso
común no paga nada, y no se calcula dos veces.

### Por qué solo donde hoy se pasa por log10

Usar la potencia exacta también en la vía directa es la opción B de #75
(`1 [pH]` pasaría a ser igual a `Exact 0.1 mol/L` y distinto de
`Value 0.1 mol/L`), que el usuario dejó fuera. Donde hoy se materializa por
log10, el resultado sale de `math.Pow` sobre una suma de log10 redondeados;
no hay ninguna comparación que preservar.

**Costo de excluir la opción B** (Fable): con factor distinto de 1, entre 147
y 172 de unos 615 exponentes enteros por unidad (25 %) no dan el float64 más
cercano en la vía directa, y siguen así. El tramo que hoy va por log10 queda
más exacto que el directo.

### `gofhir/server`

No tiene código de unidades propio (009, §1). Nada que tomar.

### Especificación

UCUM §22 (las funciones de las escalas especiales) y la regla de rango de
#59 (plan 003, §2): un resultado finito que cabe es un valor, redondeado una
vez. Sin bases nuevas: las de la tabla de `special.go`.

## 2. Cambio

1. `special.Use.ToCanonicalExact(v) (*big.Rat, bool)`: la potencia exacta,
   con la regla de 009 para decidir si el exponente es entero. Sin el tope de
   310: lo acota quien llama (punto 4). `ToCanonical` sigue igual.
2. `scalar` gana lo necesario para calcular su valor exacto al materializar:
   la escala especial y el valor de origen (o nada, si es un float64 lineal).
3. `linearTimes` (log) y la rama log10 de `combineScalars`, antes de
   `fromLog10`: si todo operando tiene valor exacto, `roundRat(exacto ·
   factor)`. El divisor cero se decide antes (`ErrDivisionByZero`), como hoy.
4. **Tope por el resultado, no por una constante:** se calcula como hoy el
   log10 del resultado; si su magnitud pasa de 330, el resultado no cabe (o
   es cero) y sigue la vía de hoy, que da `ErrOutOfRange`. Si cabe, el
   exponente de la potencia queda acotado por |log10 del resultado| más
   |log10 del factor| y del otro operando. `10*` acepta exponentes de hasta
   ±1000 (`ucumerr.MaxExponent`), así que se fija además un tope de costo
   sobre |n·log10(base)|: 5000 (`maxExactLog10`), una constante de costo, no
   derivada de `MaxExponent`. Un factor de 10^1000 en cada operando y la
   unidad del producto llevan el exponente que cabe hasta unos 3000; más
   allá sigue el log10 de hoy.

`fix:`, versión 4.4.2. Sin dependencias nuevas.

## 3. Criterio de salida

- Donde hoy se materializa por log10 y todo operando tiene valor exacto
  (`Canonical`, `Convert` a una unidad lineal o a una escala sin log10,
  `Multiply`, `Divide`), el resultado es el float64 más cercano al valor
  exacto, o `ErrOutOfRange` si no cabe.
- Donde hoy no se pasa por log10, idéntico a `main` (opción B fuera).
- Monótona en el borde: el vecino en float64 que comparte el exponente
  float64 entero también va por la vía exacta (regla de 009).
- Sin pánico con operandos no finitos ni con divisor cero.
- `make check`, `check-packages`, `compare-java` en verde.

## 4. Tests (paso 5)

Esperados con `big.Rat`: la potencia según la función de UCUM §22 por el
factor canónico, leído de `ucum-essence.xml` con la API exacta
(`CanonicalRat` de la unidad de referencia), no del código bajo prueba.

1. **Rejilla:** `B` (y prefijos d, n, f), `[hp'_*]`, `bit_s`, `[pH]`,
   `B[SPL]`, `B[V]`, `B[mV]`, `B[uV]`, `B[10.nV]`, `B[W]`, `B[kW]`: todo
   exponente entero cuyo intermedio no es normal y cuyo resultado cabe
   (incluye `[pH]` de 308 a 349 y `618 B[SPL]`).
2. **Convert** a unidades lineales con factores lejanos: `-600 B` a
   `10*-300`, `[pH]` a `mmol/L`.
3. **Multiply y Divide:** el operando del test 1 por y entre `1 "1"`, `2 m`,
   `1e300 "1"`; dos operandos normales con producto no normal
   (`170 [pH] · 150 [pH]`); `1 "1" / 308 B`; un operando `Cel`; un operando
   lineal subnormal (`320 B · 1e-310 "1"` es el producto del binario de
   1e-310, `9.99999999999997e9`, no `1e10`).
4. **Guarda de regresión:** vía directa (intermedio normal) y exponente no
   entero, también en el tramo no normal (`-308.5 B`), bit a bit como en
   `main`; `1 [pH]` = `6.022140760000001e+25`.
5. **Monotonía:** vecinos en float64 de cada exponente entero del tramo,
   incluido el vecino que comparte el exponente float64 (`-3079.9999999999995
   dB`).
6. **Errores y no finitos:** `ErrOutOfRange` si no cabe (`-400 B`,
   `700 B[SPL]`, `Multiply(-308 B, 4.9e-324 "1")`); `ErrDivisionByZero` con
   divisor 0 y -0 y dividendo exacto (`-320 B / 0 m`); NaN y ±Inf por
   `-308 B` en `Multiply` y `Divide` pasan sin error, como hoy.
7. **Ucum-java:** `make compare-java` antes y después.

## 5. Mutaciones (paso 7)

Sin vía exacta en `linearTimes`; sin vía exacta en `combineScalars`; ignorar
el factor (muere solo con unidades de factor distinto de 1: `[pH]`,
`B[kW]`); usar el exacto también en la vía directa (opción B: muere con el
test 4); aceptar un exponente no entero redondeándolo (muere con `-308.5
B`); quitar la guarda de no finitos; quitar la del divisor cero; tope de
costo menor que el medido. Además, la suite completa.

## 6. Ronda adversarial (Opus y Fable, en paralelo)

| Hallazgo | Revisor | Resolución |
|---|---|---|
| El tope no puede salir del XML: `Convert` usa el cociente de factores, `Multiply` suma el valor del otro operando y `10*` acepta ±1000; `Convert(-600 B, 10*-300)` cabe y sigue mal | Opus (alta) | **Aceptado:** tope por el log10 del resultado, más un tope de costo fijo de 5000 (§2.4) |
| El disparador por operando no ve un producto no normal de dos operandos normales: `170 [pH] · 150 [pH]`, unos 250 ulp | Opus (alta) | **Aceptado:** la regla es «donde hoy se materializa por log10», no «intermedio no normal» (§1) |
| Un operando NaN o ±Inf llegaría a `RatFromFloat`, que da nil, y `Mul` reventaría | Fable (alta) | **Aceptado:** sin valor exacto, sigue la vía de hoy; test 6 |
| `Divide(1, 308 B)` seguiría por log10 mientras `Multiply(1, -308 B)` pasa a exacto | Fable (media) | **Aceptado:** la rama de dos float64 también usa la regla (un operando de escala especial con exponente entero aporta su potencia exacta aunque su intermedio sea normal) |
| §1 y §2.4 se contradecían («float64 normal» contra «no log»); `320 B · 1e-310` cambia según cuál | Fable (media) | **Aceptado:** binario exacto de cualquier float64 finito, la regla de la librería; test 3 lo fija |
| La tabla subestimaba el defecto: `[pH]` 308–331, 56 de 182 puntos | Opus (media) | **Aceptado:** §1 |
| Tests y mutaciones: `-308.5 B`, factor 1 no mata «ignorar el factor», n/f/a/z, `Cel`, subnormal lineal, divisor 0 y -0 | Opus (media), Fable (baja) | **Aceptado:** §4 y §5 |
| El vecino que comparte el exponente float64 entero debe ir también por la vía exacta | Fable | **Aceptado:** regla de 009; test 5 |
| La vía directa queda con doble redondeo (25 % con factor ≠ 1) | Fable (baja) | **Aceptado:** escrito como costo de excluir la opción B (§1) |
| «`log10Times` sin cambios» no tiene caso alcanzable: todo par log→log va por `LogToLog` | Opus, Fable (baja) | **Aceptado:** criterio eliminado |
| La potencia se calcularía dos veces | Opus, Fable (baja) | **Aceptado:** se calcula solo al materializar |
| Monotonía en el borde y «intermedio normal, resultado subnormal» ya dan el más cercano | Opus, Fable | Medido por los dos; sin cambio |

## 7. Pasos 5 a 7

**Tests primero** (`nonnormal_test.go`). Antes de corregir fallaron donde
`main` está mal: en la rejilla, `[pH]` 29 de 43, `B[kW]` 7 de 22, `B[10.nV]`
5 de 16, `bit_s` 8 de 52 y un caso por unidad en las demás; los tres
`Convert` y los ocho `Multiply`/`Divide`. Las guardas (vía directa, no
entero, monotonía, errores, no finitos) pasaban.

**Implementación:**
- `special`: `integerExponent` (la regla de 009) separado de `powerInto`, y
  `Use.ToCanonicalExact` con un tope de costo propio (`maxExactLog10 =
  5000`). `ToCanonical` sigue con el tope de 310 y un `big.Rat` local.
- `engine/range.go`: `scalar` guarda la escala (por valor) y el valor de
  origen; `exact()` da la potencia o el binario del float64 solo al
  materializar. `linearTimes` (log) y la rama log10 de `combineScalars` la
  usan si el log10 del resultado está dentro de ±330.
- El test de fronteras de 009 (`-308 B`) pasa a esperar `1e-308`.

**Costo** (asignaciones por llamada, `main` contra la rama): `7.4 [pH]` 193
y 193; `3 B` 81 y 81; `-308 B` 80 y 98; `618 B[SPL]` 257 y 276;
`-308.5 B` 71 y 73; `Multiply(-308 B, 2 m)` 128 y 156. La vía directa no
cambia. Dos ajustes salieron de medir: guardar `special.Use` por valor y no
devolver el `big.Rat` de `ToCanonical`, porque los dos escapaban al heap
(+1 asignación en `3 B`).

**Ucum-java:** `make compare-java` da el mismo reporte que `main`
([evidencia](evidence/002-2026-10-30-compare-java.md)).

**Mutaciones** K107–K114: sin vía exacta en `linearTimes` o en
`combineScalars`; sin el factor; la vía exacta también en la directa (opción
B); un operando no finito hasta el producto exacto; sin el chequeo de divisor
cero; tope de resultado 30; tope de costo bajo 10^309.

K111 sobrevivía: un operando no finito ya queda fuera por el tope (su log10
no es finito), y ningún test tenía un operando sin valor exacto en la rama
log10 de `combineScalars`. Se agregó `-308.5 B · 2 m` al test 4, y K111 pasó
a probar la comprobación de nil para un exponente no entero. Suite completa:
152, sin resultados inesperados
([evidencia](evidence/010-2026-10-30-mutations.txt)).

## 8. Code review

### Ronda 1 (Opus y Fable, en paralelo)

Ninguno encontró un defecto de corrección en la vía exacta. Midieron la
monotonía (vecinos hasta 300 ulps), el acuerdo entre `Canonical`, `Convert`
a `1`, `Multiply` por 1 y `Divide` por 1, el tope de ±330 (ningún resultado
que cabe queda fuera), ceros, no finitos y el costo en el borde de 5000
(`Multiply(-5000 B, 5000 B)` 17 µs). Fable comparó `main` con la rama en
638 mil líneas: las 14.503 que cambian están todas donde `main` pasaba por
log10; `Np` y `cNp` no cambian.

| Hallazgo | Quién | Resolución |
|---|---|---|
| El factor de la unidad en el tope de `combineScalars` no tiene test (el mutante `l1+l2` sobrevive) | Opus (media) | **Confirmado.** `Multiply(-600 B, 1 10*300)` y `Divide(1 10*300, 600 B)` = 10^-300; mutación K115 |
| El tope de costo no tiene test sobre 600 (bajarlo a 700 sobrevive) | Opus (media) | **Confirmado.** `Convert(-900 B, 10*-600)` = 10^-300; mutación K116. §2.4 corregido: 5000 es de costo, no sale de `MaxExponent` |
| Un operando log con exponente no entero e intermedio normal no tenía valor exacto: `Multiply(170.5 [pH], 150 [pH])` quedaba a 295 ulps, y el mismo float64 escrito en la unidad canónica a 3 | Fable (media) | **Confirmado** con un test que fallaba (…098e-267 contra …726e-267). `exact()` cae al binario del float64 si no hay potencia exacta; mutación K117. §1 ajustado |
| La tabla del test repite base, paso y referencia del XML | Opus (baja) | **Aceptado:** `loadLogUnits` los lee de `ucum-essence.xml`; el test solo declara la semántica de cada función (UCUM §46–47) |
| Comentario de `integerExponent` nombra `power` y pasa de 100 columnas | Opus y Fable (baja) | **Aceptado** |
| README: «`618 B[SPL]` is 2×10^307 Pa» (es el valor canónico; en Pa es 2×10^304) | Fable (baja) | **Aceptado** |
| El segundo chequeo del tope en `ToCanonicalExact` con 310 sobrevive; tope de resultado 320 sobrevive | Fable (baja) | **Rechazado como hallazgo:** Fable no encontró un caso alcanzable (nB, fB, aB, zB con \|n\| de 311 a 1200 dan exponente float64 entero); 320 es equivalente en la banda subnormal (el menor subnormal es 10^-323.3) |

Después de corregir: `make check` en verde y suite completa de mutaciones,
155 sin resultados inesperados
([evidencia](evidence/010-2026-10-30-mutations.txt)).

### Ronda 2 (Opus y Fable, en paralelo)

Sin defectos en el respaldo de `exact()` para cero, ±0, no finitos, `Np`,
temperatura, `tan` ni `sqrt`. `Canonical`, `Convert` a la unidad canónica,
`Multiply` por 1 y `Divide` por 1 coinciden en 176.868 entradas (Opus) y en
las 17 unidades del XML a medios pasos en [-1200, 1200] (Fable).

| Hallazgo | Quién | Resolución |
|---|---|---|
| **Monotonía peor que en `main`** en `Multiply` y `Convert` con factor distinto de 1: el punto exacto queda fuera de la curva log10 que siguen sus vecinos (`Multiply(-966 B[SPL], 3 10*300)` sube a 6.0000000000000006e-185 y su vecino -965.99999999999989 baja a 5.9999999999999993e-185; `Convert(125, [hp'_Q], 3.10*-300)`) | Opus (media) | **Confirmado** con una sonda. Los vecinos llevan el error de la vía log10 de `main` (unos 1500 ulps). **Pendiente de decisión del usuario** |
| Mutante: un operando normal de exponente entero nunca toma la potencia exacta (`&& x.log`); `170 [pH] · 150 [pH]` no lo distingue | Opus y Fable (media) | **Confirmado.** `2 [pH] · 307 [pH]` = 10^-309·(mol/l)²; mutación K118 |
| El oráculo de `170.5 [pH] · 150 [pH]` salía de `Canonical` (dos redondeos), no de la regla | Opus y Fable (media) | **Confirmado.** Ahora `bin(math.Pow(10, -170.5))·10^-150·(mol/l)²`; `Cel` pasa a `bin(1+273.15)` |
| Tope del producto en 309 sobrevive | Opus (baja) | **Aceptado:** `-310 B · 3` = 3·10^-310; mutación K119 |
| Ningún operando negativo en el producto exacto | Opus (baja) | **Aceptado:** `-600 B · -1e300`; mutación K120 |
| `x.use.Handler != nil` redundante; `ToCanonicalExact` sin test directo en `special` | Fable (baja) | **Rechazado:** el chequeo es barato y explícito; los topes de `ToCanonicalExact` están cubiertos por K114 y K116 |
| `Multiply(-320 B, -273.15 Cel)` da `ErrOutOfRange` (0 K como desborde), aunque `Canonical(-273.15 Cel)` es 0 | Fable (baja) | **Fuera de alcance:** viene de antes (plan 003); queda anotado como pendiente |

Después de corregir: `make check` en verde y suite completa de mutaciones,
158 sin resultados inesperados.

## 9. Rediseño: potencia partida (tras la ronda 2)

**Por qué.** La ronda 2 confirmó que la regla de §1 empeora la monotonía
frente a `main` en `Multiply` y `Convert` con factor distinto de 1: el punto
de exponente entero sale exacto, y sus vecinos en float64 siguen la vía log10,
con un error de unos 1500 ulps (`Multiply(-966 B[SPL], 3 10*300)` y su vecino
-965.99999999999989). `main` era monótono porque todos los puntos llevaban el
mismo error. El usuario eligió corregirlo partiendo el exponente (2026-10-30).

**Regla nueva.** En una escala logarítmica de base entera, un valor que hoy
se guarda solo por su log10 (intermedio no normal) y cuyo exponente no es
entero según la regla de 009 vale

    base^n · bin(math.Pow(base, f)),   e = n + f,   n = round(e),   |f| ≤ ½,

con `e` el exponente float64 que hoy recibe `math.Pow` (`v·α/expDivisor`,
con su signo). `base^n` es exacto (`big.Rat`), y `f = e − n` también lo es
(Sterbenz: `e` y `n` están a menos de un factor 2 para |e| ≥ 1, y el tramo
no normal exige |e| ≥ 65). `math.Pow(base, f)` no es exacto: Go calcula
`Exp(f·Log(base))`, y medido contra un oráculo de 400 bits queda a ≤ 2,3 ulps
en bases 2 y 10, y a ≤ 5,5 en 100, 1000 y 50000 (ronda adversarial, abajo).
El valor partido es, entonces, una aproximación de pocos ulps, no el más
cercano. Lo que se gana es la monotonía y quitar los ~1500 ulps de la vía
log10.
`linearTimes` y `combineScalars` lo usan como hoy usan el valor exacto, con
un solo redondeo de valor · factor.

**Lo que no cambia:**
- Exponente entero: la potencia exacta (regla de 009). En `f = 0` coincide:
  `math.Pow(base, 0) = 1`.
- Intermedio normal: la vía directa (opción B fuera) y, en un producto, el
  binario del float64 de `math.Pow` (ronda 1).
- `Np` y `cNp` (base e, no entera), los valores sobre el tope de costo
  (`maxExactLog10`) y los no finitos siguen por log10. En esos casos todos los
  puntos ya van por la misma vía, así que no pierden monotonía, y no tienen
  forma exacta.
- `sqrt` **sí entra** (ronda adversarial): su valor es (α·v)², racional
  exacto, así que va como valor exacto: `(bin(v)·α)²`.

**Monotonía, el argumento.** Desde la frontera no normal, |e| ≥ 65 (base
50000; 307 en base 10). Un paso de float64 en `v` cambia `e` en al menos un
ulp de `e` (`e = fl(v·α/d)` no decrece con v), y eso mueve el valor unas
700 ulps o más. El error de la potencia partida es de ~1 ulp de `math.Pow`
más el redondeo final, muy por debajo del paso. En el borde normal y no
normal, `math.Pow` completa (de un lado) y la potencia partida (del otro)
están a pocos ulps del valor verdadero, también muy por debajo del paso.
**Posible residuo de 009:** con prefijos n, f, a, z, el punto cuyo exponente
exacto es entero vale base^n, y un vecino con `e` float64 a 1 ulp del entero
podría quedar a ~1 paso. Ninguno de los dos revisores lo observó (nB, fB, aB,
zB). El test lo cuenta; no se documenta como efecto real mientras no aparezca.

**Cambios en el código:**
1. `special.Use.ToCanonicalNear(v) (*big.Rat, bool)`: la potencia exacta de
   `ToCanonicalExact` o, si el exponente no es entero, la partida. Mismo tope
   de costo. Falso fuera de las escalas de base entera.
2. `scalar.exact()` pasa a llamarse `rat()`. En orden:
   - escala de base entera con exponente entero: la potencia exacta (también
     con intermedio normal, como hoy);
   - escalar guardado por log10 (`x.log`) de base entera: `ToCanonicalNear`;
     de `sqrt`: `(bin(v)·α)²`;
   - si no es log: el binario de un float64 **finito** (incluye subnormales,
     como `1e-310`);
   - si no, nil.
   Un intermedio normal de exponente no entero sigue aportando el binario de
   `math.Pow` (ronda 1), no la partida.

**Tests nuevos o cambiados:**
- Test 4: `-308.5 B` y `-308.5 B · 2 m` dejan de ser bit a bit `main`. El
  esperado `10^-308 · bin(math.Pow(10, -0.5))` repite la implementación: queda
  como **guarda de regresión rotulada**, no como oráculo.
- Test 5 (monotonía) se amplía: todas las unidades de `loadLogUnits`; vecinos
  ±8 ulps de cada exponente entero del tramo y del cruce `f = ±½`; con
  `Canonical`, `Convert` a una unidad con factor lejano (`3.10*-300`) y
  `Multiply` por `3 10*300`. Incluye los testigos de Opus
  (`-966 B[SPL]`, `125 [hp'_Q]`). Implementado sin contar aparte el residuo
  de 009: todas las unidades se exigen monótonas (más estricto), y los
  revisores de la ronda 3 no lo observaron en nB, fB, aB, zB, pB ni yB.
- **Exactitud de la partida (oráculo):** base^(n + k/2^m) con `big.Float` de
  400 bits, sacando m raíces sucesivas (`big.Float.Sqrt`), para fracciones
  diádicas al azar (no solo ½, donde `math.Pow` usa `Sqrt` y es exacto), en
  todas las bases de `loadLogUnits`, por `Canonical` y por `Multiply` con un
  factor que lleve el resultado al rango normal. Tolerancia por base, la
  medida: 2,5 ulps para 2 y 10, 6 para 100, 1000 y 50000.
- **sqrt:** `Convert(1.5e-160, [m/s2/Hz^(1/2)], 10*-300.m2/s4/Hz)` = 2.25e-20
  (hoy …0013e-20); `Multiply(1e-160 …, 1e300 "1")` y `Multiply(2e160 …,
  1e-300 "1")`, el más cercano a (v·α)²·factor en `big.Rat`; monotonía en
  5000 entradas seguidas desde 7e-162 (hoy 13 salidas distintas).

**Mutaciones nuevas:** `math.Pow(base, e)` entero en lugar de partido (vuelve
el error de `main`); `f` con el signo cambiado; `n` desplazado en 1; partida
también con intermedio normal; sin partida (vuelve el `nil`); `sqrt` sin
valor exacto; `sqrt` con `v·α` en float64 en vez de exacto.

**Costo** (medido por Fable con la partida): `Canonical(-308.5 B)` 2,5 → 7,2
µs y 73 → 104 asignaciones; `Multiply(-308.5 B, 2 m)` 4,8 → 10,5 µs y 135 →
163. Solo en el tramo no normal. Se vuelve a medir con la implementación.

### Ronda adversarial del rediseño (Opus y Fable, en paralelo)

Los dos implementaron o modelaron la partida. Monotonía: Opus, 1,1 millones
de pasos en 18 unidades (prefijos k a z), 0 inversiones, también en el cruce
`f = ±½` y en las dos fronteras; Fable, 3,33 millones de comparaciones en las
17 unidades: 622 violaciones sin la partida y 0 con ella. `Canonical`,
`Convert` a la canónica, `Multiply` por 1 y `Divide` por 1 coinciden en
20.238 puntos al azar. No es la opción B: la vía directa no cambia.

| Hallazgo | Quién | Resolución |
|---|---|---|
| «`math.Pow` con \|f\| ≤ ½ queda a menos de 1 ulp» es falso: hasta 2,3 ulps en base 10 y 5,4 en 50000 | Opus y Fable (media) | **Aceptado:** cota medida por base y texto corregido |
| El oráculo `10^(k/2)` solo prueba `f = ±½`, donde `math.Pow` usa `Sqrt` y es exacto | Opus y Fable (media) | **Aceptado:** fracciones diádicas al azar por raíces sucesivas, todas las bases |
| `sqrt` queda por log10 aunque tiene valor exacto: escalera de 13 salidas en 5000 entradas, hasta 1172 ulps | Opus y Fable (media) | **Aceptado** y confirmado con una sonda: `sqrt` entra como `(bin(v)·α)²` |
| El cambio 2 aplicaría la partida a intermedios normales y descartaría operandos subnormales | Opus (baja) | **Aceptado:** orden explícito en `rat()` |
| El esperado nuevo del test 4 repite la implementación | Fable (baja) | **Aceptado:** guarda rotulada |
| Costo sin medir | Opus y Fable (baja) | **Aceptado:** cifras de Fable arriba; se mide de nuevo |
| El residuo de 009 con prefijos n, f, a, z no se observa | Opus | **Aceptado:** se cuenta, no se afirma |

### Pasos 5 a 7 del rediseño

**Tests primero** (`nonnormal_split_test.go`, y el test 4 de
`nonnormal_test.go`). Con la regla de §1 fallaban así:
- test 7 (monotonía con factor): 28 pasos en `bit_s`, más los dos testigos
  de la ronda 2;
- test 8 (exactitud contra el oráculo de 400 bits): `B[SPL]` hasta 589 ulps;
- test 9 (`sqrt`): 4996 de 5001 entradas sin el más cercano;
- test 4: la guarda nueva de `-308.5 B`.

Después de implementar pasan. El peor caso medido del test 8 es 2,86 ulps
en base 10 (`B[SPL]`) y 3,80 en base 50000 (`[hp'_Q]`). La tolerancia de
base 10 subió de 2,5 a 3, porque al error de `math.Pow` se suma medio ulp
del redondeo final (los revisores midieron hasta 2,84). El test 7 recorre
uno de cada 23 medios exponentes para quedar en ~1 s.

**Implementación:** `special.Use.ToCanonicalNear` (potencia exacta, partida
con `logHandler.splitPower`, o cuadrado exacto de `sqrt`) y `scalar.rat()`
en lugar de `exact()`, con el orden de §9.

**Costo** (asignaciones por llamada, `main` contra la rama):

| Llamada | `main` | Rama |
|---|---|---|
| `7.4 [pH]` | 193 | 193 |
| `3 B` | 81 | 81 |
| `-308 B` | 80 | 98 |
| `-308.5 B` | 71 | 104 |
| `618 B[SPL]` | 257 | 276 |
| `Multiply(-308.5 B, 2 m)` | 119 | 163 |
| `Convert` de `sqrt` | 350 | 380 |

El caso común no cambia.

**Mutaciones** K121–K127:
- `math.Pow` con todo el exponente;
- la fracción con el signo cambiado;
- sin fracción;
- la partida también con intermedio normal;
- sin partida;
- `sqrt` sin cuadrado exacto;
- `sqrt` con `v·α` en float64 (K127 se espera que sobreviva: es equivalente,
  porque la única unidad `sqrt` del XML no es métrica y su α es 1).

«`n` desplazado en 1» se cambió por «sin fracción»: con |f| ≤ 1,5,
`math.Pow` sigue a pocos ulps y el mutante sería casi equivalente.

La suite completa destapó un enmascaramiento: **K21** (error de 1e-10 en
`fromLog10`) sobrevivía, porque la partida reemplazó los casos que pasaban
por `fromLog10` en `TestRangeHandWritten`. Se agregaron `-710 Np` y `-710 Np`
a `10*-300`, que siguen por log10 (base e). K111 pasó a usar un operando
`Np` no normal en `Multiply` (el único sin valor racional que llega al
producto), con su guarda en el test 4. K107, K110 y K118 se reescribieron
para `rat()`. Suite completa: 165, sin resultados inesperados
([evidencia](evidence/010-2026-10-30-mutations.txt)).

### Ronda 3 (Opus y Fable, en paralelo): converge

Ninguno dejó hallazgos de severidad media o mayor ni encontró defectos en
`splitPower`, `ToCanonicalNear` o `rat()`.
- Consistencia: `Canonical`, `Convert` a la canónica, `Multiply` por 1 (los
  dos órdenes) y `Divide` por 1 coinciden bit a bit en 39.985 puntos (Opus) y
  en 31.500 (Fable, con `sqrt`).
- Monotonía: 0 inversiones en las dos fronteras normal/no normal (960 mil
  pasos), en el tope de costo y en la frontera de `sqrt`.
- Oráculo del test 8: con 67 veces las muestras, el peor caso es 2,92 ulps
  en base 10 (tolerancia 3) y 5,12 en base 50000 (tolerancia 6).
- Fable reprodujo la suite completa: 165 de 165.

| Hallazgo | Quién | Resolución |
|---|---|---|
| Sobrevive el mutante «`ToCanonicalNear` siempre parte»; con prefijos n, f, a el exponente exacto es entero y el float64 no (`-4.57e11 nB · 1 10*457` = 1; el mutante da 590 ulps menos) | Opus y Fable (baja) | **Aceptado:** testigos de nB, fB y aB (= 1); mutación K128 |
| El tope de costo de `splitPower` no tiene test | Fable (baja) | **Aceptado:** `TestUseToCanonicalNearCostCap` (interno); mutación K129, con el tope subido en vez de quitado para no colgar la corrida |
| README: «a few ulps from the true value» no vale con prefijos cuyo α no es float64 (`zB`) | Fable (baja) | **Aceptado:** «from base^e», con la nota del redondeo de e |
| Comentarios con `exact` y con el uso de `ToCanonicalExact` | Opus y Fable (baja) | **Aceptado** |
| El plan decía que el test 7 cuenta aparte el residuo de 009 | Fable (baja) | **Aceptado:** texto alineado |
| `nonnormal_split_test.go` sin `git add`; `ucum.test` sin seguimiento | Opus (proceso) | Se agrega al commit; `ucum.test` queda fuera |
| `sqrt` acepta un valor de origen negativo (`Canonical(-2 sqrt)` = 4), igual en `main` | Fable (baja) | **Fuera de alcance:** viene de antes; pendiente |

## 10. Cierre

- `make check`, `make check-packages` y `make compare-java` en verde. El
  reporte contra Ucum-java es idéntico al de `main`
  ([evidencia](evidence/002-2026-10-30-compare-java.md)).
- Suite completa de mutaciones: 167, sin resultados inesperados
  ([evidencia](evidence/010-2026-10-30-mutations.txt)).
- `fix:`, versión 4.4.2; refiere #75. #75 sigue abierto por la opción B
  (potencia exacta en la vía directa), que queda fuera.
- Pendientes fuera de alcance, sin issue todavía:
  - `Multiply(-320 B, -273.15 Cel)` da `ErrOutOfRange` (0 K tratado como
    desborde), mientras `Canonical(-273.15 Cel)` es 0;
  - `sqrt` acepta un valor de origen negativo (`Canonical(-2 sqrt)` = 4).
