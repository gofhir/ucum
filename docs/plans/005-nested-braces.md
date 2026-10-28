# 005 — Llaves anidadas en una anotación (#65)

**Estado:** cerrado e integrado: PR #67, squash `cd0d2bd` (2026-10-28);
publicado en v4.3.3. Plan aprobado el 2026-10-28, con `fix:`.

## 1. Revisión previa

### Especificación

- **UCUM v2.2, §6■5** (`ucum-source.xml` del tag `v2.2`, línea 486):
  «Curly braces must not be nested.»
- **§6■1** (línea 464): «The full range of characters 33–126 can be used
  within a pair of curly braces», y ese rango incluye `{` (123).
- **BNF de §10** (línea 685): `annotation ::= "{" ANNOTATION-STRING "}"`, sin
  definir `ANNOTATION-STRING`.
- La suite oficial no tiene casos con `{` dentro de una anotación (medido
  sobre los 611 atributos `unit`, `srcUnit`, `dstUnit`, `u1` y `u2` de
  `UcumFunctionalTests.xml`).

**Lectura.** §6■1 y §6■5 tiran en direcciones distintas: el rango admite
`{`, la regla de anidamiento lo excluye. Que gana §6■5 es una decisión de
lectura, y se toma así:
- `}` cierra siempre: la primera `}` termina la anotación, así que nunca
  queda «dentro».
- Por eso una `{` antes de esa `}` solo puede leerse como la apertura de una
  segunda llave dentro de la primera, tenga o no su propia `}`. Aceptarla
  haría válido `m{a{b}`, que tiene una llave abierta dentro de otra.
- ucum-lhc 8.0.1 lee igual: su `VALID_ANNOTATION_REGEX`,
  `/^\{[!-z|~]*\}$/` (`source/unitString.js:69`), deja fuera el 123 y el 125
  (leído en el código, no ejecutado).
- La suite oficial no dice nada.

### Conducta actual (medida sobre `main`, y antes y después del cambio)

| Código | Hoy | Con el cambio |
|---|---|---|
| `m{a{b}` | válido | anidada, posición 3 |
| `{{}` | válido | anidada, 1 |
| `m{a{b}}` | inválido en 6 (`unexpected character '}'`) | anidada, 3 |
| `{{` | sin cerrar, 0 | anidada, 1 |
| `m{a{` | sin cerrar, 1 | anidada, 3 |
| `{a{b` | sin cerrar, 0 | anidada, 2 |
| `m{a{ b}`, `m{a{é}` | carácter inválido, 4 | anidada, 3 |
| `m{a}{b}` | inválido en 4 (otra regla, §10) | igual |
| `m{a}.s{b}`, `m{a}.{b}`, `{a}`, `m{}`, `{}` | válidos | válidos |

**Precedencia:** gana el primer byte que rompe una regla, leyendo de
izquierda a derecha. «Sin cerrar» solo cuando se llega al final sin
encontrar ninguno. Es la regla que ya sigue el lexer: el error está donde se
detiene la lectura.

Causa: `readAnnotation` (`internal/expr/lexer.go`) corta en el primer `}` y
admite cualquier byte de 33 a 126 antes, `{` incluido.

### Datos

Ningún código con `{` dentro de una anotación en `ucum-essence.xml`, en los
casos de Java (`testdata/java/cases.tsv`), en `fhir/ucum-common.tsv` ni en
los packages de ejemplos de R4, R4B y R5 (escaneo de `code` y `unit`). El
cambio no invalida ningún dato oficial, y la regla no depende de la versión
FHIR.

### `gofhir/server` y consumidores

`server` y `validator` delegan en `ucum.Validate`. **Descartar:** no hay
lógica propia. El impacto se limita a códigos escritos a mano.

### Otras implementaciones

- **Ucum-java 1.0.10**, por una sonda de un revisor con el jar fijado
  (`.cache/java/`): acepta `m{a{b}`, `{{}`, `{a{}`, `/{a{b}`, `(m{a{b})` y
  `canonical 1 m{a{b}` (`ok 1 m`). Rechaza `m{a{b}}` (posición 6) y `m{{a}}`
  (5), por el `}` que sobra, no por el anidamiento. Causa:
  `Lexer.checkAnnotation` (`Lexer.java:142-148`) avanza hasta el primer `}`
  y solo pide `isAsciiChar`. **No es evidencia del repo** hasta que lo grabe
  `make java-dump` (§2.2).
- **ucum-lhc 8.0.1** lo rechaza (ver «Lectura»).

### Decisión

**Mejorar** el lexer: `{` dentro de una anotación es un error con la
posición de esa llave. La diferencia con Java se graba con una corrida y se
clasifica con una clase propia.

## 2. Alcance

En este orden:
1. **Lexer y tests** (§4, tests 1 a 5): `readAnnotation` rechaza `{` con un
   mensaje propio, `nested curly braces in annotation`, en la posición de la
   llave interior. Godoc al día.
2. **Casos nuevos para Java**, agregados **al final** de `addSpecCases` para
   no renumerar los casos `spec:` existentes: `validate` de `m{a{b}`, `{{}`,
   `m{a{b}}`, `{a{}`, `/{a{b}` y `(m{a{b})`; `canonical 1 m{a{b}`.
   **El usuario corre `make java-dump`.** Mientras tanto, la rama queda roja
   (`TestJavaCasesFresh` falla) y no se commitea.
3. **Clase nueva** `java-accepts-nested-annotation`, `bug-java`, §6■5, con
   `JavaRef` a `Lexer.java:142-148`. Mismo esquema que
   `java-accepts-space-in-annotation`: Java `ok`, nosotros `invalid` o `err`
   con `ValidationError`, en un caso con una `{` dentro de llaves. No va a
   `javaSyntaxBugs`: esa clase toma `Args[0]` y solo `validate`.
4. **La clase de 004 no toma estos casos:** `java-accepts-space-in-annotation`
   exige además que no haya una `{` dentro de llaves antes del espacio. Si
   no, `m{a{ b}` caería en las dos clases, y el arnés falla.
5. Issue 6 en `002-java-issues.md`, solo el md, con lo que grabe la corrida.
   No dice que Java acepte `m{a{b}}`.
6. **Oráculos de 004:** `annotationOutsideRange` (`fhir/examples_test.go`)
   pasa a llamarse `annotationBreaksSection6` y suma la regla de §6■5; la
   invariante del fuzz también.

Fuera:
- La anotación vacía (`{}`): la BNF no define `ANNOTATION-STRING`. Sigue
  válida.
- Cualquier otra regla de §6.

## 3. Versión: decisión del usuario

Igual que en 004, hay dos precedentes: v3.0.0 trató como breaking «codes that
were accepted and should not have been now fail», y 4.3.2 lo hizo como
parche. **Recomendación:** `fix:` (parche). Ningún dato oficial cambia, y el
anidamiento nunca fue válido.

## 4. Criterio de salida

- `make check` en verde, y la suite con `UCUM_PACKAGES`.
- `TestCompareJava` sin diferencias sin clasificar, con los casos nuevos
  grabados por Java.
- Las mutaciones de §6 dan el resultado esperado.

## 5. Tests

Los esperados salen de §6■5 y de la precedencia de §1.

1. **Validación y posición:** las siete de la tabla de §1 que pasan a
   «anidada» más `m{a{b}}`, `{a{}`, `m{{a}}`, `/{a{b}`, `(m{a{b})`, con
   `ValidationError` y `Offset` = índice de la llave interior (3, 2, 2, 3,
   4).
2. **Precedencia:** `{{`, `m{a{`, `{a{b`, `m{a{ b}` y `m{a{é}` dan
   anidamiento en la posición de la tabla, no «sin cerrar» ni «carácter
   inválido».
3. **Mensaje:** el error nombra el anidamiento.
4. **Sin cambio:** `m{a}{b}` sigue rechazado en 4; `m{a}.s{b}`, `m{a}.{b}`,
   `{a}`, `m{}`, `{}` siguen válidos.
5. **Las demás entradas:** `ConvertRat` (origen, destino, ambos) da
   `ConversionError`; `Multiply` y `Divide` dan error.
6. **Control de la clase nueva:** test directo de su `match` con `javaDiff`
   construidos, como el de 004: `validate` y `canonical` que deben entrar;
   controles negativos (Java no `ok`, `What` no es `status`, otro error, una
   `{` después de una anotación cerrada como `m{a}.{b}`), cada uno con todo
   lo demás cumplido. Más un control de que la clase de 004 no toma
   `m{a{ b}`.
7. **Oráculo:** `TestAnnotationOutsideRange` (con el nombre nuevo) suma
   `m{a{b}` inválido y `m{a}.{b}` válido.
8. **Fuzz:** si `Validate` acepta, no hay `{` entre una `{` y su `}`. Es
   una invariante barata, no evidencia, y no cuenta en la tabla de §6.

## 6. Mutaciones (en una copia, en `scripts/mutations.py`)

| # | Mutación | Debe fallar |
|---|---|---|
| N1 | El lexer vuelve a aceptar `{` dentro | tests 1, 5 |
| N2 | El error informa `start` en vez de `l.index` | test 1 |
| N3 | El anidamiento solo cuenta si el código termina en `}` (una forma de «revisar después de encontrar la `}`») | tests 1 y 2 |
| N4 | El escáner del oráculo ignora `{` | test 7 |
| N5 | La clase nueva no toma ningún caso | `TestCompareJava` |
| N6 | La clase nueva toma cualquier `{`, también fuera de llaves | test 6 |
| N7 | La clase nueva no exige Java `ok` | test 6 |
| N8 | La clase nueva no exige `What == status` | test 6 |
| N9 | La clase de 004 pierde su guarda de anidamiento | test 6 |

## 7. Dependencias y excepciones

Ninguna dependencia nueva. Ninguna excepción a la regla 5.

## 8. Ronda adversarial del plan

Revisores: Opus y Fable, en paralelo, sin el razonamiento del plan.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Cambian errores que el plan no listaba (`{{`, `m{a{`, `{a{b`, `m{a{ b}`, `m{a{é}`), y no decía qué regla gana | Opus (alta) | **Aceptado:** precedencia en §1, test 2, mutación N3 |
| Los casos nuevos de Java dejan la rama roja hasta `make java-dump` | Opus (alta), Fable (media) | **Aceptado:** orden explícito en §2; no se commitea en rojo; casos al final de `addSpecCases` |
| `javaSyntaxBugs` no sirve (toma `Args[0]`, solo `validate`) | Opus, Fable (media) | **Aceptado:** clase propia (§2.3) |
| Falta el test directo de la clase; sobrevivirían las mutaciones de sus guardas | Opus, Fable (media) | **Aceptado:** test 6, N6 a N8 |
| La lectura de §6■5 contra §6■1 estaba afirmada, no argumentada | Opus (media), Fable (baja) | **Aceptado:** «Lectura» en §1 |
| La conducta de Java se puede medir ya con el jar en caché | Fable (media) | **Aceptado en parte:** la sonda queda en §1 como dato previo; la evidencia del repo es la corrida de `make java-dump`, que corre el usuario |
| La clase de 004 tomaría `m{a{ b}` | Opus (baja) | **Aceptado:** §2.4, N9 |
| La versión no se hereda de 004 | Opus (baja) | **Aceptado:** §3 |
| La invariante del fuzz no cuenta como evidencia | Opus (baja) | **Aceptado:** test 8 |
| `Multiply`/`Divide` no estaban en los tests | Fable (baja) | **Aceptado:** test 5 |
| El nombre del oráculo dejaría de ser cierto | Fable (baja) | **Aceptado:** se renombra |

## 9. Tests e implementación

- **Tests primero:** `TestNestedBraces` y `TestNestedBracesEverywhere`
  fallaron sobre `main`; `TestNestedBracesUnchanged` y el oráculo
  (`TestAnnotationBreaksSection6`) pasaron, como corresponde.
- **Lexer:** con el cambio, la suite completa con `UCUM_PACKAGES` en verde.
- **Casos para Java:** 7 al final de `addSpecCases` (6 `validate` y 1
  `canonical`). `TestJavaCasesFresh` falla hasta la corrida: 10887 casos
  versionados, el generador da 10894.
- **Corrida de Java** (`make java-dump`, la corrió el usuario): agregó 7
  líneas a `cases.tsv` y al resultado, sin renumerar nada. Java acepta
  `m{a{b}`, `{{}`, `{a{}`, `/{a{b}`, `(m{a{b})` y da `1 m` para el canónico de
  `1 m{a{b}`; rechaza `m{a{b}}` en la posición 6, por la `}` que sobra. Es lo
  mismo que había medido la sonda de §1.
- **Clases:** `java-accepts-nested-annotation` y la de 004 comparten
  `javaAcceptsAnnotation`. Cada una toma solo los casos cuya primera regla
  rota, leyendo de izquierda a derecha, es la suya (`annotationViolation`).
  Con eso, la guarda de §2.4 no es una condición aparte: `m{a{ b}` es de la
  clase nueva y `m{a b{c}` de la de 004. Test directo:
  `TestJavaClassNestedAnnotation`, con la frontera entre las dos.
- **Mutaciones:** K42 a K48 reescritas para el predicado compartido (N7 y N8
  son K46 y K47); K50 a K56 son N1 a N6 y N9. Suite completa:
  [evidence/005-2026-10-28-mutations.txt](evidence/005-2026-10-28-mutations.txt).
- Issue 6 en `002-java-issues.md`; una frase en el README.
- `make check` y la suite con `UCUM_PACKAGES` en verde; fuzz de
  `FuzzValidate` 30 s, sin fallas.

## 10. Code review

### Ronda 1 (Opus y Fable, en paralelo, sin el razonamiento del plan)

Ninguno dejó hallazgos de severidad media o mayor: **converge**. Los dos
corrieron `make check` y la suite de mutaciones en sus copias sin resultados
inesperados, y sumaron mutaciones propias (9 de Opus, 10 de Fable). Todas
murieron salvo las equivalentes y la del fuzz, que el plan ya no cuenta
como evidencia (test 8).

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Los escáneres leen una `{` dentro de `[...]` como anotación; el lexer no (§5) | Opus (baja) | **Aceptado** en el de la clase, que decide la atribución: lee los corchetes como el lexer; control `[x{]{a b}` y K57. En el oráculo del ValueSet y en el fuzz no cambia nada: ningún dato tiene llaves dentro de corchetes, y un código así no pasa el lexer |
| `javaAcceptsAnnotation` aplica la regla por unidad, no por caso; el comentario decía otra cosa | Opus, Fable (baja) | **Aceptado:** comentario corregido; control `convert`. Un caso con las dos reglas en unidades distintas lo detecta el arnés («more than one class») |
| K52 no es N3 tal como estaba escrito | Opus (baja) | **Aceptado:** la tabla de §6 describe K52 como es |
| Poca variedad: unos 17 códigos a mano | Opus (baja) | **Aceptado:** `TestNestedBracesSweep`, 70 códigos generados (10 contextos × 7 posiciones), también como destino de una conversión; en K50 |
| La cuenta de la suite («575 `unit=`») no se reproducía | Fable (baja) | **Aceptado:** 611 atributos de unidad, medidos con `grep` |
| «unchanged on `master`» del issue 6 sin evidencia propia | Fable (baja) | **Aceptado:** `checkAnnotation` leído en `master` el 2026-10-28, en las mismas líneas 142-148; anotado en el issue |
| `nested_test.go` aparte de `annotation_test.go` | Fable (baja) | **Rechazado:** cada incremento lleva sus tests en un archivo propio, como 004; los dos están en la raíz, uno junto al otro |

Después de las correcciones: `make check` en verde, la suite con
`UCUM_PACKAGES` en verde y las mutaciones completas sin resultados
inesperados (evidencia actualizada en el mismo archivo).

## 11. Cierre

- `make check` y la suite con `UCUM_PACKAGES` en verde, incluida la
  comparación con Java (`TestCompareJava`, sin diferencias sin clasificar).
- Mutaciones: [evidence/005-2026-10-28-mutations.txt](evidence/005-2026-10-28-mutations.txt).
- PR #67, commit `bbd5112`, que cierra #65.
