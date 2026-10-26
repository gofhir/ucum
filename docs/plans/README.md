# Punto de reanudación

**Incremento activo:** ninguno.
- [001 — `CanonicalKey` exacto (issue #54)](001-canonicalkey-exact.md):
  cerrado e integrado el 2026-10-26 (#57).
- [002 — Comparación ejecutable contra Ucum-java](002-compare-java.md): en
  pausa en el paso 1.

**Próximo paso:** retomar 002 con permiso para ejecutar `ucum-1.0.10.jar`.
Issues abiertos desde 001: #55 (NaN/Inf en `CanonicalKey`) y #56 (aliasing
de `Exact` sin test).
**Integrado:** PR #57, squash `9691ced` en `main` (2026-10-26), cierra #54.
release-please lo publicará como 4.3.1.
**Decisiones pendientes del usuario:** permiso para ejecutar el JAR de
Ucum-java (002).
**Evidencia:** 001 en [`evidence/`](evidence/) (2026-10-26).
