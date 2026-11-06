# 019 — el costo de los racionales exactos sin cota

**Estado:** en pausa en el paso 3, tras dos rondas del plan sin converger (§10). El usuario eligió el 2026-11-06 hacer antes 020, 021 y 022, y retomar con la tercera ronda.

## 1. Problema

Con el XML oficial y `ucum.New()`, un código corto del llamador cuelga la
llamada. Medido el 2026-11-06 en `main` (v4.4.10), Go 1.27.2, arm64,
`Canonical(1, ·)`:

| Código | Bytes | Tiempo | Resultado |
|---|---|---|---|
| `'` | 1 | 28 µs | `rad` |
| `'1000` | 5 | 51 ms | fuera de rango de `float64` |
| `'1000.'1000` | 11 | 229 ms | fuera de rango |
| `'1000.'1000.'1000` | 17 | 554 ms | fuera de rango |
| `'1000.'1000.'1000.'1000.'1000` | 29 | 1.9 s | fuera de rango |
| 125 bytes (018 §9) | 125 | 73 s | — |
| `[pi]100.[pi]100` | 15 | 2.7 ms | `1` (con el valor) |
| `Gb1000` | 6 | 53 ms | `C1000.s-1000` |

`Validate` tarda microsegundos en todos: la validación sintáctica no
protege. `MaxCodeLength` (1024 bytes) admite `'1000` unas 170 veces: unos
85 M bits de numerador, horas de cálculo y gigabytes de memoria.

**Causa.** `[pi]` tiene 64 decimales (denominador 10^64) y las unidades que
lo usan (`'`, `''`, `deg`, `Oe`, `Gb`, `[mu_0]`, `Lmb`) llevan unos 500 bits.
El perfil es casi entero `lehmerGCD` dentro de `big.Rat.norm`, que corre en
cada `Mul`, `Quo` y `SetFrac`. El máximo común divisor de Go es cuadrático
(medido, dos números al azar): 2^16 bits 2 ms, 2^17 8 ms, 2^18 32 ms, 2^19
128 ms, 2^20 518 ms. Multiplicar números del mismo tamaño cuesta 0.18, 0.34,
0.95, 2.8 y 9 ms. **El costo es el máximo común divisor, no el tamaño.**

Lo mismo con un XML propio (`3 [d].[d]` 8.8 s, 018 §9) y con un `*big.Rat`
enorme que el llamador pasa a la API exacta.

**Carga.** `decimal.FromString` hace `big.Int.Exp(10, e)` sin tope: un XML
propio con `value="1e1000000000"` agota la memoria en `NewFromReader`.

## 2. Revisión previa

- **`gofhir/server` (congelado):** usa `gofhir/ucum/v4` v4.2.0. Nada que
  tomar.
- **Ucum-java 1.0.10:** no es exacto (`'` = `0.000290888208665721596153948`,
  27 dígitos, en 9 ms) y es más lento: `'1000` no terminó en tres corridas
  de 3.5 a 7 minutos. No hay referencia que tomar para el costo; la
  comparación de valores sigue siendo `make compare-java`.
- **Especificación:** UCUM no exige aritmética exacta ni un tamaño de
  número. §19 ■1 deja a la implementación no representar todas las
  dimensiones; tras §20, «Most practically used units contain exponents of
  -4 to +4». Un límite de implementación documentado es legítimo, como
  `MaxCodeLength`, `MaxNestingDepth` y `MaxExponent` ya lo son.
- **Arreglos de algoritmo** (prototipo en una copia, 2026-11-06; tests de
  `decimal` y `engine` en verde, mismos resultados):
  - `Pow` multiplica potencias de una fracción reducida, que ya son
    reducidas (mcd(a, b) = 1 ⇒ mcd(aⁱ, bʲ) = 1), y aun así normaliza cada
    producto: elevar numerador y denominador por separado no necesita
    ningún máximo común divisor.
  - `Mul` normaliza el producto entero; con cancelación cruzada (mcd(a₁, b₂)
    y mcd(a₂, b₁)) cada máximo común divisor es de la mitad de tamaño, y
    lineal si un operando es chico.
  - La vía `float64` (`canonicalScalar`) multiplica `1` por el valor enorme
    y lo normaliza otra vez antes de ver que no cabe.

  Con los dos primeros: `'1000` 51 → 22 ms, ×2 229 → 130 ms, ×3 554 → 288
  ms, ×5 1.9 → 0.74 s, `'1000/'1000` 3 ms. Ayuda 2.5×, pero **no acota** un
  código de 1024 bytes.
- **Un tope de tamaño tampoco acota** (018 §9): `'1000./'1000.'1000./'1000…`
  mantiene cada intermedio bajo el tope y paga, en unos 200 términos, un
  máximo común divisor del tamaño del tope cada vez (200 × 128 ms con
  2^19 bits).

**Tomar, mejorar, descartar:** se toma el arreglo de algoritmo (mismos
resultados); se descarta el tope de tamaño de 018 (no acota el tiempo); se
agrega un presupuesto de trabajo, que acota el tiempo para cualquier forma
del código.

## 3. Cambio

> Reescrito tras la ronda 1 (§10): el presupuesto de trabajo no acotaba el
> tiempo. El usuario eligió el 2026-11-06 el tope de tamaño con
> normalización diferida.

1. **Normalización diferida en `decimal`.** `Decimal` guarda numerador y
   denominador (`big.Int`, signo en el numerador, denominador > 0) sin
   exigir términos mínimos. `Mul`, `Div` y `Pow` solo multiplican (`Pow`
   eleva cada uno con `Exp`; `Div` y un exponente negativo intercambian,
   llevando el signo al numerador): ningún máximo común divisor.
   - `Float64` divide sin reducir (el cociente es el mismo).
   - `Sign`, `IsZero` y la comparación no necesitan reducir.
   - `Rat()` reduce (un `SetFrac`), y es la única puerta hacia `big.Rat`:
     las salidas exactas, la comparación con el valor del llamador y la
     aritmética que hoy está en `engine`, `special` y `fhir` pasan por
     `decimal` o reciben un valor ya bajo el tope.
   - `IsInt` y `String` miran el valor reducido (ronda 1: `4/2` no es
     `IsInt` sin reducir).
   - El caché de definiciones (017) guarda el valor reducido: se reduce una
     vez por definición.
2. **Tope de tamaño exacto, `ErrValueTooLarge`.** Antes de cada producto,
   cociente o potencia, `decimal` acota el tamaño del resultado por los bits
   de los operandos (bits(a·b) ≤ bits(a)+bits(b); bits(aⁿ) ≤ n·bits(a)) y da
   `ErrValueTooLarge` sin calcular si pasa del tope. El tope se aplica al
   numerador y al denominador por separado, y a todo `*big.Rat` que entra
   por la API exacta (el valor del llamador).
   - Como el tamaño no depende del caché ni del orden, el resultado es
     determinista (ronda 1, media 3).
   - El tope sale de un requisito funcional, no de un reloj: **toda unidad
     del XML oficial con exponente ±1000** (el máximo que admite el análisis
     sintáctico) cabe, con margen. Se fija con un test que recorre el XML y
     se documenta en bits; no hay opción pública (ronda 1, media 3).
   - Con el tope, cada operación es un producto subcuadrático de números
     acotados, y la llamada hace a lo sumo unas pocas reducciones (`Rat()`)
     de números acotados.
3. **`Log10Rat` en tiempo casi lineal.** Hoy quita los diez finales uno por
   uno (`stripTens`, cuadrático, ronda 1 alta 1). Se reemplaza por la
   estimación con `MantExp` del numerador y del denominador y una prueba
   exacta de potencia de diez solo cuando la estimación cae cerca de un
   entero.
4. **`ConversionFactor` y las demás entradas de dos códigos** analizan los
   dos códigos antes de expandir ninguno (ronda 1, baja 6).
5. **Carga:** `decimal.FromString` acota el literal completo (dígitos de la
   mantisa y exponente) con el mismo tope en bits; un literal que no cabe es
   un error de carga que nombra la unidad o el prefijo. El XML oficial llega
   a ±30 en el exponente y 74 bytes en el literal.
6. **README:** la sección de límites explica el tope, qué códigos válidos
   rechaza (patológicos: varios exponentes de cientos sobre unidades con
   `[pi]`) y por qué (§19 ■1; el costo cuadrático del máximo común divisor).

Se descarta el atajo de `float64` del plan anterior: con la normalización
diferida el valor fuera de rango ya no cuesta un máximo común divisor, y no
ayudaba hacia destinos logarítmicos (ronda 1, baja 7).

## 4. Criterio de salida

- **Tiempo acotado en todas las entradas públicas:** una sonda (no un test
  unitario; ronda 1, media 4) con el XML oficial recorre formas de 1024
  bytes (`'1000` repetido, alternado con cocientes, `Ym1000` ×146, productos
  de unidades distintas con `[pi]`, destinos especiales y logarítmicos como
  `B[W]`, `Multiply`/`Divide`, `ConvertRat` con un `*big.Rat` enorme) y
  ninguna pasa de 1 s en la máquina de referencia. Su salida va a
  `docs/plans/evidence/`.
- Los resultados bajo el tope son idénticos a `main`: barrido del XML
  oficial byte a byte (como 018) y `make compare-java`.
- Un XML con `value="1e1000000000"` o con una mantisa enorme: error de carga
  en milisegundos.
- Sin costo en las rutas calientes: asignaciones y tiempo de los benchmarks
  iguales o mejores.
- `make check`, `make check-packages`, `make compare-java`, mutaciones
  completas.

## 5. Tests (primero)

1. Los códigos de §1 y los de la ronda 1 (`W.'1000.rad-1000` → `B[W]`,
   `Ym1000` ×146): el valor esperado por aritmética para los que caben, o
   `ErrValueTooLarge`.
2. El requisito del tope: toda unidad del XML oficial, sola, con exponente
   +1000 y -1000, en `Canonical` y `CanonicalRat`, no da `ErrValueTooLarge`.
3. `decimal` contra `big.Rat` en miles de pares al azar y de borde (signos,
   cero, denominador 1, exponentes negativos, `-2/3` a la -1): mismo valor
   reducido en `Rat()`, mismo `Float64`, mismo `IsInt`, mismo `String`, misma
   comparación.
4. El tope: se aplica exactamente donde las cotas de bits lo dicen (a un bit
   del borde, adentro y afuera); al valor del llamador; frío y caliente
   iguales; concurrente (`-race`).
5. `Log10Rat` contra la versión actual en potencias de diez exactas, vecinos
   de potencias de diez y valores al azar.
6. Carga: literales en el borde del tope, adentro y afuera; `"1/3"`.
7. `fhir` y el servicio insensible con los mismos códigos.

## 6. Mutaciones previstas

- `Pow` que no intercambia con exponente negativo, o deja el signo en el
  denominador.
- `Rat()` sin reducir; `IsInt` sobre el valor sin reducir.
- El tope sin chequear en `Mul`, `Div`, `Pow` o en el valor del llamador; con
  el borde corrido en uno.
- `Log10Rat` sin la prueba exacta de potencia de diez.
- `FromString` sin el tope.
- Una entrada de dos códigos que vuelve a expandir antes de analizar.
- Mutaciones completas.

## 7. Fuera de alcance

- La base coprima de los valores del XML (ronda 1): más rápida pero más
  invasiva; alternativa si el tope no basta.
- El mensaje `invalid UCUM code "<operando>"` de `combine` (018 §10).
- Los pendientes del punto de reanudación.

## 8. Dependencias

Ninguna.

## 9. Versión

`feat:` → 4.5.0: un sentinel nuevo (`ErrValueTooLarge`) y códigos válidos
pero patológicos que pasan a dar error.

## 10. Ronda adversarial del plan

### Ronda 1 (2026-11-06), sobre el presupuesto de trabajo

Dos Opus sobre copias, con sondas en el XML oficial (pl019a/probe,
pl019b/cmd/probe019).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| `Log10Rat` (`stripTens`, un `QuoRem` por 10 por cada diez final) es cuadrático y está fuera del presupuesto: `Convert(1, "W.'1000.rad-1000", "B[W]")`, 16 bytes y resultado finito, tarda 1.2 s según (b) y 3.0 s en mi medición en `main`; 32 bytes, 4.1 s | b (alta) | **Confirmado** |
| Los productos se cobran lineales: `Ym1000` ×146 (1021 bytes) no hace ningún máximo común divisor y tarda 2.1 s (`Canonical`) y 2.5 s (`CanonicalRat`), 11.6 M bits | a (alta) | **Confirmado** por el perfil de (a): 60 % `basicMul`, sin `lehmerGCD` |
| Hay aritmética de `big.Rat` fuera de `decimal` sin cobrar (`exact.go`, `convert.go`, `range.go`, `special.go`, `fhir/decimal.go`, el `*big.Rat` del llamador): un `Mul` entre valores bajo el presupuesto tarda 4.1 s, un `Quo` 1.7 s | a, b (alta) | Confirmado por la lista de sitios |
| Hay un diseño más simple: normalización diferida (construir `'^4000` sin máximos comunes divisores tarda 6.4 ms; un `SetFrac` final, 594 ms) más un tope de tamaño exacto calculado antes de multiplicar; o una base coprima de los numeradores del XML calculada al cargar | b (alta) | A decidir con el usuario |
| El resultado depende del caché y de los reintentos con un XML propio (`X.Y` falla en frío y funciona tras `X`) | a (media) | Desaparece con un tope de tamaño: el tamaño no depende de la historia |
| `MaxValueWork` y `WithMaxValueWork` atan la API a un modelo de costo; calibrar «1 s en la máquina de referencia» depende de la máquina | b (media) | Aceptado: el límite se fija en bits por un requisito funcional; sin opción hasta que alguien la pida |
| Los tests de §5.1 no detectan una operación sin cobrar | b (media) | Aceptado: una sonda con límite de reloj holgado (5×) sobre todas las entradas públicas, más una mutación por sitio |
| El tope de `FromString` no basta: `SetString` acepta `"1/3"` y la mantisa no tiene tope | b (media) | Aceptado: acotar el literal entero |
| Armar el `Rat` con `Num()`/`Denom()`: en un `Rat` cero, `Denom()` no es referencia (3 y 7 dan `3/1`); el signo queda en el denominador (`3/-2`); y un valor no reducido rompe `IsInt` | a (media) | Aceptado: `SetFrac` con el signo en el numerador, o un tipo propio |
| `ConversionFactor` expande `from` antes de analizar `to` (185-898 ms antes de un error de sintaxis) | b (baja) | Aceptado: analizar los dos códigos primero |
| El atajo de `float64` solo es exacto con cotas conservadoras (a−b−1 ≥ 2098; a−b+1 ≤ −2099) y sin entradas no finitas ni cero; no ayuda hacia `B[W]` | a, b (baja) | Aceptado si el atajo sigue en el plan |

### Ronda 2 (2026-11-06), sobre el tope de tamaño

(a) prototipó el §3 (normalización diferida, tope de 2^19 antes de cada
`Mul`, `Div` y `Pow`, caché con valores reducidos): los tests existentes
pasan sin cambios y, bajo el tope, todos los caminos públicos que probó
quedan bajo 300 ms salvo los de la tabla.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| `fhir.Decimal.String` (`decimalExponent`) da un paso con máximo común divisor por cada dígito decimal: `ConvertDecimal(1.5, "'300", "rad300").String()` 6.1 s en `main`; `ParseDecimal("1.0e300000").String()` 31 s sin UCUM de por medio | a (alta) | Defecto propio, independiente del tope: **incremento aparte** |
| La cláusula «o reciben un valor ya bajo el tope» deja abierta la alta 3 de la ronda 1: un `Mul` de dos valores reducidos de unos 500 k bits tarda 418-552 ms, un `Quo` 497-644 ms, y `exact.go`, `convert.go`, `range.go` y `special.go` encadenan varios; hay 145 sitios de `big.Rat` fuera de `decimal` | b (alta) | Pendiente |
| El tope no tiene número: `[cml_i]` ±1000 llega a 260 k bits, `YLmb1000` a 306 k, y las entradas de dos códigos suman los dos lados (`[cml_i]1000` → `[cml_i]1000`, 487 k; `YLmb1000` → `yLmb1000`, 601 k) | a (media), b (alta) | Pendiente: (a) propone 2^19, (b) 2^20 |
| En un fallo del caché, devolver el valor sin reducir y guardar el reducido hace que el tamaño visto cambie entre frío y caliente (164 de 582 códigos `unidad±1000`) | a (alta) | Aceptado: devolver también la copia reducida (0 de 582 difieren) |
| A 2^20 una llamada con 2-3 reducciones ya ronda 1 s; `Convert(W.'1000.rad-1000 → B[W])` hace 9 `Rat()` | a, b (media) | Pendiente: reducir a lo sumo una vez por valor y llamada, con una marca de «reducido», y contarlas en un test |
| `Float64` sin reducir con `big.Float(53).Quo` redondea dos veces en los subnormales (2^-1075·(1+2^-60) da 0 en vez de 5e-324) | a (media) | Aceptado: portar `quotToFloat64`; casos subnormales en §5.3 |
| Códigos que reducen bien pasan a fallar: `'1000/'1000.'1000`, `[pi]1000/[pi]1000.[pi]1000/[pi]1000` (en `main`, milisegundos) | a, b (media) | Aceptado: el README los nombra y §5.1 los afirma |
| El alcance es demasiado grande para un incremento | b (media) | Pendiente: partir |
| Mutaciones que sobreviven sin operandos asimétricos (`Div` que suma num+num) ni casos 2/4 contra 1/2; `productCode` puede dar `'2000`, sobre `MaxExponent` | b (baja) | Aceptado |
| `fhir` `Mul`/`Quo`/`Add` sobre valores del llamador sin cota | a (baja) | Pendiente |

Tras dos rondas sin converger, se consulta al usuario (queda una ronda).
