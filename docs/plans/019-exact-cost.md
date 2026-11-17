# 019 — el costo de los racionales exactos sin cota

**Estado:** detenido en el paso 3 (2026-11-16). La tercera y última ronda del plan no convergió (§10, ronda 3); se consulta al usuario, sin cuarta ronda. El usuario había elegido el 2026-11-06 hacer antes 020, 021 y 022, ya cerrados; §3 se reescribió para la ronda 3.

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

> Reescrito para la ronda 3 (2026-11-16). Lo que ya no está en este
> incremento:
> - `Log10Rat` cuadrático (ronda 1, alta 1): cerrado por 021 (#112).
>   `Convert(1, "W.'1000.rad-1000", "B[W]")` tarda hoy 161 ms (3.0 s antes).
> - Entradas de dos códigos que expandían antes de analizar (ronda 1, baja
>   6): cerrado por 022 (#114).
> - `fhir.Decimal.String` cuadrático (ronda 2, alta 1): cerrado por 020 (#108).
> - **Partido** (ronda 2, media 8): la aritmética de `fhir` sobre valores del
>   llamador (`ParseDecimal`, `Add`, `Mul`, `Quo`; ronda 2, baja 10) pasa a
>   un incremento propio, 023, después de este. 019 queda en `decimal`,
>   `engine`, `special` y la API exacta de la raíz.
>
> Medido hoy en `main` (v4.5.2, Go 1.27.2, arm64; sonda en el scratchpad):
> `'1000` 52 ms, ×2 230 ms, ×3 556 ms, ×5 1.8 s, `'1000/'1000.'1000` 134 ms,
> `Gb1000` 52 ms, `CanonicalRat(1, Ym1000×146)` 1.1 s.

### 3.1 Normalización diferida en `decimal`

`Decimal` guarda numerador y denominador (`big.Int`, signo en el numerador,
denominador > 0) sin exigir términos mínimos, más una marca `reduced`.

- `Mul`, `Div` y `Pow` solo multiplican (`Pow` eleva cada uno con `Exp`;
  `Div` y un exponente negativo intercambian y llevan el signo al
  numerador): ningún máximo común divisor. `Add`/`Sub` (solo los usa la
  escala de temperatura, con valores chicos) hacen a·d ± c·b sobre b·d.
- `Sign`, `IsZero` y la comparación (a·d contra c·b) no reducen.
- `Float64` divide sin reducir con un port de `quotToFloat64` de `math/big`
  (un solo redondeo, también en los subnormales; ronda 2, media 6). Las
  salidas `float64` (`Convert`, `Canonical`, `Multiply`, `Divide`, las
  escalas especiales) **no hacen ningún máximo común divisor**.
- `Rat()` reduce (un `SetFrac`) y es la única salida hacia `big.Rat`. La
  marca evita reducir dos veces el mismo valor.
- `IsInt` y `String` miran el valor reducido (ronda 1: `4/2` no es `IsInt`
  sin reducir).
- El caché de definiciones (017) guarda el valor reducido y, en un fallo,
  devuelve esa misma copia reducida (ronda 2, alta 4): el tamaño visto no
  depende del caché.

### 3.2 La aritmética exacta de `engine` y `special` pasa a `decimal`

Ronda 2, alta 2: hoy `exact.go`, `convert.go`, `product.go`, `range.go`,
`engine.go` y `special.go` toman `Value.Rat()` (reducido) y encadenan
`big.Rat.Mul`/`Quo`, cada uno con un máximo común divisor. Cada cadena pasa a
operar sobre `Decimal` y sale una sola vez: a `float64` con `Float64` (sin
reducir) o a `*big.Rat` con `Rat()` (una reducción) en las salidas de la API
exacta (`ConversionFactor`, `ConvertRat`, `CanonicalRat`).

- Sitios: los de `grep -n 'big.Rat' internal/engine internal/special`
  (`convertRatCore`, `CanonicalRat`, `ConversionFactor`, `combine` y
  `mappedRats`, `productResult`, `roundRat`/`linearTimes`, `halfPiOfPi`, los
  handlers de `special`). La lista exacta va en el registro.
- `MulExact` y `roundRat` reciben `Decimal` y redondean una vez con el port
  de `quotToFloat64`.
- **Presupuesto de reducciones:** a lo sumo **una** por llamada pública
  exacta, sobre el resultado final; cero en las salidas `float64`. Se fija
  con un contador interno de `decimal` (paquete `internal`, solo para tests)
  y un test que lo lee en cada entrada pública (ronda 2, media 5).

### 3.3 Tope de tamaño exacto, `ErrValueTooLarge`

- `MaxValueBits = 1 << 19` (524 288). Antes de cada `Mul`, `Div` o `Pow`,
  `decimal` acota el tamaño del resultado por los bits de los operandos
  (bits(a·b) ≤ bits(a)+bits(b); bits(aⁿ) ≤ n·bits(a)), por separado en
  numerador y denominador, y da `ErrValueTooLarge` sin calcular si pasa.
- **Dónde se aplica:** a la expansión de **un** código (su valor canónico)
  y a todo `*big.Rat` del llamador al entrar por la API exacta. El
  cociente de dos lados ya acotados (`src/dst` en `ConversionFactor`,
  `Convert`, `ConvertRat`, `IsComparable`/`combine`) no se vuelve a acotar:
  queda bajo 2·`MaxValueBits` por construcción, y el de `ConvertRat`
  (valor·src/dst) bajo 3·`MaxValueBits`.
- **Por qué 2^19:** el requisito funcional es que toda unidad del XML
  oficial, sola, con exponente ±1000 (el máximo de `MaxExponent`) quepa. El
  máximo medido en la ronda 2 es `YLmb1000` con 306 k bits y `[cml_i]` ±1000
  con 260 k: 2^19 deja 1.7× de margen. Las entradas de dos códigos que en la
  ronda 2 sumaban 601 k (`YLmb1000` → `yLmb1000`) quedan cubiertas porque el
  tope es por lado. 2^20 (lo que proponía (b)) cuadruplica el costo de cada
  reducción final sin cubrir ningún código de una unidad más.
- Determinista: el tamaño no depende del caché ni del orden (ronda 1,
  media 3). Sin opción pública (ronda 1, media 6).
- Los códigos que reducen bien pero pasan del tope sin reducir
  (`'1000/'1000.'1000`, `[pi]1000/[pi]1000.[pi]1000/[pi]1000`) pasan a
  fallar: el README los nombra y §5.1 los afirma (ronda 2, media 7).
- `productCode` puede formar `'2000` (sobre `MaxExponent`) en `Multiply`
  (ronda 2, baja 9): el tope lo acota igual que a un código; un test lo
  afirma.

### 3.4 Carga

`decimal.FromString` acota el literal completo con el mismo tope en bits
antes de calcular: dígitos de la mantisa (cota log2(10)·dígitos) y
exponente (cota log2(10)·|e|). Rechaza la forma `"1/3"` que hoy acepta
`SetString`. Un literal que no cabe es un error de carga que nombra la
unidad o el prefijo. El XML oficial llega a ±30 en el exponente y 74 bytes.

### 3.5 README

La sección de límites explica `MaxValueBits`, qué códigos válidos rechaza
(patológicos: exponentes de cientos sobre unidades con `[pi]`) y por qué
(§19 ■1; el costo cuadrático del máximo común divisor).

## 4. Criterio de salida

- **Tiempo acotado en todas las entradas públicas de la raíz:** una sonda
  con el XML oficial recorre formas de 1024 bytes (`'1000` repetido,
  alternado con cocientes, `Ym1000` ×146, productos de unidades distintas
  con `[pi]`, destinos especiales y logarítmicos como `B[W]`,
  `Multiply`/`Divide`, `ConvertRat` con un `*big.Rat` de `MaxValueBits`
  bits) y ninguna pasa de 1 s en la máquina de referencia. Peor caso
  esperado: una reducción de 3·2^19 bits en `ConvertRat`; se mide antes de
  implementar y, si pasa de 1 s, se reduce el valor del llamador contra el
  factor ya reducido en vez de reducir el producto. La salida va a
  `docs/plans/evidence/`.
- Resultados bajo el tope idénticos a `main`: barrido del XML oficial byte a
  byte (como 018) y `make compare-java`.
- Un XML con `value="1e1000000000"` o con una mantisa enorme: error de carga
  en milisegundos.
- Benchmarks: asignaciones y tiempo iguales o mejores.
- `make check`, `make check-packages`, `make compare-java`, mutaciones
  completas.

## 5. Tests (primero)

1. Los códigos de §1 y de las rondas: el valor esperado por aritmética
   (Python `fractions`) para los que caben, o `ErrValueTooLarge`, incluidos
   `'1000/'1000.'1000` y el de `[pi]` de 3.3.
2. El requisito del tope: toda unidad del XML oficial, sola, con exponente
   +1000 y -1000, en `Canonical`, `CanonicalRat` y como los dos lados de
   `ConversionFactor` (`u1000` → `u1000` con otro prefijo), no da
   `ErrValueTooLarge`.
3. `decimal` contra `big.Rat` en miles de pares al azar y de borde (signos,
   cero, denominador 1, exponentes negativos, `-2/3` a la -1, 2/4 contra
   1/2, operandos asimétricos): mismo `Rat()`, mismo `Float64` (con
   subnormales y el caso 2^-1075·(1+2^-60)), mismo `IsInt`, mismo `String`,
   misma comparación.
4. El tope a un bit del borde, adentro y afuera, en `Mul`, `Div`, `Pow`, el
   valor del llamador y `FromString`; frío y caliente iguales; concurrente
   (`-race`).
5. El presupuesto de reducciones de 3.2 en cada entrada pública.
6. El servicio insensible con los mismos códigos.

## 6. Mutaciones previstas

- `Pow` que no intercambia con exponente negativo, o deja el signo en el
  denominador; `Div` que suma num+num.
- `Rat()` sin reducir; `IsInt` sobre el valor sin reducir; `Float64` con
  `big.Float(53).Quo`.
- El tope sin chequear en `Mul`, `Div`, `Pow`, el valor del llamador o
  `FromString`; con el borde corrido en uno.
- Una cadena de 3.2 que vuelve a `big.Rat` (una reducción de más).
- El caché que devuelve el valor sin reducir en un fallo.
- Mutaciones completas.

## 7. Fuera de alcance

- 023: `fhir` sobre valores del llamador.
- La base coprima de los valores del XML (ronda 1): alternativa si el tope
  no basta.
- El mensaje `invalid UCUM code "<operando>"` de `combine` (018 §10).
- Los pendientes del punto de reanudación (021 §7, 022 §7, 020 §7).

## 8. Dependencias

Ninguna. El port de `quotToFloat64` viene de la biblioteca estándar
(licencia BSD, la misma del repo de Go) y cita su origen.

## 9. Versión

`feat:` → 4.6.0: un sentinel y una constante nuevos (`ErrValueTooLarge`,
`MaxValueBits`), y códigos válidos pero patológicos que pasan a dar error.

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

### Ronda 3 (2026-11-16), sobre el §3 reescrito

Dos Codex sobre copias (pl019r3a, pl019r3b). (b) no pudo compilar Go en su
sandbox y razonó con sondas en Python sobre el XML; (a) falló en la primera
corrida y se relanzó, con sondas en Go
(`pl019r3a/.cache/review019/evidence.txt`). Mis mediciones (Go 1.27.2,
arm64): `SetFrac` de dos números al azar de 2^19 bits 128 ms, 2^20 517 ms,
3·2^19 1.155 s.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El presupuesto de reducciones se contradice: en frío, expandir `'` normaliza las entradas del caché (`[pi].rad/360`, `deg/60`) antes de la salida, y en las salidas `float64` | b (alta) | Confirmado por lectura del §3.1 (el caché guarda valores reducidos) |
| La cota de tamaño no cubre la aritmética de las escalas especiales: un valor del llamador de 2^19 bits más `273.15` pasa del tope por lado; con un XML propio, `CanonicalRat(v, qCel)` llega a 1.57 M bits | b (alta) | Confirmado en aritmética; sin reproducir por la API |
| El requisito de 1 s no se cumple: la reducción de 3·2^19 bits de `ConvertRat` tarda 1.155 s; la alternativa del §4 hace más de una reducción, contra el §3.2 | b (alta) | **Confirmado** por mi medición |
| `essence.go:147` cambia un valor de unidad inválido por `1` en silencio: acotar `FromString` no da el error de carga prometido | b (media) | **Confirmado** por lectura; defecto propio previo al plan |
| `big.Rat.SetString` acepta `1p9999999` (10 M bits), `0x10`, `1_000` y `1/3`: rechazar solo `/` no basta | b (media) | **Confirmado** por sonda: hace falta una gramática decimal estricta |
| Las cadenas alternadas de `'1000` se rechazan en el tercer término; `'1000/'1000` (igual a 1) ocupa 437 k bits por lado | b (media) | Confirmado en aritmética; esperado por el diseño, a fijar en tests |
| La vía logarítmica depende de la representación: `Convert(1, "2.W/(25.10*310)", "B[W]")` da -311.09691001300803 en `main` y -311.09691001300808 con el factor sin reducir en `Log10Rat` (un ulp) | a (media) | Confirmado por sonda de (a) |
| La cota previa de bits rechaza según la agrupación: `2.10*2.B` y `(2.10*2).(B)` dan el mismo entero de 524 288 bits en `main`; la cota rechaza el primero (estima 524 289) | a (media) | Confirmado por sonda de (a) |
| La promesa de cero reducciones choca con `IsInt` (lo usa `integerExponent` en `Convert(·, "nB", "B")`) y con `productUnit.compose` (`Multiply` de `2/3` por `3/4` da `1/2`) | a (media) | Confirmado por sonda de (a) |

Tras tres rondas sin converger, se detiene el incremento y se consulta al
usuario (NUCLEO §10; sin cuarta ronda).

**Decisión del usuario (2026-11-16).** Se achica 019: solo el arreglo de
algoritmo que da los mismos resultados (`Pow` sin máximo común divisor,
`Mul` con cancelación cruzada; §2) y el costo sin cota documentado como
límite conocido en el README. El tope de tamaño queda para un rediseño que
responda los hallazgos de la ronda 3. Antes, un incremento aparte (023)
corrige los dos defectos de carga: el valor inválido que pasa a `1` y la
gramática de `FromString`. La aritmética de `fhir` sobre valores del
llamador queda pendiente, sin número.

