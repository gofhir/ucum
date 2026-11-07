# 020 — `fhir.Decimal` cuadrático en el exponente decimal

**Estado:** paso 9 (cierre). Code review convergió en la ronda 1; `make check`, `make check-packages` y `make compare-java` en verde (2026-11-06; el informe de compare-java es idéntico al de 2026-11-03). Falta el commit, que aprueba el usuario.

## 1. Problema

`decimalExponent` (`fhir/decimal.go`) busca la posición del primer dígito
significativo dividiendo o multiplicando por 10 una vez por dígito, y cada
paso es un `big.Rat.Quo` o `Mul` que normaliza con un máximo común divisor.
Es cuadrático en la cantidad de dígitos. Medido el 2026-11-06 en v4.4.10 (Go
1.27.2, arm64), `ParseDecimal(s)` es instantáneo y `String()` tarda:

| `s` | `String()` |
|---|---|
| `1.0e3000` | 5 ms |
| `1.0e10000` | 44 ms |
| `1.0e30000` | 345 ms |
| `1.0e100000` | 3.7 s |
| `1.0e-100000` | 7.7 s |
| `1.0e300000` | 31 s (019 §10) |

`Rat.SetString` de Go rechaza exponentes de más de 10^6 (`ratconv.go`: «avoid
excessively large exponents»), así que el peor caso por `ParseDecimal` es
±999 999: `ParseDecimal("1.0e-999999").String()` no terminó en 500 s (ronda
1). Las mezclas son peores: `1.0e30000` + `1.0e-30000` tarda 21.9 s, y
`NewDecimal(1/(3·10^300000), 3).String()`, 85 s. Por UCUM se llega sin
analizar nada: `ConvertDecimal(1.5, "'300", "rad300").String()` tarda 6.1 s
(019 §10). `decimalExponent` también está en `Add` y `Sub`
(`decimalPlaces`, `figuresForPlaces`), y `mantissa` hace un `Quo` con
máximo común divisor sobre el valor entero.

## 2. Revisión previa

- **Especificación FHIR** (R4, R4B y R5, `datatypes.html`, `decimal`): la
  representación XML es «union of xs:decimal and xs:double», «JSON is
  restricted to the precision limits documented in XML schema for xs:double
  and xs:decimal» y «Irrespective of this, the limits documented in XML
  Schema apply». La forma léxica de xs:decimal no lleva exponente, así que
  `1.0e300000` solo puede ser xs:double, cuyo máximo ronda 1.8e308: **hay base
  normativa para una cota al analizar** (ronda 1, alta). R5 agrega «Decimals
  in FHIR cannot have more than 18 digits and a decimal point»; las regex
  (R4 `-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?`, R5 con `{0,17}`,
  `{1,17}` y `{1,9}`) son «informative, not normative».
- **Decisión:** la cota al analizar queda fuera de 020 (§7) porque no basta:
  `NewDecimal`, `ConvertDecimal` y la aritmética construyen valores enormes
  sin pasar por `ParseDecimal` (`'300` → `rad300`). El arreglo de costo hace
  falta igual, con la misma salida.
- **`gofhir/server` (congelado):** usa `gofhir/ucum/v4` v4.2.0, que ya trae
  esta función. Nada que tomar.
- **Ucum-java:** `Decimal` propio en base diez, con la escala explícita;
  no tiene este bucle (019 §2). No es referencia de salida para `fhir`.

## 3. Cambio

1. `decimalExponent(r)` en tiempo casi lineal: estima e = ⌊(bn−bd)·log10 2⌋
   + 1 con los bits del numerador y del denominador, y lo corrige con
   comparaciones exactas de enteros, |num| ≥ 10^(e-1)·den y |num| <
   10^e·den (`Exp`, `Mul`, `CmpAbs`; ningún máximo común divisor). Como
   log10|r| está en ((bn−bd−1)·c, (bn−bd+1)·c) con c = log10 2 < 1, la
   estimación se equivoca a lo sumo en 1, y el redondeo de `float64` en el
   borde puede sumar otro: las correcciones son **dos bucles**, no un paso
   fijo (ronda 1).
2. `mantissa` sin `Rat.Quo`: como en `scientific` siempre n < exp, el
   cociente es num / (den·10^(exp−n)) con `QuoRem` de enteros, redondeado
   como `FloatString` (sube cuando 2·resto ≥ divisor sobre la magnitud y
   después pone el signo: mitades lejos del cero).
3. La comprobación de acarreo de `round` lee el exponente del texto
   redondeado (la posición del primer dígito no nulo respecto del punto) en
   vez de `SetString` y `decimalExponent` otra vez: 9 ms en vez de 35 ms con
   n = 200 000 (ronda 1).

La salida es byte a byte la misma para todo valor.

## 4. Criterio de salida

- `String()`, `Add` y `Sub` casi lineales en el peor caso real, |exp| =
  999 999: `1.0e999999` y `1.0e-999999`, `1.0e30000` + `1.0e-30000`,
  `NewDecimal(1/(3·10^300000), 3)` y `ConvertDecimal(1.5, "'300",
  "rad300")`, cada uno en menos de 100 ms (la salida de `1.0e-999999` mide un
  millón de caracteres: es la salida, no el cálculo). La sonda de reloj va a
  la evidencia.
- La salida es correcta según un oráculo independiente (§5) y, además, igual
  a la de v4.4.10 en el mismo barrido.
- Benchmarks de `String` iguales o mejores en valores chicos.
- `make check`, `make check-packages`, mutaciones completas.

## 5. Tests (primero)

1. `decimalExponent` contra la definición (la posición del primer dígito
   significativo) en potencias de diez exactas 10^k para k de -400 a 400,
   sus vecinos (10^k ± 10^-30 relativo, ×1/3, ×9.995, ×0.999), denominadores
   impares y valores al azar; los esperados salen de contar dígitos de la
   representación decimal, no de la función. Casos que fuerzan **cada rama
   de corrección** (numeradores justo bajo 10^k sobre denominadores justo
   bajo una potencia de dos) y un contador que afirma que las dos se
   recorren (ronda 1).
2. `String` contra un **oráculo independiente** (NUCLEO: los esperados no
   salen del código propio): el literal se redondea como cadena de dígitos,
   mitades lejos del cero; para valores sin expansión finita (1/3·10^k) se
   toma `FloatString` con muchos lugares de más y se redondea como cadena.
   La implementación actual, copiada al test, queda solo como comprobación
   extra de que nada cambió.
3. Mitades exactas en el último dígito conservado, en notación normal y
   científica: 2.5, 9.5, −0.5·10^k, `NewDecimal(25, 1)` → `3e1`, `−25` →
   `−3e1`, `35` → `4e1` y los acarreos `995` → `1.0e3` con dos cifras y `95`
   → `1e2` con una (ronda 1; el plan decía `95` con dos cifras, que da `95`).
   9.5 con una cifra da `"10"`, que aparenta dos cifras: por la regla del
   propio código correspondería `1e1`. Es un defecto previo y 020 no cambia
   la salida: queda pendiente (§7) y el test lo fija tal como está.
4. Los tiempos de §4 como cota de asignaciones con las entradas en |exp| =
   999 999, no de reloj (017).

## 6. Mutaciones previstas

- La estimación sin la corrección hacia arriba o hacia abajo, o con un paso
  fijo en vez del bucle.
- La comparación con `<=` en vez de `<` en el borde de la potencia de diez.
- `mantissa` que redondea las mitades hacia el par.
- La lectura del exponente del texto redondeado corrida en uno.
- Mutaciones completas.

## 7. Fuera de alcance

- Una cota al analizar (`ParseDecimal`): tiene base normativa (xs:double,
  §2) pero no basta para el costo. Queda como pendiente, con su ADR si se
  hace, porque rechaza entradas que hoy se aceptan.
- El costo de `Mul`, `Quo` y `Add` sobre valores enormes del llamador (019
  §10, ronda 2, baja): es de 019.
- `Log10Rat` (021) y las entradas de dos códigos (022).
- Un acarreo en `round` hacia un dígito entero más (9.5 con una cifra) da
  `"10"` en vez de notación científica (§5.3): defecto previo, pendiente.

## 8. Versión

`fix:` → 4.4.11.

## 9. Ronda adversarial del plan

### Ronda 1 (2026-11-06)

Dos Opus sobre copias. (a) prototipó el §3: la misma salida que el código
actual en 135 346 casos; `1.0e±300000` en 12-18 ms; nunca más de una
corrección.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| §2 lee mal la especificación: «union of xs:decimal and xs:double» y «the limits documented in XML Schema apply» dan base a una cota al analizar | b (alta) | **Aceptado:** §2 reescrito; la cota queda fuera como decisión (§7) |
| El criterio no es el peor caso: `Rat.SetString` corta en 10^6 (verificado en `ratconv.go`), `1.0e-999999` no terminó en 500 s, `1.0e30000` + `1.0e-30000` 21.9 s, `1/(3·10^300000)` 85 s | b (alta) | **Aceptado:** §1 y §4 |
| El oráculo con el código viejo rompe NUCLEO | b (media) | **Aceptado:** oráculo independiente (§5.2); el viejo, solo como comprobación extra |
| Mutaciones que sobreviven sin mitades exactas en notación científica ni casos que fuercen cada rama de corrección | a, b (media) | **Aceptado:** §5.1 y §5.3 |
| Una corrección fija de un paso puede fallar por el redondeo de `float64` en el borde | a (baja) | **Aceptado:** dos bucles (§3.1) |
| El acarreo con `SetString` cuesta 35 ms frente a 9 ms | a (baja) | **Aceptado:** leer el exponente del texto (§3.3) |
| `mantissa` puede dividir por den·10^(exp−n) sin escalar los dos lados | a (baja) | **Aceptado** (§3.2) |
| Las cotas de asignaciones deben usar entradas en el tope de 10^6 | b (baja) | **Aceptado** (§5.4) |

Las dos altas eran del texto del plan (lectura de la especificación y peor
caso), no del diseño, que (a) validó con el prototipo.

## 10. Code review

### Ronda 1 (2026-11-06)

Dos Opus sobre copias. (a) comparó con el código viejo unos 260 000 valores
(potencias de diez de -400 a 400 y vecinos, acarreos, mitades, fracciones
periódicas, 2^±2000, 20 000 fracciones al azar, todos también negados) con
11 precisiones: `decimalExponent`, `String`, `Add` y `Sub`, idénticos en
todos; la estimación con `float64` queda a 1.56e-7 de un entero como mínimo
en ±3.4 M bits. (b) midió las entradas de `fhir` en |exp| = 999 999 (todas
bajo 1 s salvo la de abajo), corrió 23 mutaciones propias (16 muertas, 7
equivalentes) y `-race`; `String` en valores chicos, 6× más rápido.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| `CanonicalKey` de un `Exact` 1e-999999 tarda más de 20 s (1e-300000, 79 s): `big.Float.Text` en el mensaje de `Quantity.exactError` | b (media) | **Fuera de alcance**, no es regresión: pendiente con plan propio (usar `NewDecimal(q.Exact, 6).String()`) |
| El godoc de `decimalExponent` dice que `float64` puede sumar otra corrección; K226 y la medición muestran que no | a, b (baja) | **Corregido** |
| Un comentario de `round` mezclaba inglés y español | a (baja) | **Corregido:** en español |
| `Add` de 1e999999 + 1e-999999 tarda 415 ms: `atLeastPow10` recalcula `Exp(10, k)` en cada comparación | b (baja) | **Se mantiene:** bajo 1 s; mejora posible si 019 lo necesita |
| Mutantes equivalentes que sobreviven (`TrimPrefix` de `textExponent` redundante, el bucle hacia abajo como `if`, la cota `places < 0`) | b (baja) | Registrado; K226 cubre el caso del bucle |

Mutaciones completas antes de la ronda: 265 líneas, todas `ok`
([evidencia](evidence/020-2026-11-06-mutations.txt); 9.4 min, 292 MB de RSS).
Converge en la ronda 1.
