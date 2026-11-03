# ADR 0001 — el dominio de las escalas de tangente

**Estado:** aceptado el 2026-11-02 (plan [014](../plans/014-tan-domain.md)).

## Contexto

UCUM §44 ■4 (tabla 18) define las escalas de tangente (dioptría prismática,
porcentaje de pendiente) con el par:

> f_PD(α) = tan(α) × 100
> f_PD⁻¹(x) = arctan(x / 100)

La tabla no dice para qué ángulos α vale. tan tiene período π y arctan devuelve
solo (-π/2, π/2), así que el par es inverso solo en ese intervalo. Fuera, la
librería daba el valor literal de tan, que no vuelve al ángulo y cambia de
signo al cruzar π/2 (2 rad daba -218.5 `[p'diop]`, que vuelve como -1.14 rad).

## Decisión

Un ángulo α con |α| ≥ π/2 hacia una escala de tangente es `ErrOutOfDomain`. El
intervalo es abierto: en ±π/2 tan no está definida. Es la lectura de 013 para
`sqrt`: el valor de la escala es la imagen de f⁻¹, y un ángulo que f⁻¹ no
devuelve no tiene valor en ella. El valor de la escala hacia el ángulo no
cambia (arctan da (-π/2, π/2) para todo x finito).

- **π sale del XML.** El borde es la mitad del valor de `[pi]` en
  `ucum-essence.xml`, un `big.Rat`, no `math.Pi`: `math.Pi/2` queda bajo el π/2
  del XML y es un valor, y su float64 siguiente no. `[pi]` se busca por su
  código porque es la constante que la propia definición de `deg`
  (`2 [pi].rad/360`) nombra. El valor es el canónico de `1 [pi]` que calcula el servicio una vez en `New`, así que un `[pi]` escrito como 0.314…×10* también vale π. Un XML propio sin `[pi]`, o con uno que no es un número sin dimensión (unidad canónica `1`) y positivo, no tiene el chequeo. La API exacta da `ErrNotRational` hacia una escala de tangente, como hacia `sqrt`.
- **El dominio va antes que el rango.** `1e300 Yrad` hacia `[p'diop]` es
  `ErrOutOfDomain`, no `ErrOutOfRange`: el ángulo no existe en la escala antes
  de preguntar si su valor cabe. Sin ángulo exacto (origen logarítmico sobre el
  tope de costo, o de base no racional como `ln`) decide log10 sin
  materializar, y un ángulo que sí se materializa se compara exacto, por el
  binario de su float64, con el mismo π/2 racional.
- **`nativeUnit` es `rad`.** El XML se contradice en `%[slope]`: su `<value>`
  dice `Unit="100tan(1 rad)"` pero su `<function>` dice `Unit="deg"` (líneas
  1476-1483). La librería toma el argumento de tan en radianes para las dos
  unidades, como §44 ■4 (tan de un ángulo).
- **#55 va antes.** Una entrada no finita sigue pasando sin verificar:
  `+Inf rad` hacia `[p'diop]` da NaN sin error.

## Consecuencias

Un ángulo que daba un número fuera del dominio da un error; ese número no
volvía al ángulo. Un cambio de versión `fix:` (4.4.6).

Una definición cíclica (de `[pi]` o de cualquier unidad) hacía caer el proceso
por desborde de pila al canonicalizar; ahora da un error que nombra la unidad
(`is part of or leads to a definition cycle`). Con un `[pi]` cíclico no hay
chequeo de tangente.

## Verificación

`tan_test.go` (esperados del XML y de `big.Float`, no de la librería) y
`internal/engine/tanoracle_test.go`; el oráculo de Java en
`TestJavaOracleKnowsTheTanDomain`. Las mutaciones K181-K193 de
`scripts/mutations.py`. `>=` por `>` en la comparación del float64 materializado
es un mutante equivalente: un float64 no puede valer el π/2 racional del XML.
