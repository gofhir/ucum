# 007 — Ningún test ve si un `*big.Rat` de quien llama se modifica (#56)

**Estado:** cerrado e integrado: PR #70, *rebase and merge* (2026-10-28). Plan aprobado el 2026-10-28 con la opción A: `CanonicalKey` copia, `fix:`.

## 1. Revisión previa

### El hueco

Hoy no hay ningún bug: todo el código copia los `*big.Rat` que recibe y los
que devuelve. Lo que falta es un test que se dé cuenta si una copia
desaparece. El issue nombra una (`Quantity.rat()`); medí cinco de la misma
clase en una copia del repo, con la suite completa y `UCUM_PACKAGES`:

| # | Mutación | Suite completa |
|---|---|---|
| M1 | `Quantity.rat()` devuelve `q.Exact` sin copiar (la del issue) | **sobrevive** |
| M2 | `toCanonicalRat`, unidad lineal, devuelve la entrada | muere (`TestExactAPIDoesNotMutateInput`) |
| M3 | `special.Use.ToCanonicalRat` multiplica sobre la entrada (`v.Mul(v, …)`) | **sobrevive** |
| M4 | `NewDecimal` guarda el `*big.Rat` de quien llama | **sobrevive** |
| M5 | `Decimal.Rat` devuelve el valor interno | **sobrevive** |

M3 multiplica la entrada por `Alpha`, el factor del **prefijo**
(`internal/engine/canonicalize.go`), que vale 1 sin prefijo. Con `Cel` o
`[degF]` deja el mismo valor y nadie lo ve; con `mCel` o `kCel` lo cambia
(medido por Opus y Fable: 37 → 37/1000 y 37 → 37000). Llega a la API
pública por `CanonicalRat`, `ConvertRat` y `Comparator.CanonicalKey` con una
escala especial **con prefijo**. Ninguna unidad no métrica (`[degF]`) puede
matarla.

`TestExactAPIDoesNotMutateInput` (`internal/engine/exact_test.go:217`) cubre
solo `ConvertRat` de `L` a `mL`, una unidad lineal. Por eso M2 muere y M3 no.

**`Decimal` también opera sobre `*big.Rat`** (ronda adversarial). Medido por
Opus con la suite completa:

| Mutación | Suite completa |
|---|---|
| `Div` escribe sobre el receptor | **sobrevive** |
| `Div` escribe sobre el divisor | **sobrevive** |
| `Sub` escribe sobre el receptor | **sobrevive** |
| `Mul`, `Add` escriben sobre el receptor | mueren, por tests de precisión |

**`CanonicalKey` no copia `q.Exact`** (`fhir/quantity.go:300`): se lo pasa
al servicio tal cual, mientras `Compare`, `Add` y `Sub` copian con `rat()`.
Con el servicio de este módulo no cambia nada, porque `CanonicalRat` copia.
Pero `NewComparatorWith` acepta cualquier `ExactService`, y con uno que
escribe sobre su entrada, `CanonicalKey` modifica el `Exact` de quien llama
(medido por Fable: después de `Compare`, `a` = 1; después de `CanonicalKey`,
`a` = 2).

### Qué promete la API

- `Decimal.Rat`: «returns the exact value, as a copy» (`fhir/decimal.go`).
  Promesa escrita y sin test (M5).
- `NewDecimal` y `Decimal`: no lo dicen, pero `Decimal` es un valor; si
  guardara el `*big.Rat` de quien llama, cambiarlo después cambiaría el
  `Decimal`.
- `ExactService` (`ucum.go:161-178`) y `Quantity.Exact` no dicen si la
  entrada se modifica ni si el resultado comparte memoria con ella. Es una
  interfaz: lo que diga es un requisito para quien la implementa, y la
  garantía solo se puede dar para la implementación de este módulo.

### Especificación

No aplica: es un contrato de la API de Go, no de UCUM ni de FHIR.

### `gofhir/server` y otras implementaciones

- `server` usa solo la API `float64` (`internal/terminology/ucum`). No tiene
  nada que tomar. **Descartar.**
- **Ucum-java** usa `Decimal` propio e inmutable. No aplica.

## 2. Alcance

1. **Tests** de que ninguna operación pública modifica un `*big.Rat` que
   recibe, ni devuelve uno que comparte memoria con su entrada (§4).
2. **Godoc**, en un commit `docs:` aparte:
   - `ExactService`: una implementación no modifica el valor que recibe, y
     el resultado es de quien llama. Las de `New` y `NewFromReader` lo
     cumplen.
   - `Quantity.Exact`: los métodos de `Comparator` no lo modifican.
3. **`CanonicalKey` copia `q.Exact`** con `q.rat()`, como ya hacen
   `Compare`, `Add` y `Sub` (decisión del usuario, §7). Así la promesa sobre
   `Quantity.Exact` vale con cualquier servicio, y no depende de que el que
   pasó quien llama cumpla la interfaz.
4. **Mutaciones** M1 a M10 en `scripts/mutations.py` (§5).

Fuera: cualquier otro cambio de código de producción. Si un test encuentra
un caso que hoy modifica la entrada, el incremento se detiene y vuelve al
plan.

**Versión:** `fix:` (parche). Con un servicio propio, `CanonicalKey` hoy
modifica el valor de quien llama. El usuario eligió que copie (opción A,
2026-10-28), en lugar de solo documentarlo (opción B, `test:` y `docs:`, sin
versión).

## 3. Criterio de salida

- `make check` en verde.
- M1 a M10 mueren.

## 4. Tests

Todos guardan una copia de cada `*big.Rat` antes de la llamada y la
comparan después, y mutan cada resultado para ver que no cambia ninguna
entrada.

1. **`fhir`, cada método de `Comparator`** (`Compare`, `Comparable`,
   `CanonicalKey`, `Add`, `Sub`, `ConvertDecimal`), con:
   - operandos con `Exact` distintos y con el **mismo** `*big.Rat` en los dos
     (hoy `fhir/arithmetic_test.go` pasa siempre el mismo, y por eso no ve M1);
   - unidades lineales (`mL`, `L`, `mg/dL`), especiales racionales sin
     prefijo y **con prefijo** (`Cel`, `[degF]`, `mCel`, `kCel`: solo las
     con prefijo ven M3) y especiales sin forma racional (`[pH]`), que van
     por `float64`;
   - para `CanonicalKey`, además, un `ExactService` de prueba que escribe
     sobre su entrada, conectado con `NewComparatorWith`: el `Exact` de
     quien llama no cambia.
   - valores negativos, cero, fracciones sin forma decimal finita (1/3) y
     grandes (10^30).
2. **`Decimal`:** cambiar el `*big.Rat` pasado a `NewDecimal` no cambia el
   `Decimal`; cambiar el que devuelve `Rat` tampoco. `Mul`, `Div`, `Add` y
   `Sub` no cambian ni el receptor ni el operando, con operandos distintos
   y con el mismo `Decimal` en los dos lados.
3. **API exacta del paquete raíz** (`ConvertRat`, `CanonicalRat`,
   `ConversionFactor`), con los mismos códigos y valores: la entrada no
   cambia, y mutar el resultado no cambia la entrada ni la próxima llamada.

Los esperados no salen de correr el código: son la copia tomada antes de la
llamada.

## 5. Mutaciones

| # | Mutación | Debe fallar |
|---|---|---|
| M1 | `rat()` no copia | test 1 |
| M2 | `toCanonicalRat` lineal devuelve la entrada | tests 1 y 3 |
| M3 | `special.ToCanonicalRat` escribe sobre la entrada | tests 1 (`CanonicalKey` con `mCel`) y 3 (`mCel`, `kCel`) |
| M4 | `NewDecimal` no copia | test 2 |
| M5 | `Decimal.Rat` no copia | test 2 |
| M6 | `Decimal.Div` escribe sobre el receptor | test 2 |
| M7 | `Decimal.Div` escribe sobre el divisor | test 2 |
| M8 | `Decimal.Sub` escribe sobre el receptor | test 2 |
| M9 | `Decimal.Mul` escribe sobre el receptor | test 2 (hoy muere por casualidad) |
| M10 | `CanonicalKey` pasa `q.Exact` sin copiar | test 1, servicio que escribe |

`special.FromCanonicalRat` no entra: su entrada es siempre un valor
intermedio del motor, nunca el de quien llama, así que escribir sobre ella
es una mutación equivalente. La equivalencia depende de la copia de
`toCanonicalRat` (M2), que el test 3 fija.

## 6. Dependencias y excepciones

Ninguna dependencia nueva. Ninguna excepción a la regla 5.

## 7. Ronda adversarial

Opus y Fable, en paralelo, sin el razonamiento del plan.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| M3 no se ve con `Cel` ni `[degF]`: `Alpha` es el factor del prefijo, 1 sin prefijo | Opus, Fable (alta) | **Aceptado:** verificado en `special.go` y `canonicalize.go`. Tests con `mCel` y `kCel`; §1 corregido |
| `Decimal.Mul`, `Div`, `Add` y `Sub` faltan; tres mutaciones sobreviven | Opus (media), Fable (baja) | **Aceptado:** test 2 y M6 a M9 |
| La promesa sobre `Quantity.Exact` depende del servicio: `CanonicalKey` no copia, y con `NewComparatorWith` el servicio puede ser cualquiera | Opus, Fable (media) | **Aceptado:** medido por Fable. Decisión del usuario: que `CanonicalKey` copie, o que el godoc diga que la garantía vale solo con el servicio de este módulo |
| La equivalencia de M6 (`FromCanonicalRat`) depende de M2 | Opus, Fable (baja) | **Aceptado:** dicho en §5 |
| El godoc va en un commit `docs:` aparte, para no esconder el contrato en un `test:` | Opus, Fable (baja) | **Aceptado** |

## 8. Tests e implementación (pasos 5 a 7)

**Tests primero.** Antes del arreglo, con el servicio de este módulo, todos
pasan: no hay ningún bug con él. Con el servicio que escribe sobre su
entrada (`TestComparatorProtectsOperandsFromService`) falla solo
`CanonicalKey`, con 350 errores: 7 pares × 25 combinaciones de valores × 2
(operandos distintos y compartidos). `Compare`, `Comparable`, `Add`, `Sub`
y `ConvertDecimal` ya pasaban copias.

La primera versión del test arrastraba el error: un operando cambiado por
`CanonicalKey` hacía fallar también a los métodos siguientes (1950
errores). Ahora cada método recibe operandos nuevos.

- `fhir/aliasing_test.go`: tests 1 y 2.
- `exact_aliasing_test.go`: test 3.

**Implementación:** `CanonicalKey` pasa `q.rat()` en lugar de `q.Exact`.
Godoc de `ExactService` y de `Quantity.Exact`, que irá en un commit `docs:`
aparte.

**Mutaciones** (`scripts/mutations.py`, K63 a K72):

| # | Plan | Mutación |
|---|---|---|
| K63 | M1 | `rat()` no copia |
| K64 | M2 | `toCanonicalRat` lineal devuelve la entrada |
| K65 | M3 | escala especial con prefijo escribe sobre la entrada |
| K66 | M4 | `NewDecimal` no copia |
| K67 | M5 | `Decimal.Rat` no copia |
| K68 | M6 | `Div` escribe sobre el receptor |
| K69 | M7 | `Div` escribe sobre el divisor |
| K70 | M8 | `Sub` escribe sobre el receptor |
| K71 | M9 | `Mul` escribe sobre el receptor |
| K72 | M10 | `CanonicalKey` pasa `q.Exact` sin copiar |

La mutación M2 del plan 001 apuntaba a la línea que cambió; ahora apunta a
la nueva y sigue muriendo.

Suite completa: [evidence/007-2026-10-28-mutations.txt](evidence/007-2026-10-28-mutations.txt),
sin resultados inesperados.

**Costo de los tests nuevos** (binario compilado, `/usr/bin/time -l`):
- los de `fhir`: 0,02 s y 15 MB de RSS;
- el de la raíz: menos de 0,01 s y 8 MB.

**Verificación:** `make check` en verde, y la suite con `UCUM_PACKAGES` en
verde.

## 9. Code review

### Ronda 1 (Opus y Fable, en paralelo, sin el razonamiento del plan)

Ninguno dejó hallazgos de severidad media o mayor: **converge**. Los dos
corrieron `make check` y la suite de mutaciones en sus copias sin resultados
inesperados, y verificaron frase por frase el godoc contra el código. Entre
los dos inyectaron unas 20 mutaciones propias. Murieron todas, salvo las que
escriben sobre un racional recién creado por el motor (equivalentes) y las
del hallazgo 1 y 3.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Ningún test ve que `ConvertDecimal` guarde el `*big.Rat` que devolvió el servicio | Opus (baja) | **Aceptado:** el servicio de prueba reutiliza su resultado y lo cambia en la llamada siguiente; el test convierte dos veces. K73 |
| Un `Decimal` sin límite de precisión (0) nunca pasa por un test de aliasing: dos mutaciones que copian solo con precisión declarada sobreviven | Fable (baja) | **Aceptado:** tests 1 y 2 con precisión 0; K74 |
| Los tests recorren un `map`, y el orden de los errores cambia entre corridas | Opus (baja) | **Aceptado:** slices |
| La cuenta de casos fija 25 en lugar de calcularla de los valores | Fable (baja) | **Aceptado** |
| No hay un par de la misma unidad lineal (`L` a `L`) | Fable (baja) | **Aceptado:** en los tests 1 y 3 |
| Las fechas del plan y de la evidencia eran UTC (2026-10-29); la fecha local es 2026-10-28 | Fable (baja) | **Aceptado:** corregidas, y también la de la v4.3.4 en el punto de reanudación |
| `internal/decimal.Decimal.Rat` sin copia sobrevive | Opus (baja) | **Fuera de alcance:** es interno, y hoy no se expone porque la forma canónica se recalcula en cada llamada. Hay que volver a mirarlo si algún día se cachea |

Después de las correcciones: `make check` en verde, la suite con
`UCUM_PACKAGES` en verde y las mutaciones completas sin resultados
inesperados (evidencia actualizada en el mismo archivo).

## 10. Cierre

- `make check` y la suite con `UCUM_PACKAGES` en verde.
- Mutaciones: [evidence/007-2026-10-28-mutations.txt](evidence/007-2026-10-28-mutations.txt).
- PR #70: `570eb4f` (`fix:`, código y tests) y `0b043a4` (`docs:`, godoc);
  cierra #56.
