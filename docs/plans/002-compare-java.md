# 002 — Comparación ejecutable contra Ucum-java

**Estado:** en pausa en el paso 1 (decisión del usuario, 2026-10-26: cerrar
antes lo pendiente de 001). Para seguir hace falta permiso para ejecutar el
JAR de referencia. **Fecha:** 2026-10-26.

## Motivo
La carta (§10, reglas de evidencia) pide que toda comparación contra la
referencia sea código del repo, con su arnés, la versión de la referencia y
un comando que la reproduzca. Hoy la tabla «Differences from the Java
reference» del README se sostiene leyendo el código de Java («verified by
reading its source»); nada la reproduce ni detecta si Java cambia.

## 1. Revisión previa (en curso)

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
`getCanonicalForm(Pair)`, `convert`, `isComparable`, `multiply`, `divideBy`,
`getCanonicalUnits`, `analyse`.

| Fuente | Qué hace | Decisión |
| --- | --- | --- |
| `gofhir/server` | No compara contra Java | — |
| `go-searchindex/scripts/server-diff.sh` y `testdata/expected-gaps.json` | Script que corre el dumper de la referencia a `.cache/`, test Go con variable de entorno que compara y clasifica contra un archivo versionado de diferencias con su motivo | **Tomar** el patrón |

## Próximo paso
Con permiso del usuario para ejecutar el JAR: sonda del formato de salida de
Java (unidad canónica, precisión de `Decimal`, errores) y escribir el plan
(paso 2).
