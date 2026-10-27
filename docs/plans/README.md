# Punto de reanudación

**Incremento activo:** 002, paso 9 (cerrado sin commit; plan aprobado el
2026-10-26). La sonda contra Java la corrió el usuario; el diseño versiona la
salida de Java para que la comparación no necesite ejecutar Java en la sesión
ni en CI. Orden: generador de casos → dumper → `make java-dump` (lo corre el
usuario) → comparación y clases → mutaciones.
- [001 — `CanonicalKey` exacto (issue #54)](001-canonicalkey-exact.md):
  cerrado e integrado el 2026-10-26 (#57).
- [002 — Comparación ejecutable contra Ucum-java](002-compare-java.md): plan
  aprobado, en implementación (paso 5).

**Próximo paso:** 002 cerrado en el árbol de trabajo (§6 del plan); falta
el commit y el PR, cuando el usuario lo pida. Después: #59 o los issues para
Ucum-java.
Issues abiertos desde 001: #55 (NaN/Inf en `CanonicalKey`) y #56 (aliasing
de `Exact` sin test).
**Integrado:** PR #57, squash `9691ced` en `main` (2026-10-26), cierra #54.
Publicado como v4.3.1 (#58).
**Decisiones pendientes del usuario:** ninguna. Java lo ejecuta el usuario
(`make java-dump`).
**Evidencia:** 001 en [`evidence/`](evidence/) (2026-10-26).
