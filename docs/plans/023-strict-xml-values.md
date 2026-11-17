# 023 — los valores del XML se leen sin gramática

**Estado:** paso 9 (cierre), 2026-11-16. Code review convergido en la ronda 2 (§11). Mutaciones: [`evidence/023-2026-11-16-mutations.txt`](evidence/023-2026-11-16-mutations.txt), todas como se esperaba. `make compare-java` sin cambios respecto del informe del 2026-11-03; `make check` y `make check-packages` en verde. Hallado en la ronda 3 de 019 (§10).

## 1. Problema

`NewFromReader` lee los atributos `value` de `<prefix>`, `<unit>` y
`<function>` con `decimal.FromString`, que delega en `big.Rat.SetString`.
Dos defectos, medidos el 2026-11-16 en `main` (v4.5.2) con el XML oficial
cambiando solo el `value="5280"` de `[mi_i]` (sonda en el scratchpad,
`Canonical(1, "[mi_i]")`):

1. **Un valor de unidad inválido pasa a `1` sin error.** `essence.go:145-149`
   descarta el error de `FromString` («Some special units have empty value;
   default to 1»). Pero `FromString("")` ya da 1 sin error, así que la rama
   solo atrapa valores inválidos: `abc` y `1e` cargan y dan 0.3048 (la milla
   vale un pie). En prefijos y funciones el error ya se propaga.
2. **`SetString` acepta mucho más que un decimal.** Cargan sin error, con el
   valor que `SetString` les da: `1p20` (2^20; 319 605.9648), `0x10` (16;
   4.8768), `1_000` (1000), `1/3` (0.1016), `+5`, `5.`, `1e+3`, ` 7 ` (el
   `TrimSpace`). `1p999999` arma un número de un millón de bits en la carga.

## 2. Revisión previa

- **`gofhir/server` (congelado):** no lee el XML; usa la librería. Nada que
  tomar.
- **Especificación:** UCUM no define una gramática para el atributo `value`
  de `ucum-essence.xml` (el XML es el formato de intercambio de la
  especificación, sin esquema publicado con un tipo para ese atributo).
  Los 329 literales del XML oficial (atributo `value` de `<value>` y
  `<function>`) tienen seis formas: `9` (256), `9.9` (31), `9e-9` (19), `9e9`
  (13), `9.9e-9` (9), `9.9e9` (1); ninguno vacío, con signo, `+` ni espacios.
- **Ucum-java 1.0.10** (misma sonda, `UcumEssenceService(InputStream)`):
  rechaza al cargar `abc`, `1p20`, `0x10`, `1_000`, `1/3`, `+5`, `5.`, `1e`,
  `1e+3` y ` 7 ` con «'…' is not a valid decimal»; acepta `-5`, `.5`, `1E3` y
  `1.5e-3`. Con `value=""` carga y da 0 para `[mi_i]`.

Más sondas de la ronda 1 (§9, a): Ucum-java también acepta `0008`,
`00.10`, `-0`, `-.5`, `1e-0`, `1E-3`, mantisas de 10 000 dígitos, `-` (da 0)
y dígitos no ASCII (`&#xFF11;`, `1e&#x663;`); rechaza `1e+0`, `1E+3` y
`1e2147483648` (desborde de `int`).

**Gramática elegida (deliberada, no la de Ucum-java):**
`-?(D+(\.D+)?|\.D+)([eE]-?D+)?`, con D un dígito ASCII `0-9`. Cubre los 329
literales del XML oficial y coincide con Ucum-java en todo lo que esa
implementación rechaza; difiere en tres cosas, documentadas y con test: `-`
solo (Java da 0, aquí error), los dígitos no ASCII (Java los acepta, aquí
error: ningún literal del XML oficial los usa)
y el exponente acotado de 3.2.
**Mejorar:** el error nombra la unidad o el prefijo y el literal.
**Descartar:** el 0 de Ucum-java para un valor vacío u omitido; aquí sigue
siendo 1, como hoy (no hay unidad oficial con `value` vacío, y cambiarlo
rompería un XML propio que funciona).

## 3. Cambio

1. **Gramática.** `decimal.FromString` valida el literal completo contra la
   gramática de §2 antes de llamar a `SetString`, sin `TrimSpace`. El vacío
   sigue dando 1; cualquier otro literal fuera de la gramática es un error
   que lo cita.
2. **Exponente acotado** (ronda 1, alta de a y b). `FromString` rechaza un
   exponente decimal de magnitud mayor que `maxLiteralExponent = 9999`
   (constante interna) antes de calcular la potencia: hoy `1e1000000000`
   agota la memoria al cargar. La cota no es el tope de valores de 019 (que
   sigue pendiente de rediseño): solo impide que un literal de pocos bytes
   produzca un número arbitrario. Con ella, un literal de k bytes da a lo
   sumo unos 3.33·(k + 9999) bits; la mantisa crece con el XML que entrega el
   llamador, sin amplificación. El XML oficial llega a ±30.
3. **Valor de unidad.** `essence.go` propaga el error del valor de una
   unidad como ya lo hace con prefijos y funciones.
4. **Función sin atributos en `<value>`** (ronda 1, a y b). Hoy el bloque
   entero depende de `value != "" || Unit != ""`, así que
   `<value><function value="abc"/></value>` no valida la función. La función
   se lee siempre que el elemento exista.
5. **Mensajes** (ronda 1, baja de b), con el formato de los que ya existen:
   `prefix <código> value: <error>`, `unit <código> value: <error>`,
   `unit <código> function value %q: <error>` (sin cambio); el error de
   `FromString` cita el literal una vez.
6. El valor de un literal válido es el mismo que hoy (lo calcula el mismo
   código después de la validación).

## 4. Criterio de salida

- Cada literal de la tabla de §5.1 da el resultado esperado, sacado de
  Ucum-java (sonda de §2, versionada en el repo, ver 5.5) salvo las tres
  diferencias de §2.
- El XML oficial carga igual: un test compara todas las definiciones (valor
  y función) con un dorado generado en `main`; `make check`, `make
  compare-java` sin cambios.
- `make check-packages`; mutaciones completas.

## 5. Tests (primero)

1. `FromString`, tabla con valor esperado escrito como fracción (numerador y
   denominador enteros, no por la gramática): válidos (las seis formas del
   XML oficial, `-5`, `.5`, `-.5`, `-0`, `0008`, `00.10`, `1e-0`, `1E-3`, `1e9999`,
   `1e-9999`); inválidos (`abc`, `1p20`, `0x10`, `1_000`, `1/3`, `+5`, `5.`,
   `1e`, `1e+3`, `1E+3`, ` 7 `, `-`, `.`, `e5`, `1e-`, `--1`, `1.2.3`, `１`,
   `1e٣`, `NaN`, `Inf`, `1 `, `\t1`, `1e10000`, `1e-10000`,
   `1e2147483648`).
2. Carga (`NewFromReader`), matriz (ronda 1, a y b): atributo `value` vacío,
   omitido y elemento `<value>` omitido, para prefijo, unidad y función, con
   y sin `Unit`; un literal inválido en cada uno de los tres sitios con el
   mensaje exacto de 3.5; `<value><function value="abc"/></value>`. Los
   esperados de vacío y omitido son los de hoy (se fijan antes de cambiar el
   código y se citan las 21 unidades oficiales con función).
3. Los 329 literales del XML oficial pasan la gramática (recorridos desde el
   XML embebido).
4. Fuzz: `FromString` acepta exactamente `s == "" || gramática(s) && |e| ≤
   9999` (con `""` en el corpus); la gramática del oráculo está escrita aparte
   y no detecta por sí sola un error compartido, por eso el criterio son las
   tablas 5.1 y 5.2 y las mutaciones (ronda 1, media de b). El fuzz solo calcula potencias bajo la cota: compara solo la aceptación.
5. Referencia (ronda 1, media de a): una sonda Java versionada en
   `tools/javadump/` que carga el XML oficial con cada literal de 5.1 en
   `[mi_i]` y escribe aceptación y valor; su salida va a `testdata` y el test
   de 5.1 la compara, salvo las tres diferencias de §2. Se corre con
   `make java-dump`, como la comparación de 002.

## 6. Mutaciones previstas

- La validación desactivada (vuelve `SetString` directo).
- El fallback a `1` restituido en `essence.go`.
- La gramática sin el `-` inicial, sin la forma `.D+`, o con `+` en el
  exponente (en el código y en el oráculo a la vez: deben fallar 5.1 o 5.5).
- `TrimSpace` restituido.
- La cota del exponente quitada o corrida en uno.
- El vacío que da 0 en vez de 1.
- La función leída solo con `value` o `Unit` (el gate de hoy).

## 7. Dependencias

Ninguna.

## 8. Versión

`fix:` → 4.5.3. Un XML propio con un literal fuera de la gramática o un
exponente de más de cuatro cifras deja de cargar; con el XML oficial nada
cambia.

## 9. Ronda adversarial del plan

### Ronda 1 (2026-11-16)

Dos Codex sobre copias (pl023a, pl023b). Ninguno pudo correr Go en su
sandbox: sondas de Python y regexp sobre el XML, y (a) sondas Java con el
jar de Ucum-java 1.0.10.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Dejar fuera la cota del literal es inseguro: la gramática acepta `1e1000000000`, que agota la memoria al cargar | a, b (alta) | **Aceptado**: exponente ≤ 9999 (3.2) |
| La gramática no es la de Ucum-java: Java acepta `-` (da 0) y dígitos no ASCII, y rechaza `1e2147483648` | a (media) | **Aceptado**: se describe como gramática deliberada con tres diferencias documentadas y con test |
| El oráculo de fuzz rechaza `""`, que §3 acepta | a, b (media) | **Aceptado**: oráculo `s == "" || …`, `""` en el corpus, mutación del vacío a 0 |
| Un oráculo regexp no detecta un error compartido con el código (`+` en ambos pasa) | b (media) | **Aceptado**: el criterio son las tablas con valores como fracción y la sonda Java versionada; el fuzz solo compara aceptación |
| Vacío y omitido dependen del contexto; `<value><function value="abc"/></value>` salta la validación de la función por el gate `value != "" \|\| Unit != ""` | a, b (media) | **Aceptado** (confirmado por lectura de `essence.go:143`): 3.4 y la matriz de 5.2 |
| `make compare-java` no cubre XML propios | a (media) | **Aceptado**: sonda Java versionada (5.5) |
| Los formatos de error difieren entre prefijo, unidad y función | b (baja) | **Aceptado**: 3.5 fija los tres |

## 10. Registro de la implementación

- **`<value>` sin atributos con `<function>`:** la función se valida (error
  `unit <código> function value %q: …` si el literal es inválido) pero no se
  arma la conversión: el modelo queda como en `main`. Un primer intento la
  armaba (valor 1, `Unit` vacío), y la revisión halló el caso no especial
  (`isSpecial="no"`): `main` fallaba con «has no value definition» y esa
  versión lo cargaba como adimensional, un valor equivocado en vez de un error.
  Los tres casos de la matriz de 5.2 vuelven a los resultados de `main` y se
  añade el caso no especial.
- **Fuzz:** `FuzzFromString` sí llama a `FromString` con exponentes acotados
  (≤ 9999, microsegundos); lo de 5.4, «no calcula potencias», es «solo potencias
  bajo la cota». El oráculo no calcula ninguna.
- **Mutación de la cota:** «la cota quitada» cuelga o agota la memoria en la
  corrida; se reemplazó por la cota corrida a nueve cifras (K278), junto con la
  corrida en uno (K277).
- **Sonda Java:** Ucum-java no termina con `1e9999` ni con dígitos no ASCII;
  esos literales quedan solo en las tablas de Go. `java-values.tsv` (39
  literales) sale igual en dos corridas. `scripts/java-dump.sh` completo no se
  corrió (necesita red); la sección nueva se corrió a mano.
- Mutaciones nuevas K271–K281, todas detectadas con `runmut2.py`.

## 11. Code review

Dos Codex sobre copias (cr023a, cr023b).

| Ronda | Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|---|
| 1 | Un `<value>` sin atributos con solo `<function>` armaba una conversión (valor 1, `Unit` vacío): una unidad no especial cargaba como adimensional (`{1 1}`) en vez de fallar con «has no value definition» | b (media, sin verificar) | **Confirmado** por sonda contra `main`; se revirtió el desvío: la función se valida y no se arma conversión (K280) |
| 1 | Ningún test combina mantisa negativa con exponente: `negativeExponent` cambiado por `strings.Contains(s, "-")` sobrevive | a (baja) | **Confirmado** por mutación; `-5e3`, `-5e-3`, `-.5e2` en la tabla y en la sonda Java (K281) |
| 1 | El fuzz calcula potencias, contra §5.4 | b (baja, sin verificar) | Aceptado: con la cota son microsegundos; §5.4 dice «solo potencias bajo la cota» |
| 2 | Sin hallazgos | a, b | Converge |

Un revisor de la ronda 1 escribió una copia del repo en `.cache/tmp/review023`
del repo real (ignorada por git), que rompía el gofmt de `make check`; se
borró tras revisarla.

