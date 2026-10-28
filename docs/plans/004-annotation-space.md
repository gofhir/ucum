# 004 — Espacio dentro de una anotación (PR #52)

**Estado:** cerrado (paso 9), sin commit de lo propio. Plan aprobado por el usuario el
2026-10-28, con `fix:` (§3). Origen: PR #52 de
kaptinlin, commit `f1a7ba2`, que cambia el límite inferior de
`readAnnotation` de `0x20` a `0x21`.

## 1. Revisión previa

### Especificación

- **UCUM v2.2, §6■1** (`ucum-source.xml` del tag `v2.2`, línea 464): «The
  full range of characters 33–126 can be used within a pair of curly braces».
  El espacio es el 32: queda fuera.
- **§6■5** (línea 486): «Curly braces must not be nested.»
- **§3, comentario** (línea 366): «White space is not recognized in a unit
  term and should generally not occur. UCUM implementations may flag
  whitespace as an error rather than ignore it.»
- **El rango es exhaustivo**, aunque §6■1 diga «can be used»: la suite
  oficial lo lee así en 1-115a («Unicode is not allowed»), y Java corta en
  `'~'` (126).
- La suite oficial (`UcumFunctionalTests.xml`) no tiene casos con espacio
  dentro de llaves.
- **FHIR:** el ValueSet `ucum-common` (`draft`, «for purposes of helping
  populate look ups») trae los mismos 11 códigos con espacio en R4 4.0.1 y en
  R4B 4.3.0 (`{# of fetuses}`, `{3 times}/d`, `{clock time}`,
  `%{Negative Control}`…). R4 los repite en `Bundle-valueset-expansions.json`.
  R5 5.0.0 no tiene ninguno: bajó de 1364 a 848 conceptos. En los ejemplos,
  solo R5 `Ingredient-example` usa uno, `{delivered dose}`, dos veces. El
  `fhir/ucum-common.tsv` del repo es el de R5.

### `gofhir/server` (congelado) y consumidores

- **`server`** delega en `ucum.Validate`
  (`internal/terminology/ucum/ucum_validate.go:24`) y carga los conceptos de
  `ucum-common` para los displays (`ucum_loader.go`), sin validarlos.
  **Descartar:** no hay lógica propia que tomar. Con R4, un
  `$validate-code` de los 11 códigos daría `false` aunque estén en el
  ValueSet que carga.
- **`validator`** (`pkg/ucumvalidator/ucumvalidator.go:110`) llama a
  `ucum.Validate` sobre el código de cada Quantity: contenido R4/R4B con esos
  11 códigos pasa a dar error. El validador de HL7 (Ucum-java) lo acepta.
- **Índices persistidos:** un `CanonicalKey` guardado de `{delivered dose}`
  deja de poder recalcularse.

Los dos repos están fijados en v4.2.0: el cambio les llega al subir de
versión.

### Otras implementaciones

- **Ucum-java 1.0.10:** acepta el espacio. Resultado de la referencia:
  `internal/engine/testdata/java/ucum-java-1.0.10.tsv`, casos `fhir:00346`
  (`validate`, `ok`) y `fhir:00347` (`canonical`, `ok 1`), los dos con
  `{delivered dose}`. Causa: `Lexer.java:142-148` (`checkAnnotation`) valida
  con `Utilities.isAsciiChar`, `Utilities.java:87-88`:
  `ch >= ' ' && ch <= '~'` (commit `db9d22a`, tag 1.0.10). Es una
  comprobación de ASCII, no la regla de §6■1. Nuestro `0x20` era la
  traducción literal de esa línea (`lexer.go` es un port de `Lexer.java`).
  Los propios tests de Java escriben `{clock_time}`
  (`UcumCommonUnitsTest.java:270`), no el `{clock time}` de R4.
- **ucum-lhc 8.0.1** (último tag): lo rechaza.
  `source/unitString.js:69`, `VALID_ANNOTATION_REGEX = /^\{[!-z|~]*\}$/`,
  que empieza en `!` (33) y excluye `{` y `}`. No se ejecutó: el modo
  automático no dejó instalar el paquete de npm. La regla queda verificada
  leyendo el código.

### Decisión

**Tomar** el cambio de #52 y sus tests. **Mejorar:** clasificar la
diferencia con Java con su causa en el código, cubrir los datos FHIR de las
tres versiones con un oráculo independiente del lexer, fijar los bordes y la
posición del error. Lo de Java es un defecto de Java (§6■1).

## 2. Alcance

Dentro:
1. `readAnnotation` rechaza todo byte fuera de 33–126 (el commit de #52).
2. **Clase nueva** en `javaclasses_test.go`:
   `java-accepts-space-in-annotation`, `bug-java`, §6■1, con `JavaRef` a
   `Lexer.java:142-148` y `Utilities.java:87-88`. Cubre `validate` y las
   demás operaciones: Java `ok` y nosotros `invalid` o `err`, en un caso cuyo
   código tiene un byte 0x20 dentro de llaves. No se extiende
   `javaSyntaxBugs`: esa clase es de la gramática (§7, §10), solo mira
   `validate` y lista expresiones fijas. Esta es del juego de caracteres de
   §6■1. Sin la clase, `TestCompareJava` falla con 2 diferencias sin
   clasificar (medido sobre `main` + #52).
3. **Issue 5** en `002-java-issues.md` (solo el md, sin publicar).
4. **Observación a HL7** (principio 10.4), en [hl7-issues.md](hl7-issues.md).
   No se publica.
5. Godoc de `readAnnotation` y README, si nombran el rango de caracteres.
6. `fhir/doc.go`: anotar que regenerar `ucum-common.tsv` desde la versión R4
   del ValueSet haría fallar `TestCommonUnitsAreValidUCUM` por estos 11
   códigos.

Fuera, como hallazgos registrados:
- **Llaves anidadas:** `m{a{b}` se acepta hoy (y en Java), contra §6■5. Es
  otra regla, con su propio caso: va a un issue nuevo, no a este incremento.
- **Anotación vacía** (`{}`, `m{}`): la BNF no define `ANNOTATION-STRING`
  (línea 685). No cambia.
- **Recortar o normalizar el espacio:** el comentario de §3 permite marcarlo
  como error, no exige ignorarlo. No se «arregla» la entrada.
- **`server`:** está congelado.

## 3. Versión: decisión del usuario

Hay dos precedentes en el repo:
- **v3.0.0** (`CHANGELOG.md:191`, #28): «Codes that were accepted and should
  not have been now fail» fue un cambio que rompe compatibilidad, y llevó a
  un major.
- **v4.3.2** (#62): resultados que no daban error y ahora lo dan, como
  `fix:` de parche.

Opciones:
- **`fix:`** (4.3.3). La nota del release nombra el cambio, el impacto en
  `validator` y `server` con R4/R4B, y la reindexación de `CanonicalKey`.
- **`fix!:`** (v5). Cambia la ruta de importación por un carácter.

**Recomendación:** `fix:`, con la nota explícita. Un major por un byte que
la especificación nunca admitió obliga a todos los consumidores a cambiar el
import. El precedente de v3 venía junto con otros cambios de API.

## 4. Criterio de salida

- `make check` en verde.
- `TestCompareJava` sin diferencias sin clasificar, y los dos casos de
  `{delivered dose}` en la clase nueva.
- Las mutaciones de §6 dan el resultado esperado.

## 5. Tests

Los esperados salen de §6■1, no de correr el código.

1. **Bordes del rango:** 0x1F y 0x20 inválidos, 0x21 `!` y 0x7E `~`
   válidos, 0x7F inválido. Basta un servicio: los dos comparten el lexer.
2. **Posición:** espacio al inicio, al medio y al final de la anotación, una
   anotación sola (`{ }`), dentro de un denominador (`/{a b}`) y en un
   término agrupado (`(m{a b})`). Todos dan `ValidationError` y la aserción
   incluye el `Offset`, que es el índice del espacio (§ medido por los
   revisores: `{a }` → 2, `(m{a b})` → 4, `%{Negative Control}` → 10).
3. **Conversión:** origen, destino y ambos dan `ConversionError` (de #52).
4. **ValueSet `ucum-common` de las tres versiones**, leído del package
   oficial con su SHA-256 (`ValueSet-ucum-common.json`, con un lector nuevo
   en `internal/fhirexamples`). El oráculo es independiente del lexer: un
   escaneo de bytes del código. Un concepto es inválido si y solo si tiene un
   byte fuera de 33–126 entre `{` y `}`; en los datos, todos esos bytes son
   espacios. Se fija además la cuenta: 11 en R4, 11 en R4B,
   0 en R5.
5. **`TestOfficialExamples`** (`fhir/examples_test.go`): hoy los códigos
   inválidos solo se registran como «skipped». Se fija el conjunto esperado
   por versión: R4 {`TAB`, `drop`, `ea`, `mEq`, `mcg`, `ml/hr`, `month`,
   `tbl`}, R4B ∅, R5 {`{delivered dose}`}. El plan decía R4 ∅, como afirmó
   un revisor sin medirlo: los ocho de R4 ya eran inválidos antes de este
   cambio. Ninguno es un átomo de UCUM, y Ucum-java también los rechaza
   (casos `fhir:00038`, `00120`, `00122`, `00160`, `00188`, `00241`, `00260`
   y `00266`).
6. **Control de la clase:** test directo de su `match` con `javaDiff`
   construidos (`validate` y `canonical`, con y sin 0x20). No se fabrica una
   corrida de Java: `loadJavaRun` verifica el SHA-256 de los archivos de la
   corrida contra `manifest.json`.
7. **Fuzz:** en `FuzzValidate`, invariante barata: si `Validate` acepta un
   código, ningún byte entre `{` y `}` está fuera de 33–126. Repite la regla
   del lexer; no cuenta como evidencia independiente.

## 6. Mutaciones (en una copia)

| # | Mutación | Debe fallar |
|---|---|---|
| M1 | Límite inferior vuelve a `0x20` | tests 1, 2, 4, 5 |
| M2 | Límite inferior `0x22` | test 1 (`!`) |
| M3 | Límite superior `0x7D` | test 1 (`~`) |
| M4 | La clase no exige el byte 0x20 | test 6 |
| M5 | Sin la clase nueva | `TestCompareJava` |
| M6 | El lector del ValueSet devuelve 0 conceptos | test 4 (cuentas) |
| M7 | El error informa `start` en vez de `l.index` | test 2 |

## 7. Dependencias y excepciones

Ninguna dependencia nueva. Ninguna excepción a la regla 5: el rango sale de
§6■1, prosa de la especificación citada en el código.

## 8. Integración

El cambio de #52 entra con su autoría (`f1a7ba2`). Lo que agrega este plan
va en commits propios encima, en un PR nuestro que lo incluye, y #52 se
cierra apuntando a ese PR. Si la rama del fork admite cambios del
mantenedor, se puede usar esa rama en su lugar.

Los 11 códigos R4 nunca llegaron a Java: `javacases_test.go:273` lee el
`ucum-common` de R5. Agregarlos exige `make java-dump`, que corre el
usuario. No es necesario para el criterio de salida.

## 9. Observación a HL7 (sin publicar)

Registrada como observación 1 de [hl7-issues.md](hl7-issues.md). El usuario
decidió no publicarla (2026-10-28).

## 10. Ronda adversarial del plan

Revisores: Opus y Fable, en paralelo, sin el razonamiento del plan. El
primer intento de Fable falló por un error de red; el segundo terminó.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| El cambio rompe `validator` y los índices persistidos, no solo `server` | Opus (alta) | **Aceptado:** §1 y §3 |
| Semver: v3.0.0 trató lo mismo como breaking | Fable (media) | **Aceptado:** §3, decisión del usuario |
| §6■5 prohíbe anidar llaves; el motivo dado para `{` era falso | Opus, Fable (media) | **Aceptado:** §1 y §2, hallazgo aparte |
| La clase no cubre `canonical` (estado `err`) | Opus, Fable (media) | **Aceptado:** §2.2 |
| Falta la causa en el código de Java (`JavaRef`) | Opus, Fable (media) | **Aceptado:** §1, verificado en `db9d22a` |
| El control sintético de M4 no se puede hacer con la corrida sellada | Fable (media) | **Aceptado:** test 6 |
| Test 4 tomaba el esperado de `main` | Opus (baja), Fable (media) | **Aceptado:** oráculo de bytes y cuentas fijas |
| `TestOfficialExamples` deja pasar la regresión | Opus, Fable (media) | **Aceptado:** test 5 |
| `fhirexamples.Read` no lee conceptos de ValueSet | Opus, Fable (baja) | **Aceptado:** lector nuevo |
| Falta la mutación de la posición | Opus (baja) | **Aceptado:** M7 |
| El borrador para HL7 no estaba en el plan | Opus (baja) | **Aceptado:** §9 |
| `fhir/doc.go` debe advertir sobre la versión R4 | Opus (baja) | **Aceptado:** §2.6 |
| La lectura de «can be used» como exhaustiva debe justificarse | Fable (baja) | **Aceptado:** §1 |
| El test 1 en los dos servicios es redundante | Fable (baja) | **Aceptado:** un servicio |
| La propiedad de fuzz repite la regla | Fable (baja) | **Aceptado:** se mantiene, rotulada como invariante |
| Ejecutar ucum-lhc | Opus (baja) | **Rechazado:** la regla de §6■1 basta, y la lectura del código está citada; ejecutarlo queda a criterio del usuario |

## 11. Tests, implementación y validación

- **Tests primero:** los tests 1 a 6 fallaron sobre `main` (con el
  conjunto de R4 ya corregido), por los motivos previstos: los 11 códigos de
  R4/R4B, `{delivered dose}` en R5 y la clase que todavía no existía.
- **Implementación:** el commit de #52, con su autoría (`eea1627`, un
  cherry-pick de `f1a7ba2`); la clase `java-accepts-space-in-annotation`;
  `fhirexamples.ValueSetCodes`; la nota de `fhir/doc.go`; un párrafo del
  README; el issue 5 de `002-java-issues.md`.
- **Mutaciones:** K38 a K45 en `scripts/mutations.py` (las M1 a M7 de §6;
  M1 en dos, una por paquete). La primera versión de M5 renombraba la clase,
  que seguía clasificando: sobrevivió por un defecto de la mutación, no del
  oráculo. Corregida para que la clase no tome ningún caso. Suite completa,
  con las de 001 a 003:
  [evidence/004-2026-10-28-mutations.txt](evidence/004-2026-10-28-mutations.txt).
- **`make check`** en verde; la suite con `UCUM_PACKAGES` también. Fuzz de
  `FuzzValidate`, 30 s, con la invariante del test 7: sin fallas.

## 12. Code review

### Ronda 1 (Opus y Fable, en paralelo, sin el razonamiento del plan)

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Dos controles negativos de la clase («java rejects too», «not a status difference») tenían nuestro estado en `ok`, así que los rechazaba otra condición: sobrevivían quitar el guard de `What` o el de Java `ok` | Opus (media) | **Confirmado** leyendo el test y por sus mutaciones. Los controles cumplen todo lo demás; K46 y K47 |
| Quitar `case '}'` del escáner de la clase y del oráculo de bytes sobrevive: ningún control tiene un espacio después de una anotación cerrada | Opus (baja) | **Aceptado:** control `m{a}.s b` en la clase y `TestAnnotationOutsideRange` para el oráculo; K48 y K49 |
| El issue 5 afirmaba que ucum-lhc rechaza sin decir que no se ejecutó; «Both should be errors» no vale para `validate` | Opus (baja) | **Aceptado** |
| El plan decía «byte 0x20» y el código «fuera de 33–126» | Opus (baja) | **Aceptado:** el plan dice lo del código |
| Reflujo de la documentación del paquete `fhirexamples`; comentario interno de #52 en inglés | Opus (baja) | **Aceptado** |
| Una versión sin entrada en `wantInvalidExamples` pasaba en silencio | Fable (baja) | **Aceptado:** `t.Fatalf` si falta |
| El comentario del test 4 atribuía el «solo si» a §6■1 | Fable (baja) | **Aceptado:** el comentario separa la regla de la propiedad de los datos |
| Tres escáneres de bytes casi iguales | Fable (baja) | **Rechazado:** están en tres paquetes de test; compartirlos exige un helper interno nuevo, y cada uno es el oráculo de su test, probado con su mutación |
| `TestExternalAnnotationCharacters` (#52) y los tests 1-3 se solapan | Fable (baja) | **Rechazado:** el de #52 es lo único que fija el tab (mutación X5 de Fable); se conserva con su autoría |

Fable corrió además X1 a X8: las dos operaciones de Java las toma la clase
nueva y ninguna otra, y X6 (leer solo el primer `include`) es equivalente
porque cada recurso tiene uno.

Después de las correcciones: `make check` en verde, suite con
`UCUM_PACKAGES` en verde y las mutaciones completas sin resultados
inesperados (evidencia actualizada en el mismo archivo).

### Ronda 2 (Opus y Fable, solo con los hallazgos anteriores y el delta)

Sin hallazgos. Los dos repitieron las mutaciones que sobrevivían en la
ronda 1 (ahora K46 a K49, todas muertas) y comprobaron que cada control
negativo lo rechaza solo la condición que nombra. En sus copias, `make check`
y la suite de mutaciones pasaron sin resultados inesperados. **Converge.**

## 13. Cierre

- `make check` en verde; suite con `UCUM_PACKAGES` en verde, incluida la
  comparación con Java (`TestCompareJava`, sin diferencias sin clasificar).
- Mutaciones: [evidence/004-2026-10-28-mutations.txt](evidence/004-2026-10-28-mutations.txt).
- Pendiente del usuario: el commit, el PR (con `eea1627` de kaptinlin) y
  cerrar #52 apuntando a él; el issue aparte de llaves anidadas (§6■5);
  ejecutar ucum-lhc o no. La observación a HL7 no se publica.
