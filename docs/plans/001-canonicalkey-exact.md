# 001 — `CanonicalKey` exacto (issue #54)

**Estado:** cerrado el 2026-10-26 (sin commit: pendiente del usuario). **Fecha:** 2026-10-25. **Versión base:** v4.3.0 (`629a874`).

## Problema

`fhir.Comparator.CanonicalKey` pasa por `c.svc.Canonical(q.float(), q.Code)`
(`fhir/quantity.go:281`). `q.float()` convierte `Quantity.Exact` a `float64`
antes de canonicalizar, así que un `Exact` se redondea dos veces. Eso
contradice la documentación del campo (`fhir/quantity.go:22`: «overrides Value
when non-nil, and is used exactly as given»).

Reproducido el 2026-10-25 con un test temporal:

```
CanonicalKey(Exact=0.07, mg/dL) → g.m-3 0.7000000000000001
CanonicalRat(0.07, mg/dL)       → g.m-3 7/10  → float64 0.7
```

El bug está **solo en `Exact`**. El camino de `Value` ya redondea una vez:
`engine.Canonical` (`internal/engine/convert.go:31-49`) pasa el `float64` a
racional exacto. Sonda del 2026-10-25: 840 códigos de `ucum-common` × 10
valores, `Canonical(v)` contra `CanonicalRat(SetFloat64(v)).Float64()`,
**0 diferencias en 8.350 casos**.

## 1. Revisión previa

| Fuente | Qué hace | Decisión |
| --- | --- | --- |
| `gofhir/server` (`internal/terminology/ucum/ucum.go:61`) | Solo usa `parser.Canonical(value, code)` en `float64`; no conoce `CanonicalKey` ni `Exact` | **Descartar:** no lleva el decimal exacto |
| `go-searchindex/ucum/ucum.go` | Llama a `ExactService.CanonicalRat` y redondea una vez al final (comentario de la línea 4: no usa `CanonicalKey` por este bug) | **Tomar** la semántica: exacto y un solo redondeo |
| `Comparator.Compare` (este repo) | Intenta primero `CanonicalRat` (`canonicalPair`) y pasa a `float64` solo ante `ucum.ErrNotRational` | **Tomar** el mismo patrón |

**Especificación** (verificada el 2026-10-25):
- FHIR R4 §2.24.0.1, `decimal`: «Rational numbers that have a decimal
  representation». El valor de un `Quantity` es racional; un `float64` no lo
  representa (0.07 no tiene forma binaria finita).
- FHIR R4 §3.1.1.4.11, `quantity`: «The search processor may choose to perform
  a search based on canonical units». `CanonicalKey` existe para eso.
- UCUM §43 ■4 (`[degF]`): «f_F⁻¹(x) = 5/9 (x + 459.67)»; §30 ■4 (`Cel`):
  «f_C⁻¹(x) = x + 273.15». Los offsets no están en `ucum-essence.xml`, que
  solo dice `degf(5 K/9)` y `cel(1 K)`.
- **Versiones:** R5 §2.1.28.0.1 agrega un tope de 18 dígitos al `decimal`, que
  R4 y R4B no tienen. Es una regla de **validación** del dato, no de
  conversión: esta librería recibe un `*big.Rat` ya leído y no valida
  decimales. La afirmación se limita a esto: para un mismo valor válido en las
  tres versiones, la conversión es la misma. Los datos de prueba vienen de las
  tres versiones. `fhir/ucum-common.tsv` es el ValueSet de R5 (5.0.0); la
  cobertura de R4 y R4B descansa en sus packages de ejemplos.

## 2. Plan

### Alcance
- `CanonicalKey`: si `q.Exact != nil` (o, en general, si `q.rat()` no es nil),
  `CanonicalRat(q.rat(), q.Code)` y `Float64()` del resultado (redondeo al más
  cercano, una sola vez). Ante `ucum.ErrNotRational` (pH, bel, `%[slope]`…), o
  si `q.rat()` es nil (valor no finito), sigue por el camino `float64` actual
  con `q.float()`. Cualquier otro error se devuelve tal cual.
- Comentario de `CanonicalKey`: dice que el valor es exacto hasta un único
  redondeo final.

### Cambios que un consumidor ve (van al cuerpo del commit; release-please arma el CHANGELOG desde el asunto)
- **Solo entradas con `Exact`** (las de `Value` no cambian: sonda de arriba).
- El valor puede cambiar en el último bit (0.7000000000000001 → 0.7). Un
  índice que guardó claves de `CanonicalKey` con `Exact` en v4.3.0 tiene que
  reindexarse.
- Desborde y subdesborde: un `Exact` que no cabía en `float64` antes de
  convertir ahora da el valor correcto si el canónico sí cabe. `1e-330 Ym`:
  antes `0`, ahora `1e-306` m; `1e330 ym`: antes `+Inf`, ahora `1e306` m.
  Sonda Go del 2026-10-25 (lo encontró Codex).
- Es un `fix:`, así que release-please lo publica como patch (4.3.1).

### Fuera de alcance
- **`CanonicalKeyRat`** (lo sugiere el issue): sin consumidor; `searchindex`
  llama directo a `ExactService.CanonicalRat`.
- Rechazar NaN/Inf en `CanonicalKey` (hoy devuelve `NaN, nil`, mientras que
  `Compare` devuelve error): es otra conducta, para otro issue. Los tests de
  bordes solo la fijan como regresión.
- El camino `float64` de `Compare` para escalas no racionales.
- Validar el tope de dígitos de R5.

### Criterio de salida
1. `CanonicalKey(Exact=0.07, mg/dL)` devuelve `(g.m-3, 0.7)`.
2. Los tests de abajo pasan y cada mutación del §7 muere con el oráculo que se
   le asigna.
3. `make check` y `make check-packages` en verde (targets nuevos; ver
   decisión 1).

### Tests (paso 5; ningún esperado sale de correr `CanonicalKey`)
1. **Casos a mano.** El esperado se calcula a mano y se escribe en el test
   como racional; el test solo lo redondea con `big.Rat.Float64`. Cada caso
   cita su fuente.
   - 0.07 mg/dL → 7/10 g.m-3 (el del issue; `ucum-essence.xml`: dL = 0.1 L,
     L = dm³, mg = 10⁻³ g).
   - 1.5 mL → 3/2000000 m3; 0.1 [in_i] → 0.00254 m (`[in_i]` = 2.54 cm);
     100 [degF] → 279835/900 K (UCUM §43 ■4); 1e-330 Ym → 1e-306 m.
   - **M4** (truncar en vez de redondear) lo detectan 3/2000000, 279835/900 y
     0.00254, donde el más cercano ≠ hacia cero. El caso del issue no basta:
     en 7/10 coinciden.
2. **No racionales con `Exact`** (`[pH]`, `B[V]`, `%[slope]`): esperado
   calculado a mano desde las funciones de `ucum-essence.xml`, con tolerancia
   relativa de 1e-15 (cambió en el review, rondas 1 y 2). Usar `Exact` con un
   `Value` distinto detecta M7.
3. **Propagación de errores con un servicio de prueba:** un `ExactService`
   envoltorio cuyo `CanonicalRat` devuelve un error centinela y cuyo
   `Canonical` falla el test si se llama. `CanonicalKey` debe devolver el
   centinela (`errors.Is`). Detecta M5, que con el servicio real sería
   equivalente.
4. **Bordes fijados (solo regresión):** NaN y ±Inf en `Value` dan lo mismo que
   v4.3.0; un código inválido da error; un `System` no UCUM,
   `ErrNotUCUMSystem`.
5. **Metamórfico de escala:** para cada código `u` de `fhir/ucum-common.tsv`
   con `ConversionFactor(u, u)` sin error (unidad lineal; excluye `Cel` y
   `[degF]`, que dentro de un término algebraico pierden el offset, y las
   escalas logarítmicas, que no se pueden envolver), y para ~15 decimales
   variados como `Exact`: `CanonicalKey(v, u)` y `CanonicalKey(v/1000,
   "10*3.(u)")` dan la misma clave y el mismo `float64`, bit a bit (desplazar
   la coma es exacto en texto). Lo mismo con `v×1000` y `10*-3.(u)`. El test
   cuenta los códigos excluidos y exige que sean exactamente los que dan
   `ErrNotLinear`. No hay exclusión silenciosa.
   - Nota: `10*3.(/min)` se acepta porque el parser tolera un `/` inicial
     dentro de paréntesis, que la gramática de UCUM no permite. Afecta a unos
     60 códigos; se deja registrado.
6. **Metamórfico contra `Compare`, entre unidades distintas:** pares (a en
   `u1`, b en `u2`) con `u1`, `u2` lineales y conmensurables, y b construido
   con `ConversionFactor(u1, u2)` como `Exact`: iguales, y desplazados un
   dígito en la última cifra decimal. Se exige `Compare == 0` ⇒ valores
   iguales y `Compare < 0` ⇒ `va <= vb`. Solo unidades lineales: si una cae al
   camino `float64`, `Compare` compara en `float64` y la propiedad no vale
   (contraejemplo de Codex: `1e330 10*-40.V` contra `582 B[V]`). Con el código
   actual, la sonda de Fable encontró 26 + 12 violaciones en 120 pares.
7. **Datos oficiales** (`hl7.fhir.r4.examples` 4.0.1, `r4b.examples` 4.3.0,
   `r5.examples` 5.0.0; 231, 241 y 273 pares (valor, código) UCUM distintos).
   El valor se lee como texto del JSON (`json.Number`) y entra como `Exact`.
   Esas cantidades alimentan los oráculos 5 y 6. Packages con SHA-256 fijado
   en `scripts/packages.sha256` (las tres líneas de
   `go-searchindex/scripts/packages.sha256`) y descargados por
   `scripts/fetch-packages.sh` (se toma el de `go-searchindex`). Corre con
   `make check-packages`. Sin `UCUM_PACKAGES`, `go test` lo salta, pero el
   cierre exige correr el target.
8. **`Value` sin `Exact`:** sobre los mismos datos, el resultado es igual a
   v4.3.0. Es regresión, no oráculo del bug.

**Descartados como oráculo:**
- Los casos de conversión de `UcumFunctionalTests.xml` (Java): están
  redondeados a cifras significativas, y una tolerancia relativa de 1e-6 no ve
  un ulp. La conversión ya la cubre `TestFunctionalConversion`.
- Comparar contra `CanonicalRat(...).Float64()`: es la implementación misma
  (lo señaló Fable).

### Excepciones a la regla 5
Ninguna. Test 8 compara con `svc.Canonical` a propósito: es regresión contra
v4.3.0, no oráculo del bug, y la conducta que fija es la medida en la sonda
(0/8350). En producción no se escribe ningún nombre FHIR ni factor UCUM. La
exclusión de las unidades no lineales sale de la librería (`ErrNotLinear`,
derivado de `isSpecial` en `ucum-essence.xml`), no de una lista.

### Dependencias nuevas
Ninguna. El test de packages usa solo la biblioteca estándar.

### 7. Mutaciones previstas (en una copia del repo)
| # | Defecto inyectado | Debe morir con |
| --- | --- | --- |
| M1 | Volver a `Canonical(q.float(), …)` | 1, 5, 6 |
| M2 | Usar `q.Value` en vez de `q.rat()` (ignorar `Exact`) | 1, 5 |
| M3 | Devolver el error en vez de caer a `float64` ante `ErrNotRational` | 2 |
| M4 | Redondear hacia cero | 1 (los casos señalados) |
| M5 | Caer a `float64` ante cualquier error | 3 |
| M6 | Oráculo 5 debilitado: comparar solo la clave | M1 debe **sobrevivir** al débil y morir con el completo (las claves coinciden en los dos caminos) |
| M7 | En la vuelta a `float64`, usar `q.Value` en vez de `q.float()` | 2 |
| M8 | Oráculo 6 con pares de la misma unidad | M1 debe sobrevivir (el doble redondeo en una unidad es monótono) |
| M9 | Desempatar lejos del cero | 1 (2^53 + 1) — agregada en el review, ronda 1 |
| M10 | Redondear pasando por un `big.Float` de 64 bits | 1 (punto medio ± ε) — ronda 1 |
| M11 | Redondear pasando por un `big.Float` de 56 bits | 1 (punto medio ± ε) — ronda 1 |
| M12 | Redondear pasando por un `big.Float` de 200 bits | 1 (punto medio ± 1e-200) — ronda 2 |
| M13 | Respetar `Exact` solo si `Value` es cero | 1 (casos con `Value: 999`) — ronda 2 |
| M14 | Redondear pasando por un `big.Float` del ancho del racional | 1 (1.0000021281657826 m) — ronda 2 |
| M15 | Perder el signo del valor exacto | 1 (casos negativos) — ronda 3 |

## 3. Ronda adversarial (2026-10-25: Codex y Fable, en paralelo)

Hallazgos verificados con sondas Go por quien planificó, salvo donde se indica.

| # | Origen | Hallazgo | Resolución |
| --- | --- | --- | --- |
| 1 | Codex | Desborde y subdesborde cambian (`1e-330 Ym`: 0 → 1e-306) | **Aceptado** (sonda Go). Se declara como cambio y se prueba |
| 2 | Codex | `Compare` y la clave discrepan cuando un lado cae a `float64` | **Aceptado** (sonda Go: `Compare(b,a) = -1` con `vb > va`). Oráculo 6 solo con unidades lineales |
| 3 | Codex | M6 exigía detectar algo que el oráculo debilitado ya no ve | **Aceptado**: M6 debe sobrevivir |
| 4 | Codex | M5 no es equivalente con un servicio a medida | **Aceptado**: test 3 con un servicio de prueba |
| 5 | Codex | R5 agrega un tope de dígitos al `decimal` | **Aceptado** (verificado en R5 §2.1.28.0.1). La afirmación se limita a la conversión |
| 6 | Codex, Fable | No existen `make check` ni `check-packages` | **Aceptado**: se agregan (decisión 1) |
| 7 | Codex, Fable | Escalar `Cel`/`[degF]` pierde el offset; los logarítmicos no se pueden envolver | **Aceptado** (sonda: `10*3.(Cel)` → 1000 K). Filtro `ErrNotLinear` y conteo explícito |
| 8 | Codex, Fable | El offset de `[degF]` no está en el XML | **Aceptado**: se cita UCUM §43 ■4 |
| 9 | Fable | El camino de `Value` no cambia; el plan afirmaba lo contrario sin medir | **Aceptado** (sonda: 0/8350). Alcance corregido; test 8 queda como regresión |
| 10 | Fable | Pares de la misma unidad no discriminan en el oráculo de `Compare` | **Aceptado**: pares entre unidades; M8 lo comprueba |
| 11 | Fable | El oráculo de datos oficiales era la implementación misma | **Aceptado**: descartado; los datos alimentan 5 y 6 |
| 12 | Fable | Reindexar índices hechos con v4.3.0 | **Aceptado**: va al commit y al CHANGELOG |
| 13 | Fable | M4 no muere con el caso del issue | **Aceptado**: se nombran los casos que lo matan |
| 14 | Fable | Falta mutación: `q.Value` en la vuelta a `float64` | **Aceptado**: M7 |
| 15 | Fable | `NaN` en `CanonicalKey` frente al error de `Compare` | **Aceptado** como nota; fuera de alcance |
| 16 | Fable | `10*3.(/min)` depende de la tolerancia del parser | **Aceptado** como nota |
| 17 | Fable | `ucum-common.tsv` es solo R5 | **Aceptado** como nota |

Ninguno rechazado.

## Decisiones del usuario
2. ~~Comentario de documentación de `CanonicalKey` (API pública)~~: en inglés,
   como el resto de la API pública (decisión del usuario, 2026-10-26). Es una
   excepción a la regla de idioma de la carta §3 limitada a la documentación
   pública; los comentarios internos siguen en español.
1. ~~Agregar `make check` (`gofmt` sin cambios + `go vet` + `golangci-lint` +
   `go test`) y `make check-packages` (descarga verificada + test con
   `UCUM_PACKAGES`).~~ Aprobado el 2026-10-26; CI no corre `check-packages`.

## Registro
- 2026-10-25: paso 1 (revisión previa), paso 2 (plan), paso 3 (ronda
  adversarial con Codex y Fable; 17 hallazgos, todos aceptados).
- 2026-10-26: paso 4, plan aprobado por el usuario, junto con `make check`;
  `check-packages` queda fuera de CI (el usuario no pidió agregarlo).
- 2026-10-26: paso 5, tests en `fhir/canonicalkey_test.go` (1–6) y
  `fhir/examples_test.go` (7–8). Contra v4.3.0 fallan `ExactByHand`,
  `PropagatesErrors`, `ScaleCommonUnits`, `MatchesCompareCommonUnits` y
  `OfficialExamples` (R4, R4B, R5); pasan los no racionales y los bordes, que
  ya funcionaban.
- 2026-10-26: paso 6, corrección en `fhir/quantity.go` (`CanonicalKey`). El
  comentario de documentación queda en inglés como el resto de la API
  pública; el comentario interno, en español.
- 2026-10-26: paso 7. Evidencia reproducible:
  - `make check`: `gofmt`, `go vet`, `golangci-lint` y `go test`.
  - `make check-packages`: SHA-256 verificado y `TestOfficialExamples`; el log
    del test informa, por versión, las cantidades distintas, cuántas se
    escalaron, los pares entre unidades y lo excluido (no lineales y códigos
    inválidos de los ejemplos).
  - `make mutations` (`scripts/mutations.py`): M1–M8 (luego M1–M15, por el
    review), más M1 contra cada
    oráculo por separado. Cada mutante muere con su oráculo y M6/M8
    sobreviven a los oráculos debilitados, como exige el §7.

## 8. Code review

### Ronda 1 (2026-10-26: Codex, Fable y Opus)
Codex no pudo correr Go en su sandbox (no podía escribir la caché), así que
solo revisó los scripts; para cubrir el código Go se sumó un revisor Opus. Los
tres revisores trabajaron cada uno en su copia del repo.

| # | Origen | Sev. | Hallazgo | Resolución |
| --- | --- | --- | --- | --- |
| 1 | Codex, Fable | media | `mutations.py` contaba un error de compilación como «kill» | **Confirmado** (sonda: copia con un error de sintaxis). `run()` separa `[build failed]`/`[setup failed]` y exige `--- FAIL: Test`; línea base sin mutar obligatoria (sin packages, la línea base falla) |
| 2 | Opus | media | El redondeo final casi no tiene oráculo: un `big.Float` de 64 bits sobrevive a todo | **Confirmado** (mutante de Opus). Casos en el punto medio ± ε, en `m` y a través de `mg`; M10 y M11 mueren |
| 3 | Fable | baja | El desempate (lejos del cero) sobrevive | **Confirmado**. Caso 2^53 + 1; M9 |
| 4 | Codex, Fable | baja | `fetch-packages.sh` falla con lista relativa | **Confirmado** (sonda). Ruta absoluta |
| 5 | Fable, Opus | baja | Test 2 tomaba el esperado de `svc.Canonical` | **Aceptado**: esperados a mano desde las funciones de `ucum-essence.xml` (`pH`, `lgTimes2`, `100tan`), con tolerancia 1e-12 |
| 6 | Opus | baja | El conteo de excluidos solo se registraba | **Aceptado**: se fijan 825 lineales y 15 especiales del ValueSet embebido |
| 7 | Opus | baja | Faltaba -Inf | **Aceptado** |
| 8 | Opus | baja | JSON que no decodifica se saltaba sin registro | **Aceptado**: se cuenta y se registra (0 en las tres versiones) |
| 9 | Opus | baja | `Cel` nunca pasa por la vía exacta en los datos oficiales | **Aceptado**: caso a mano 37.5 Cel (UCUM §30 ■4) |
| 10 | Fable, Opus | baja | Log repetido (`[Cel Cel …]`) y `continue` en vez de `break` | **Aceptado** |
| 11 | Fable | baja | `.DS_Store` no ignorado | **Aceptado** |
| 12 | Fable | baja | Comentario de documentación pública en inglés | **Decisión del usuario** (ver abajo) |
| 13 | Opus | baja | `q.rat()` copia `Exact` y el motor lo copia otra vez | **Rechazado**: una asignación por llamada, igual que `Compare`; cambiarlo solo aquí rompe la simetría |

Después de corregir: `make check`, `make check-packages` y `make mutations`
en verde (línea base + 15 mutaciones según lo esperado).

### Ronda 2 (2026-10-26: Fable y Opus)
Los dos verificaron que las 13 resoluciones de la ronda 1 son correctas;
Fable volvió a derivar cada valor a mano con fracciones exactas.

| # | Origen | Sev. | Hallazgo | Resolución |
| --- | --- | --- | --- | --- |
| 1 | Fable | media | Respetar `Exact` solo si `Value == 0` sobrevive: todos los casos racionales dejaban `Value` en 0 | **Confirmado** (mutante sobrevive a `./fhir` con packages). Test 1 con `Value: 999`; M13 |
| 2 | Fable | baja | Un `big.Float` del ancho del racional sobrevive (los casos del punto medio tienen numeradores de ~197 bits) | **Confirmado**. Caso 1.0000021281657826 m (16 cifras); M14 |
| 3 | Opus | baja | Tolerancia del test 2 (1e-12) 4 órdenes más floja de lo necesario | **Aceptado**: 1e-15 (desviación medida: 1 ulp en `[pH]`, 0 en los demás) |
| 4 | Opus | baja | Un intermedio de 200 bits sobrevive (ε ≈ 1e-59) | **Aceptado**: punto medio con ε ≈ 1e-200; M12 |
| 5 | Fable | baja | Un regex de `-run` sin tests da «survive» y vaciaría M6/M8 | **Confirmado** (sonda). `run()` devuelve «no tests» |
| 6 | Fable | baja | `rat()` sin copia sobrevive: ningún test ve el aliasing de `Exact` (p. ej. en `Add`) | **Fuera de alcance**: `rat()` no cambió desde v4.3.0. Se propone un issue aparte |
| 7 | Opus | info | Vía exacta solo con `Exact != nil` es equivalente | **Registrado**: equivalente por la sonda 0/8350 |

Después de corregir: `make check`, `make check-packages` y `make mutations`
en verde (línea base + 18 mutaciones según lo esperado).

### Ronda 3 (2026-10-26: Fable y Opus) — converge
Ninguno de los dos dejó hallazgos de severidad media o mayor. Los dos
verificaron las resoluciones de la ronda 2 y probaron mutantes propios (signo,
desempates, `==` en vez de `errors.Is`, `float32`, `q.Code`, sin
`checkSystems`…): todos mueren, salvo los ya registrados como equivalentes.

| # | Origen | Sev. | Hallazgo | Resolución |
| --- | --- | --- | --- | --- |
| 1 | Opus, Fable | baja | Ningún oráculo de `make check` usa un valor negativo; `Abs` solo lo mata `TestOfficialExamples` | **Confirmado** (mutante de los dos). Casos -5 Cel y -0.07 mg/dL; M15 |
| 2 | Opus | baja | La descripción del test 2 en el plan quedó vieja | **Aceptado** |
| 3 | Opus | baja | El registro del paso 7 decía M1–M8 | **Aceptado** |
| 4 | Fable | baja | El comentario de `%[slope]` citaba `100tan(1 deg)`; `ucum-essence.xml:1480-1481` dice `100tan(1 rad)` en `value` y `Unit="deg"` en `function` | **Aceptado**: el comentario cita las dos y que el XML no es coherente. El esperado (rad) no cambia |

## 9. Cierre (2026-10-26)
- Evidencia generada por los comandos, con fecha, en `docs/plans/evidence/`:
  [`make check`](evidence/001-2026-10-26-check.txt),
  [`make check-packages`](evidence/001-2026-10-26-packages.txt),
  [`make mutations`](evidence/001-2026-10-26-mutations.txt).
- `CONTRIBUTING.md` describe `make check`, `check-packages` y `mutations`. El
  repo no tiene `DESIGN.md`, `CLAUDE.md` ni ADR; el incremento no los
  necesita (no hay excepción a la regla 5).
- Pendientes, fuera de este incremento:
  - Ningún test ve el aliasing de `Exact` en `rat()` (ronda 2, #6):
    gofhir/ucum#56.
  - `CanonicalKey(NaN)` devuelve `NaN, nil` y `Compare`, error (plan, fuera
    de alcance): gofhir/ucum#55.
  - `%[slope]` en `ucum-essence.xml` dice `rad` en `value` y `deg` en
    `function` (ronda 3, #4): ya está reportado upstream como
    ucum-org/ucum#261 (2022-12-15, abierto). No se abre otro.
