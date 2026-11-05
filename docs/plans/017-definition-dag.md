# 017 — una definición compartida se expande una vez

**Estado:** cerrado e integrado el 2026-11-05 (#102); v4.4.9 (#103).

## 1. Revisión previa

**La especificación.** Una unidad definida vale su número por la expresión que
la define (§18 ■5, «real number scalar» y «unit term»); nada impide que dos
unidades se definan sobre la misma, ni que una expresión nombre una unidad
dos veces. El valor de una unidad no depende de dónde aparece: es una función
de su definición.

**El defecto.** `canonicalizeSymbol` expande la definición de cada unidad cada
vez que la encuentra, sin memoria: en un grafo de definiciones que comparte
nodos el trabajo se multiplica por cada camino. Medido sobre `main` (v4.4.8)
con un XML propio `[e_i]` = 2 `[e_{i+1}]/[e_{i+1}]`, `[e_n]` = 1 m:

| n | `Canonical(1, "[e_0]")` |
|---|---|
| 10 | 1.0 ms |
| 14 | 11 ms |
| 16 | 41 ms |
| 18 | 168 ms |
| 20 | 682 ms |

Se duplica por nivel: a 30 niveles son unos 12 minutos por llamada, y no lo
ataja ninguna cota (§ «Bounds» del README acota el código del llamador, no las
definiciones). La guarda de ciclos de 014 (`maxDepth` = número de unidades
definidas) acota la profundidad, no los caminos.

**Referencias.** `gofhir/server` usa esta librería (v4.2.0) y no canonicaliza
por su cuenta. El XML oficial no tiene definiciones compartidas profundas, pero
sí repetidas (`[ft_i]`, `[in_i]`, `[lb_av]`… se reexpanden en cada conversión).

## 2. Cambio

Memoizar la forma canónica de cada expresión de definición:

1. Un caché `defCanonical` (`sync.Map`, `string` → `*model.Canonical`) junto a
   `defCache`, sin tope por la misma razón que él: sus claves son las
   expresiones del XML, no entrada del llamador.
2. En `canonicalizeSymbol`, la expansión de `unitExpr` (siempre
   `specialStandalone` y del vocabulario sensible, así que su resultado es
   función solo del texto) consulta el caché antes de recursar y guarda el
   resultado exitoso. Los errores no se guardan: un ciclo nunca completa y un
   error corta la expansión en el primer camino.
3. Lo guardado no se modifica: se guarda una **copia** del resultado de la
   expansión, antes de que `can.Value = …` y `raiseCanonical` lo muten, y en
   cada acierto se devuelve otra copia con su propio `Units` (§9: copiar solo
   en el acierto corrompe ya la primera llamada). `Value` es inmutable
   (`decimal.Decimal` opera sin mutar), así que basta copiar la cabecera y
   `Units`.
4. La guarda de ciclos queda igual: un ciclo nunca llega a guardarse.

Queda igual: todo resultado y todo error salvo el nombre en el de un ciclo;
solo cambia cuántas veces se expande una definición. La guarda de ciclos va
después del caché (§10): un subárbol sano ya expandido no cuenta para la
profundidad, así que el error de un ciclo nombra una unidad del ciclo en
cualquier orden. En `main` nombraba la unidad donde se agotaba la profundidad,
que podía estar fuera del ciclo (`[z]` en `[a]` = `[x].[a]`,
`[x]` → `[y]` → `[z]` → rad).

## 3. Criterio de salida

- Cada definición se expande una vez por servicio: en el DAG
  `[e_i]` = 2 `[e_{i+1}]/[e_{i+1}]` el trabajo crece lineal con n, no con los
  caminos (n = 30 y n = 200, en `New` vía `[pi]`, `Canonical`, `CanonicalRat`,
  el servicio insensible, `ValidateInProperty` y `Multiply`). No acota el
  tamaño del valor (§6).
- Resultados idénticos a `main` en todas las unidades del XML oficial y en los
  XML propios de los tests (cíclicos, valor 0, factor negativo).
- Sin pérdida en los benchmarks (`internal/engine/benchmark_test.go`):
  asignaciones y tiempo medidos antes y después, en corridas intercaladas.
- Concurrencia: el caché es seguro con `-race`.
- `make check`, `make check-packages`, `make compare-java` y las mutaciones
  completas.

## 4. Tests (primero)

En `dag_test.go`:

1. El DAG `[e_i]` = 2 `[e_{i+1}]/[e_{i+1}]`: valor 2 y adimensional
   (2·(v/v) = 2 por nivel) con n = 30 y n = 200, en `Canonical`, `Convert`,
   `CanonicalRat`, el servicio insensible, `ValidateInProperty` y `Multiply`.
   El costo se mide sin reloj: `testing.AllocsPerRun` de la primera llamada en
   un servicio nuevo con n = 50 y n = 100, cuya razón debe quedar cerca de 2
   (lineal); sin el caché es 2^50. También con `[pi]` como raíz del DAG: `New`
   termina (hoy 4.2 s a n = 22).
2. Un DAG con valores distintos por rama (`[d_i]` = 3 `[d_{i+1}].[d_{i+1}]`,
   `[d_n]` = 1 m) para n = 6: valor 3^(2^n − 1) m^(2^n), calculado a mano, que
   detecta un caché que devuelve el valor equivocado o comparte `Units`.
3. Aislamiento: canonicalizar `[d_0]2`, `[d_0]-1` y `[d_0]` en ese orden y en
   el inverso da lo mismo (un acierto que no copia `Units` ve el exponente
   multiplicado por la llamada anterior).
4. Equivalencia con el XML oficial: para cada unidad del XML, `Canonical` con
   el caché frío y caliente (dos servicios) da el mismo par.
5. Concurrencia: muchas goroutines canonicalizan el DAG y las unidades del XML
   a la vez (`go test -race`).
6. Los ciclos de 014 siguen dando el error de ciclo con el caché caliente
   (cargar primero una unidad sana que comparte definición).

## 5. Mutaciones previstas

- Sin el caché (vuelve a expandir): el test 1, por la razón de asignaciones.
- Guardar el puntero sin copiarlo: tests 2, 3 y 4 (y tests existentes).
- Un acierto que devuelve el puntero guardado sin copiar `Units`: test 3.
- Una clave que ignora el texto (p. ej. guarda por unidad y no por expresión, o
  clave constante): test 2.
- Guardar también los errores sin distinguir: test 6 si cambia el mensaje.
- Mutaciones completas.

## 6. Fuera de alcance

- Un caché de la forma canónica de los códigos del llamador (otro tamaño y otro
  tope: es entrada externa).
- Validar las definiciones al cargar (descartado el 2026-11-05).
- **El tamaño del valor** (§9): con `[d_i]` = 3 `[d_{i+1}].[d_{i+1}]` el caché
  quita los caminos, pero el racional dobla sus bits por nivel (n = 26: 9.8 s
  y 436 MB). Acotar los bits y exponentes que salen de las definiciones es
  otro frente.
- **El exponente de una definición desborda `int` sin error** (§9,
  preexistente): `[e_i]` = `[e_{i+1}]2` con n = 64 da `{1 1}` y con n = 63
  `m-9223372036854775808`. Un resultado equivocado sin error: frente propio,
  el siguiente.
- **Una cadena que termina en un ciclo** cuesta O(N²) en
  `ValidateInProperty` (N = 8000: 11.5 s), porque los errores no se guardan.

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.4.9. Una definición compartida por muchos caminos ya no se
expande una vez por camino.

## 9. Ronda adversarial del plan (2026-11-05)

Dos Opus independientes, cada uno con un prototipo del caché. El de (b), con
copia al guardar y al devolver: barrido de todas las unidades del XML, los dos
servicios, 13 formas por unidad y tres órdenes, `Canonical`, `CanonicalRat` y
propiedades (20 650 líneas), idéntico a `main` byte a byte; `-race` limpio; la
suite entera en verde. Asignaciones iguales en las rutas simples y entre 18 %
y 70 % menos en las especiales (`pH` 200 → 72, `B[W]` 245 → 71).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Copiar solo en el acierto no basta: el `can` guardado es el que mutan `can.Value = …` y `raiseCanonical` (`[e_0]2` sale mal en la primera llamada; 10 681 líneas del barrido difieren) | a, b (alta) | **Aceptado:** copia al guardar y al devolver (§2.3), mutación «guardar sin copiar» |
| El caché acota los caminos, no el tamaño del valor (`3 [d].[d]`: n = 26, 9.8 s y 436 MB) | a, b (media) | **Aceptado:** §3 y §8 limitados a los caminos; el tamaño, a §6 |
| Un tope de reloj en un test falla al azar con `-race` o en CI | a (media) | **Aceptado:** razón de `AllocsPerRun` entre n = 50 y n = 100 |
| El exponente de una definición desborda `int` sin error (n = 64 da `{1 1}`), preexistente | a, b (media) | **Aceptado como frente propio** (§6), el siguiente |
| Una cadena que termina en un ciclo cuesta O(N²) en `ValidateInProperty` | b (baja-media) | **Fuera de alcance** (§6) |
| `New` cuelga si `[pi]` es la raíz del DAG (`halfPiOfPi`: 4.2 s a n = 22) | b (baja) | **Aceptado:** test 1 |
| El resultado es función del texto (contexto fijo, `defParser`, `NativeUnit` antes de la clave); ciclos y errores no se esconden | a (confirmado) | Sin cambio |

## 10. Code review

### Ronda 1 (2026-11-05)

Dos Opus independientes sobre copias del repo. (a) verificó que nada muta un
`Canonical` guardado (`decimal.Decimal` no muta; `raiseCanonical` y
`can.Value = …` actúan sobre la copia), que la clave es completa y que la
medición por `Mallocs` es robusta (acumulativa, mínimo de dos). (b) comparó con
una copia sin el cambio: 611 mil líneas en nueve mundos (oficial sensible e
insensible, factor negativo, prefijo cero, cuatro mundos de ciclos) y tres
órdenes, byte a byte idénticas salvo un mundo de ciclos; `-race` limpio con 16
goroutines; el caché retiene 193 entradas y unos 64 KB; RSS de los tests igual;
asignaciones 18-69 % menos en las especiales, iguales en las simples.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El error de un ciclo nombra otra unidad y depende del orden: un acierto salta un subárbol sano y la profundidad se agota en otro lugar (504 de 1394 líneas del mundo `[a]` = `[x].[a]` difieren; en `main` el nombre, `[z]`, ya estaba fuera del ciclo) | b (baja) | **Confirmado:** la guarda pasa después del caché (`expandDefinition`); test `TestDagCycleErrorNamesAUnitOfTheCycle` en tres órdenes; K215 (guarda antes del caché) muere; §2 corregido |
| `Clone` sin mutación propia (compartir `Units` solo lo nota un test) | a (baja) | **Confirmado:** K214 muere |
| Un `Fatal` dentro del `sync.Once` deja la medición en cero y la razón en NaN: los tests de costo pasan callados | a (baja) | **Confirmado:** `dagRatio` da +Inf con denominador 0 |
| «Calientar»; un comentario más ancho que el resto | a (baja) | **Corregido.** Las líneas de código largas quedan: el linter del repo no las rechaza |

Mutaciones completas antes de la ronda: 249 de 249 `ok`. K192 (sin la guarda
de ciclos) sigue muriendo con la guarda movida.

### Ronda 2 (2026-11-05): converge

Dos Opus sobre el diff incremental. Ninguno deja hallazgos de severidad media
o mayor. (a) probó que la guarda después del caché sigue completa (un ciclo
nunca se guarda; solo un fallo de caché recursa; una cadena de unidades
distintas no llega a maxDepth + 1) y que el error nombra una unidad del ciclo
en general: al repetirse una unidad, los hermanos previos ya están en el
caché y la pila se vuelve periódica. Lo comprobó con cuatro formas (dos ciclos
que comparten un nodo, un ciclo por dos subárboles sanos, uno por la
referencia de `Cel`, uno largo con cola sana) en dos órdenes. (b) comparó con
la copia sin el cambio: 153 720 líneas por versión (XML oficial, factor
negativo, prefijo cero, los ciclos de tan, los mundos de 017 y uno propio con
ciclos por subárboles sanos; los dos servicios; frío, caliente y al revés),
idénticas salvo el nombre en los errores de ciclo, que ahora siempre es de un
ciclo (`main` nombraba `[z]`, `[sh]`, `[in_i]`, `[ft_i]`); `-race` limpio
mezclando unidades cíclicas y sanas. Llamar a N hojas que llegan al mismo
ciclo es cuadrático en `main` y aquí (N = 2000: 3.9 s en `main`, 2.2 s aquí),
ya en §6.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El comentario del test nuevo decía que con la guarda antes del caché el nombre dependía del orden: bajo K215 nombra `[x]` en los tres órdenes del test | a (baja) | **Corregido** el comentario |
