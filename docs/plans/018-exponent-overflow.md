# 018 — el exponente canónico desborda `int` sin error

**Estado:** paso 9 (cierre). Code review convergió en la ronda 2; `make check`, `make check-packages` y `make compare-java` en verde (2026-11-05; el informe de compare-java es idéntico al de 2026-11-03). Falta el commit, que aprueba el usuario.

## 1. Revisión previa

**La especificación.** UCUM no pone tope a los exponentes («The exponent is an
integer number», §9 ■1) ni a la profundidad de las definiciones; un exponente
canónico es la suma y el producto de los exponentes de la cadena (§18 ■3-■4:
u/v = u·v⁻¹, uⁿ = Π u). Pero no exige representarlos todos: «The set D of
dimensions is infinite in principle, but only a finite subset of dimensions
are used in practice. Thus, implementations of The Unified Code for Units of
Measure need not be able to represent the infinite set of dimensions» (§19 ■1;
y tras §20: «Most practically used units contain exponents of -4 to +4»). Una
dimensión que la implementación no representa es, entonces, un error; dar
otra dimensión en su lugar no lo es.
La librería acota la entrada del llamador (`MaxExponent` = ±1000 por literal,
`MaxCodeLength`, § «Bounds» del README), pero no lo que sale de componer
definiciones de un XML propio: allí los exponentes se multiplican nivel a
nivel.

**El defecto.** Dos, con la misma causa: lo que sale de componer
definiciones de un XML propio no tiene cota, porque cada nivel multiplica.
Medido sobre `main` (v4.4.9) por la ronda adversarial (§9):

| Definiciones (`[e_i]` en `[e_{i+1}]`) | Resultado |
|---|---|
| `[e_{i+1}]2`, valor 1, 62 niveles, hoja 1 m | `m4611686018427387904` (2^62) |
| ídem, 63 niveles | `m-9223372036854775808` (signo dado vuelta) |
| ídem, 64 niveles | `1`, adimensional; `IsComparable(…, "1")` = **true** |
| `[e_{i+1}].[e_{i+1}]` y `/[e_{i+1}]2`, 64 niveles | `1`, adimensional |
| `[e_{i+1}]2`, hoja 10 m, 26 niveles | 14.7 s y 765 MB |
| `[e_{i+1}]2`, valor 2, 30 niveles | 1.6 s y 3.3 GB |
| `[e_{i+1}]2`, hoja `km`, 62 niveles | sin terminar en 60 s (1000^(2^62)) |
| `[e_{i+1}]2`, valor 2, hoja `1` (adimensional), 30 niveles | 1.9 s y 3.5 GB |
| `Multiply([s_0], [s_0])` con 2^62 cada uno | `[s_0]2` callado; con unidad especial, `1` |

El desborde da un resultado equivocado sin error (también en `CanonicalRat`,
`Convert`, `ConversionFactor`, `Multiply`/`Divide` y `fhir`), pero solo se
alcanza si todos los valores de la cadena son exactamente 1: con cualquier
otro valor o prefijo, `decimal.Pow` (exacto, sin tope) cuelga o agota la
memoria antes. Un tope sobre el exponente no lo evita: una cadena adimensional
no tiene `Units` y cuesta lo mismo, y los productos `3 [d].[d]` (017 §6)
doblan los bits sin exponente alguno.

Sitios: el exponente canónico se calcula en `raiseCanonical`
(`Exponent *= n`) y en `mergeUnitLists` (`+= ru.Exponent * sign`, y
`ru.Exponent * sign` al agregar), que usan `multiplyCanonicals`,
`divideCanonicals` y `combine` (`convert.go`, Multiply/Divide). El valor
crece en `powValue` y en cada `Mul`/`Div` de la canonicalización. En
`productResult` (`addSymbol`) los exponentes son del código del llamador,
acotados por `MaxExponent` y `MaxCodeLength`, y el código compuesto se vuelve
a parsear: no desbordan.

**Referencias.** Ucum-java usa `int` y no carga XML propios; `gofhir/server`
usa esta librería.

## 2. Cambio

> Reescrito dos veces (§9). Alcance final: solo el exponente.

1. Helpers `mulExponent` y `addExponent` (y el cambio de signo: el opuesto de
   `math.MinInt` no cabe) que dan `ucumerr.ErrExponentTooLarge` cuando el
   resultado no cabe en `int`.
2. `raiseCanonical` multiplica el exponente con el helper **antes** de
   `powValue`, para no pagar la potencia de un exponente perdido.
3. `mergeUnitLists` suma y cambia de signo con los helpers y pasa a devolver
   error; lo propagan `multiplyCanonicals`, `divideCanonicals` y `combine`
   (`convert.go`, Multiply/Divide). `product_test.go`, que la llama, se ajusta.
4. El error es el sentinel que ya existe (`errors.Is(err,
   ucum.ErrExponentTooLarge)`), con el código: §19 ■1 permite no representar
   una dimensión, no dar otra.

Queda igual: todo resultado cuyo exponente canónico cabe en `int`, y cuyos
exponentes intermedios también caben: `[ovn]/[ovn]` con un exponente
`math.MinInt` vale la unidad, pero el cociente niega `math.MinInt` antes de
sumar, así que da `ErrExponentTooLarge` (§19 ■1 lo ampara: la implementación
no representa esa dimensión intermedia). El valor
puede seguir creciendo sin cota y costar segundos u horas: es 019.

## 3. Criterio de salida

- Ningún exponente canónico desborda en silencio: las cadenas de §1 con valor
  1 (y sus variantes por productos, cocientes, exponentes negativos hasta
  `math.MinInt` y `Multiply`/`Divide` de dos operandos válidos) dan
  `ErrExponentTooLarge` en `Canonical`, `CanonicalRat`, `Convert`,
  `ConversionFactor`, `IsComparable`, `ValidateInProperty`, `Multiply`,
  `Divide` y `fhir`, en los dos servicios.
- El XML oficial: idéntico a `main` (barrido).
- Sin costo en las rutas calientes (benchmarks, asignaciones).
- `make check`, `make check-packages`, `make compare-java` y las mutaciones
  completas.

## 4. Tests (primero)

En `expoverflow_test.go`, con XML propios de valor 1 (para que el valor no
explote antes, §9); esperados por aritmética.

1. `[e_i]` = `[e_{i+1}]2`, hoja 1 m: 62 niveles dan `m4611686018427387904`
   (2^62); 63 y 64, `ErrExponentTooLarge`. Igual con `[e].[e]` (63, 64),
   `/[e]2` y `[e]-2`, y un exponente negativo que llega a `math.MinInt` y se
   niega en un cociente.
2. `Multiply`/`Divide` de dos operandos válidos (2^62 cada uno) cuyo exponente
   suma de más: error, no `[s_0]2` ni `1`.
3. `IsComparable("[e_0]", "1")` con 64 niveles: error, no `true`.
4. `fhir` (`CanonicalKey`, `Compare`, `ConvertDecimal`) y el servicio
   insensible con las mismas cadenas.
5. El XML oficial, igual que antes en `Canonical` (barrido de unidades,
   prefijos y exponentes).

## 5. Mutaciones previstas

- Sin el chequeo en `raiseCanonical` (1).
- Sin el chequeo en la suma de `mergeUnitLists` (1, 2) y en el cambio de
  signo (1, `MinInt`).
- `combine` que descarta el error de `mergeUnitLists` (2).
- Mutaciones completas.

## 6. Fuera de alcance: 019

- **El costo de los racionales exactos** (§9, ronda 2): con el XML oficial,
  `'1000` (5 bytes) cuesta 53 ms y `'1000.'1000.'1000.'1000.'1000` (29 bytes)
  1.9 s; 125 bytes, 73 s; 1024 bytes, horas. `[pi]` tiene 64 decimales y
  las unidades que lo usan (`'`, `''`, `Oe`, `deg`, `Gb`, `[mu_0]`) valen
  unos 500 bits con denominador; el máximo común divisor de cada `big.Rat`
  domina. Un tope de bits no acota el tiempo (un producto bajo 2^26 cuesta
  81 s) y uno bajo rechazaría códigos válidos.
- Con XML propio, el valor crece sin cota (`3 [d].[d]`, hoja 10 m, `km`):
  el mismo frente.
- `decimal.FromString` hace `big.Int.Exp(10, e)` sin tope al cargar: un XML
  con `value="1e1000000000"` agota la memoria en `NewFromReader`.
- Una cadena que termina en un ciclo es cuadrática en `ValidateInProperty`
  (017 §6).

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.10. Un exponente que desborda pasa de un resultado equivocado a
`ErrExponentTooLarge`.

## 9. Ronda adversarial del plan (2026-11-05)

Dos Opus independientes, con sondas sobre `main` y un prototipo del chequeo de
desborde: el prototipo cierra todos los resultados equivocados (18 cadenas,
15 entradas, los dos servicios) y deja idénticos a `main` 138 142 casos del
XML oficial.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El valor explota antes que el exponente: con un valor ≠ 1 o un prefijo, `Pow` cuelga (hoja 10 m, 26 niveles: 14.7 s, 765 MB; hoja `km`, 62 niveles: sin terminar), y un tope de exponente no lo evita (cadena adimensional: 3.5 GB) | a, b (alta) | **Aceptado:** el usuario eligió (2026-11-05) un tope de bits, `MaxCanonicalBits` (§2.2) |
| El chequeo de exponente va antes de `Pow` | a (media) | **Aceptado:** §2.1 |
| `combine` (`convert.go`) también llama a `mergeUnitLists`: `Multiply([s_0], [s_0])` con 2^62 da `[s_0]2` callado | b (media) | **Aceptado:** §2.1, test 2 |
| La tabla de §1 no se reproducía (62 niveles dan 2^62, no 2^61) | b (baja) | **Corregido** con sus mediciones |
| El mensaje crece con la profundidad (`wrapExpand`: 5412 bytes a 300 niveles) | b (baja) | **Aceptado:** tipo propio (§2.4), test 7 |
| `product_test.go` llama a `mergeUnitLists` y cambia con su firma | b (baja) | **Aceptado:** se ajusta |
| `addSymbol` no desborda; no hay más sitios; la referencia de una especial pasa por los mismos helpers; el `MinInt` del plan es el sitio del cambio de signo | a (confirmado) | Sin cambio |

### Ronda 2 (2026-11-05), sobre el tope de bits

Dos Opus; (b) prototipó todo el §2 de entonces (tope 2^26, estimación antes
de `Pow`, chequeo después de cada `Mul`/`Div`): el XML oficial queda idéntico
a `main` en 974 001 líneas y el exponente se corrige, pero:

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Con el XML oficial un código corto del llamador ya cuelga: `'1000` ×5 (29 bytes) 1.9 s, ×10 10.7 s, 125 bytes 73 s (verificado: 5 bytes 53 ms, 29 bytes 1.9 s); `[pi]` y sus derivadas valen ~500 bits con denominador | a, b (alta) | **019** |
| El tope no acota el tiempo: el costo es el máximo común divisor, no el tamaño (un `Mul` bajo 2^26 con denominador: 81 s; `3 [d].[d]` hasta el error: 8.8 s); y un código válido de 1024 bytes pasa de 2^26 | a, b (alta) | **019**; el tope sale de 018 |
| bits·\|n\| desborda con \|n\| ≈ 2^62 y la estimación es cota superior | a (media), b (baja) | Sale con el tope (019) |
| `decimal.FromString` sin tope al cargar | a (media) | **019** (§6) |
| El chequeo de bits copia el `Rat`: +4 a 12 asignaciones | b (media) | Sale con el tope |
| `dimensionlessCanonical` y `prefixVal·unitValue·can` sin chequeo | b (baja) | Sale con el tope |

Tras dos rondas sin converger en el tope, el usuario eligió (2026-11-05)
separar: 018 solo con el exponente, ya probado por las dos rondas; el costo,
019.

## 10. Code review

### Ronda 1 (2026-11-05)

Dos Opus sobre copias del repo. (a) comparó `mulExponent` y `addExponent`
contra `math/big` en 3.36 millones de pares (bordes, la grilla -300..300 y
pares al azar): 0 diferencias. (b) comparó con la copia sin el cambio: 805 746
líneas del XML oficial (los dos servicios, unidades, prefijos, exponentes
-4..4, productos y cocientes de pares, `Multiply`/`Divide`, `CanonicalRat`,
`CanonicalKey`), byte a byte idénticas con sus errores; 25 códigos con
desborde por 19 entradas públicas (con especiales cuya referencia desborda y
el caché frío y caliente) dan todos `ErrExponentTooLarge`; asignaciones
iguales en los 15 benchmarks, tiempo dentro del ruido.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El mensaje crece con la profundidad (5412 bytes a 300 niveles): §9 lo daba por aceptado con un tipo propio que el §2 final ya no tenía | a (media) | **Confirmado:** `exponentError`, que `wrapExpand` no envuelve; test de largo a 300 niveles; K220 |
| `combine` nombra solo un operando en el error | a (baja) | **Corregido:** el mensaje nombra la operación |
| El godoc de `ErrExponentTooLarge` no menciona el desborde canónico | a (baja) | **Corregido** en los dos archivos |
| Guardas redundantes en `mulExponent` (`a == 0`, `b == MinInt && a == -1`): mutantes equivalentes | a, b (baja) | **Corregido:** quedan `b == 0` y `a == MinInt && b == -1` |
| La guarda `b == 0` no la cubre ningún test de 018 (`"N0"` divide por cero) | b (baja) | **Corregido:** `[ovs_2]0` = 1; K221 |
| Los casos válidos de `Multiply`/`Divide` no afirmaban el resultado | a (baja) | **Corregido:** se afirma la forma canónica |
| El barrido del test compara el código nuevo consigo mismo | a, b (nota) | El criterio «idéntico a `main`» queda respaldado por el barrido externo de (b), arriba |

La corrida completa de mutaciones previa a la ronda se cortó por una señal
externa a las 84 líneas, todas `ok`. Repetida antes de la ronda 2, después de
las correcciones: 257 líneas, todas `ok`, K216-K221 incluidas
([evidencia](evidence/018-2026-11-05-mutations.txt); 45 min, 305 MB de RSS).

### Ronda 2 (2026-11-05)

Dos Opus nuevos con los hallazgos de la ronda 1, el diff de las correcciones y
el diff completo.
(a) repitió la comparación con `math/big` (5 625 pares de borde y 2 millones
al azar: 0 diferencias) y siguió `errors.Is` por todas las entradas públicas;
(b) comparó con la copia sin el cambio en 859 008 líneas, byte a byte
idénticas, probó cadenas de 64, 300 y 1000 niveles (mensaje de 104 bytes como
máximo) y midió asignaciones iguales en 11 benchmarks, tiempo dentro del
ruido. Ninguno dejó hallazgos de severidad media o mayor: **converge**.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El mensaje de `combine` no tiene test: dos mutantes sobreviven | a (baja) | **Corregido:** test de «times» y «divided by»; K222 |
| El error de una cadena profunda no nombra ninguna unidad | a, b (baja) | **Corregido:** `exponentError` guarda la unidad que anota el primer `wrapExpand` (`[ovl_237]` a 300 niveles, por aritmética); K223 |
| El godoc de `mulExponent` y `addExponent` dice que devuelven el sentinel | a (baja) | **Corregido** |
| El error de `Multiply`/`Divide` empieza con `invalid UCUM code "[ovs_2]"` aunque el operando es válido | a (nota), b (baja) | **Se mantiene:** es la forma de los errores de rango de `combine` (`Validation(v1.Code, combineError(…))`); cambiarla sola para el exponente los haría inconsistentes. Si se cambia, es para todos, en otro incremento |

Las correcciones de esta ronda son de mensaje; K219-K223 se corrieron una por
una después (todas muertas). La corrida completa es la de antes de la ronda.
