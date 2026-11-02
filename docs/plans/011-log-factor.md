# 011 — Un solo redondeo con el factor de la unidad (#75, opción B)

**Estado:** paso 9 (cierre), con commit. Plan aprobado por el usuario el 2026-10-31, con `fix:` 4.4.3 y «Closes #75»; la ronda adversarial (§9) cambió §2 a §5.

## 1. Revisión previa

### El defecto

Desde 009, un exponente entero en una escala logarítmica de base entera
da la potencia exacta redondeada una vez (`Use.ToCanonical`). Después
`linearTimes` la multiplica por el factor de la unidad, y eso redondea
otra vez (`decimal.MulExact(x.v, factor)`, `internal/engine/range.go`). Con
factor distinto de 1, el resultado puede quedar a 1 ulp del float64 más
cercano a base^n·factor:

- `Canonical(1, "[pH]")` = `6.022140760000001e+25`; (1/10)·6.02214076e26
  redondeado una vez es `6.02214076e+25`.

En un producto son tres redondeos: la potencia, `x1.v·x2.v` y el factor
(`combineScalars`, vía directa).

Medido con una sonda contra `main` `a0b2d9f`: exponentes enteros con
intermedio y resultado normales y v = n·perStep exacto. Las unidades y la
semántica salen del XML (`loadLogUnits`); el esperado es
`nearest(base^n · factor)` en `big.Rat`.

| Operación | Puntos | Fuera del más cercano en `main` |
|---|---|---|
| `Canonical`, 17 unidades | 10.632 | 1.122, todos en unidades con factor ≠ 1 (147 a 172 por unidad: `B[SPL]`, `B[V]`, `B[uV]`, `B[10.nV]`, `B[W]`, `B[kW]`, `[pH]`) |
| `Multiply(·, w 1)`, w ∈ {3, 0.7, 1.1, 1e10, 7e-5} | unos 10.080 por w | 2.228 a 2.885 por w, también en unidades sin factor |
| `Convert` a `3.10*K.ref`, K ∈ {-2, 0, 5} | 10.087 por K | 1.788 a 2.102 |

En `B`, `dB`, `nB`, `fB`, `B[mV]`, las potencias homeopáticas y `bit_s`,
`Canonical` no cambia: su factor es 1.

### La sonda con la opción B aplicada

En una copia del repo (scratchpad, `optB`), con el cambio de §2:

- todos esos puntos dan el float64 más cercano;
- monotonía: con los 4 float64 vecinos de cada lado de cada punto, 0
  inversiones en las tres operaciones;
- la suite completa pasa salvo los dos tests que fijan la exclusión de la
  opción B (`TestLogScaleDocumentedBoundaries`,
  `TestNonNormalLeavesOtherPathsAlone`);
- `TestCompareJava`: las mismas cifras por clase; solo cambia el texto del
  ejemplo `canonical 1 [pH]`. Ucum-java trata `[pH]` como lineal (clase
  `java-special-as-linear`, plan 002), así que no sirve de referencia aquí.
- Asignaciones por llamada, `main` → copia: `7.4 [pH]` 193 → 195, `1 [pH]`
  193 → 190, `3 B` 81 → 81, `3 B[W]` 232 → 235, `60 B[SPL]` 252 → 251,
  `Multiply(2 [pH], 3 L)` 250 → 258.

Una sonda anterior, que comparaba con los vecinos de `main`, dio unas 60
inversiones por operación: era un artefacto. El vecino `-2010 + 1 ulp` en
`dB` tiene el mismo exponente float64 entero (`f = -201`) y también cambia.

### El intercambio (issue #75)

Un `Value` se lee como su binario exacto. Hoy `Canonical(1, "[pH]")` y
`Canonical(0.1, "mol/L")` coinciden porque los dos redondean
float64(0.1)·factor. Medido con `fhir.Comparator.Compare`:

| | `main` | Opción B |
|---|---|---|
| `Compare(1 [pH], Value 0.1 mol/L)` | 0 | -1 |
| `Compare(1 [pH], Exact 0.1 mol/L)` | 1 | 0 |

Ninguna vía float64 deja las dos en 0. La opción B da la correcta para el
valor que cada operando representa: `Exact 1/10` es igual a 10^-1, y el
binario de 0.1 es mayor.

### `gofhir/server`

No tiene código de unidades propio (009, §1). Nada que tomar.

### Especificación

UCUM §22 (r = f(m/u)/α: la función y después la unidad) y las definiciones
de `ucum-essence.xml`. La regla de rango de #59 (plan 003, §2): un
resultado finito que cabe es un valor, redondeado una vez. Sin bases ni
factores nuevos: los del XML y la tabla de funciones de `special.go`.

## 2. Cambio

Donde la vía float64 materializa una escala logarítmica con intermedio
normal y **exponente entero** (la regla de 009: decide el exponente float64,
o el exacto en los prefijos n, f, a, z), el resultado es
`base^n · (resto) · factor` en `big.Rat`, redondeado una vez:

1. **`linearTimes`, vía directa** (`Canonical`, `Convert` a una unidad
   lineal o a una escala sin forma log10): si `ToCanonicalExact(src)` da la
   potencia y el factor no es 1, `roundRat(potencia · factor)`. Con factor
   1 el resultado ya es la potencia redondeada una vez, y no se paga
   aritmética racional.
2. **`combineScalars`, vía directa** (`Multiply`, `Divide`): siempre
   `roundRat(r1 ⊙ r2 · factor)`, cada operando con su potencia exacta si la
   tiene y si no con el binario exacto de su float64 (como `rat()` en el
   tramo no normal). Cambió en el paso 6 (§10): la versión aprobada lo hacía
   solo si algún operando tenía potencia exacta. Acá
   no hay atajo con factor 1: el producto por el otro operando redondea
   igual (`Multiply(-1 B, 3 1)` debe dar `nearest(3/10)`). Un divisor cero no
   llega a la vía directa (`x1.v/x2.v` es infinito o NaN) y sigue el camino
   de hoy (`ErrDivisionByZero`).
3. **`toDestination` hacia una escala con forma log10**, con `x` normal: el
   intermedio m es `roundRat(potencia · factor)` en vez de
   `MulExact(x.v, factor)`, y después la f del destino, como hoy. Con el XML
   incluido no se llega (entre dos escalas logarítmicas va
   `special.LogToLog`, y ninguna referencia logarítmica de base entera tiene
   la dimensión de la de `sqrt`), pero sí con `NewFromReader`: con
   `sqrt` definida sobre `3 1`, `Convert(-303, "B", "[m/s2/Hz^(1/2)]")` da
   hoy `1.8257418583505536e-152` y por la vía de 1 daría
   `1.8257418583505538e-152` (ronda adversarial, revisor a). Cubrirla deja la
   misma regla en todas las vías.

Lo demás queda igual que en `main`:

- exponentes no enteros (`7.4 [pH]`: el resultado es irracional), `Np` y
  `cNp`, las escalas sin base entera, en `Canonical` y `Convert`. En un
  producto su float64 entra como binario exacto y se redondea una vez
  (§10);
- el tramo no normal (010);
- la API exacta: `CanonicalRat` sigue dando `ErrNotRational` en toda escala
  logarítmica.

**Exponente.** El mismo que 009, así que dos valores con el mismo exponente
float64 dan el mismo resultado; la sonda no encontró inversiones.

**Rango.** `roundRat` da `ErrOutOfRange` solo si el resultado redondeado es
±Inf, o cero desde un racional distinto de cero; un subnormal es un valor.
Hoy el chequeo lo hace `linearTimes` sobre el resultado de `MulExact`, con
la misma regla. Como el racional cambia, un resultado justo en el borde
puede moverse legítimamente de un lado al otro: el test 7 lo fija con el
esperado exacto, no con el de `main`.

## 3. Criterio de salida

- Todo exponente entero con intermedio y resultado normales da
  `nearest(base^n · factor)` en `Canonical`, en `Convert` y en
  `Multiply`/`Divide` por un operando lineal, en todas las unidades de base
  entera del XML.
- Monotonía: 0 inversiones con los vecinos float64 de cada punto, con
  factor 1 y distinto de 1.
- Exponentes no enteros, `Np`, el tramo no normal y las escalas no
  logarítmicas: idénticos bit a bit a `main` en `Canonical` y `Convert`; en
  un producto, `nearest(bin(math.Pow) ⊙ otro · factor)` (§10).
- `make check`, `make check-packages`, `make compare-java` (mismas cifras
  por clase), mutaciones completas.
- Costo, con un benchmark versionado (`internal/engine/benchmark_test.go`,
  `-benchmem`, `-count` 10) y `testing.AllocsPerRun` sobre la API pública:
  el caso no entero (`7.4 [pH]`) con las mismas asignaciones que `main` (la
  sonda midió +2: se corrige); el caso entero, no más asignaciones que
  `main`. En los productos, el costo de §10 (unas +2 a +9 asignaciones),
  aceptado por el usuario. Go, sistema y arquitectura quedan en la
  evidencia.

## 4. Tests (primero)

Los esperados salen del XML y de `big.Rat` (UCUM §22), no del código. El
factor de la referencia sale de `CanonicalRat`, la API exacta, que validan
la suite oficial de HL7 y la comparación con Ucum-java; además, casos a
mano con el factor escrito desde el XML (test 10).

Las unidades se generan del XML: cada átomo cuya función tiene base entera
(`logFunctions`, la semántica de UCUM §46-47), sin prefijo y con cada
prefijo del XML, filtrando las combinaciones que la gramática no acepta.
Así entran `aB`, `zB` y los prefijos sobre unidades con factor (`kB[W]`…).
Para que la corrida sea corta, los exponentes se recorren con un paso que
deja unos 20.000 puntos por operación, más todos los |n| ≤ 30 y los bordes.

1. **Rejilla de `Canonical`** (reemplaza la guarda de doble redondeo de
   `TestNonNormalLeavesOtherPathsAlone` y el `1 [pH]` de
   `TestLogScaleDocumentedBoundaries`): todo n con intermedio y resultado
   normales.
2. **Rejilla de `Convert`** a `3.10*K.ref` con K ∈ {-2, 0, 5} y a una
   unidad de la misma dimensión con otro factor (`[pH]` → `mmol/L`,
   `B[SPL]` → `kPa`, `B[W]` → `mW`).
3. **Rejilla de `Multiply` y `Divide`** por w ∈ {3, 0.7, 1.1, 1e10, 7e-5,
   -3, un subnormal} (`Pair{w, "1"}` y con unidad), en los dos órdenes, y
   `1 / (n [pH])`; incluye unidades con factor 1 (`Multiply(-1 B, 3 1)` =
   `nearest(3/10)`).
4. **Dos operandos logarítmicos**: `Multiply(n [pH], m B[W])` y `Divide`,
   con los dos exponentes enteros, con uno solo y con ninguno (este último,
   `nearest(bin(math.Pow)·bin(math.Pow)·factor)`, §10).
5. **Monotonía** en la vía directa: ±4 vecinos float64 de cada punto de 1-4,
   con factor 1 y distinto de 1, en el sentido que corresponde (un w
   negativo o la división lo invierten), y en la frontera normal/subnormal
   del resultado.
6. **No cambia** (bit a bit contra la fórmula de `main`,
   `round(round(math.Pow)·F)` con la potencia de `math.Pow`): exponentes no
   enteros en todas las unidades, `Np` y `cNp`, `Cel`, `[p'diop]`.
7. **Rango y errores**, con esperados exactos: a cada lado de los umbrales
   efectivos de redondeo (el punto medio entre `MaxFloat64` y 2^1024, y la
   mitad del menor subnormal), con empates; un subnormal aceptado como
   valor; resultados negativos; `ErrDivisionByZero` con divisor +0 y -0 y el
   dividendo con potencia exacta; los topes de costo (310/311 y 5000/5001
   en log10); NaN y ±Inf siguen como hoy.
8. **`fhir`**: `Compare(1 [pH], Exact 0.1 mol/L)` = 0;
   `Compare(1 [pH], Value 0.1 mol/L)` = -1; `CanonicalKey(1 [pH])` igual al de
   `Exact 0.1 mol/L`; y lo mismo con `2 [pH]` y `0.01`.
9. **Definiciones propias** (`NewFromReader`): el caso de §2.3, con el
   esperado `sqrt(nearest(10^n / 3))` en `big.Rat`/`big.Float`.
10. **A mano**, con el factor tomado del texto del XML (`mol` =
    6.02214076e23, `l` = dm3, `B[SPL]` = 2e-5 Pa): `Canonical(1, "[pH]")` =
    `6.02214076e+25`, `Canonical(2, "[pH]")` = `6.02214076e+24`, y uno de
    `B[SPL]` y `B[W]`.
11. **README**: el párrafo de las escalas logarítmicas cambia: la frase de
    `1 [pH]` (+1 contra `Exact`, 0 contra `Value`) pasa a describir el
    intercambio, y se avisa que las claves de `CanonicalKey` guardadas para
    esas unidades cambian en 1 ulp y hay que regenerarlas.

## 5. Mutaciones previstas

- Quitar la vía exacta de `linearTimes` (tests 1, 2, 5, 8, 10).
- Quitar la de `toDestination` (9).
- Alterar el factor de una unidad en el XML que usa el motor, sin tocar el
  de los tests a mano (10).
- Quitar la de `combineScalars` (3, 4).
- Usar la potencia exacta también con exponente no entero (`rat()` en vez
  de `ToCanonicalExact`; 6).
- Factor 1 por factor cualquiera en el atajo (1, 2); atajo de factor 1
  también en `combineScalars` (3).
- En `combineScalars`, el otro operando redondeado distinto (por ejemplo
  `SetFloat64(float32(x))`; 3).
- `Quo` por `Mul` en la división (3, 4).
- Divisor cero por la vía exacta (7).
- Las mutaciones K1-K129 completas, como en cada ronda.

## 6. Fuera de alcance

- Exponentes no enteros correctamente redondeados (exp/log de precisión
  arbitraria).
- `CanonicalRat` exacto para exponentes enteros (cambiaría el contrato de
  `ErrNotRational` en las escalas logarítmicas).
- Los dos defectos anotados en 010, §10.
- Un arnés aparte para las cifras de §1: las mide la rejilla de los tests
  1-5, que queda en el repo; las de §1 son de la sonda y no se citan como
  evidencia final.

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.3, «Closes #75». Es una corrección numérica: el resultado pasa
a ser el float64 más cercano al valor que define UCUM §22. Cambia lo
observable: `Compare(1 [pH], Value 0.1 mol/L)` de 0 a -1, y los valores de
`CanonicalKey` de unas 1.100 de 10.600 potencias enteras en las unidades
con factor, en 1 ulp (quien guarde claves las regenera). Queda escrito en
el README y en el mensaje del commit, y lo aprueba el usuario (paso 4).

#75 propone A y B y deja fuera los exponentes no enteros en las dos; 009
hizo A, 010 el tramo no normal y 011 hace B, así que se cierra.

## 9. Ronda adversarial del plan (2026-10-31)

Dos revisores Codex independientes, en copias separadas. El b no pudo
ejecutar Go en su sandbox; el a sí.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| `toDestination` hacia escala log10 queda fuera; con `NewFromReader` se llega (`-303 B` → `sqrt(3 1)` difiere en 1 ulp) | a (media), b (baja) | **Aceptado:** verificado con su sonda; §2.3 lo cubre, test 9 |
| `loadLogUnits` es una lista fija de 17: faltan `aB`, `zB` y prefijos sobre unidades con factor | a, b (media) | **Aceptado:** unidades generadas del XML (§4) |
| La monotonía no cubre dos operandos logarítmicos, factor 1, la frontera subnormal ni el signo | a, b (media) | **Aceptado:** test 5 |
| El test 7 pedía error en puntos que redondean a un valor (`MaxFloat64+1`, ¾ del menor subnormal) | a, b (media) | **Aceptado:** umbrales efectivos y empates; §2 «Rango» reescrito |
| El factor del esperado sale de `CanonicalRat`, del mismo motor | a (media) | **Aceptado en parte:** la API exacta ya la validan la suite de HL7 y Ucum-java, y el tema del incremento es el redondeo, no el factor; se agregan casos a mano con el factor del XML (test 10) y una mutación del factor |
| §1 mide +2 asignaciones en `7.4 [pH]` y §3 pide ninguna | a, b (media) | **Aceptado:** §3 exige igual a `main` en el caso no entero y no más en el entero, con benchmark versionado |
| Cifras de la sonda sin comando en el repo | a (baja), b (media) | **Aceptado:** la rejilla de los tests es la medición; §6 |
| `Compare`/`CanonicalKey` cambian juntos; README documenta lo viejo; justificar `fix:` y cerrar #75 | a (media) | **Aceptado:** §8 y test 11; decide el usuario |
| `Multiply(-1 B, 3 1)`: el camino exacto debe aplicar con factor 1 en productos | b (media) | **Ya en el plan** (§2.2 no tiene atajo); explícito y con test 3 |
| El cambio debe quedarse en `engine` | b | Coincide con el plan |
| Puntos: 10.649 contando n = 0 | a (baja) | La sonda los filtraba (resultado 1, sin redondeo); sin cambio |

## 10. Cambio en el paso 6: productos siempre exactos (2026-10-31)

**El hallazgo.** Al implementar, el test de monotonía con n = 0 falló en
`Divide(v [pH], 0.7)`: en v = 0 el exponente es entero y el resultado se
redondeaba una vez, pero sus vecinos ±5e-324 tienen exponente no entero,
`math.Pow` da exactamente 1 y el producto se redondeaba dos veces (primero
`x1.v / x2.v`, después el factor). Difieren en 1 ulp sin orden. La sonda del
plan había filtrado n = 0. `Canonical` y `Convert` no tienen el problema:
su vía directa ya redondea una vez (`MulExact`).

**La decisión** (del usuario, entre esto y dejar la inversión documentada):
la vía directa de `Multiply` y `Divide` multiplica siempre los racionales de
los operandos y redondea una vez, como el tramo no normal desde 010. Con
n = 0 incluido, el test de monotonía da 0 inversiones.

**Lo que cambia además.** Un producto con exponente no entero, `Np` o
`cNp` puede moverse 1 ulp, hacia el valor de sus binarios:
`Multiply(7.4 [pH], 3 L)` pasa de `7.192372255915457e+16` a
`7.192372255915456e+16`. En dos operandos sin exponente entero, 286 de
1.655 puntos del test 4 cambian.

**Costo** (asignaciones por llamada, `testing.AllocsPerRun`, `main` → plan
aprobado → esta versión): `7.4 [pH] · 3 L` 250 → 246 → 254;
`2 [pH] · 3 L` 250 → 252 → 252; `3.5 B[W] / 0.7` 245 → 241 → 254;
`0.5 Np · 3` 92 → 88 → 95; `20 Cel · 2 m` 124 en las tres (va por la vía
racional).

## 11. Mutaciones (paso 7)

`make mutations`: K130-K142 son las nuevas (§5). Ajustes a las de antes y a
las del plan:

- **K110** (la opción B como defecto) se quitó: ahora es el comportamiento.
- **K10** (la vía directa también con un producto no normal) pasa a
  equivalente: la vía directa es exacta (`exactDirect`), así que un producto
  no normal da el mismo valor redondeado una vez.
- **«Divisor cero por la vía exacta»** (§5) no se puede expresar: el
  chequeo era inalcanzable (un divisor cero da `x1.v/x2.v` infinito o NaN,
  que no entra en la vía directa). Se quitó del código; el test 7 sigue
  fijando `ErrDivisionByZero`.
- **K135** («`rat()` en vez de `ToCanonicalExact`», §5) es equivalente:
  el binario de un float64 es lo que `MulExact` y el producto ya usan.

## 12. Code review

### Ronda 1 (2026-10-31)

Dos revisores Codex. El b no pudo escribir en el scratchpad (el sandbox de
Codex solo escribe dentro del repo); se relanzó con la copia en `.cache/`
(ignorada por git). Antes de la ronda, `make mutations` completo:
[evidencia](evidence/011-2026-10-31-mutations.txt).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El test de entradas no finitas solo exige `err == nil`: una mutación que devuelve 0 sobrevive | a (media) | **Confirmado** con su mutación; el test compara el valor de IEEE 754 (signo del infinito, NaN, cero). K143 |
| La monotonía salta todo error y NaN pasa la comparación: dos mutaciones de `exactDirect` sobreviven | a (media) | **Confirmado**; solo se salta `ErrOutOfRange`, otro error o un no finito falla, y se exige comparar al menos el 90 % de los pasos. K144, K145 |
| El README promete un solo redondeo sin el tope de costo (10^±5000): `Divide(10002 B[SPL], 4991 B)` queda a 1 ulp, como en `main` | b (baja) | **Aceptado:** el README nombra el tope y el ejemplo |
| Sin defectos numéricos: 370.132 comparaciones contra `big.Rat`, 640.614 pasos de monotonía, 506 resultados de rutas fuera del cambio idénticos a `main`; tres mutaciones propias detectadas | b | — |

**Fuera de alcance:** la biblioteca no conserva el signo de un cero
(`Divide(2 m, -Inf)` es +0, también en `main`); el test de no finitos no lo
compara.

### Costo (paso 7)

Asignaciones por llamada, deterministas (`-benchmem`), `main` → rama:
`7.4 [pH]` 193 → 189, `1 [pH]` 193 → 184, `3 B` 81 → 77, `3 B[W]` 232 → 229,
`60 B[SPL]` 252 → 245, `Multiply(2 [pH], 3 L)` 250 → 252 (§10). El atajo
de `decimal.Decimal.Float64` para enteros chicos explica la baja. Los
tiempos salieron con mucho ruido en esta máquina (medianas de 6 rondas
intercaladas): diferencias dentro del ruido salvo, quizá, `60 B[SPL]`
(10,3 → 11,9 µs) y el producto (11,9 → 17,5 µs de mediana, con mínimos
iguales). [Evidencia](evidence/011-2026-10-31-bench.txt).

### Ronda 2 (2026-10-31)

Esperó al límite de uso de Codex. Antes de la ronda, `make mutations`
completo: 180 de 180 con el resultado esperado. El diff de la ronda es el de
los cambios de la ronda 1.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Los dos hallazgos de la ronda 1 | a | **Resueltos:** K143-K145 mueren |
| El 90 % de pasos deja pasar un `ErrOutOfRange` espurio en un tramo chico (B con exponente 16-70: 392 de 432 pasos, mínimo 388) | a (media) | **Confirmado** con su mutación; `ErrOutOfRange` solo se acepta junto al borde (\|log10\| del canónico ≥ 300) y todo punto lejos del borde compara sus 8 pasos. K146 |
| README, tests de no finitos y monotonía | b | Sin hallazgos; el ejemplo del README medido: `2e+08` y `2.0000000000000003e+08` |

### Ronda 3 (2026-10-31): converge

Antes de la ronda, `make mutations` completo: 181 de 181 con el resultado
esperado ([evidencia](evidence/011-2026-10-31-mutations.txt)).

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Ninguno: K146 y cinco variantes (un solo vecino, solo `Divide`, multiplicador negativo, entrada negativa, exponentes con \|log10\| 250-299) mueren; la regla del borde es correcta (con \|log10 c\| < 300 los resultados quedan entre unos 2.8e-302 y 3e300) | a | — |
| Ninguno: la suite pasa en arm64 y amd64, con los mismos 361 puntos interiores; los canónicos subnormales caen en el borde en las dos | b | — |

## 13. Cierre (2026-10-31)

- `make check`, `make check-packages` y `make compare-java` en verde. El
  reporte contra Ucum-java tiene las mismas cifras por clase que el de 010;
  solo cambia el ejemplo `canonical 1 [pH]`, ahora `6.02214076e+25`
  ([reporte](evidence/002-2026-10-31-compare-java.md)).
- Mutaciones: 181 de 181 con el resultado esperado
  ([evidencia](evidence/011-2026-10-31-mutations.txt)).
- Costo: [benchmarks](evidence/011-2026-10-31-bench.txt) y §12, «Costo».
- Pendiente: commit y PR cuando el usuario lo pida (`fix:` → 4.4.3,
  «Closes #75»; agregar `factor_test.go`, `fhir/logfactor_test.go`, el plan y
  la evidencia).
- Fuera de alcance, sin issue: los dos defectos de 010, §10, y el signo del
  cero (§12, ronda 1).
