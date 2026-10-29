# 009 — Potencia exacta en las escalas logarítmicas (#75, opción A)

**Estado:** cerrado (paso 9), pendiente de commit. Plan aprobado el 2026-10-29; la regla del exponente cambió en la ronda 1 (§8).

## 1. Revisión previa

### El defecto

`logHandler.toCanonical` (`internal/special/special.go:367`) calcula
`math.Pow(base, e)`, con `e = v·α/expDivisor`, negado en las escalas que
cuentan hacia abajo. `math.Pow` no está correctamente redondeado. Medido
contra el float64 más cercano a la potencia exacta (`big.Rat`), en `main`
`62871d8`:

| Caso | Fuera del float64 más cercano | Peor |
|---|---|---|
| `math.Pow(10, n)`, n entero en [-323, 308] | 505 de 632; el primero en \|n\| = 23, y 83 de los \|n\| ≥ 23 sí son el más cercano | 7 ulp |
| `math.Pow(100, n)`, `1000^n`, `50000^n` en rango (Fable) | 255 de 316, 85 de 210, 46 de 134 | — |
| `math.Pow(2, n)` en rango (Fable) | 0 de 2098: `ld` ya es exacto | — |
| `Canonical(n, "B")`, mismos n | 505 de 632 | 7 ulp |
| `Canonical(v, "dB")`, v múltiplo de 10 en [-3070, 3080] | 504 de 616 | — |

Ejemplos: `Canonical(-300, "B")` = `9.999999999999994e-301` (más cercano:
`1e-300`); `Canonical(300, "B")` = `1.0000000000000006e+300`.

En `dB` además el exponente pasa por `v * α.Float64()` (`Use.ToCanonical`,
`special.go:117`): con α = 1/10, que no es un float64, `v·0.1` puede no ser
el entero v/10 aunque lo sea. Por eso el exponente tiene que calcularse
exacto, con α racional.

### Fuera de alcance (issue #75)

- **El doble redondeo con el factor de la unidad** (opción B del issue):
  `1 [pH]` sigue siendo `6.022140760000001e+25`, porque 10^-1 ya es el
  float64 más cercano y el error está en multiplicarlo después por el factor.
  Corregirlo cambia `Compare(1 [pH], Value 0.1 mol/L)` de 0 a -1.
- **Exponentes no enteros** (`7.4 [pH]`): su resultado es irracional;
  redondearlo bien exige exp/log de precisión arbitraria, que `math/big` no
  tiene.
- **`Np`** (`ln`, base e, irracional) y `fromCanonical` (hacia la escala).
- **`fromLog10`** (`internal/engine/range.go:105`) y
  `sqrtHandler.fromCanonicalLog10`: su argumento es un log10 ya redondeado,
  y un entero ahí no significa una potencia exacta.

### `gofhir/server`

No tiene código propio de unidades: usa esta librería (`grep math.Pow` solo
encuentra `internal/mdm`). Nada que tomar.

### Especificación

UCUM §22 y las definiciones de `ucum-essence.xml`: `lg` es 10^x, `lgTimes2`
10^(x/2), `ld` 2^x, `pH` y `hpX` 10^-x, `hpC` 100^-x, `hpM` 1000^-x, `hpQ`
50000^-x. Las bases ya están en la tabla de funciones de `special.go:277-287`;
no se agrega ninguna.

## 2. Cambio

> 2.1 a 2.4 son la versión aprobada; cambiaron en la ronda 1 del code review
> (§8). Hoy decide el exponente float64 cuando es entero, el tope es 310 y
> se aplica también al exponente exacto, y la potencia sale de un
> `big.Int.Exp`. El resultado es el float64 más cercano a base^f, con f el
> exponente float64 (el que recibía `math.Pow`), no a base^e con e el exacto.

1. En `Use.ToCanonical`, para un `logHandler` de base entera (todas menos
   `ln`), con v finito:
   - **Filtro barato en float64:** `f = v·α/expDivisor`. Si f no está a
     menos de 1e-9·|f| de un entero, o si |round(f)·log10(base)| > 400, se
     sigue por la vía de hoy. Así el caso común (`7.4 [pH]`) no paga
     aritmética racional.
   - **Exponente exacto:** `e = RatFromFloat(v)·α/expDivisor` (con su
     signo). Si `e` es entero, el resultado es (el tope del §2.3 acota |n| a
     unos 1329 antes de llegar aquí, así que n cabe en int64),
     `decimal.FromInt(base).Pow(n).Float64()`: la potencia exacta, redondeada
     una vez. `Decimal.Pow` ya existe (`decimal.go:80`).
2. En cualquier otro caso, la vía de hoy, sin cambios: `math.Pow`. Un v NaN
   o ±Inf no entra a la vía exacta (`RatFromFloat` da nil; #55 los deja
   pasar a propósito).
3. **El tope de 400** es un tope de costo: un resultado con |log10| > 324
   (mitad del menor subnormal, 4.9e-324) redondea a 0, y uno con
   log10 > 308.26 desborda, así que más allá de 400 no hay nada que redondear
   bien. Peor caso medido dentro del tope: 2^-1328 y 10^-400, 2–4 µs.
4. **Costo:** medir antes y después `Canonical(7.4, "[pH]")`,
   `Canonical(3, "B")`, `Canonical(-300, "B")` y `Canonical(-3000, "dB")`.
   Presupuesto: el caso no entero, sin asignaciones nuevas; el entero, lo que
   cuesta la potencia exacta (los revisores midieron, sin filtro, de 74 a 116
   asignaciones en `3 B`).

`fix:`, versión 4.4.1. Sin dependencias nuevas.

## 3. Criterio de salida

- Para cada función de base entera, con prefijo donde la API lo admite (solo
  `B` es métrica: `dB`, `cB`, `mB`, `kB`), y todo exponente entero cuyo
  resultado es **normal**: `Canonical` de la unidad sin factor (`B`,
  `[hp'_X]`, `[hp'_C]`, `[hp'_M]`, `[hp'_Q]`, `bit_s`) da el float64 más
  cercano a la potencia exacta.
- **Resultados subnormales, excluidos:** `mapScalar` (`range.go:54-66`)
  descarta un `m` no normal y lo rehace por log10 con `math.Pow`, porque el
  factor de la unidad puede volver a llevarlo al rango normal.
  `Canonical(-308, "B")` sigue en `9.999999999999994e-309`. Queda en #75.
- Con exponente no entero, tanto el float64 como el exacto, el resultado es
  idéntico al de `main`.
- `make check`, `check-packages` y `compare-java` en verde.

## 4. Tests (paso 5)

Los esperados salen de la fórmula de la especificación evaluada con
`big.Rat` (base^n exacto y `Float64()`), no de correr el código propio.

1. **API pública, rejilla completa:** las unidades sin factor del §3 × todo n
   entero con resultado normal, y `dB`, `cB`, `mB` con v = n/α. Los casos
   donde `main` está mal (505 en `B`, 255 en `[hp'_C]`…) son los que matan la
   mutación «sin vía exacta»; `bit_s` (`ld`) ya es exacto y el test solo lo
   protege.
2. **`internal/special`:** `Use.ToCanonical` con los prefijos que la API no
   admite (`k[pH]` no parsea), para cubrir `negate` con α ≠ 1.
3. **Guarda de regresión, no derivada de la especificación:** unos cientos
   de v no enteros (semilla fija; 7.4, 0.5, 1e-17) dan `math.Pow(base, e)`
   bit a bit, como en `main`.
4. **Fronteras documentadas:**
   - `Canonical(1, "[pH]")` sigue en `6.022140760000001e+25` (opción A: el
     factor redondea otra vez).
   - `Canonical(0.3, "kB")` = `1e300`, como `300 B` y como en `main`:
     `0.3*1000` da 300 en float64, el exponente que usa `math.Pow` (regla
     de la ronda 1, §8).
   - `Canonical(-308, "B")` sigue en `9.999999999999994e-309` (subnormal).
   - `Canonical(3, "Np")` sigue en `math.Pow(e, 3)`: la base no es entera.
     Mata la mutación «`ln` como base entera».
5. **Ucum-java:** `make compare-java` antes y después. Las escalas
   logarítmicas ya están clasificadas `java-special-as-linear` (172 casos);
   se verifica que no aparezca ninguna clase nueva.

## 5. Mutaciones (paso 7)

Quitar la vía exacta; ignorar α; ignorar el signo; base fija en 10; tratar
`ln` como base entera; tope en 40 en lugar de 400 (310 desde la ronda 1); aceptar un exponente no
entero redondeándolo; quitar el filtro barato (debe morir con el test de
costo, si se escribe, o quedar como mutación equivalente documentada). Además,
la suite completa de `scripts/mutations.py`.

## 6. Ronda adversarial (Opus y Fable, en paralelo)

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Los resultados subnormales no pasan por la potencia exacta en la API: `mapScalar` rehace un `m` no normal con `math.Pow`; `Canonical(-308, "B")` sigue mal con el parche | Opus (alta), Fable (media) | **Aceptado:** el criterio de salida se limita a resultados normales y lo dice; el subnormal queda en #75 y en el test 4.4 |
| `lgTimes2` con exponente impar es 10^k·√10, que `big.Float.Sqrt` redondea bien | Opus (media) | **Rechazado:** las cinco unidades `lgTimes2` llevan factor (`2 10*-5.Pa`, `V`, `mV`, `uV`, `10 nV`; `ucum-essence.xml`), así que su resultado público se redondea otra vez igual. Es la opción B; queda en #75 |
| `0.3 kB` y `300 B` coincidían en `main` y dejan de coincidir | Opus (media), Fable (baja) | **Aceptado** (y superado en la ronda 1: con la regla del exponente float64 vuelven a coincidir): test 4.4, documentado; es la regla de la librería (un `Value` es su binario exacto) |
| El test metamórfico B/dB/cB no prueba nada: en `main` ya coinciden | Opus (media) | **Aceptado:** eliminado; lo reemplaza la rejilla del test 1 |
| Costo: sin filtro, `3 B` pasa de 74 a 116 asignaciones y `-3000 dB` de 68 a 155; el filtro `α = 1` no sirve para `dB` | Opus, Fable (media) | **Aceptado:** filtro en float64 por cercanía a un entero, que vale con cualquier α (§2.1) y presupuesto en §2.4 |
| La medición «todos los \|n\| ≥ 23» es falsa; `[hp'_C] 2` no discrimina; `ld` ya es exacto | Fable (media) | **Aceptado:** §1 corregido con las cifras de Fable; tests sobre la rejilla, no sobre un caso |
| `IsInt64` antes de convertir el exponente | Opus, Fable (baja) | **Aceptado:** §2.1 |
| Un v no finito no debe entrar a la vía exacta | Opus (baja) | **Aceptado:** §2.2 |
| Prefijos en `[pH]` y `[hp'_*]` no llegan a la API | Fable (baja) | **Aceptado:** test 2 en `internal/special` |
| Falta la mutación «`ln` como base entera»; `math.Exp(3)` es el más cercano y `math.Pow(e, 3)` no | Fable (baja) | **Aceptado** la mutación y el test de `Np`. El cambio a `math.Exp` queda en #75: tampoco está garantizado correctamente redondeado |
| El test de no enteros es una guarda de regresión | Fable (baja) | **Aceptado:** rotulado así |
| `CanonicalRat(300, "B")` da `ErrNotRational` y la API float64 da ahora un valor exacto | Opus (baja) | **Sin cambio:** las escalas logarítmicas no tienen forma racional en general (README); fuera de alcance |
| El tope de 400 se justifica como tope de costo | Fable (baja) | **Aceptado:** §2.3 |
| Ucum-java: sin riesgo nuevo; log ya es `java-special-as-linear` | Fable | Se verifica en el test 5 |

## 7. Pasos 5 a 7

**Tests primero** (`logpow_test.go`, `internal/special/logpow_test.go`).
Antes de corregir, la rejilla falló donde `main` está mal: `B`, `dB`, `cB`,
`mB` y `[hp'_X]` en 504 de 616 exponentes; `[hp'_C]` en 254 de 308;
`[hp'_M]` en 85 de 205; `[hp'_Q]` en 46 de 131. `bit_s` ya pasaba. Las
fronteras del test 4 pasaban, salvo `300 B`.

**Implementación:** `logHandler.exactPower`, llamada desde
`Use.ToCanonical`. Dos ajustes respecto del plan, por costo:
- α se lee como float64 una vez, en `ToCanonical`.
- Sin prefijo (α = 1) y con expDivisor 1 o 2, el exponente en float64 ya es
  exacto y se salta el racional. La potencia se calcula con un solo
  `big.Int.Exp`, no con `Decimal.Pow`.

**Costo,** asignaciones por llamada a `Canonical` (benchmark de 20.000
iteraciones × 2–3 corridas, `main` contra la rama):

| Caso | `main` | Rama |
|---|---|---|
| `7.4 [pH]` (no entero) | 193 | 193 |
| `3 B` | 74 | 81 |
| `-300 B` | 80 | 90 |
| `-3000 dB` | 68 | 100 |

**Ucum-java:** `make compare-java` da el mismo reporte en la rama y en
`main`, salvo la fecha ([evidencia](evidence/002-2026-10-29-compare-java.md)).

**Mutaciones** K95–K104 en `scripts/mutations.py` (en la ronda 1 se
sumaron K105 y K106, y K101 pasó a ser equivalente; ver §8): todas mueren menos K103,
quitar el filtro barato, que es equivalente (solo costo) y se espera que
sobreviva. Para K102 (sin prefijo, saltarse el chequeo exacto ante un
exponente casi entero) se agregaron a la guarda de no enteros tres valores a
menos de 1e-9 de un entero. Suite completa: 142, sin resultados inesperados
([evidencia](evidence/009-2026-10-29-mutations.txt)).

## 8. Code review

### Ronda 1 (Opus y Fable, en paralelo, sin el razonamiento del plan)

**No converge:** dos hallazgos medios de Opus y uno de Fable.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| `Canonical` deja de ser monótona en `dB`: `-2040 dB` da `1e-204` y su vecino en float64 `9.999999999999996e-205` (154 de 616 múltiplos de 10). Los dos tienen el mismo exponente float64, −204; el exacto acepta uno y rechaza el otro, que cae en `math.Pow`. `Compare` los ordenaría al revés; `main` era consistente | Opus (media) | **Aceptado, cambia la regla del §2.1:** si el exponente **float64** (el de `math.Pow`) es entero, se usa la potencia exacta de ese entero: es el `math.Pow` de `main` bien redondeado, y dos valores con el mismo exponente dan lo mismo. El exponente exacto solo decide cuando la multiplicación en float64 no da el entero. Test 5 (monotonía con los vecinos en float64); K106 |
| El filtro de casi enteros nunca se ejercita: con `d`, `c`, `m`, `v·α` da exacto el entero; el ejemplo «2900 dB» del comentario era falso. Solo `n`, `f`, `a`, `z` lo necesitan | Opus (media) | **Aceptado:** la rejilla suma `nB`, `fB`, `aB`, `zB` (solo los v cuyo producto por α es exactamente n: `n·1e21` no siempre es un float64); la guarda de no enteros suma casos de `dB` casi enteros; comentario corregido |
| La guarda de NaN no tiene test: sin ella, NaN llega a `RatFromFloat`, que da nil, y `e.Mul` revienta | Fable (media), Opus (baja) | **Aceptado:** NaN en `B`, `dB` y `[pH]` en `TestRangeNonFinitePassesThrough`; K105. La guarda es ahora la comparación negada del tope, que descarta NaN y ±Inf |
| El tope de 400 es más ancho que lo que se usa: más allá de 310 el resultado no es normal y `mapScalar` lo rehace | Opus (baja) | **Aceptado:** tope 310 |
| `IsInt64` es código muerto por el tope | Opus, Fable (baja) | **Se mantiene,** con un tope también sobre el exponente exacto: la mutación K96 (sin α) hizo que `nB` pidiera 10^(10^11) y `big.Int.Exp` no terminara. El costo queda acotado por construcción y no por el razonamiento de que el exacto está cerca de f. K101 pasa a ser equivalente por eso |
| Costo: `alpha.Rat()` copia, `SetFloat64(expDivisor)` asigna, `Log10(base)` por llamada | Opus, Fable (baja) | **Sin cambio:** con la regla nueva el racional solo corre con prefijos n, f, a, z. `Log10(base)` sí corre en cada llamada de base entera, porque el tope va antes que `f != n` para descartar NaN; el caso no entero queda en 193 asignaciones, como `main`. Medido abajo |
| La tolerancia relativa cambiada por absoluta sobrevive | Fable (baja) | **Aceptado:** anotada junto a K103, equivalente |
| Docs: #75 falta en «Issues abiertos»; la frase de `0.3 kB` | Fable (baja) | **Aceptado;** y `0.3 kB` vuelve a ser `1e300` con la regla nueva |

**Costo con la regla nueva** (asignaciones por llamada; `main` en §7):
`7.4 [pH]` 193, `3 B` 81, `-300 B` 90, `-3000 dB` 78 (era 100 con la regla
de §2.1).

Suite completa después de la ronda 1: 144 mutaciones, sin resultados
inesperados; K101 y K103 son equivalentes y se espera que sobrevivan
([evidencia](evidence/009-2026-10-29-mutations.txt)).

### Ronda 2

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Sin defectos de corrección: monotonía medida en 27 unidades (B y sus prefijos de y a k, B[V], dB[mV], B[W], [pH], [hp'_*], bit_s), ±64 vecinos en float64 de cada exponente entero en [-320, 320], unos 1,9 millones de puntos, 0 inversiones. K101 y K103 son equivalentes | Opus | — |
| Donde el exponente float64 es entero pero el exacto no, el resultado puede quedar más lejos del valor exacto que en `main`: contra mpmath a 50 cifras, en 6.116 puntos, `zB` peor en 544 y mejor en 19, `pB` peor en 93, `uB` en 50. Los dos ya están a cientos de ulp; la diferencia entre ellos es de unos 4 | Opus (baja) | **Aceptado como costo de la monotonía,** documentado en el README («the float64 exponent… decides»): es el exponente que `main` ya pasaba a `math.Pow` |
| El README, §2 y §3 del plan describían la regla anterior; §7 no nombraba K101, K105, K106; §8 decía que `Log10` solo corre con exponente casi entero | Opus (baja) | **Aceptado:** corregidos |
| Sin defectos de código: exactitud en todos los prefijos de `B` (Y a y, Ki a Ti), `[hp'_*]` y `bit_s`, 0 fuera del más cercano; la vía racional solo con n, f, a, z; monotonía en unos 400.000 valores por unidad (25 prefijos × B, B[V], B[SPL], B[W], más dB[mV], [pH], hp, bit_s, Np), 0 inversiones; la salida de las mutaciones coincide línea a línea con la evidencia | Fable | — |
| El README decía a la vez que un subnormal es el valor correctamente redondeado y que un subnormal logarítmico va por `math.Pow` | Fable (baja) | **Aceptado:** el subnormal es un valor y no un error; correctamente redondeado en una escala lineal |
| §2, §5 y §6 del plan seguían con la versión aprobada sin decirlo; el costo de la regla de la ronda 1 no estaba escrito (hB, daB, zB, yB, pB, uB) | Fable (baja) | **Aceptado:** nota al comienzo del §2 (el resultado es el más cercano a base^f, no a base^e) y en §5 y §6 |

**Converge:** ninguno de los dos deja hallazgos de severidad media o mayor.
