# 006 — La caché retiene la entrada de quien llama (#61)

**Estado:** cerrado (paso 9), sin commit. Plan aprobado el 2026-10-28, con `fix:`.

## 1. Revisión previa

### El defecto

`termCache.store` (`internal/engine/cache.go`) guarda como clave el `string`
que recibe, tal cual. Si es un trozo de un string más grande (`s[i:j]`),
comparte su arreglo de bytes, y la caché lo mantiene vivo mientras guarde la
entrada. El límite de v4.2.0 (`MaxCacheEntries`, 4096 por generación) cuenta
entradas, no bytes.

**Medido sobre `main` (`875c812`):** la reproducción del issue, 8 códigos de
6 bytes (`mg{r0}`…; el issue dice «10-byte») recortados de strings de 10 MB, retiene 79 979 984 bytes después de
`runtime.GC()`.

**Un camino que el issue no nombra:** `load` promueve una entrada de la
generación vieja llamando a `store(code, t)` con el `code` de quien consulta.
Esa clave también es suya, y retiene su entrada. El arreglo tiene que ir en
`store`, que cubre los dos caminos.

**Una segunda caché con el mismo defecto** (ronda adversarial, Opus y Fable):
`canonicalFormsOfProperty` (`internal/engine/property.go:94-118`) guarda en
`propertyForms` la clave `strings.ToLower(strings.TrimSpace(property))`.
`TrimSpace` devuelve un trozo de la entrada y `ToLower` devuelve el mismo
string si ya está en minúsculas, así que la clave es un trozo del string de
quien llama a `ValidateInProperty`, que es público. Solo se guardan
propiedades conocidas (101), pero cada clave puede retener un string de
cualquier tamaño, el del primero que preguntó, durante toda la vida del
servicio. Medido por los revisores: unos 10 MB retenidos por una propiedad
precedida de 10 MB de espacios.

**El resto no retiene nada** (verificado por los dos revisores):
- el AST (`internal/expr/ast.go`) guarda punteros al modelo y enteros; una
  anotación pasa a `&Factor{1}`;
- `defCache` (`engine.go`) tiene como claves textos del modelo;
- `ValidationError.Code` guarda el código de quien llama solo en el error
  devuelto, que no se cachea;
- el paquete `fhir` no tiene cachés: sus mapas salen de datos embebidos.

**`sync.Map` se queda con la última clave** (Go ≥ 1.24,
`internal/sync/hashtriemap.go:603-606` en Go 1.24.1 y 1.27.2: `swap` hace
`newEntryNode(key, new)`): guardar una clave que ya existe
crea una entrada nueva con la clave que llega. Hoy, una promoción o dos
goroutines que guardan el mismo código dejan la clave del último. Con la
copia, cada `store` guarda su propia copia, y la que pierde es basura.

### Especificación

No aplica: es un defecto de recursos, no de UCUM ni de FHIR. La regla que
gobierna es la del repo: «optimizar siempre los tests y las corridas:
memoria…», y el motivo de v4.2.0 para acotar la caché (entrada que el
servicio no puede rechazar).

### `gofhir/server` y otras implementaciones

- `server` delega en `ucum.Validate` (`ucum_validate.go:24`). No tiene caché
  propia que tomar. **Descartar.**
- **Ucum-java** no cachea el parseo, y desde Java 7u6 `substring` copia, así
  que no tiene el problema.
- **Go:** `strings.Clone` (Go 1.20; el módulo pide 1.24) copia el string.
  `unique.Make` (Go 1.23) interna, pero agrega una tabla global que no se
  necesita. **Tomar** `strings.Clone`.

### Costo

Hoy un `Validate` que acierta en la caché hace 0 asignaciones (medido con
`testing.AllocsPerRun`). La copia va solo al guardar: una asignación por
código nuevo o promovido, ninguna por acierto. Medido (benchmark de 100 000
códigos distintos, `main` contra el arreglo, Go 1.27.2): un código nuevo pasa
de 8 a 9 asignaciones (330 → 346 B), sin cambio de tiempo fuera del ruido.
Un acierto sigue en 0, con y sin `-race`, en Go 1.24, 1.25 y 1.27 (medido
por los revisores).

## 2. Alcance

1. `termCache.store` guarda `strings.Clone(code)`.
2. `canonicalFormsOfProperty` guarda `strings.Clone(key)` en
   `propertyForms`.
3. El comentario de `MaxCacheEntries` dice que el límite acota también la
   memoria: una clave guarda solo los bytes del código.

Fuera: cambiar la política de la caché o su límite.

**Versión:** `fix:` (parche). No cambia ningún resultado.

## 3. Criterio de salida

- `make check` en verde.
- Los tests de §4 pasan, y las mutaciones de §5 dan lo esperado.

## 4. Tests

1. **Interno, determinista** (`internal/engine`): después de una llamada con
   un trozo de un string grande, ninguna clave apunta dentro de ese string
   (`unsafe.StringData` y la longitud; aserción de tipo con dos valores, por
   `forcetypeassert`). Tres caminos:
   - guardar un código nuevo, mirando la generación joven;
   - promover: el código entra con un string propio, una rotación forzada
     (`count.Store(MaxCacheEntries + 1)` y `rotate()`, sin 4097 códigos) lo
     pasa a la generación vieja, se consulta con un trozo de un string
     grande, y se mira la **joven**: la vieja guarda todavía la clave
     propia;
   - `ValidateInProperty` con una propiedad recortada de un string grande,
     mirando `propertyForms`.
2. **Público, de punta a punta** (la reproducción del issue, más chica para
   no gastar memoria en los tests): 4 trozos de strings de 2 MB; lo retenido
   después de `runtime.GC()` queda bajo 1 MB. Hoy son unos 8 MB. Usa su
   propio `ucum.New()` y `runtime.KeepAlive`; ningún test del repo usa
   `t.Parallel`, así que otros tests no ensucian la medida (medido por los
   revisores: entre −40 KB y +11 KB con el arreglo, en tres versiones de Go
   y con `-race`).
3. **Costo:** un `Validate` que acierta sigue en 0 asignaciones.
4. **Corrección:** el código recortado sigue validando y canonicalizando
   igual que el mismo código escrito aparte.

## 5. Mutaciones

| # | Mutación | Debe fallar |
|---|---|---|
| C1 | `store` no copia | tests 1 (los dos caminos) y 2 |
| C2 | Se copia en `parseCached` y no en `store` | test 1, camino de promoción |
| C3 | Se copia en `load`, en cada consulta | test 3 |
| C4 | `propertyForms` guarda la clave sin copiar | test 1, camino de la propiedad |

## 6. Dependencias y excepciones

Ninguna dependencia nueva (`strings` y `unsafe` son de la biblioteca
estándar; `unsafe` solo en el test). Ninguna excepción a la regla 5.

## 7. Ronda adversarial

Opus y Fable, en paralelo, sin el razonamiento del plan.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| `propertyForms` (`property.go:94-118`) retiene el string de quien llama a `ValidateInProperty`; el plan no la veía | Opus, Fable (alta) | **Aceptado:** verificado leyendo el código. Entra al alcance (2), al test 1 y a la mutación C4 |
| `sync.Map` reemplaza la clave al guardar otra vez la misma | Fable (media) | **Aceptado:** verificado en `hashtriemap.go:603-606`. No cambia el arreglo; queda en §1 y el comentario no promete «una copia por entrada» |
| El camino de promoción necesita forzar la rotación y mirar la generación joven | Opus (media), Fable (baja) | **Aceptado:** test 1 dice cómo |
| El test 2 debe usar su propio servicio y `KeepAlive`, con un umbral y no contra 0 | Opus (baja) | **Aceptado:** ya era un umbral; se agrega lo demás |
| `AllocsPerRun` hace una corrida de calentamiento: no sirve para medir la asignación de una promoción | Fable (baja) | **Aceptado:** el test 3 mide solo el acierto, que es lo que mata C3 |
| Costo, versiones de Go, concurrencia | Opus, Fable (baja) | **Verificado, sin cambio:** cifras en §1 |

## 8. Tests e implementación (pasos 5 a 7)

**Tests primero.** Antes del arreglo fallan los tres caminos del test 1 y el
test 2 («the cache keeps 8386632 bytes»). Los tests 3 y 4 pasan, como
deben: documentan lo que no puede cambiar.

- `internal/engine/retention_test.go`: tests 1 y 3.
- `retention_test.go`: tests 2 y 4.

**Implementación:** `strings.Clone` en `termCache.store` y en el `Store` de
`canonicalFormsOfProperty`, más el comentario de `MaxCacheEntries`.

**Lo que cambió al validar:**
- La mutación C3 (copiar en cada consulta) sobrevivía con el código corto
  del plan (`mg/dL{x}`). El compilador deja en el stack la copia de un
  string corto que no escapa (`go build -gcflags=-m`: `make([]byte, …)
  does not escape`), así que no asigna. El test 3 usa ahora un código de
  71 bytes, y C3 muere («1 allocations, want 0»).
- Las mutaciones que sacan la copia la cambian por `strings.TrimSpace`, que
  devuelve un trozo del mismo string sin copiar. Sacarla del todo dejaba
  `strings` sin uso y no compilaba.

**Mutaciones** (`scripts/mutations.py`, K58 a K62):

| # | Plan | Mutación |
|---|---|---|
| K58 | C1 | `store` no copia (test 1) |
| K59 | C2 | se copia en `parseCached` y no en `store` (test 1, promoción) |
| K60 | C3 | se copia en `load`, en cada consulta (test 3) |
| K61 | C4 | `propertyForms` guarda la clave sin copiar (test 1, propiedad) |
| K62 | C1 | `store` no copia (test 2, de punta a punta) |

Suite completa: [evidence/006-2026-10-28-mutations.txt](evidence/006-2026-10-28-mutations.txt),
sin resultados inesperados.

**Costo de los tests nuevos** (binario de test compilado, `/usr/bin/time -l`):
- los de la raíz: 0,08 s y 19 MB de RSS;
- los internos: 0,01 s y 14 MB.

**Verificación:** `make check` en verde, y la suite con `UCUM_PACKAGES` en
verde.

## 9. Code review

### Ronda 1 (Opus y Fable, en paralelo, sin el razonamiento del plan)

Ninguno dejó hallazgos de severidad media o mayor: **converge**. Los dos
corrieron `make check` y la suite de mutaciones en sus copias sin resultados
inesperados. Corrieron los tests nuevos con `-race` en Go 1.24, 1.25 y 1.27
(Opus también con `-shuffle=on`), y revisaron todo el código que no es de
test buscando otra estructura que retenga la entrada: no hay. Fable inyectó
9 mutaciones propias: murieron todas, salvo dos equivalentes
(`string([]byte(code))`, `strings.Clone(code)[:len(code)]`).

| Hallazgo | Revisor | Resolución |
|---|---|---|
| El comentario de `MaxCacheEntries` acota la memoria solo junto con `MaxCodeLength` | Opus (baja) | **Aceptado:** lo nombra |
| «8 códigos de 10 bytes»: `mg{r0}` tiene 6 | Opus (baja) | **Aceptado:** §1 corregido (el error venía del issue) |
| La descripción de K62 no describe la mutación | Opus (baja) | **Aceptado** |
| Copiar la clave de la propiedad en cada consulta sobrevive | Opus (baja) | **Rechazado:** un acierto de `ValidateInProperty` ya hace 41 asignaciones y la mutación lo lleva a 42 (medido por Opus); no hay una garantía de costo que fijar ahí, a diferencia del acierto de la caché de parseo, que hace 0 |
| La cifra de costo era de los revisores, no medida por el autor | Opus (baja) | **Aceptado:** medida de nuevo, en §1 |
| El docstring de `mutations.py` no nombra el plan 006 | Fable (baja) | **Aceptado** |
| La cita de `hashtriemap.go` no dice la versión de Go | Fable (baja) | **Aceptado:** versión y línea citada |

Después de las correcciones: `make check` en verde, la suite con
`UCUM_PACKAGES` en verde y las mutaciones completas sin resultados
inesperados (evidencia actualizada en el mismo archivo).

## 10. Cierre

- `make check` y la suite con `UCUM_PACKAGES` en verde.
- Mutaciones: [evidence/006-2026-10-28-mutations.txt](evidence/006-2026-10-28-mutations.txt).
- Pendiente del usuario: el commit y el PR, que cierra #61.
