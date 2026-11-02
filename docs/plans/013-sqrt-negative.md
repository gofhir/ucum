# 013 — un valor negativo en una escala de raíz cuadrada

**Estado:** paso 9 (cierre). Plan aprobado el 2026-11-02; review convergido en la ronda 2 (§10). Evidencia: `evidence/013-2026-11-02-mutations.txt` (`make mutations`, todas con el resultado esperado) y `evidence/002-2026-11-02-compare-java.md` (solo cambia `xml-special:00097`). Falta commit y PR.

## 1. Revisión previa

**El defecto.** El XML define una sola escala de raíz cuadrada,
`[m/s2/Hz^(1/2)]` (`isMetric="no"`, sin prefijos), como
`<function name="sqrt" value="1" Unit="m2/s4/Hz"/>`. UCUM §47 ■4 (tabla 21):
«The function pair denoted "sqrt" is defined as the square root with its
respective inverse function, the square» (verificado en ucum.org/ucum el
2026-11-02; `special.go` cita §48 por error). Un valor de esa escala es la raíz
cuadrada de una magnitud en m2/s4/Hz, así que no hay valores negativos: la
imagen de `sqrt` es [0, ∞). Hoy la librería eleva al cuadrado sin mirar el
signo (`sqrtHandler.toCanonical`, `v * v`). Medido sobre `main` (v4.4.4):

| Llamada | Hoy | Esperado |
|---|---|---|
| `Canonical(-2, "[m/s2/Hz^(1/2)]")` | 4 m2.s-3 | `ErrOutOfDomain` |
| `Convert(-2, "[m/s2/Hz^(1/2)]", "[m/s2/Hz^(1/2)]")` | 2 (pierde el signo) | `ErrOutOfDomain` |
| `Multiply(-2 [m/s2/Hz^(1/2)], 3 B)` | 4000 m2.s-3 | `ErrOutOfDomain` |
| `fhir.Compare(-2 [m/s2/Hz^(1/2)], 4 m2/s4/Hz)` | 0 (iguales) | error con `ErrOutOfDomain` |
| `Canonical(-1e-310, …)`, `Canonical(-1e200, …)` | `ErrOutOfRange` | `ErrOutOfDomain` |
| `Canonical(0, …)`, `Canonical(2, …)` | 0, 4 | sin cambio |

El caso `Convert` a la misma unidad muestra que no es una convención: la
librería devuelve un valor distinto al de entrada sin avisar.

**Referencias.** Ucum-java 1.0.10 no tiene manejador de `sqrt` (sus
`special/` son `CelsiusHandler`, `FahrenheitHandler` y `HoldingHandler`): no
hay comparación posible. `gofhir/server` (congelado) usa esta librería
(v4.2.0) y no tiene lógica propia.

**La librería ya lo resuelve en el otro sentido.** Hacia la escala, un
negativo da `ErrOutOfDomain` (`sqrtHandler.inDomain`, `v >= 0`; la
documentación de `ErrOutOfDomain` en `errors.go` y en
`internal/ucumerr/errors.go` lo nombra: «a negative number into a
square-root unit»). Falta el sentido de la entrada.

**Dónde entra.** Todo valor finito de una escala no racional pasa por
`mapSpecial` (`internal/engine/range.go`): `Canonical`
(`canonicalScalar`), `Convert` (`convertFloat`), `Multiply` y `Divide`
(`combine`), y con ellos `fhir.Comparator`. `sqrt` no tiene forma racional,
así que la API exacta ya da `ErrNotRational`.

## 2. Cambio

> Reescrito tras la ronda adversarial (§9).

1. **`Use.HasValue(v)`** dice si v es un valor de la escala: falso solo en
   `sqrt` cuando α·v < 0 (UCUM §22 ■4: x = f⁻¹(α x'), así que se mira α·v,
   como `ToCanonical`; un prefijo propio negativo no lo esquiva). Sin tocar
   la interfaz `Handler`: un `case sqrtHandler` en `Use`, como
   `ToCanonicalNear`.
2. **`mapSpecial`** devuelve `ucumerr.ErrOutOfDomain` cuando
   `!use.HasValue(value)`, antes de mapear (antes de `ToCanonicalLog10`, que
   con `Log10Abs` borra el signo). Cubre `Canonical`, `Convert`, `Multiply` y
   `Divide` con operandos finitos.
3. **`combine` con un operando no finito** (la rama de #55, `mapFloat`):
   el otro operando, si es finito, se verifica igual. `Multiply(-2 S, +Inf m)`
   da `ErrOutOfDomain`, no `+Inf`.
4. **`fhir`:** `firstError` aparta `ErrOutOfDomain` como ya aparta
   `ErrOutOfRange`, para que un error de código o de unidades siga primero
   (`Compare(-2 S, 1 kg)` sigue siendo «not comparable»). `canonicalFloat`
   nombra el `Exact` también en un error de dominio.
5. `-0` es un valor (`-0 >= 0`): da 0 como hoy. Una entrada no finita en la
   escala `sqrt` sigue pasando (#55): `-Inf` da `+Inf`.
6. **Oráculos de tests que fijan el defecto:** `javaclasses_test.go`
   (`"sqrt": x*x`, caso `xml-special:00097`, -1 → 1) y
   `range_oracle_test.go` (`Abs(av)` en `sqrt`) pasan a dar error de dominio
   para α·v < 0. La cita de `javaclasses_test.go` (§47■4) es la correcta.
7. **Documentación:** `ErrOutOfDomain` (los dos godoc, más el comentario
   general de `internal/ucumerr`), los godoc de `CanonicalKey` y `Compare`,
   el párrafo de README y la cita de `special.go` (§48 → §47 ■4).

No cambia nada en las escalas logarítmicas, `tan` ni las de temperatura.

**Cambio de clase de error:** un negativo que hoy no cabe (`-1e-310`,
`-1e200`, `Exact` de `-1e-400` en `fhir`) da hoy `ErrOutOfRange`; pasa a
`ErrOutOfDomain`.

## 3. Criterio de salida

- Un valor finito con α·v < 0 en una escala `sqrt` da `ErrOutOfDomain` en
  `Canonical`, `Convert` (a sí misma, a `m2/s4/Hz`, a otra escala especial),
  `Multiply` y `Divide` (en los dos órdenes, con el otro operando finito,
  cero o no finito), y en `fhir.Comparator` (`CanonicalKey`, `Compare`, con
  `Value` o `Exact`).
- El orden de errores de `fhir.Compare` no cambia: código, unidades, valor.
- Cero, `-0`, positivos (normales, subnormales, enormes) y no finitos de la
  escala `sqrt` dan lo mismo que en `main`.
- Ninguna otra escala especial cambia.
- `make check`, `make check-packages`, `make compare-java` (el reporte solo
  puede cambiar en `xml-special:00097`) y las mutaciones completas.

## 4. Tests (primero)

En `sqrt_test.go` (paquete `ucum_test`) y `fhir/`. La escala `sqrt` se busca
en el XML (`internal/essence`, la función `sqrt`).

1. Negativos: `-2`, `-1e-310`, `-5e-324`, `-1e300`, `-math.MaxFloat64`, en
   cada operación de §3; también `S{x}` (anotación) y el servicio
   case-insensitive. `errors.Is(err, ErrOutOfDomain)`, y el mensaje nombra
   el valor y la unidad.
2. Productos: `Multiply(-2 S, 0 m)`, `Multiply(-2 S, ±Inf m)`,
   `Multiply(-2 S, NaN m)`, `Divide(1 m, -2 S)` (hoy 0.25),
   `Divide(-2 S, 0 m)` (dominio antes que cero: el operando no existe).
3. No cambia: los mismos valores positivos, 0 y `-0`, contra v² por el
   factor del XML en `big.Rat`; `NaN`, `±Inf` como en `main`.
4. Otras escalas: un negativo en cada escala especial del XML que no es
   `sqrt` no da `ErrOutOfDomain` en `Canonical`.
5. Definiciones propias (`NewFromReader`): `sqrt` sobre K con un prefijo de
   valor -1 (`Q`): `Canonical(2, Q[sqK])` es dominio y `Canonical(-2,
   Q[sqK])` = 4 K; la ida y vuelta `Convert(2, Cel, Q[sqK])` →
   `Convert(r, Q[sqK], Cel)` vuelve a 2.
6. `fhir`: `CanonicalKey` y `Compare` de `-2 S` (`Value` y `Exact`, también
   `Exact` de `-1e-400`, cuyo mensaje nombra el `Exact`); `Compare(-2 S, 1
   kg)` y `Compare(-2 S, [foo])`, en los dos órdenes, dan el error de hoy.
7. Los oráculos de §2.6 corregidos: `TestCompareJava` y `TestRangeSweep`.

## 5. Mutaciones previstas

- `HasValue` siempre verdadero (1, 2, 6).
- `HasValue` con `v > 0`: el cero deja de ser valor (3).
- `HasValue` mirando v y no α·v (5).
- `mapSpecial` sin el chequeo (1); `combine` no finito sin el chequeo (2).
- `firstError` sin `ErrOutOfDomain` (6).
- El oráculo de `range_oracle_test.go` con `Abs` otra vez.
- Mutaciones K1-K160 completas.

## 6. Fuera de alcance

- `-Inf` en `sqrt` da `+Inf` (#55: una entrada no finita pasa sin
  verificar).
- El signo del cero.
- Una temperatura bajo el cero absoluto (`-300 Cel`): la función de UCUM
  está definida ahí.
- Que la carga acepte un prefijo propio de valor no positivo (§9): uno de -1 se resuelve por α·v; uno de 0 da NaN o +Inf, igual que en `main` (§10).
- `[p'diop]` tiene un defecto análogo hacia la escala: `Convert(2, "rad",
  "[p'diop]")` da -218.5 y de vuelta -1.14 rad, porque la imagen de `atan`
  es (-π/2, π/2) (§9). Pendiente propio.
- Las citas de los orígenes de temperatura en tests y planes de 012 dicen
  UCUM §21-22; los orígenes están en §30 ■4 (`Cel`, tabla 4) y §43 ■4-■5
  (`[degF]`, `[degRe]`, tabla 17). Corrección de comentarios, aparte.

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.5. Un valor que hoy da un número pasa a dar un error, y un
negativo que hoy da `ErrOutOfRange` pasa a `ErrOutOfDomain`; se trata como
corrección porque el número era falso (`Convert` a la misma unidad perdía el
signo; `Compare` daba iguales -2 y 2). El PR y el CHANGELOG lo dicen.

## 9. Ronda adversarial del plan (2026-11-02)

Dos Opus independientes (Codex sin cuota hasta el 2026-11-04). Los dos
confirmaron el diagnóstico con sondas; ninguna vía esquiva `mapSpecial` salvo
la de §2.3.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Dos tests fijan el defecto: `TestCompareJava` (oráculo `"sqrt": x*x`, `xml-special:00097`) y `TestRangeSweep` (`Abs(av)`) | b (alta) | **Confirmado** leyendo `javaclasses_test.go:1222` y `range_oracle_test.go:303`; §2.6 y test 7 |
| Un operando no finito esquiva el chequeo: `Multiply(-2 S, +Inf m)` da `+Inf` | a (media) | **Aceptado:** §2.3, test 2 |
| Cambio de clase de error no listado: `-1e-310`, `-1e200` pasan de `ErrOutOfRange` a `ErrOutOfDomain` | a, b (media) | **Aceptado:** §1, §2, §8 |
| El chequeo debe mirar α·v (UCUM §22 ■4); un prefijo propio de -1 rompe la ida y vuelta | b (media) | **Aceptado:** §2.1, test 5. Que la carga acepte ese prefijo queda en §6 |
| `fhir.firstError` aparta solo `ErrOutOfRange`: un error de dominio taparía «not comparable» y errores de código | b (media) | **Aceptado:** §2.4, test 6 |
| `hasValue` en la interfaz obliga a escribir `true` en tres handlers | a (baja) | **Aceptado:** `case sqrtHandler` en `Use` |
| Godoc de `ucumerr`, `CanonicalKey` y `Compare`; mensaje con el `Exact` | a, b (baja) | **Aceptado:** §2.4, §2.7 |
| Faltan `S{x}`, case-insensitive, `Divide(1 m, -2 S)`, `Multiply(-2 S, 0 m)`, `Divide(-2 S, 0 m)` | a, b (baja) | **Aceptado:** tests 1-2 |
| `[p'diop]` tiene un defecto análogo hacia la escala | a (baja) | **Fuera de alcance:** §6, pendiente propio |

## 10. Code review

### Ronda 1 (2026-11-02, dos Opus independientes)

Sin defectos en el código: los dos probaron cada vía pública contra `main`
(el revisor b, 610 casos) y todas las diferencias son las del plan.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Cuatro mutantes sobreviven: el mensaje de dominio con `Exact` diría «out of range»; el `ValidationError.Code` del producto no finito nombraría el otro operando; ese error ofrecería la API exacta; `Compare` con los dos lados fuera del dominio podría nombrar el segundo | a, b (media) | **Confirmado** con mutaciones K175-K178, ahora muertas: `wantDomainOperand` mira `Code`, el texto del dominio y que no ofrezca `NewExact`; `Compare(-2 S, -3 S)` nombra -2 |
| La documentación dice «un negativo» y el código decide por α·v; el README usaba la palabra «antes» y no nombraba la API exacta | a, b (baja) | **Aceptado:** `errors.go`, `ucumerr` y README lo precisan; la API exacta da `ErrNotRational` |
| `checkHaveValue` no es buen inglés y su comentario confunde | a (baja) | **Aceptado:** `checkOperandValues`, comentario reescrito |
| El oráculo de Java mira el signo de v, no de α·v | a (baja) | **Aceptado como está**, con comentario: la única unidad `sqrt` del XML tiene α = 1; el barrido de rango mira α·v |
| Un prefijo propio de valor 0 da NaN o +Inf (igual en `main`) | b (baja) | **Fuera de alcance:** se suma a §6 junto con el prefijo negativo |

### Ronda 2 (2026-11-02, dos Opus independientes): converge

Sin defectos en el código ni hallazgos de severidad media. Los revisores
corrieron 13 y 5 mutaciones propias (muertas salvo las de abajo) y sondas con
prefijos propios (-1, hecto) y anotaciones; ninguna llamada a `ToCanonical`
fuera de `mapSpecial` recibe un valor finito.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El mensaje del producto no finito puede nombrar la unidad del otro operando sin que falle un test (el prefijo del `ValidationError` ya trae una unidad) | b (baja) | **Confirmado:** `wantDomain` exige «valor unidad» juntos; mutación K179 |
| El error de dominio de `fhir` con `Exact` no se comprueba como `*ValidationError` (código, sin posición) | a (baja) | **Confirmado:** aserción como la del error de rango; mutación K180 |
| El README decía que -Inf «pasa»; da +Inf | b (baja) | **Aceptado:** «-Inf gives +Inf, as before (#55)» |
| La precisión del `Exact` en el mensaje (`Text('g', 6)`) no la fija un test | b (nota) | Viene de antes (la comparte el error de rango); sin cambio |
