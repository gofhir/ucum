# 021 — `Log10Rat` cuadrático en la cantidad de dígitos

**Estado:** paso 9 (cierre). La ronda 3 del code review no convergió y el usuario eligió probar el cociente directamente, sin una cuarta ronda (§10). `make check`, `make check-packages` y `make compare-java` en verde (2026-11-14; el informe de compare-java es idéntico al de 2026-11-03). Falta el commit, que aprueba el usuario.

## 1. Problema

`Log10Rat` (`internal/decimal/decimal.go`) tiene dos costos cuadráticos en la
cantidad de dígitos del racional:

1. `stripTens` quita los diez finales uno por uno: un `QuoRem` por 10 por
   cada diez, sobre el número entero (019 §10, ronda 1, alta).
2. `new(big.Rat).SetFrac(num, den)` vuelve a calcular el máximo común divisor
   (Lehmer, cuadrático) de dos enteros que ya son coprimos: un `big.Rat`
   siempre está normalizado, y quitar los diez de cada lado no lo cambia.

Medido el 2026-11-14 en `main` (v4.5.0, Go 1.27, arm64):

| `r` | k = 10^4 | 3·10^4 | 10^5 | 3·10^5 |
|---|---|---|---|---|
| 10^k, 10^-k, 3·10^k/7 | 14 ms | 131 ms | 1.4-1.6 s | 15.5-18.9 s |
| 2^3k / 3^2k (sin diez) | 1 ms | 5 ms | 55 ms | 586 ms |

La primera fila es `stripTens`; la segunda, el `SetFrac`. Se llega por UCUM
sin ningún literal grande: el factor de `Convert(1, "W.'1000.rad-1000",
"B[W]")` pasa 1.0 s en `Log10Rat` (ronda 1, b); la conversión entera midió
entre 0.75 y 3 s según la corrida y la máquina (019 §10). `Log10Rat` está en `internal/engine/range.go` (la cota de
`float64` de 003, el dominio de la tangente de 014, la vía logarítmica de
`LogToLog`), así que lo paga toda conversión con un factor enorme.

## 2. Revisión previa

- **Especificación:** UCUM no fija cómo se calcula el logaritmo; lo que importa
  es la salida, que no cambia (§3). El requisito propio viene de 003 y 009: el
  log10 de una potencia de diez es exacto en toda plataforma
  (`TestLog10RatPowersOfTen`).
- **`gofhir/server` (congelado):** usa `gofhir/ucum/v4` v4.2.0; `Log10Rat`
  llegó en v4.3.2 (#62, plan 003). Nada que tomar.
- **Ucum-java:** `Decimal` propio en base diez, sin logaritmo de racionales
  (019 §2). No es referencia.
- **019 §5.3** proponía estimar con `MantExp` y probar la potencia de diez
  solo cerca de un entero. Se descarta: cambia la salida en los valores que no
  son potencias de diez (3·10^-920 deja de sumar −920 exacto a log10 3), y un
  cambio de un ulp en `l10` puede mover una decisión de borde en `range.go`.
  El cambio de §3 da la misma salida bit a bit.

## 3. Cambio

1. `stripTens(n)` cuenta los diez finales con potencias 10^(2^j): sube
   dividiendo por 10, 10^2, 10^4… mientras la división es exacta, y después
   baja de la mayor a la menor dividiendo por cada una que entra. Si quedan
   v diez después de fallar en 10^(2^j), v < 2^j, y la bajada los quita en
   binario. Un `n` impar (o cero) sale sin dividir: tiene cero diez. La
   subida no eleva al cuadrado una potencia más larga que `n` (no puede
   dividirlo), y la tabla se suelta al terminar (ronda 1, a5: el pico era 8
   veces el operando).
2. El cociente de 64 bits se calcula como `Float.Quo` de `num` y `den`
   pasados a `big.Float` exactos, que es lo que hace `Float.SetRat` por
   dentro, sin el `SetFrac` y su máximo común divisor.

Prototipo (2026-11-14): la salida de `Log10Rat` es idéntica a la actual en
216 608 casos (10^k para k de -400 a 400 por 13 mantisas y 8 denominadores,
con signo, y 50 000 racionales al azar de hasta 600 bits con hasta 79 diez en
cada lado). 10^±300000 baja de 19 s a 19 ms; 10^±3000000, entre 1 y 5 s
(code review, ronda 1: el costo es subcuadrático, n^1.6 por la división
recursiva de Go, no lineal).
En `Convert(1, "W.'1000.rad-1000", "B[W]")`, `Log10Rat` desaparece del
perfil y la conversión queda en unos 220-250 ms. Lo que queda (ronda 1, a7 y
b2) es el máximo común divisor de la canonicalización exacta, el `Rat.Quo`
de `convertRatCore` y el `MulExact` de `directTimes` (`range.go`), que hace un
`Rat.Mul` completo para un `float64` que de todos modos se va a cero (98 ms en
ese factor; con el código ×4, 4.3 s). Todo eso es de 019 (§7).

## 4. Criterio de salida

- `stripTens` hace a lo sumo 2·⌈log2(t+1)⌉ + 1 divisiones para t diez
  finales, y ninguna para un `n` impar o cero, contadas con un gancho de test
  (como `exponentCorrection` de 020). Las asignaciones no sirven de cota: el
  `stripTens` viejo reutiliza sus dos enteros y asigna lo mismo o menos que
  el nuevo (ronda 1, alta, a y b).
- El `SetFrac` no vuelve: `Log10Rat` de 2^2700000/3^1800000 (sin diez) termina
  en menos de 2 s en una goroutine con plazo, como en 020; el código actual
  tarda varios segundos y el nuevo, unos milisegundos.
- La salida es la misma bit a bit que la de v4.5.0 en el barrido de §5, y
  correcta según el oráculo independiente.
- `make check`, `make check-packages`, mutaciones completas.

## 5. Tests (primero)

1. **`stripTens` contra la definición:** n = m·10^t con m no divisible por 10
   (m impar, m par no múltiplo de 5, m múltiplo de 5 no de 2, m = 1), para
   todo t de 0 a 300 y los t que fuerzan la bajada (2^j − 1, 2^j, 2^j + 1,
   potencias y vecinos hasta 4097). El esperado es (m, t), que se conoce por
   construcción y no sale de la función. Más n = 0.
2. **`Log10Rat` contra un oráculo independiente:** r se construye como
   ±m·10^a / (d·10^b) con `SetFrac` y el esperado sale de la construcción,
   nunca de `Num()` o `Denom()` (`big.Rat` normaliza: 10^3/4 es 250). Es
   exacto (`==`) para las potencias de diez y, si no, a − b + log10 m −
   log10 d, con log10 de un entero grande como (cantidad de dígitos − 1) +
   log10 de sus 17 primeros dígitos, y la tolerancia de
   `TestLog10RatPowersOfTwo`, escalada por |a − b| + log10 m + log10 d (ronda
   1, b5; code review, ronda 1). Casos: r = ±1, −10^k, 3/10, 50,
   2^a·5^b con a ≠ b, 5^k/2^k, enteros (den = 1) y sus inversos, 2^900000
   (par sin diez), 10^k por 4 mantisas y 4 denominadores para k de -400 a
   400, y 4000 racionales al azar (ronda 1, a6 y b7). Es menor que el barrido
   del prototipo para que el paquete no pase de 1 s.
3. **La misma salida que v4.5.0:** la función vieja copiada al test, solo
   como comprobación de que nada cambió, en el mismo barrido; y tres casos
   fijos de doble redondeo con el esperado derivado de la regla en el test:
   (2^65 + 2^12 + 1)/2^65 → 0, /2^66 → −log10 2 y (2^64 + 2^11 + 1)/2^64 → 0,
   que matan los mutantes de `Quo` con la precisión de los operandos y con 65
   bits (ronda 1, a3 y b3; code review, ronda 1; §7).
4. **Costo:** el contador de divisiones de §4 en t de 0 a 300 y en 2^j − 1,
   2^j, 2^j + 1 hasta 4097, y el plazo del caso sin diez.
5. **Sin aliasing:** r no cambia después de `Log10Rat`, con dos diez o más
   en el numerador y en el denominador (ronda 1, b6).
6. Los tests actuales de `Log10Rat` quedan como están.

## 6. Mutaciones previstas

- La subida sin la bajada (quita 2^j − 1 diez de más o de menos).
- La bajada que termina en 1 en vez de 0. La que empieza en j en vez de
  j − 1 es equivalente en la salida (la división que falló vuelve a fallar);
  el contador puede o no matarla (ronda 1, b4).
- El atajo de `n` impar quitado o invertido: lo mata el contador (ninguna
  división para un impar); sin contador sería equivalente (ronda 1, a4).
- La subida de a un diez en vez de al cuadrado: lo mata el contador.
- `SetFrac` de vuelta: lo mata el plazo.
- `Quo` con la precisión de los operandos en vez de 64 (cambia el redondeo).
- Mutaciones completas.

Registradas como K230-K246 en `scripts/mutations.py` (2026-11-14): mueren
todas salvo K237, el corte de la subida, que solo ahorra memoria y queda
registrada como equivalente. K232 (la bajada desde la potencia que falló) la
mata el contador. Los tests nuevos fallaban antes del cambio en el contador
(una división para un impar) y en el plazo (más de 2 s).

Mutaciones completas antes del code review: 274 líneas, todas `ok` (14.3
min, 300 MB de RSS; la evidencia de §10 las reemplaza).

## 7. Fuera de alcance

- La canonicalización exacta sin cota (019).
- Las entradas de dos códigos (022).
- `atLeastPow10` de `fhir/decimal.go` recalcula `Exp(10, k)` (020 §10): no es
  cuadrático.
- El `MulExact` de `directTimes` y el `Rat.Quo` de `convertRatCore`
  (`range.go`): de 019, con las cifras de §3.
- **Doble redondeo** (ronda 1, a3, a8 y b3): `Log10Rat` redondea a 64 bits y
  después a 53, y el resultado puede errar en un ulp:
  (2^65 + 2^12 + 1)/2^65, mayor que 1, da 0 (el log10 verdadero es
  4.8e-17, y sin el redondeo a 64 bits sale 1.1e-16, tampoco correcto: el
  error absoluto viene de sumar log10 de la mantisa y exponente·log10 2).
  Arreglarlo cambia la salida y puede mover decisiones de borde de
  `range.go`: queda pendiente; 021 fija la salida actual (§5.3).
- `Float.Quo` crece como n^1.5 cuando el numerador es mucho más largo que el
  denominador (3^300000/7^100000, 21 ms; ronda 1, b8): no es cuadrático.

## 8. Versión

`fix:` → 4.5.1.

Además, el godoc de `Log10Rat` dice que su costo es subcuadrático (ronda 1,
b10; code review, ronda 1). Nada público cambia: sin README.

## 9. Ronda adversarial del plan

### Ronda 1 (2026-11-14)

Dos Opus sobre copias, con prototipos propios de §3: la salida, idéntica a
la actual en todo lo que corrieron ((a): 30 000 racionales y 130 000 enteros
e inversos, y `stripTens` contra (m, t) para t hasta 4200).

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| Una cota de asignaciones no detecta el `stripTens` viejo: reutiliza `q` y `m` y asigna menos (9 mallocs y 262 KB contra 79 y 2.1 MB en 10^300000) | a, b (alta) | **Aceptado:** contador de divisiones y plazo de reloj (§4, §5.4) |
| La parte del máximo común divisor tampoco se distingue bien por asignaciones (2×) | a (media) | **Aceptado:** plazo con una entrada mayor (§4) |
| El mutante de `Quo` con la precisión de los operandos sobrevive a un barrido al azar; el doble redondeo es un defecto previo | a, b (media) | **Aceptado:** casos fijos (§5.3); el defecto, pendiente (§7) |
| El atajo de `n` impar es un mutante equivalente | a (media) | **Aceptado:** lo mata el contador (§6) |
| La afirmación de punta a punta es incompleta: queda `MulExact` en `directTimes` y `Rat.Quo` en `convertRatCore` | a (baja), b (media) | **Aceptado:** §3 y §7, de 019 |
| La bajada desde j es equivalente | b (media) | **Aceptado** (§6) |
| El oráculo depende de la función vieja para valores grandes | b (media) | **Aceptado:** log10 por cantidad de dígitos (§5.2) |
| La tabla de potencias pesa 8 veces el operando | a (baja) | **Aceptado:** no elevar más allá de `n`, soltarla (§3) |
| Faltan enteros, inversos, ±1, 3/10, 2^a·5^b, aliasing | a, b (baja) | **Aceptado** (§5.2, §5.5) |
| `Float.Quo` crece como n^1.5 | b (baja) | **Registrado** (§7) |
| Go 1.26 sin medir | b (baja) | Contador y plazo no dependen de la versión; CI corre las dos |
| Godoc sin el costo | b (baja) | **Aceptado** (§8) |

Sin altas abiertas: el diseño no cambió, cambió cómo se prueba.

## 10. Code review

### Ronda 1 (2026-11-14)

Dos Opus sobre copias. (a) verificó `stripTens` con t hasta 10^6 y la salida
bit a bit contra la copia de v4.5.0, y corrió 20 mutantes propios; (b) midió
antes y después (10^300000: 34.2 s → 25 ms; `Convert(1, "W.'1000.rad-1000",
"B[W]")`: 1.1-1.3 s → 213-234 ms, mismo resultado) y comprobó que K230-K238
calzan una vez y se comportan como dicen. Los dos, con `-race`.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El mutante de `Quo` con 65 bits sobrevive: los casos fijos solo matan 53 | a (media) | **Corregido:** caso (2^64 + 2^11 + 1)/2^64 → 0 y K239, que muere |
| El godoc dice casi lineal y es subcuadrático (n^1.6-1.7); 10^3000000 tarda de 1.6 a 4.7 s | a, b (media) | **Corregido:** godoc y §3 |
| Los esperados del doble redondeo no muestran de dónde salen | b (baja) | **Corregido:** derivados de la regla en el comentario del test |
| La tolerancia del oráculo crece con los diez, que son exactos | b (baja) | **Corregido:** escala con \|a − b\| |
| El barrido es menor que el del prototipo | b (baja) | **Corregido** el texto de §5.2 |
| Nombres de casos, una línea de 121 columnas, «los diez finales» ambiguo, la descripción de K237 | b (baja) | **Corregido** |
| El plazo de 2 s deja la goroutine viva si vence; el gancho es una variable de paquete | a (baja) | **Se mantiene:** margen de 33× con `-race`; ningún test del paquete es paralelo |
| Mutantes propios que sobreviven: el corte con 2·BitLen, `> 0` en el resto, `n.Set(q)`, sin el atajo `num == den` | a (baja) | Equivalentes; el atajo protege amd64, donde log10(0.5) + log10(2) puede no dar 0 |
| `Float.Quo` crece como n^1.5 con num ≫ den (3^2000000/7^500000, 320 ms) | a (baja) | Registrado en §7: subcuadrático; truncar el numerador lo haría lineal |

Las dos medias quedan corregidas: hace falta la ronda 2.
Mutaciones completas después de las correcciones: 275 líneas, todas `ok` (12.6
min, 289 MB de RSS).

### Ronda 2 (2026-11-14)

Dos Opus sobre copias, con los hallazgos de la ronda 1, los cambios y el diff
nuevo. (a) verificó la derivación del doble redondeo (también que
`math.Log10(0.5)` es exactamente `-math.Log10(2)` en toda arquitectura) y el
margen de la tolerancia (3.3×, también con `GOARCH=amd64`); sin hallazgos.
(b) corrió 24 mutantes nuevos de la división.

| Hallazgo | Revisor (severidad) | Resolución |
|---|---|---|
| El cociente de 63 bits y el redondeo dirigido (`ToZero`, `ToNegativeInf`) sobreviven | b (media) | **Corregido:** dos casos derivados de la regla, (2^63 + 2^10 + 1)/2^63 y (2^65 + 2^13 + 2^12 − 1)/2^65, con el esperado `Log10Rat` del valor ya redondeado (1 + 2^-52, 1 + 2^-51), y K240-K241, que mueren. Un primer esperado con `math.Log10` aparte falló en el código sin mutar: el compilador puede fusionar la suma final con la multiplicación de `Log10` |
| Redondear `num` o `den` a 64 bits antes de dividir sobrevive: solo cambia cerca de un empate | b (media) | **Corregido:** `TestLog10RatWideOperands` con casos de búsqueda y K242-K243, que mueren. El caso de (b) para el numerador solo mataba el redondeo de los dos; una búsqueda propia encontró 280926306166160480009/83 y 29/96901176502440991122967 |
| Una línea del comentario de `stripTens` larga | a (baja) | Ya estaba corregida en el repo |

Las dos medias quedan corregidas: hace falta la ronda 3, la última.
Mutaciones completas después de las correcciones: 279 líneas, todas `ok` (13.9
min, 317 MB de RSS; la evidencia final las reemplaza).

### Ronda 3 (2026-11-14, la última)

Dos Opus sobre copias. (a) verificó a mano las derivaciones nuevas y que el
esperado metamórfico no lo mueven los mutantes; los tests pasan en arm64 y en
amd64, con y sin `-race`. (b) corrió 22 mutantes nuevos de la división: 17
muertos, 2 equivalentes (`mant.SetPrec(53)`, que redondea como `Float64`, y
negar `num` con `f.Abs`, porque el redondeo al par es simétrico).

| Hallazgo | Revisor (severidad) | Estado |
|---|---|---|
| K242 y K243 sobreviven con `GOARCH=amd64` (verificado): los casos de `TestLog10RatWideOperands` salieron de una búsqueda en arm64, y sin FMA el término `exp·log10 2` absorbe el ulp que mueven | a (media) | **Corregido** tras la decisión (abajo) |
| Redondear los operandos a 128 bits antes de `Quo` sobrevive (numerador, denominador o los dos); (b) dio entradas que lo matan | b (media) | **Corregido** tras la decisión: K244-K246 |
| El tercer caso de `TestLog10RatWideOperands` no detecta nada en ninguna arquitectura | a (baja) | **Corregido:** el test se reemplazó |

**No converge en la ronda 3:** se detiene y decide el usuario. Las dos medias
son huecos de los tests, no del código, y tienen la misma causa: el cociente
de 64 bits se prueba a través del logaritmo, que puede tapar un ulp según la
arquitectura. Propuesta: sacar el cociente a una función propia y probarla
contra la definición, con un oráculo exacto en `big.Rat` (el resultado de 64
bits es el más cercano y, en un empate, el par), sin pasar por `math.Log10`.

**Decisión del usuario (2026-11-14):** probar el cociente directamente y
cerrar sin una cuarta ronda. El cociente pasó a `quotient64` (la misma línea,
sin cambio de comportamiento), y `TestLog10RatQuotientAgainstDefinition` lo
compara con la definición en racionales exactos, sin `math.Log10`: a lo sumo
64 bits, |q − x| ≤ ulp/2 y, en un empate, la mantisa par. Los casos son
empates T/2^j con T impar de 65 bits, casi empates T·(2^k + 1) ± 1 sobre
2^k + 1 (y al revés) para k hasta 1000, los de las búsquedas de las rondas 2
y 3, y 3000 pares al azar de hasta 448 bits. Reemplaza a
`TestLog10RatWideOperands`. Las diez mutaciones del cociente (K235, K236,
K239-K246) mueren en arm64 y con `GOARCH=amd64`.

Mutaciones completas después: 282 líneas, todas `ok`
([evidencia](evidence/021-2026-11-14-mutations.txt); 11.7 min, 286 MB de RSS).

