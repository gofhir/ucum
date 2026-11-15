# 022 — Las entradas de dos códigos expanden uno antes de analizar el otro

**Estado:** paso 9 (cierre), 2026-11-15. Code review convergido en la ronda 3 (§10). Mutaciones: [`evidence/022-2026-11-15-mutations.txt`](evidence/022-2026-11-15-mutations.txt), todas como se esperaba (K256 sobrevive, equivalente). `make compare-java` sin cambios respecto del informe del 2026-11-03.

## 1. Problema

`Convert` analiza los dos códigos y recién después expande ninguno. Las otras
entradas de dos códigos del motor (`internal/engine`) expanden el primero
antes de mirar la sintaxis del segundo, con `canonicalParts` o `getCanonical`
en sucesión:

- `IsComparable(code1, code2)` (`convert.go`);
- `Multiply` y `Divide` (`combine`, `convert.go`);
- `ConversionFactor(from, to)` y `ConvertRat(value, from, to)` (`exact.go`).

Con un primer código válido y caro y un segundo mal escrito, el error llega
tarde. Medido el 2026-11-15 en `main` (v4.5.1, Go 1.27, arm64), con `from` =
`'1000.'1000.'1000.'1000.'1000` (válido, 29 bytes, XML oficial) y `to` = `m/`:

| Entrada | Hasta el error de sintaxis de `m/` |
|---|---|
| `Convert` | 0 ms |
| `IsComparable` | 1.33 s |
| `Multiply` | 1.30 s |
| `ConversionFactor` | 1.29 s |
| `ConvertRat` | 1.29 s |

019 §10 (ronda 1, b, baja) lo midió en `ConversionFactor` (185-898 ms) y lo
dejó para este incremento. En una llamada que termina bien no se pierde nada:
hay que expandir los dos códigos igual. El costo de expandir es de 019. Una
llamada que falla puede tardar más que antes: con un primer código caro y un
segundo que falla al expandirse (`m/0`), la regla expande el primero antes,
como ya hace `Convert` (`Compare(exp, m/0)`, 1.85 s; code review, ronda 1).

## 2. Revisión previa

- **Especificación:** UCUM no dice en qué orden se validan dos códigos. El
  orden de los errores es una decisión de esta librería.
- **Orden actual:** `Convert` informa, en este orden, el error de sintaxis de
  `from`, el de sintaxis de `to`, el de expansión de `from` y el de expansión
  de `to`. Las demás informan el de `from` (sintaxis o expansión) antes que
  cualquiera de `to`.
- **`gofhir/server` (congelado):** usa `gofhir/ucum/v4` v4.2.0, que tiene el
  mismo orden. Nada que tomar.
- **`fhir`:** `ConvertDecimal` y `Comparable` llaman a `ConvertRat` e
  `IsComparable` del servicio, así que se arreglan con el motor. `Add` y `Sub`
  llaman a `ConversionFactor(b.Code, a.Code)`, al revés de sus argumentos.
  `Compare` canonicaliza cada cantidad por su cuenta con `CanonicalRat`:
  `Compare` de una cantidad cara y otra con `m/` tarda 2 s (ronda 1, a y b).
- **Errores que solo aparecen al expandir, con el XML oficial** (ronda 1,
  alta, a y b): `m/0` (división por cero), una unidad especial con exponente
  (`Cel2`, `[pH]2`, `[degF]-1`, `Np3`) o dentro de un término (`[pH]/m`).
  Pasan `Validate`. Una unidad desconocida (`xyz`) ya es un error de análisis.
  El exponente que no cabe de 018 no llega con el XML oficial: el analizador
  acota los exponentes en ±1000.
- **Caché (006, #61):** analizar el segundo código antes guarda a lo sumo un
  AST más en una llamada que falla; las claves se siguen clonando.

## 3. Cambio

1. Un ayudante del motor analiza los dos códigos (`parseCached`, con su
   caché) y devuelve el error sin envolver; cada entrada lo envuelve como hoy
   (`Validation(code1|code2)` en `IsComparable`, `Validation(v1|v2.Code)` en
   `combine`, `Conversion(from, to)` en las exactas). Lo usan `IsComparable`,
   `combine`, `ConversionFactor` y `ConvertRat` (después del control de
   `nil`). `Convert` ya tiene ese orden.
2. `ConversionFactor` decide `ErrNotLinear` con los AST, antes de expandir:
   `specialUseForTerm` solo mira el término (ronda 1, b3; hoy `CF(exp, Cel)`
   tarda 1.3 s).
3. `fhir.Compare` llama a `Validate` de `a.Code` y de `b.Code` después de
   `checkFinite` y antes de canonicalizar (`Add` y `Sub`: ver el desvío más
   abajo). Un `Validate` con el código en caché cuesta unos 11 ns; en el code
   review, un `Compare` de 1 m con 100 cm subió unos 25 ns (de 705-725 a
   731-744 ns, un 3.5 %) en una medición y no cambió en otra.
4. El godoc de `IsComparable`, `ConversionFactor` y `ConvertRat` dice el
   orden de los errores.

**Desvío en la implementación (2026-11-15), resuelto en el code review.** El
`Validate` previo de §3.3 en `Add` y `Sub` sobraba: `ConversionFactor` ya
analiza los dos códigos antes de expandir, y el mutante que lo quitaba (K258)
sobrevivió. Para el orden de los argumentos, la implementación llamaba a
`IsComparable(a, b)` cuando `ConversionFactor(b, a)` fallaba por un código. Eso
cambiaba el tipo del error de `ConversionError` a `ValidationError` y expandía
dos veces los dos códigos cuando la unidad de a vale cero (code review, ronda
1). Por decisión del usuario, `Add` y `Sub` vuelven a v4.5.1: informan los
errores en el orden de la conversión de b a a (la sintaxis de b, la de a, la
expansión de b y la de a), con el mismo tipo, el mismo mensaje y el mismo
costo. El godoc de `Add` lo dice y §5 se ajusta. El caso lento de §1 ya lo
resuelve el motor. `Compare` conserva el `Validate` previo.

**Cambio observable** (va en el cuerpo del commit, que release-please lleva
al CHANGELOG): cuando el primer código falla solo al expandirse (`m/0`,
`Cel2`, `[pH]/m`) y el segundo tiene un error de análisis, hoy se informa el
del primero y después el del segundo, como ya hace `Convert`; `Multiply` y
`Divide` nombran otro `ValidationError.Code`. `ConversionFactor(Cel, m/0)` da
`ErrNotLinear` en vez de la división por cero. En `Add(a, b)` y `Sub(a, b)` el
primer código es b, el origen de la conversión. Los centinelas y los tipos de
cada error no cambian.

## 4. Criterio de salida

- Las entradas del motor, `fhir.Compare`, `Add` y `Sub` fallan por el
  análisis del segundo código sin expandir el primero. Se comprueba por la
  identidad del error, sin reloj ni gancho en producción: con un primer código
  que solo falla al expandirse (`m/0`, `Cel2`), expandirlo primero daría su
  error y no el del segundo (ronda 1, a4 y b5).
- El orden de los errores es el de `Convert` en todas, con un test por cada
  combinación.
- Los resultados de las llamadas válidas no cambian (los tests actuales y
  `make compare-java`).
- `make check`, `make check-packages`, mutaciones completas.

## 5. Tests (primero)

1. Tabla de precedencia por entrada (`IsComparable`, `Multiply`, `Divide`
   por separado, `ConversionFactor`, `ConvertRat`, `fhir.Compare`, `Add`,
   `Sub`, `ConvertDecimal`, `Comparable`): (análisis, análisis) → el del
   primero; (expansión, análisis) → el del segundo; (análisis, expansión) → el
   del primero; (expansión, expansión) → el del primero. Errores de análisis:
   `m/`, `xyz`, `""`, `m{`; de expansión: `m/0`, `Cel2`, `[pH]/m` y un ciclo
   en un XML propio. En `Add` y `Sub` el primero es b, el origen de
   `ConversionFactor(b, a)` (§3). Los esperados salen de la regla de §3, no de correr el
   código.
2. `ConversionFactor(Cel, m/0)` → `ErrNotLinear`; `CF(m/0, Cel)` →
   `ErrNotLinear` del destino, sin expandir `m/0`.
3. `Multiply(m/0, m/)` nombra `m/` en `ValidationError.Code`.
4. `ConvertRat(nil, "m/", "m")` → `ErrNilValue` (el control de `nil` va
   primero).
5. El servicio que ignora mayúsculas: `IsComparable("m/0", "M/")`.

## 6. Mutaciones previstas

- Una entrada que vuelve a expandir el primer código antes de analizar el
  segundo (una por entrada, `Divide` aparte de `Multiply`).
- El ayudante que analiza solo el primero.
- El orden de los errores de expansión invertido.
- `ErrNotLinear` de vuelta después de expandir.
- `Compare`, `Add` o `Sub` sin el `Validate` previo, o con el de `b` primero.
- El análisis antes del control de `nil` de `ConvertRat`.
- El ayudante con `parser.Parse` en vez de `parseCached`: misma salida, sin
  caché; se registra como equivalente si ningún test lo mata.
- Mutaciones completas.

## 7. Fuera de alcance

- El costo de expandir (019), incluidos dos códigos iguales que se expanden
  dos veces (`IsComparable(exp, exp)`, 2.7 s) y el segundo segundo de
  `Multiply` después de expandir (ronda 1, b8 y b9).
- `ValidateCanonicalUnits`: su segundo código es la forma esperada, y un
  error en él se informa como «does not reduce», a propósito. Con un primer
  código caro y `m/` tarda 1.9 s (ronda 1, a7).

## 8. Versión

`fix:` → 4.5.2.

## 9. Ronda adversarial del plan

### Ronda 1 (2026-11-15)

Dos Opus sobre copias. (a) prototipó el análisis previo en las cuatro
entradas: `go test ./...` en verde y los casos lentos, de 1.3 s a 1 µs.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Los errores que solo aparecen al expandir no piden un XML propio: `m/0`, `Cel2`, `[pH]/m` con el oficial (verificado: `Validate("m/0")` es nil); el desborde de 018 no llega con el oficial | a, b (alta) | **Aceptado:** §2, §3 y §5 |
| `fhir.Compare` fuera de alcance deja a `fhir` inconsistente consigo mismo, y `Validate` en caché cuesta 11 ns | a, b (alta) | **Aceptado:** dentro (§3.3) |
| `ErrNotLinear` se decide con el AST; hoy espera las dos expansiones (1.3 s) | b (media) | **Aceptado** (§3.2), con su cambio de orden |
| `Add` y `Sub` llaman a `ConversionFactor(b, a)` | a (media) | **Aceptado:** `Validate` de `a` y de `b` primero (§3.3) |
| No hace falta un gancho: la identidad del error prueba el orden | a, b (media) | **Aceptado** (§4) |
| Mutantes que sobrevivirían: el control de `nil`, `Divide` aparte, `parser.Parse` | a (media) | **Aceptado** (§5, §6) |
| Faltan `xyz`, `""`, `m{` y el servicio sin mayúsculas | b (media) | **Aceptado** (§5) |
| Los envoltorios no cambian si el ayudante no envuelve | b (baja) | **Aceptado** (§3.1, §5.3) |
| La caché de 006 no cambia | b (baja) | Registrado (§2) |
| Dos códigos iguales se expanden dos veces; `Multiply` gasta 1 s más | b (baja) | §7, de 019 |
| `ValidateCanonicalUnits` tarda 1.9 s | a (baja) | §7 |
| El godoc no dice el orden | a (baja) | **Aceptado** (§3.4) |

Sin altas abiertas: el alcance creció con `Compare`, `Add`, `Sub` y
`ErrNotLinear`.

## 10. Code review

### Ronda 1 (2026-11-15)

Dos Opus sobre copias, con mutantes propios (a: 13 más, todos muertos).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| `Add` y `Sub` pasan de `ConversionError` a `ValidationError` por el respaldo con `IsComparable` | a (alta), b (media) | **Aceptado:** el respaldo se quitó; `Add` y `Sub` como en v4.5.1, por decisión del usuario (§3) |
| El respaldo expande dos veces cuando la unidad de a vale cero (`Add("0", …)`: 2.58 → 5.16 s) | a, b (media) | **Aceptado:** se quitó con el respaldo |
| `IsComparable` puede nombrar code1 por un error de code2, y `combine` code2 por uno de code1, sin que falle un test | b (media) | **Aceptado:** la tabla mira `ValidationError.Code`; K262 y K263 mueren |
| El orden de `ErrNotLinear` con origen y destino especiales no tiene test | b (baja) | **Aceptado:** test y K264 |
| Una llamada que falla puede tardar más (`Add(exp, m/0)`, 2.75 µs → 1.28 s) | a, b (baja) | §1; con `Add` como en v4.5.1, `Add` vuelve a fallar rápido; `Compare` lo registra §1 |
| `Compare` sube unos 25 ns, no «menos del 1 %» | a (baja) | **Aceptado:** §3.3 corregido |
| Falta K258 en la numeración | a, b (baja) | **Aceptado:** registrado en `scripts/mutations.py` (también K260 y K261, retirados con el respaldo) |

### Ronda 2 (2026-11-15)

Los dos revisores confirman resueltos los hallazgos de la ronda 1 (a midió
`Add("0."+exp, exp)` en 1.43 s y `Add(exp, m/0)` en 3.3 µs, como en v4.5.1).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El error de expansión del segundo código puede nombrar al primero en `ValidationError.Code` (`canonicalPairParts`, y el `can2` de `IsComparable`, anterior a 022) | b (media) | **Aceptado:** `TestTwoCodeNamesTheFailingCode` con un código válido de cada lado; K265 y K266 mueren |
| Nada mira `ConversionError.From`/`To` de `ConvertRat` y `ConversionFactor` (también en `main`) | a (baja) | **Aceptado:** la tabla los mira; K267 muere |

### Ronda 3 (2026-11-15)

Converge: los dos revisores confirman resueltos los hallazgos de la ronda 2
(K265-K267 y sus propios mutantes, muertos) y no dejan hallazgos de severidad
media o mayor.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| `From`/`To` invertidos en el error de expansión del destino sobreviven en `ConversionFactor`, `ConvertRat` y `Convert` (anterior a 022) | a, b (baja) | **Aceptado:** `TestTwoCodeNamesTheFailingCode` corre en todas las entradas y mira `From`/`To`; K268-K270 mueren |
| El costo de `Compare` dice «menos del 1 %» | a (baja) | Ya corregido en la ronda 1 (§3.3) |
