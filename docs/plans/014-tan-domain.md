# 014 — un ángulo fuera de (-π/2, π/2) hacia una escala de tangente

**Estado:** cerrado e integrado el 2026-11-02 (#92). `make check`, `make check-packages`, `make compare-java` y las mutaciones completas K1-K193 en verde el 2026-11-02 ([evidencia](evidence/014-2026-11-02-mutations.txt)). Decisión del usuario (2026-11-02): fuera de (-π/2, π/2) es `ErrOutOfDomain`.

## 1. Revisión previa

**La especificación.** UCUM §44 ■4 (tabla 18), verificado en ucum.org/ucum
el 2026-11-02: «f_PD(α) = tan(α) × 100 to convert from a plane angle α to a
prism diopter value» y «f_PD⁻¹(x) = arctan(x / 100) to convert from prism
diopter (or slope percent) value x back to a plane angle». No fija el dominio
del ángulo.

**El XML.** Dos unidades, sin prefijos (`isMetric="no"`): `[p'diop]`, con
`<value Unit="100tan(1 rad)">` y `<function name="tanTimes100" value="1"
Unit="rad"/>`; `%[slope]`, con `<value Unit="100tan(1 rad)">` pero
`<function name="100tan" value="1" Unit="deg"/>`. El XML se contradice en
`%[slope]` (rad en el texto, deg en la función). La librería toma el argumento
de tan en radianes para las dos (`tanHandler.nativeUnit`), que es lo que dice
§44 ■4 (tan de un ángulo): 100 `%[slope]` son 45 deg. Va en la nota para
UCUM (§2.7). `[pi]` vale
`3.1415926535897932384626433832795028841971693993751058209749445923` y `deg`
es `2 [pi].rad/360`.

**El defecto.** tan tiene período π y arctan devuelve solo (-π/2, π/2): el par
(f, f⁻¹) de §44 ■4 es inverso solo ahí. Fuera, la librería da el valor
literal de tan, que no vuelve al ángulo. Medido sobre `main` (v4.4.5):

| Ángulo | `[p'diop]` | De vuelta a `rad` |
|---|---|---|
| 1 rad | 155.74 | 1 |
| `math.Pi/2` rad | 1.6e18 | `math.Pi/2` |
| `math.Pi/2` + 1 ulp | -6.2e17 | -1.5708 |
| 2 rad | -218.5 | -1.14 |
| π rad | -1.2e-14 | ≈ 0 |
| 200 deg → `%[slope]` | 36.4 | |

Además cambia de signo al cruzar π/2: una inversión de monotonía.

**Referencias.** Ucum-java 1.0.10 no maneja `tan` («Not handled yet»);
`gofhir/server` usa esta librería. No hay comparación posible.

**Decisión (usuario).** Un ángulo α con |α| ≥ π/2 hacia una escala de
tangente es `ErrOutOfDomain`: no es un ángulo que f⁻¹ devuelva, así que no
tiene valor en la escala que vuelva a él. Es la lectura de 013 para `sqrt`
(el valor de la escala es la imagen de f⁻¹), ahora del lado del destino. Va
en un ADR (§2.5), porque la especificación no lo dice.

## 2. Cambio

> Reescrito tras la ronda adversarial (§9).

1. **El borde sale del XML:** π/2 es la mitad del valor de `[pi]` en
   `ucum-essence.xml`, el mismo con que `deg` y `gon` se llevan a radianes;
   no `math.Pi`. Se calcula una vez al cargar el modelo. `[pi]` se busca por
   su código: es la constante que la propia definición de `deg` nombra (ADR).
   Un XML propio sin `[pi]` no tiene el chequeo (como hoy), y queda escrito.
2. **El dominio va antes que el rango:** en `toLinearScale`, hacia una escala
   de tangente, antes de `linearTimes`. `1e300 Yrad` → `[p'diop]` pasa de
   `ErrOutOfRange` a `ErrOutOfDomain`.
   - Con el ángulo exacto (`x.rat()` por el factor, el mismo producto que la
     rama de temperatura ya arma, sin calcularlo dos veces): |ángulo| ≥ π/2
     es `ErrOutOfDomain`. El intervalo es abierto: en ±π/2 tan no está
     definida.
   - Sin exacto (un origen logarítmico sobre el tope de costo, o de base no
     racional como `ln`): por `log10Times`, sin materializar. log10 |ángulo| >
     log10(π/2) es `ErrOutOfDomain`; un ángulo que sí se materializa se mide
     exacto por el binario de su float64 contra el mismo π/2 racional (nunca
     contra un π/2 en float64, que rechazaría `math.Pi/2`, que está bajo el
     π/2 del XML). El borde de ese caso sin exacto queda escrito.
3. **Dónde vive:** `special.Use` dice si su escala es de tangente, sin tocar
   la interfaz `Handler` (como `HasValue` en 013); el engine guarda π/2.
4. **No finitos:** siguen pasando (#55): `+Inf rad` → `[p'diop]` da NaN sin
   error. #55 va antes que el dominio; el ADR lo dice.
5. **Oráculos de tests:** el de Java (`specForward` de `100·tan`,
   `javaclasses_test.go`) suma el dominio, para que un caso futuro lo
   codifique.
6. **ADR** en `docs/adr/0001-tan-domain.md` (el primero del repo): el dominio
   de las escalas de tangente, con la cita de §44 ■4, la decisión, el π del
   XML, #55 y el test.
7. **Nota para UCUM** en `docs/plans/hl7-issues.md` (sin publicar): §44 ■4 no
   fija el dominio de α, y el XML se contradice en `%[slope]`.
8. **Documentación:** `ErrOutOfDomain` (los dos godoc) y el párrafo del
   README nombran el caso.

Queda igual: el valor de la escala hacia el ángulo (`[p'diop]` → `rad`), que
ya da (-π/2, π/2) para todo x finito; los ángulos en (-π/2, π/2); las demás
escalas.

## 3. Criterio de salida

- `Convert` de un ángulo con |α| ≥ π/2 (en `rad`, `deg`, `gon`, `'`, `''`,
  `krad`, `Yrad`, `10*400.rad`) hacia `[p'diop]` y `%[slope]` da
  `ErrOutOfDomain`, antes que `ErrOutOfRange`; con |α| < π/2, lo mismo que
  en `main`.
- El borde: 90 deg exacto da `ErrOutOfDomain`; `math.Pi/2` rad (bajo el π/2
  del XML) da un valor, como hoy; `math.Pi/2` + 1 ulp, `ErrOutOfDomain`.
- Ningún cambio de signo entre dos ángulos aceptados vecinos.
- `make check`, `make check-packages`, `make compare-java`, las mutaciones
  completas y un benchmark en el repo: el costo de `Convert(45, "deg",
  "[p'diop]")` medido antes y después.

## 4. Tests (primero)

En `tan_test.go`. Las escalas de tangente se buscan en el XML (las funciones
`tanTimes100` y `100tan`), y π sale de `[pi]` del XML, no de `math.Pi`.

1. Fuera del dominio: 2 rad, π rad, -2 rad, 10 rad, 200 deg, -90 deg, 90 deg,
   100 gon, 5400 `'`, 324000 `''`, 0.002 krad, 1e300 rad, 1e300 Yrad,
   `10*400.rad`, con las dos unidades; el error es `ErrOutOfDomain` (no
   `ErrOutOfRange`) y el mensaje nombra el valor, la unidad de origen y la de
   destino.
2. Dentro: 1.6 mrad, y una rejilla en (-π/2, π/2) en `rad` y `deg`. Guarda
   de regresión: igual que `main` (`math.Tan` del ángulo canónico, la regla
   de 010); y contra un oráculo independiente (tan en `big.Float` por serie),
   a pocos ulps, con el margen escrito: 45 deg es 100 a 2 ulps.
3. El borde con `big.Rat`: los float64 vecinos de π/2 del XML en `rad`;
   90 deg ± 1 ulp en `deg`.
4. Monotonía: sin cambio de signo entre vecinos aceptados.
5. Sin exacto: una escala `ln` de ángulo propia (`NewFromReader`,
   `ln(1 rad)`): 700 y 710 dan `ErrOutOfDomain` los dos; -10 da un valor.
6. No cambia: `[p'diop]` → `rad` para valores enormes, 0 y negativos; no
   finitos (#55); un XML propio sin `[pi]`.
7. El oráculo de Java con el dominio (§2.5).

## 5. Mutaciones previstas

- Sin el chequeo (1).
- Intervalo cerrado (`>` en vez de `≥`): 90 deg pasa (3).
- π de `math.Pi` en vez del XML (3).
- El chequeo sin el factor de la unidad (1: `0.002 krad`; 2: `1.6 mrad`).
- El chequeo después de `linearTimes`: `1e300 Yrad` vuelve a `ErrOutOfRange`.
- Sin la rama sin exacto (5).
- K152 y K153 apuntan a la rama de temperatura de `toLinearScale`: se
  actualizan sin que apaguen también el chequeo nuevo.
- Mutaciones K1-K180 completas.

## 6. Fuera de alcance

- El signo del cero, y un prefijo propio de valor 0 (013 §6).
- Otras funciones periódicas: UCUM no define otras.
- De una escala de tangente a otra (§9): `1e308 [p'diop]` → `[p'diop]` da
  1.63e18 y `100 [p'diop]` → `%[slope]` da 99.99999999999997; el resultado
  debería ser la entrada. Pendiente propio.
- Un ángulo diminuto: `1e-300 yrad` → `[p'diop]` da `ErrOutOfRange`, aunque
  tan·100 ≈ 1e-322 cabe. Pendiente propio.
- Una escala `ln` con valor de función negativo (p. ej. `[nlnB]` = -ln(1 [tiny]))
  pierde el signo por la vía log10: `714.25 [nlnB]` → rad da +1.566 rad en main y
  en la rama (debería ser negativo). Pendiente propio; mientras tanto el `Abs`
  de la comparación del ángulo materializado no se puede matar (equivalente).
- De `main`: `Validate`/`Analyze` dan nil para una unidad cíclica. Un DAG sin
  ciclo `[e_i]=[e_{i+1}]/[e_{i+1}]` crece exponencial (≈8 min a 30 niveles):
  falta memoizar por unidad. Pendiente propio.

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.6. Un ángulo que hoy da un número pasa a dar un error; el
número no volvía al ángulo.

Un cambio más: una definición cíclica (`[pi]` en `[pi]`, o un ciclo que no pasa
por la raíz) hacía caer el proceso en la canonicalización por desborde de pila;
ahora da un error (`unit "[pi]" is part of or leads to a definition cycle`). Con
un `[pi]` cíclico no hay chequeo de tangente.

## 9. Ronda adversarial del plan (2026-11-02)

Dos Opus independientes (Codex sin cuota hasta el 2026-11-04). Los dos
confirmaron con sondas que solo `Convert` llega a un destino de tangente
(`toLinearScale`), que el borde es sólido (todo ángulo exacto bajo π/2 del XML
redondea a lo sumo a `math.Pi/2`) y que ningún test actual fija el valor de
hoy fuera del dominio.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El orden dominio/rango no está fijado: `1e300 Yrad` y `1e306 krad` dan hoy `ErrOutOfRange`; sin exacto (`ln` propio) 700 daría dominio y 710 rango | a (alta), b (media) | **Aceptado:** §2.2, el dominio antes; tests 1 y 5 |
| El paso 3 (π/2 en float64) rechazaría `math.Pi/2`, que el exacto acepta | a, b (media) | **Aceptado:** se compara el binario del float64 contra el π/2 racional |
| El test 2 solo confirma el código (`math.Tan`) | a (media) | **Aceptado:** se marca como guarda de regresión y se suma un oráculo independiente en `big.Float` |
| De una escala de tangente a otra no da la entrada | a (media) | **Fuera de alcance:** §6, con la evidencia |
| Un XML propio sin `[pi]` | b (media) | **Aceptado:** sin chequeo, como hoy; test 6 |
| K152/K153 anclan en la rama que el cambio toca | b (media) | **Aceptado:** §5 |
| La cita de `%[slope]` estaba mal: el XML dice `100tan(1 rad)` en el texto y `deg` en la función | b (baja) | **Confirmado** leyendo el XML (líneas 1476-1483); §1 corregido y a la nota para UCUM |
| `[pi]` por su código, y `nativeUnit` en rad | a (baja) | **Aceptado:** el ADR lo justifica |
| Mensaje, `0.002 krad` y `1.6 mrad`, ±Inf, el oráculo de Java, costo | a, b (baja) | **Aceptado:** §2.4, §2.5, §3, tests 1-2 |
| `1e-300 yrad` da `ErrOutOfRange` | a (baja) | **Fuera de alcance:** §6 |

## 10. Code review

### Ronda 1 (2026-11-02, dos Opus independientes)

Ninguno encontró un defecto de resultado: todo ángulo probado queda del lado
correcto de π/2 y el dominio va antes que el rango. Mutaciones completas
antes de la ronda: todas `ok` (K1-K186).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| La rama sin exacto cerca de π/2 no tiene test: sobreviven quitar la comparación final y mover el margen de 1e-9 | a, b (media) | **Confirmado** por los mutantes de los dos; test con `[lnB]` = ln(1 `[tiny]`), `[tiny]` = 1e-310 rad, y K187-K188 |
| `IsTangent` → `true` sobrevive: el test de `[lgK]` → `Cel` usa un XML sin `[pi]` | b (media) | **Confirmado**; test con `[pi]` y K189 |
| El ángulo sin exacto se materializa dos veces; `if !x.log` inalcanzable | a, b (baja) | **Aceptado:** una sola vez, sin la rama muerta |
| K185 apaga el chequeo en vez de solo moverlo | a (baja) | **Aceptado:** K185 fiel |
| `halfPiOf` exige `Unit="1"` y apaga el chequeo con un `[pi]` propio en `10*` | b (baja) | **Aceptado:** el valor canónico de 1 `[pi]` |
| El godoc de `ErrOutOfDomain` no dice que la API exacta da `ErrNotRational` para tan | a, b (baja) | **Aceptado** |
| `docs/plans/README.md` desactualizado | a (baja) | **Rechazado:** la copia del revisor era anterior a la actualización al paso 8 |
| `>=` → `>` en la comparación float64 | a, b | **Equivalente:** un float64 nunca es igual al π/2 racional del XML; queda comentado |

### Ronda 2 (2026-11-02, los mismos dos Opus)

Los dos confirmaron cerrados los hallazgos de la ronda 1, volviendo a correr
los mutantes que sobrevivían. Mutaciones completas antes de la ronda: K1-K189
en `ok`.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Un `[pi]` cíclico (`Unit="[pi]"`) hace caer `NewFromReader` con `fatal error: stack overflow`: `halfPiOfPi` canonicaliza en `New` | a (media) | **Confirmado** con una sonda: en `main` el mismo XML carga y cae recién en `Canonical(1, "[pi]")`. La causa, anterior a 014, es que la canonicalización no detecta ciclos: se corrige ahí, con test y mutación |
| El margen de 1e-9 solo se prueba grueso: `-1e-12` sobrevive y rechaza ángulos válidos (`[lnB]` en v*-2e-12 … v*-2e-13) | b (media), a (baja) | **Confirmado** por los dos; puntos a -1e-12, -2e-13 y los float64 vecinos de v*, y K190 |
| `pi.Sign() <= 0` → `< 0` sobrevive: falta `[pi]` = 0 | b (baja) | **Aceptado** |
| Los casos «sin chequeo» comparan con `100*math.Tan` | a (baja) | **Aceptado:** el oráculo en `big.Float` |
| Quitar `Abs` del ángulo materializado sobrevive | b (baja) | **Equivalente hoy:** un `ln` de valor negativo pierde el signo por la vía log10 (defecto de `main`, §6), así que no llega un ángulo negativo sin exacto |
| `log10Times` dos veces en la vía sin exacto | a (nit) | **Rechazado:** es la vía rara; separarlo complica `toLinearScale` sin ganancia medible |
| Margen 0 o positivo mayor, `tangent && r == nil` → `tangent` | a, b | **Equivalentes**; quedan comentados |
| `docs/plans/README.md` en la ronda 1 | b (baja) | **Rechazado:** la copia del revisor era anterior a la actualización |

### Ronda 3 (2026-11-02, los mismos dos Opus): no converge

Los dos confirmaron cerradas las correcciones de la ronda 2 (margen `-1e-12`,
`Sign() < 0` y la guarda quitada, todos muertos) y no encontraron un resultado
incorrecto: ningún ciclo (propio, de tres unidades, por prefijo, por
exponente, por la unidad de una función especial) hace caer ni colgar una
entrada pública, y ninguna cadena válida da un falso positivo (`[pi].[pi]`,
`rad/rad`, un diamante, 20 y 30 niveles). `go test -race ./...` en verde.
Mutaciones completas antes de la ronda: K1-K192 en `ok`.

| Hallazgo | Revisor (severidad) | Estado |
|---|---|---|
| La guarda asigna en toda canonicalización: `new(expanding)` escapa al heap (`-gcflags=-m`), +1 asignación y +160 B; `IsComparable` de ~300 ns a 425-570 ns. El comentario «no asigna memoria» es falso | a (media), b (baja) | **Corregido:** profundidad `int` por valor con tope `len(m.DefinedUnits)` (`Service.maxDepth`); un `*cycleError` sin envolver por nivel. Asignaciones iguales a `main` en `ConvertSimple` (49) e `IsComparable` (25) |
| Sobrevive comparar solo contra la raíz: los dos ciclos del test vuelven a la raíz. Falta `[pi]`→`[pi2]`→`[pi3]`→`[pi2]` | b (media) | **Corregido:** el ciclo `[pi2]`→`[pi3]`→`[pi2]` y una cadena acíclica de 30 niveles; K192 (sin la guarda) y K193 (tope 8) |
| El cambio de comportamiento del ciclo (antes caída, ahora error) no está en §8, el ADR ni el README; un `[pi]` cíclico apaga el chequeo de tan | b (baja) | **Corregido:** §8, el ADR y el README |
| Bajo K192 el hijo crece la pila hasta 1 GB (~0.6 s por mutante): `debug.SetMaxStack` | a (baja) | **Corregido:** `debug.SetMaxStack(64<<20)` en el hijo |
| De `main`: `Validate`/`Analyze` dan nil para una unidad cíclica; un DAG `[e_i]=[e_{i+1}]/[e_{i+1}]` crece exponencial (8 min a 30 niveles) | b (baja) | Fuera de alcance, §6 |

Tope de 3 rondas alcanzado: se consultó al usuario, que eligió corregir y cerrar sin cuarta ronda (2026-11-02). La verificación es por mutaciones completas y benchmark contra `main`.
