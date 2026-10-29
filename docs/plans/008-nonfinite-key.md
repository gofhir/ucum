# 008 — `CanonicalKey` acepta NaN y ±Inf, que `Compare` rechaza (#55)

**Estado:** cerrado (paso 9), pendiente de commit. Plan aprobado el 2026-10-28, con `feat:` y el sentinela. Opción A elegida por el usuario: solo
el paquete `fhir` rechaza lo no finito; la API base no cambia.

## 1. Revisión previa

### El defecto

Medido sobre `main` (`1c18bdb`), con `Value` NaN, +Inf y −Inf:

| Llamada | Resultado |
|---|---|
| `Comparator.CanonicalKey({v, "mg"})` | `"g"`, v, nil |
| `Comparator.CanonicalKey({v, "Cel"})` | `"K"`, v, nil |
| `Comparator.Compare({v, "mg"}, {1, "g"})` | error «quantity value is not finite» |
| `Comparator.Add`, `Sub` | el mismo error (`fhir/quantity.go:253`) |
| `Service.Canonical(v, "mg")`, `Convert` | v, nil |

**El mismo defecto con un `Exact` finito** (ronda adversarial, Opus y Fable;
lo pedía el primer comentario de #55). En una escala sin forma racional
(`[pH]`, `B`), `CanonicalKey` y `Compare` pasan por `q.float()`, que
redondea un `Exact` enorme a ±Inf, y la API base deja pasar ese ±Inf.
Medido sobre `main`:

| Llamada | Resultado |
|---|---|
| `CanonicalKey({Exact: 1e400, "[pH]"})` | `"m-3"`, 0, nil |
| `CanonicalKey({Exact: -1e400, "[pH]"})` | `"m-3"`, +Inf, nil |
| `CanonicalKey({Exact: 1e400, "B"})` | `"1"`, +Inf, nil |
| `Compare(1e400 [pH], 1e401 [pH])` | **0**, nil: dos valores distintos «iguales» |
| `Compare(1e400 B, 1e401 B)` | **0**, nil |

Es la regresión que #59 cerró para `Value` (`TestCompareOutOfRange`) y para
la vía racional (`fhir/quantity.go:309-321`), pero no para `Exact` en la vía
`float64`.

**El orden de los errores no es el mismo** (Opus y Fable): `Compare`
verifica la finitud antes de leer el código (`canonicalPair`), y
`Add`/`Sub` después de `ConversionFactor`: `Add(NaN Cel, 1 Cel)` da
`ErrNotLinear`, y `Add(NaN mg, 1 m)` da «not comparable».

**Un test existente fija lo contrario:** `TestCanonicalKeyEdges`
(`fhir/canonicalkey_test.go:166-176`) espera `("g", NaN, nil)` y lo mismo
para ±Inf.

Un índice que se alimenta de `CanonicalKey` puede guardar NaN o ±Inf como
valor. `Compare` y `Add` rechazan la misma cantidad, pero con un error sin
sentinela: quien llama no lo puede distinguir con `errors.Is`.

La API base deja pasar una entrada no finita a propósito (plan 003, #59; el
README lo dice en «Known limitations»: «an input that is already NaN or ±Inf
is passed through»). Eso no cambia.

### Especificación

FHIR no admite esos valores en un `decimal`. El elemento `decimal` lo
define como «A rational number with implicit precision», y su comentario
dice «Do not use an IEEE type floating point type» (los tres packages,
`StructureDefinition-decimal.json`). En JSON es un número, y JSON (RFC 8259)
no tiene NaN ni Infinity. Además, la expresión regular de
`decimal.value` (`StructureDefinition-decimal.json`, extensión `regex`) no
acepta `NaN`, `INF` ni `-INF` en ninguna versión (medido con
`re.fullmatch`):

- R4 4.0.1 y R4B 4.3.0: `-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?`
- R5 5.0.0: `-?(0|[1-9][0-9]{0,17})(\.[0-9]{1,17})?([eE][+-]?[0-9]{1,9}})?`

El patrón de R5 tiene una `}` de más que rechaza `1e5`. Queda como
Observación 2 en [hl7-issues.md](hl7-issues.md), sin publicar; no afecta a
este incremento.

Una `Quantity` de `fhir` representa un `Quantity` de FHIR, así que un
`Value` no finito no viene de un recurso: es un error de quien llama, y
`Compare` ya lo trata así.

### `gofhir/server` y otras implementaciones

- `server` no usa el paquete `fhir` de esta librería. **Descartar.**
- **Ucum-java** no tiene `CanonicalKey`; su `Decimal` no admite NaN. No
  aplica.

## 2. Alcance

Una regla para el paquete `fhir`: **una `Quantity` cuyo valor no es un número
finito de float64 no tiene clave ni se compara ni se suma**, y el error dice
cuál de los dos casos es.

1. **Sentinela** `fhir.ErrNotFinite`, con el texto de hoy («quantity value is
   not finite»), que se devuelve tal cual: no dentro de un `ValidationError`,
   porque no es un problema del código de unidad, y sin nombrar el operando,
   para no cambiar el texto que ya dan `Compare`, `Add` y `Sub`.
2. **`Value` no finito, sin `Exact`:** `CanonicalKey`, `Compare`, `Add` y
   `Sub` devuelven `ErrNotFinite`.
3. **`Exact` que no cabe en float64, en la vía `float64`** (escala sin forma
   racional): se lee como el float64 finito y distinto de cero más cercano,
   con su signo (el mayor, o el menor subnormal), en lugar de ±Inf o 0. Así
   la API base lo trata como la entrada finita y distinta de cero que es, y
   la regla de rango de #59 se aplica al **resultado**: `1e400 B` y
   `1e-400 [p'diop]` son `ErrOutOfRange`; `1e400 [p'diop]` es π/2 y
   `1e-400 [pH]` tiene la clave de 0. El error nombra el `Exact` de quien
   llama. En `Compare`, cada operando de la vía `float64` se lleva a float64
   con la regla de `CanonicalKey`: una unidad lineal con `Exact` se redondea
   una vez desde su forma exacta, y las unidades se comparan antes que el
   rango. (Rondas 1 y 2 del code review, §9.)
4. **Orden:** primero el sistema (`ErrNotUCUMSystem`), después el valor
   (`ErrNotFinite`), después el código y las unidades. Es el orden que ya
   sigue `Compare`. `Add` y `Sub` pasan a verificar el valor antes que
   `ConversionFactor`: `Add(NaN Cel, 1 Cel)` da `ErrNotFinite` en lugar de
   `ErrNotLinear`.
5. **Godoc** de `CanonicalKey`, `Compare`, `Add`, `Sub` y `ErrNotFinite`; el
   README nombra la regla de `fhir` junto a la de la API base.

Con `Exact` puesto, un `Value` NaN no da error: `Exact` manda (godoc de
`Quantity.Exact`).

Fuera:
- la API base (`Canonical`, `Convert`, `Multiply`, `Divide`): sigue dejando
  pasar lo no finito, como decidió #59;
- `ConvertDecimal`: `ParseDecimal("NaN")` y `("Inf")` ya fallan (medido por
  Opus), y un `*big.Rat` es finito; con una escala sin forma racional,
  `ConvertDecimal` ya falla con `ErrNotRational`;
- `Comparable`: no mira el valor.

**Versión:** `feat:` (minor, 4.4.0), por el sentinela exportado. Sin él
sería un `fix:`, como #62, que también convirtió un +Inf en error. Decisión
del usuario (§7).

## 3. Criterio de salida

- `make check` en verde.
- Los tests de §4 pasan y las mutaciones de §5 mueren.

## 4. Tests

1. **`CanonicalKey` rechaza un `Value` no finito:** NaN, +Inf y −Inf, con
   unidades lineales (`mg`, `mL`), especiales racionales (`Cel`, `mCel`), sin
   forma racional (`[pH]`, `B`) y arbitrarias (`[IU]`): `ErrNotFinite`,
   clave vacía y valor 0.
2. **`Exact` manda:** `Value` NaN con `Exact` 37 da la clave de 37, sin
   error.
3. **Desborde de `Exact` en la vía `float64`:** `Exact` ±1e400 y ±1e-400 con
   `[pH]` y `B` en `CanonicalKey`, y en `Compare` contra sí mismo y contra
   un valor finito: `ErrOutOfRange` dentro de un `*ucum.ValidationError`.
   `Compare(1e400 B, 1e401 B)` deja de dar 0.
4. **Orden:** `Value` NaN con un `System` que no es UCUM da
   `ErrNotUCUMSystem`; con un código inválido (`"xyz"`) o unidades no
   comparables (`Add(NaN mg, 1 m)`) o especiales (`Add(NaN Cel, 1 Cel)`),
   `ErrNotFinite`, en `CanonicalKey`, `Compare`, `Add` y `Sub`.
5. **`Compare`, `Add` y `Sub`** envuelven `ErrNotFinite` con el no finito a la
   izquierda y a la derecha, y el texto del error no cambia.
6. **Invariante (reemplaza al metamórfico, que era tautológico):** en un
   barrido de cantidades (los no finitos, 0, −1,5, 1/3, 1e300, `Exact`
   ±1e400 y ±1e-400, por las unidades del test 1), si `CanonicalKey` no da
   error, el valor que devuelve es finito; y si `Compare(a, b)` da 0 sin
   error, las dos claves son iguales. Hoy falla.
7. **`TestCanonicalKeyEdges`** se reescribe: hoy fija el comportamiento que
   este incremento cambia.
8. **La API base no cambia:** `Canonical(NaN, "mg")` sigue dando NaN sin
   error.

Los esperados salen de la regla de §2, no de correr el código.

## 5. Mutaciones

| # | Mutación | Debe fallar |
|---|---|---|
| N1 | `CanonicalKey` sin la verificación de `Value` | tests 1 y 6 |
| N2 | La verificación mira solo NaN | test 1 (±Inf) |
| N3 | La verificación mira solo +Inf | test 1 (NaN, −Inf) |
| N4 | La verificación mira `q.float()` en lugar de `Value` sin `Exact` (rechaza un `Exact` enorme como no finito) | tests 2 y 3 |
| N5 | La verificación va antes que la del sistema | test 4 |
| N6 | `CanonicalKey` lee el código antes de verificar el valor | test 4 |
| N7 | `Add` verifica después de `ConversionFactor` | test 4 |
| N8 | `Compare` no envuelve el sentinela | test 5 |
| N9 | `CanonicalKey` sin la verificación de desborde en la vía `float64` | tests 3 y 6 |
| N10 | `Compare` sin la verificación de desborde en la vía `float64` | tests 3 y 6 |
| N11 | El desborde mira solo ±Inf, no el 0 de un `Exact` distinto de 0 | test 3 (±1e-400) |

## 6. Dependencias y excepciones

Ninguna dependencia nueva. Ninguna excepción a la regla 5.

## 7. Ronda adversarial

Opus y Fable, en paralelo, sin el razonamiento del plan.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Un `Exact` enorme en una escala sin forma racional da ±Inf o 0 sin error, y `Compare` llama iguales a 1e400 y 1e401 | Opus, Fable (alta) | **Aceptado:** reproducido. Entra al alcance (§2.3), con los tests 3 y 6 y N9 a N11 |
| `TestCanonicalKeyEdges` fija lo contrario y el plan no lo nombraba | Opus, Fable (media/alta) | **Aceptado:** test 7 |
| El orden respecto de la validación del código no estaba fijado; `Add` verifica después de `ConversionFactor` | Opus, Fable (media) | **Aceptado:** §2.4, test 4, N6 y N7 |
| El test metamórfico era tautológico; una mutación sobre `q.float()` sobrevivía | Opus, Fable (media) | **Aceptado:** reemplazado por la invariante del test 6; N4 |
| La forma del error: un sentinela suelto no es un `ValidationError` | Opus (baja), Fable (baja/media) | **Decidido:** `ErrNotFinite` va suelto, como `ErrNotUCUMSystem`, porque el problema no es el código. El desborde sí va dentro de `ValidationError`, como el de la vía racional |
| `feat:` o `fix:` | Opus (baja: `feat:`), Fable (baja: el precedente de #62 es `fix:`) | **Decisión del usuario:** con sentinela, `feat:`; sin él, `fix:` |
| La cita más fuerte es la definición y el comentario de `decimal`, no solo la expresión regular | Fable, Opus (baja) | **Aceptado:** verificado en los tres packages; §1 |

## 8. Tests e implementación (pasos 5 a 7)

**Tests primero**, con `ErrNotFinite` declarado y nada más, para que fallen
por la regla y no por no compilar. Fallaron todos menos los dos que fijan lo
que no cambia (test 2, `Exact` manda; test 8, la API base). El test 4
mostró el orden de hoy: `CanonicalKey(NaN xyz)` daba el error del código, y
`Add(NaN mg, 1 m)` «not comparable».

- `fhir/nonfinite_test.go`: tests 1 a 6 y 8.
- `fhir/canonicalkey_test.go`: `TestCanonicalKeyEdges` reescrito (test 7).

**Implementación** (`fhir/quantity.go`):
- `ErrNotFinite`, y `checkFinite`, que corre después de `checkSystems` en
  `Compare`, `Add`, `Sub` y `CanonicalKey`;
- `Quantity.float` devuelve `ErrOutOfRange` para un `Exact` que no cabe en
  float64; `Compare` y `CanonicalKey` lo usan en la vía `float64`;
- `outOfRange`, el error que comparten la vía racional y la de `float64`;
- godoc de `Compare`, `Add`, `CanonicalKey` y `ErrNotFinite`; README.

**Lo que cambió al validar:** la invariante del test 6 tal como estaba en el
plan («si `Compare` da 0, las dos claves son iguales») era demasiado fuerte.
En una escala racional `Compare` es exacto: `Compare(1e400 mL, 1e400 mL)` da
0, y es correcto, mientras que `CanonicalKey` no puede dar una clave float64
y da `ErrOutOfRange`, también correcto. La invariante compara las claves
solo cuando las dos existen.

Dos mutaciones del plan 001 (M2 y M7) apuntaban al código que cambió; ahora
apuntan al nuevo y siguen muriendo.

**Mutaciones** (`scripts/mutations.py`, K75 a K85):

| # | Plan | Mutación |
|---|---|---|
| K75 | N1 | `CanonicalKey` sin la verificación |
| K76 | N2 | solo NaN |
| K77 | N3 | solo +Inf |
| K78 | N4 | la verificación ignora `Exact` |
| K79 | N5 | el valor antes que el sistema |
| K80 | N6 | el código antes que el valor |
| K81 | N7 | `Add`/`Sub` después de `ConversionFactor` |
| K82 | N8 | `Compare` sin el sentinela |
| K83 | N9 | `CanonicalKey` sin el rango en la vía `float64` |
| K84 | N10 | `Compare` sin el rango en la vía `float64` |
| K85 | N11 | el rango no ve el 0 de un `Exact` distinto de 0 |

Suite completa: [evidence/008-2026-10-28-mutations.txt](evidence/008-2026-10-28-mutations.txt),
sin resultados inesperados.

**Costo:** los tests nuevos suman 0,13 s; la invariante, 0,12 s.

**Verificación:** `make check` en verde, y la suite con `UCUM_PACKAGES` en
verde.

## 9. Code review

### Ronda 1 (Opus y Fable, en paralelo, sin el razonamiento del plan)

**No converge:** tres hallazgos medios, los tres de la vía `float64`. Los
dos corrieron `make check` y la suite de mutaciones (123, sin resultados
inesperados) e inyectaron unas 25 mutaciones propias.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| La regla rechazaba **entradas** cuya clave cabe: `1e-400 [pH]` y `1e-400 B` tienen la clave de 0, y `1e400 [p'diop]` la de π/2; en `main` las daban bien | Opus (media) | **Aceptado:** reproducido contra `main`. Un `Exact` enorme se lee como el mayor float64 finito, y la regla de rango se aplica al **resultado** (§2.3). Test 3b y K85, K86 |
| Ningún test fija que un `Exact` 0 en la vía `float64` se acepta | Opus (media) | **Aceptado:** test 3b usa 0 como referencia |
| `Compare` leía el rango de `b` antes que su código: el error de un código inválido dependía del lado | Fable (media) | **Aceptado:** con la regla nueva el rango lo decide la API base, después de leer el código. Test 4 con los dos lados |
| `Compare(1e400 mg, 1 [pH])` daba `ErrOutOfRange` en lugar de «not comparable» | Fable (baja) | **Aceptado:** se resuelve con lo anterior; en el test 4 |
| El orden sistema → valor no estaba probado en `Add` y `Sub` | Opus, Fable (baja) | **Aceptado:** test 4; K87 |
| Los campos del error de rango (código, posición) no estaban fijados | Opus, Fable (baja) | **Aceptado:** test 3; K88 |
| §2.1 decía «envuelto con `%w`», pero se devuelve tal cual | Fable (baja) | **Aceptado:** §2.1 dice lo que hace el código, y por qué |
| El README decía que `Add`/`Sub` dan `ErrOutOfRange` en la vía `float64`, a la que nunca llegan; y omitía «sin `Exact`» | Fable (baja) | **Aceptado:** README corregido |
| Comentarios de funciones no exportadas en dos idiomas | Fable (baja) | **Aceptado:** en español, la regla del repo |
| El mensaje «invalid UCUM code "[pH]"» culpa a un código válido | Opus (baja) | **Fuera de alcance:** viene de la vía racional (#59); se cambiaría en las dos vías a la vez |

Las mutaciones K83 a K85 del paso 7 cambiaron con la regla: ahora quitan el
redondeo hacia el mayor finito, lo quitan solo del operando derecho de
`Compare` y vuelven a la regla estricta de la ronda 1. M7 (plan 001) vuelve
a su texto original, porque la vía `float64` de `CanonicalKey` volvió a su
forma. Suite completa: 126, sin resultados inesperados (evidencia
actualizada).

### Ronda 2

**No converge:** un hallazgo alto y uno medio, otra vez en la vía `float64`.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Un par que mezcla `B` con una unidad **lineal** de `Exact` enorme: la lineal se leía como el mayor float64 y se multiplicaba por su factor. `Compare(308 B, 1e400 %)` daba +1; la respuesta es −1 (en `main` salía bien por casualidad, por +Inf) | Opus (alta) | **Aceptado:** reproducido. `Compare`, en la vía `float64`, lleva cada operando a float64 con la regla de `CanonicalKey` (`key`), y compara las unidades antes (`IsComparable`), para que un error de rango no tape uno de unidades. Test 3c, K84, K91, K92 |
| Un `Exact` diminuto se leía como 0 y esquivaba la regla de rango: `1e-400 [p'diop]` daba clave 0, mientras `Value 5e-324 [p'diop]` es `ErrOutOfRange` | Fable, Opus (media) | **Aceptado:** se lee como el menor subnormal con su signo. Test 3 con `[p'diop]`, `%[slope]` y `[m/s2/Hz^(1/2)]`; K89 |
| El error de rango nombraba el float64 en que se leyó el `Exact` (`1.79e+308`), no el valor de quien llama | Fable, Opus (baja) | **Aceptado:** `canonicalFloat` lo reemplaza por el error de `outOfRange`. Test 3; K90 |
| K88 lo mata un test cuyo nombre dice «float path» | Opus (baja) | **Sin cambio:** `TestExactOutOfRangeOnFloatPath` fija ahora el error de rango de las dos vías |

**Encontrado al escribir el test 3c, fuera de alcance:** `Canonical(-300, "B")`
da `9.999999999999994e-301` y `Canonical(300, "B")` `1.0000000000000006e+300`:
3 ulp lejos de 10^∓300, porque la API base calcula `math.Pow(10, x)`, que no
está correctamente redondeado. Es un defecto de la API base, candidato a
issue propio; el test usa `2 B`, cuyo 10^2 es exacto.

M7 (plan 001) apunta ahora a `canonicalFloat`, donde vive la vía `float64`.
Suite completa: 130, sin resultados inesperados (evidencia actualizada).

### Ronda 3 (tope del proceso)

**No converge por un hallazgo medio, cuya causa está en la API base.** Opus
midió 438.550 pares y Fable 332.800 en la vía `float64`: 0 violaciones de
«`Compare` = 0 ⇔ claves iguales», de antisimetría o de «error con las dos
claves existentes». Los errores de código inválido son idénticos byte a byte
a `main`. `make check` y las mutaciones, en verde para los dos.

| Hallazgo | Revisor | Resolución |
|---|---|---|
| Resultados ordinarios cambian respecto de `main`: `Compare(1 [pH], Exact 0.1 mol/L)` y `Compare(2 [pH], Exact 0.01 mol/L)` pasan de 0 a +1; `Compare(14 [pH], Exact 1e-5 nmol/L)` de −1 a 0; `Compare(−300 dB, Exact 1e-28 %)` de 0 a −1 | Opus (media), Fable (observación) | **Confirmado y medido** (sonda contra `main`): en `main`, `Compare` contradecía a `CanonicalKey` en esos pares (0 con claves distintas, −1 con claves iguales); 008 los hace coincidir. La respuesta queda mal porque la API base calcula 10^x con `math.Pow`: `1 [pH]` da `6.022140760000001e+25` m-3, 1 ulp sobre el valor correctamente redondeado. `main` acertaba en los dos primeros por un doble redondeo que compensaba. Defecto de la API base: no se rodea en `fhir` (regla 10). **Decisión del usuario** |
| El texto «not comparable» de la vía `float64` nombra los códigos de entrada; la vía racional y `main`, los canónicos | Opus, Fable (baja) | Pendiente, junto con la decisión |
| `Compare(1e300 B, 1 foo{)` da el error de `foo{` y no `ErrOutOfRange` | Opus (baja) | Intencional (§2.3); falta en la nota de versión |
| La vía `float64` hace 272 asignaciones por operación contra 194 en `main` (+40 %): `IsComparable` y `key` repiten canonicalizaciones | Opus (baja), Fable (nit) | Pendiente |
| El godoc de `Compare` puede leerse como si el `Exact` de una unidad lineal también se leyera como el mayor finito | Opus (baja) | Pendiente |

**Decisión del usuario (opción A, 2026-10-29):** cerrar 008 y dejar el
redondeo de las escalas logarítmicas como issue aparte. La sonda del paso 1 de
un posible 009 mostró que el error de `1 [pH]` **no** es de `math.Pow`: es un
doble redondeo, 10^-1 → float64(0.1) → × factor, el mismo que da un `Value`
de `0.1 mol/L`. Redondear una vez haría `1 [pH]` igual a `Exact 0.1 mol/L`
pero distinto de `Value 0.1 mol/L`; ninguna vía da «igual» en las dos. Aparte,
`math.Pow(10, n)` no da el float64 más cercano en 505 de los 632 enteros
n ∈ [-323, 308] (caso `-300 B`): ese sí sería una mejora sin costo. El cambio
para `Compare` contra `Exact` queda documentado en el README.

**Hallazgos bajos de la ronda 3, aplicados al cerrar:**
- «not comparable» nombra los códigos de quien llama en las dos vías
  (`notComparable`); test 4b; K93, K94. Cambia el texto de la vía racional
  (`m vs g` → `[in_i] vs g`).
- `IsComparable` solo corre ante un error de rango (`firstError`): en el caso
  común, la vía `float64` canonicaliza cada operando una vez. Medido con un
  benchmark de dos pares (`2 B` contra `Exact 1/10 %`, `7.4 [pH]` contra
  `1e-7 mol/L`): 700 asignaciones por iteración, contra 705 en `main`.
- El godoc de `Compare` separa el operando de escala sin forma racional del
  lineal.
- La precedencia `Compare(1e300 B, 1 foo{)` → error de `foo{` queda en el
  README por la regla «código antes que rango».

No hubo cuarta ronda: el proceso tiene tope de 3 y el usuario decidió cerrar.
