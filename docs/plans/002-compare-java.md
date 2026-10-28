# 002 — Comparación ejecutable contra Ucum-java

**Estado:** cerrado e integrado el 2026-10-27 (#60, squash `30048dc`; plan aprobado el 2026-10-26). **Fecha:** 2026-10-26.

## Motivo
La carta (§10, reglas de evidencia) pide que toda comparación contra la
referencia sea código del repo, con su arnés, la versión de la referencia y
un comando que la reproduzca. Hoy la tabla «Differences from the Java
reference» del README se sostiene leyendo el código de Java («verified by
reading its source»); nada la reproduce ni detecta si Java cambia.

## 1. Revisión previa

**Versiones verificadas el 2026-10-26** (SHA-256 contra upstream):
- `internal/essence/ucum-essence.xml` = `ucum-org/ucum` v2.2 = el que trae
  embebido `ucum-1.0.10.jar` (`dfccea1b…`). Los datos son idénticos, así que
  toda diferencia será del motor.
- `internal/engine/testdata/UcumFunctionalTests.xml` = el de `FHIR/Ucum-java`
  (último cambio 2025-04-22, `e7d054e`), que es el que carga
  `UcumFunctionalTests.java`. `UcumFunctionalTests.2.xml` es una variante
  vieja que Java no usa.
- ValueSet `ucum-common` 5.0.0 = hl7.org/fhir (848 conceptos).

**Referencia:** `org.fhir:ucum:1.0.10` (Maven Central, 2025-04-22), la última.
JAR de 70 KB sin dependencias de runtime (solo de test en el pom). SHA-1
publicado `85b23fb0…` verificado; SHA-256 calculado
`784fb5ba7d74726962fb67624d0f98231e513124a83507a09c2862bbc2b4f3bd`. Java 24 y
21 instalados (`javac` disponible; no hay Maven, no hace falta).

**API de Java que se comparará** (`UcumService.java`): `validate`,
`getCanonicalForm(Pair)`, `convert`, `isComparable`, `multiply`, `divideBy`.

| Fuente | Qué hace | Decisión |
| --- | --- | --- |
| `gofhir/server` | No compara contra Java | — |
| `go-searchindex/scripts/server-diff.sh` y `testdata/expected-gaps.json` | Script que corre el dumper de la referencia a `.cache/`, test Go con variable de entorno que compara y clasifica contra un archivo versionado de diferencias con su motivo | **Tomar** el patrón; **mejorar**: la salida de la referencia se versiona, así la comparación corre en `go test` y en CI sin Java |

**Sonda del 2026-10-26** (`scratchpad/java/probe/Probe.java`, corrida por el
usuario; Java ejecutado fuera de la sesión porque el permiso de ejecutar
código externo lo tiene el usuario):

| Entrada | Java 1.0.10 | Nosotros |
| --- | --- | --- |
| `0.07 mg/dL` canónico | `0.7 g.m-3` | igual |
| `7.4 [pH]` | `4456384162400000000000000000 m-3` (= 7,4 × 6,022e26: lineal) | 10^-7,4 mol/l ≈ 2,4e19 m-3 |
| `1 B[V]` | `1000 C-1.g.m2.s-2` (factor 1, sin logaritmo) | 10^0,5 V ≈ 3162,3 |
| `100 [degF]`, `37.5 Cel` | `UcumException: Not handled yet (special unit with offset from 0 at intersect)` | 310,93 K, 310,65 K |
| `0.000123 /min` | `0.00000205000000000000000000000000000041 s-1` | `0.00000205` exacto |
| `1 [IU]` | código canónico vacío | `[IU]` (dimensión propia, UCUM §24) |
| `validate('')` | válido | inválido (suite, caso 1-103) |
| `validate('10*3.(Cel)')` | válido | válido (limitación conocida del README) |
| `convert 6.3 s/m/mg → s.m-1.g-1` | `6300` | igual (suite 3-111) |

Formato: `Decimal.asDecimal()` da un decimal sin exponente, que se lee exacto
con `big.Rat.SetString`; el código canónico usa la misma sintaxis que el
nuestro (`g.m-3`, `C-1.g.m2.s-2`, cadena vacía para adimensional).

## 2. Plan (ajustado tras la ronda adversarial, §3)

Las citas de Ucum-java van fijadas al commit `db9d22ad` (tag 1.0.10).

### Tres fuentes por caso
Cada caso tiene el resultado de Java, el nuestro y, donde exista, un
**esperado independiente de los dos motores**:
- la suite oficial (`UcumFunctionalTests.xml`);
- `UcumCommonUnitsTest.java` de Ucum-java (954 unidades con el canónico que
  esperan sus autores, incluidos los que saltan con «BUG?»), descargado del
  commit fijado con SHA-256;
- una regla de la especificación calculada desde `ucum-essence.xml` para las
  unidades especiales (función de cada unidad) y para los offsets (UCUM §30,
  §43).

Una diferencia se acepta solo si **los dos lados** quedan explicados: el valor
de Java coincide con el modelo de Java de su clase **y** el nuestro coincide
con el esperado independiente. Una clase nunca excusa nuestro valor.

### Diseño
1. **Casos** (`testdata/java/cases.ndjson`, versionado): los genera
   `TestWriteJavaCases` (con `UCUM_JAVA_CASES=<ruta>`), determinista y
   ordenado. Falla, en vez de achicar el corpus, si falta `UCUM_PACKAGES`. Cada
   caso tiene un `id` estable. Los valores son **literales de texto fijos**: el
   `Decimal(String)` de Java toma su precisión del literal
   (`Decimal.java:135-145`), así que el formato del valor forma parte del caso.
   Los de FHIR van tal cual están en el JSON. Fuentes:
   - **Obligación de cobertura desde el XML**, no una lista elegida: cada
     átomo de `ucum-essence.xml` (`validate` y `canonical` de `1`), cada
     prefijo sobre un átomo métrico y sobre uno no métrico (que tiene que
     fallar, §11), cada función especial, cada unidad arbitraria, cada átomo
     en su forma `CODE` (§4: Java no la acepta), anotaciones, división por
     cero y conversiones incompatibles.
   - La suite oficial, con la regla de conteo explícita (los `<case>` dentro de
     comentarios XML no cuentan).
   - Los 848 conceptos de `ucum-common` (840 códigos distintos).
   - Las cantidades UCUM de los ejemplos de R4, R4B y R5, tal cual.
   - `UcumCommonUnitsTest.java` (954 unidades).
   - **Pares elegidos sin nuestro motor:** átomos con el mismo atributo
     `property` del XML (positivos) y con distinto (negativos), los primeros k
     de cada grupo por orden del XML. Lo mismo para `convert`, `isComparable`,
     `multiply` y `divide`, que así pasan de 5 casos de la suite a cientos.
   - `validateInProperty` para cada átomo con su `property` declarada.
   - **Presupuesto:** a lo sumo 25.000 casos. Se mide el tamaño del resultado
     versionado y el tiempo del test, y se registran.
2. **Dumper Java** (`tools/javadump/JavaDump.java`, un archivo, sin
   dependencias): lee los casos por stdin y escribe, por `id`, el resultado o
   la excepción (clase y mensaje), y la precisión del `Decimal` de salida. No
   interpreta nada.
3. **Resultado versionado** (`testdata/java/ucum-java-1.0.10.ndjson`) y
   **manifiesto** (`testdata/java/manifest.json`) con SHA-256 del JAR, del
   dumper, de los casos, del resultado y de `UcumCommonUnitsTest.java`. La
   fecha y `java -version` van en `testdata/java/run-info.json`, aparte, para
   que el resultado sea reproducible byte a byte entre fechas y JDK.
4. **`make java-dump`** (fuera de CI; lo corre quien tenga Java): descarga el
   JAR y `UcumCommonUnitsTest.java` con SHA-256 fijado, regenera los casos,
   compila y corre el dumper, reescribe resultado y manifiesto, y **avisa si
   Maven Central tiene una versión más nueva** que la fijada
   (`maven-metadata.xml`). Así se detecta que la referencia cambió.
5. **Comparación** (`TestCompareJava`, en `internal/engine` para llegar a las
   API exactas de producto; corre en `go test` y en CI, sin Java):
   - **Integridad:** SHA-256 de casos, resultado y dumper contra el
     manifiesto; los `id` del resultado son exactamente los de los casos, en
     el mismo orden, sin faltantes ni duplicados.
   - **Frescura:** las partes del corpus que no necesitan packages (XML,
     suite, `ucum-common`, `UcumCommonUnitsTest`) se regeneran en el test y
     tienen que coincidir con las de `cases.ndjson`; con `UCUM_PACKAGES`, el
     corpus entero.
   - **Contrato numérico** (`Decimal.java`: semilla de 31 dígitos en
     `Converter.java:62,130`, cocientes truncados a `prec`, `prec = 100` en
     el caso no terminante): unidad lineal, nuestro valor exacto
     (`CanonicalRat`, `ConvertRat`, producto exacto) contra el decimal de
     Java; coinciden si nuestro valor truncado a la precisión que Java
     declaró es el valor de Java. Escala no lineal: nuestro `float64`; ahí
     Java no tiene una respuesta comparable y la diferencia va a su clase.
   - **Código canónico:** se comparan como multiconjuntos átomo→exponente,
     con un tokenizador propio del test que no usa nuestro parser (Java
     ordena alfabéticamente, `Converter.java:119-123`; nosotros no).
6. **Clasificación** (`testdata/java/differences.json`). Cada clase es un
   **predicado angosto escrito en Go**, con su `kind` (`bug-ours`, `bug-java`
   o `deliberate`), su motivo, su sección de UCUM y su cita de Ucum-java en
   `db9d22ad`. El modelo de Java sale de su código (`Registry.java:57-64`:
   handlers por código, `[p'diop]` y `%[slope]` contra `deg`, `[hp_X]` y
   `[hp_C]` contra `1`, `B[SPL]` con `Decimal(2)`; mensajes de
   `Converter.java:164` y `:170`), no del XML. El test falla con:
   - una diferencia sin clase, o con más de una (las clases no se solapan);
   - una clase muerta;
   - un `bug-ours` sin issue enlazado;
   - un caso clasificado cuyo valor nuestro no coincide con el esperado
     independiente;
   - un caso de la suite o de `UcumCommonUnitsTest` en el que **los dos**
     motores se apartan del esperado: el acuerdo con Java no excusa un error.
7. **Reporte generado**: con `UCUM_JAVA_REPORT=<ruta>`, conteos por operación
   y por clase, con ejemplos. `make compare-java` lo deja en
   `docs/plans/evidence/` con fecha. El README enlaza el reporte, y cada fila
   de su tabla se respalda con una clase con casos o con los casos
   concretos que la muestran (las filas estructurales: «Reading the
   definitions» y «Registered handlers» se ven en `[degRe]`, `B[10.nV]`,
   `[m/s2/Hz^(1/2)]`, `[hp'_M]` y `[hp'_Q]`).

### Alcance
Lo de arriba. El motor no cambia en este incremento: un `bug-ours` se reporta
como issue y va en su propio incremento.

### Fuera de alcance
- `ucum-lhc` (NLM, JavaScript): el mismo arnés, en otro incremento.
- La suite de FHIRPath.
- Comparar `NewCaseInsensitive`. Sí entra que Java rechaza la forma `CODE`
  (fila del README), con los casos de la obligación de cobertura.

### Criterio de salida
1. `make java-dump` es reproducible: dos corridas dan el mismo resultado byte
   a byte.
2. `go test ./...` corre `TestCompareJava` sin Java y en verde: integridad,
   frescura, toda diferencia en exactamente una clase, sin clases muertas,
   ningún `bug-ours` sin issue y ningún caso con los dos motores en error.
3. Cada fila de la tabla del README respaldada según el punto 7.
4. Todas las mutaciones del §7 detectadas.
5. `make check` en verde.

### Excepciones a la regla 5
Ninguna en producción (no cambia código de producción). En el test, el modelo
de Java se escribe a mano a propósito: es la conducta de la referencia, citada
línea por línea, no un dato de UCUM.

### Dependencias nuevas
Ninguna en el módulo Go. `make java-dump` necesita un JDK (17+) y queda fuera
de CI. El usuario ejecuta Java: en la sesión, el permiso de ejecutar código
externo no está concedido.

### 7. Mutaciones previstas
| # | Defecto inyectado | Debe fallar por |
| --- | --- | --- |
| J1 | Motor: ignorar el exponente de un prefijo | diferencia sin clase |
| J2 | Motor: aceptar la cadena vacía (coincide con Java) | clase muerta `java-accepts-empty` y `TestFunctionalValidation` 1-103 |
| J3 | Motor: factor mal en una unidad derivada | diferencia sin clase, y esperado de `UcumCommonUnitsTest` |
| J4 | Predicado que acepta todo | diferencia con más de una clase |
| J5 | Un dígito del resultado de Java cambiado **dentro** de la ventana de truncamiento | contrato numérico, y SHA-256 del resultado |
| J6 | `cases.ndjson` cambiado sin regenerar el resultado | SHA-256 del manifiesto |
| J7 | Una clase sin casos | clase muerta |
| J8 | Constante mal en el modelo de Java de una clase (p. ej. `B[SPL]` con 1 en vez de 2); ver §5 | diferencia sin clase |
| J9 | Motor y esperado en error a la vez en un caso de la suite; ver §5 | regla «los dos motores en error» |
| J10 | Un registro del resultado borrado o duplicado | integridad de `id` |

## 3. Ronda adversarial (2026-10-26: Codex y Fable, en paralelo)

| # | Origen | Sev. | Hallazgo | Resolución |
| --- | --- | --- | --- | --- |
| 1 | Codex | alta | Las clases explican a Java pero pueden excusar un valor nuestro roto | **Aceptado**: tres fuentes por caso; una diferencia se acepta solo con los dos lados explicados |
| 2 | Codex | alta | J2 (aceptar `''`) hace coincidir con un bug de Java | **Aceptado**: clase muerta más la suite (1-103); regla de «los dos motores en error» |
| 3 | Codex, Fable | alta | Sin contrato numérico; el `Decimal` de Java trunca con precisión del literal | **Aceptado**: contrato de truncamiento a la precisión declarada por Java, con citas; J5 dentro de la ventana |
| 4 | Codex | alta | El manifiesto no ata resultado ni dumper; registros faltantes o duplicados pasan | **Aceptado**: SHA-256 de todo y correspondencia 1:1 de `id`; J10 |
| 5 | Codex, Fable | alta | Un snapshot fijo no detecta que la referencia cambie | **Aceptado**: `make java-dump` avisa si Maven tiene una versión más nueva; el motivo se reformula: el snapshot detecta cambios **nuestros** contra una referencia fijada |
| 6 | Codex, Fable | alta | Elegir pares por «mismo canónico» usa nuestro motor | **Aceptado**: pares por `property` del XML, positivos y negativos |
| 7 | Codex, Fable | alta | El alcance no cubre todas las filas del README | **Aceptado**: entra `validateInProperty`; las filas estructurales se respaldan con casos concretos; el rechazo de `CODE` por Java entra |
| 8 | Fable | alta | El modelo de Java no sale del XML: `Registry.java:57-64`; falta el mensaje «Not handled yet (special unit)» | **Aceptado**: el modelo de Java se cita de su código, por unidad |
| 9 | Codex | media | Cobertura clínica, no exhaustiva | **Aceptado**: obligación de cobertura desde el XML |
| 10 | Codex | media | Clases solapadas | **Aceptado**: exactamente una clase por diferencia; J4 y J8 |
| 11 | Codex | media | Fecha y `java -version` rompen la reproducibilidad | **Aceptado**: van en `run-info.json`, aparte |
| 12 | Codex, Fable | media | Costo: 705.600 pares si se combinan todos | **Aceptado**: presupuesto de 25.000 casos, tamaño y tiempo medidos |
| 13 | Fable | media | Valores con formato no fijado: la precisión de Java depende del literal | **Aceptado**: literales fijos, FHIR tal cual |
| 14 | Fable | media | Multiply/divide: solo 5 casos; la API pública es `float64` | **Aceptado**: pares desde el XML; el test vive en `internal/engine` |
| 15 | Fable | media | Java ordena el canónico alfabéticamente | **Aceptado**: comparación como multiconjunto con tokenizador propio |
| 16 | Fable | media | `UcumCommonUnitsTest.java` es evidencia de la referencia sin usar | **Aceptado**: tercer oráculo |
| 17 | Fable | baja | J2 muere por la clase muerta, no por «diferencia sin clase»; falta una mutación de constante | **Aceptado**: J2 corregida y J8 agregada |
| 18 | Fable | baja | Conteos inconsistentes (840/848; 529/530) | **Aceptado**: reglas de conteo explícitas |
| 19 | Fable | baja | Sin packages, el generador podría emitir un corpus menor | **Aceptado**: falla |

Ninguno rechazado.

## 5. Implementación

### Hecho (2026-10-26)
- `internal/fhirexamples`: la lectura de los packages, que antes estaba en
  `fhir/examples_test.go`, para que la usen ese test y el generador. Mismos
  conteos que antes (231/241/273).
- Generador (`internal/engine/javacases_test.go`): 11.235 casos, 549 KB,
  dentro del presupuesto. Corrida con packages: 7,2 s y 283 MB de RSS (la
  descompresión de los packages; solo en `make java-dump`).
  `TestJavaCasesFresh` sin packages: 0,03 s.
- `tools/javadump/JavaDump.java`, `scripts/java-dump.sh`, `scripts/java.sha256`,
  objetivos `java-dump` y `compare-java`.
- Comparación (`javacompare_test.go`) y armazón de clases
  (`javaclasses_test.go`), a la espera del primer resultado de Java.

### Desvíos del plan
| Plan | Implementación | Motivo |
| --- | --- | --- |
| `cases.ndjson` y resultado en NDJSON | TSV, con escapes `\t`, `\n`, `\r` y `\\` en los mensajes de Java | el dumper no tiene dependencias, y leer JSON en Java sin ellas es más código que el resto del dumper |
| `testdata/java/` en la raíz | `internal/engine/testdata/java/` | el test vive en `internal/engine` |
| `differences.json` más predicados en Go | solo `javaclasses_test.go` | la documentación de cada clase queda junto a su predicado, sin sincronizar por nombre |
| Java declara la precisión de su `Decimal` | se lee por reflexión del campo privado `precision` | `Decimal` no la expone (no hay `getPrecision`) |
| Contrato: nuestro valor truncado a esa precisión | dos niveles: coincidencia si el error relativo es ≤ 1e-22 (el redondeo de trabajo de Java; máximo medido 5,15e-24); y la clase `java-decimal-precision` si la diferencia cabe en una unidad de la última de las cifras que Java declara | `divide` redondea (`Decimal.java:735-758`), no trunca. Un primer intento con una cota armada desde los átomos dejó 106 casos sin explicar; la precisión declarada es evidencia de la propia referencia |
| J5 dentro de la ventana de truncamiento | J5 altera un dígito justo fuera de la tolerancia (cifra significativa 20), con el manifiesto actualizado para que no lo frene el SHA-256 | sigue la cota real |
| Mensajes de toda excepción de Java | solo los de `UcumException`; de las demás, la clase | los mensajes de `NullPointerException` cambian entre JDK y romperían la reproducibilidad |
| `UcumCommonUnitsTest.java` como oráculo | sus datos extraídos a `common-units.tsv` (versionado, BSD 3-Clause, NOTICE) | la comparación corre en CI sin red |

### Clasificación (2026-10-26, dos corridas de `make java-dump` del usuario)
11.235 casos; 1.983 diferencias, todas en exactamente una de 11 clases
(cifras después de la revisión, ver abajo):
`java-decimal-precision` (435), `java-special-as-linear` (172),
`java-decimal-shift` (1), `java-offset-unsupported` (59),
`ours-float-overflow` (9), `java-special-unregistered` (99),
`java-lexer-bracket` (15), `java-no-case-insensitive` (251),
`java-arbitrary-dimensionless` (726), `java-inproperty-base-only` (212) y
`java-accepts-invalid-syntax` (4). Un `bug-ours`: `canonical 1000 B` es
10^1000, no cabe en float64 y vuelve `+Inf` sin error (#59).
Oráculo independiente: 277 resultados de unidades
especiales contra las fórmulas de la especificación (§30■4, §43■4-5,
§44■3-4, §45■3, §46■3, §47■4, §48■3), sin desvíos; 7 expectativas de
`UcumCommonUnitsTest` descartan unidades arbitrarias, contra §24■3, y se
cuentan aparte.

Contrato numérico, en dos niveles:
1. Coincidencia: error relativo ≤ 1e-22 (máximo medido 5,15e-24, el redondeo
   de trabajo de Java).
2. Redondeo de Java (`javaArithmetic`): el valor de Java se reproduce con la
   aritmética de Java sobre los valores que Java dio para cada átomo, dentro
   de dos unidades de la última cifra que declara para el resultado; y
   nuestro valor de cada átomo lineal coincide con el de Java dentro de lo
   que Java declara para ese átomo. No usa nuestro valor del caso.

Intentos descartados: una cota armada desde los átomos dejó 106 casos sin
explicar; sumar la precisión que Java declara por la cadena de definiciones
daba cotas de hasta 2.400 % (Java declara una cifra para `mol`, que es
exacto) y habría excusado cualquier error nuestro.

Hallazgo: `convert 2.5 [bu_us] → [pk_us]` da `1.00` en Java; su propia
aritmética da 9,9997. Bug de Java, para reportar upstream.

### Mutaciones (2026-10-26)
`make mutations` corre las de 001 y las de 002; las 33 dan lo esperado
(`evidence/002-2026-10-26-mutations.txt`). Cambios respecto del §7:
- J2 acepta `""` como la unidad, como Java. Sacar solo el chequeo del parser
  sobrevivía, porque el parser igual rechazaba `""`. Ahora muere porque cada
  entrada de `javaSyntaxBugs` tiene que seguir siendo una diferencia.
- J5 cambia la cifra 10 de `1 deg` y resella el manifiesto (J5/sha, sin
  resellar, muere por el SHA-256).
- J6/fresh: casos cambiados y resellados; los frena `TestJavaCasesFresh`.
- J8 saca `[pH]` de `javaHolding`. El valor del modelo de Java ya no sale de
  constantes (sale de lo que Java respondió para cada átomo), así que la
  mutación de la constante de `B[SPL]` dejó de tener sentido.
- J9 reemplazó la regla «los dos motores en error»: rompe la fórmula de pH
  del oráculo de la especificación. La suite la exige `TestFunctional*`.
- J11: nuestro manejador de pH sin el signo; lo atrapa el oráculo.

### Costo
`TestCompareJava` 0,55 s y 28 MB de RSS (después de la ronda 2); `TestJavaCasesFresh` 0,02 s; datos
versionados 1 MB. La corrida de mutaciones, 250 s.

### Revisión de código, ronda 1 (2026-10-26; Codex y Fable)
| # | Hallazgo | Decisión |
|---|---|---|
| C1 | Ninguna clase miraba nuestro valor del caso: `java-decimal-shift` aceptaba 123 en vez de 10 | `ourComposition`: nuestro valor es la composición de nuestros valores por átomo (lineales) o de las fórmulas de la especificación (especiales, con prefijo como α de §22■4, y hacia una unidad especial con f). Aparece el `bug-ours` de overflow, que `java-special-as-linear` tapaba |
| C2 | El oráculo tomaba el destino de canonical de nuestra respuesta, y un fallo nuestro daba NaN = «se esperaba error» | el destino sale de la unidad de referencia de la especificación |
| C3 | NaN pasaba la comparación del oráculo | NaN e Inf son desvío, salvo Inf cuando el valor verdadero tampoco cabe en float64 (lo registra `ours-float-overflow`) |
| C4 | `relErr` con nuestro valor en 0 aceptaba 1e-33 de Java | 0 solo coincide con 0 |
| C5 | Ids duplicados en la parte FHIR pasaban sin packages | `checkJavaCases` también sobre lo cargado |
| C6, F7 | README: `[m/s2/Hz^(1/2)]` entre los sin handler; 7.4 `[pH]` sin caso | corregido; el ejemplo es `canonical 1 [pH]` |
| F1 | `ourAtomsAgree` usaba la tolerancia de Java: un entero del XML le vale una cifra (200 %), y `t` al doble pasaba todo `go test` | `ourAtomFromXML`: cada átomo nuestro es exactamente su definición evaluada con nuestros átomos, hasta las bases; J12 (`t` al doble) y J13 (resultado de Convert mal, átomos bien) |
| F2 | Evidencia desactualizada | regenerada al cierre de la ronda |
| F3 | `java-arbitrary-dimensionless` no exigía que nuestras formas difirieran | exigido |
| F4 | Las clases de estado aceptan nuestro `ok` sin mirar el valor en expresiones compuestas con unidades especiales (`Cel/min`) | aceptado como límite: el oráculo cubre la unidad especial sola, con prefijo; las compuestas las cubre `TestFunctional*` |
| F5 | El extractor saltaba en silencio otras formas de `new CommonUnit(` | toda ocurrencia no comentada que no calce es error; la regeneración da archivos idénticos |
| F6 | `java-dump.sh` abortaba sin red aunque hubiera cache; `javac` sin `-encoding` | `|| latest=unknown`; `-encoding UTF-8` |
| F8 | La parte FHIR de los casos no se revisa sin packages | `make check-packages` corre también `TestJavaCasesFresh` |

Las 35 mutaciones dan lo esperado; `make check` en verde. (Después de la
ronda 2 son 37, ver abajo.)

### Revisión de código, ronda 2 (2026-10-26)
Codex confirmó cerrados C1, C3-C6, F3, F5, F6 y F8, y encontró:
| # | Hallazgo | Decisión |
|---|---|---|
| R1 | El oráculo seguía tomando el factor de nuestro `ConversionFactor` y el código canónico de nuestro motor, y un error ahí pasaba por «inconmensurable» | dimensión y factor salen del XML: `xmlDims` (recursivo; las arbitrarias son su propia dimensión, §24■3) y `xmlFactor` con `xmlAtomValue`; si algo no se puede evaluar, el oráculo lo reporta en vez de aceptarlo |
| R2 | f⁻¹(v) que desborda solo en el paso intermedio (620 `B[uV]`) se tomaba por overflow verdadero | el producto se decide en log10; `ours-float-overflow` solo si el valor verdadero no cabe |
| R3 | `java-arbitrary-dimensionless` aceptaba otra unidad arbitraria (`[arb'U]` por `[IU]`) | nuestra dimensión, por `xmlDims`, tiene que ser la de la entrada |
| R4 | De una unidad especial a otra (`[degF]`→`Cel`) el oráculo no miraba nada | f⁻¹ de la primera y f de la segunda (§22■4); 263 resultados revisados |
| R5 | Piso absoluto 1e-300 en la tolerancia: perder 10^-313 pasaba | sin piso: un esperado de 0 solo acepta 0, y uno por debajo del menor normal no se modela (se reporta) |
| R6 | `ourAtomFromXML` no era recursivo | los átomos de la definición se comprueban igual (`xmlAtomValue`) |

Cada escenario se probó inyectando la falla en una copia: todos fallan.

Fable (ronda 2) confirmó cerrados los de la ronda 1 inyectando cada falla, y
encontró:
| # | Hallazgo | Decisión |
|---|---|---|
| G1 | De una unidad lineal a una especial (`K`→`[degRe]`) nada miraba el valor: +1 en esa conversión pasaba todo `go test ./...` | `checkToSpecial` en el oráculo; J14 |
| G2 | `ourAtomFromXML` comparaba contra el modelo del mismo cargador: `[foz_m]` ×1,5 en `essence.go` pasaba todo `go test ./...` | prefijos, definiciones, funciones y banderas salen de `readEssence`, que lee el XML con `encoding/xml` y `big.Rat`, sin `internal/essence` ni `internal/model`; J15 |
| G3 | Las clases unían `[IU]` con `[iU]` y el motor no | gana el motor, que sigue §24■3 («no comparable con ninguna otra arbitraria», `canonicalize.go:104`): cada arbitraria es su propia dimensión |
| G4 | 16 casos con valor bajo clases de estado | 7 eran G1; quedan 9 compuestos con unidades especiales (`Cel/min`, `2 Cel × 3 m`, `K × [degRe]`, `[MET] / [hp'_X]`): límite aceptado, los cubren otros tests |
| G5 | Tolerancia 1e-12 | 1e-13 (el ruido de float64 aquí es ~1e-15) |
| G6 | Evidencia desactualizada | regenerada: 277 resultados del oráculo |
| G7 | Comentario: el `deg` del XML es de `%[slope]`, no de `[p'diop]` | corregido |

El overflow tiene su issue: #59 (pedido por el usuario). Las 37 mutaciones
dan lo esperado; `make check` en verde.

### Revisión de código, ronda 3 (2026-10-26, la última del §10)
Codex cerró R2-R4 y R6; Fable cerró G1-G3, G5 y G7 repitiendo sus
inyecciones (17 de 18 mueren; la que vive es el límite G4).
| # | Hallazgo | Decisión |
|---|---|---|
| H1 | f⁻¹(v) que se pierde en el paso intermedio (`325 [pH]`) daba 0 y lo certificaba | la vía log10 cubre los dos extremos; el valor verdadero bajo el menor float64 cuenta para `ours-float-overflow`, que ahora toma también `1000 [pH]` → 0 (9 casos, #59) |
| H2 | Un esperado infinito aceptaba cualquier finito (`Inf <= Inf`) | `floatClose` rechaza un esperado no finito; `toSpecial` no da esperado si sale no finito |
| H3 | El parser compartido podía resolver `nV` como `uV` y confirmarse | `parse`/`defParse` exigen que los átomos resueltos estén escritos, en orden, en el texto. Queda un límite: un parser que confunda una unidad entera con otra (Cel por [degF]) pasa esta comparación; lo atrapan seis tests de la suite (`TestPrefixedSpecial*`, …) |
| H4 | `273.15 mK` como canónico de `0 Cel` pasaba por tener la dimensión de K | el código canónico tiene que valer exactamente 1 según el XML (`canonicalScaleOne`), en el oráculo y en las clases numéricas |
| H5 | Un `ConvertRat` que copiara el bug de Java con arbitrarias pasaba todo `go test ./...`: solo se miraban las diferencias | `checkCommensurable`: todo `comparable` y `convert`, coincida o no con Java, se acepta si y solo si las dimensiones del XML son iguales; J16 |
| H6 | `property` todavía salía del cargador del motor | sale de `readEssence` |

Cada escenario se probó inyectando la falla. Las 38 mutaciones dan lo
esperado; `make check` en verde; reporte regenerado (277 resultados del
oráculo de especiales).

## 6. Cierre (paso 9)
Tres rondas de revisión, todas corregidas; la tercera no dejó hallazgos
altos abiertos. Límites aceptados: expresiones compuestas con unidades
especiales (G4) y un parser que confunda unidades enteras (H3), ambos
cubiertos por otros tests de la suite. Un `bug-ours`, con issue: #59.

Pendiente fuera de este incremento:
- defectos de Ucum-java con evidencia (desplazamiento decimal en `divide`,
  lexer de `[m/s2/Hz^(1/2)]`, sintaxis inválida aceptada, arbitrarias sin
  dimensión): redactados en [002-java-issues.md](002-java-issues.md); el
  usuario decidió no publicarlos (2026-10-28);
- corregir #59 en el motor.
