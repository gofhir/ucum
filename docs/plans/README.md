# Punto de reanudación

**Incremento activo:** [003 — resultados fuera de `float64` (#59)](003-float-range.md),
paso 8 (code review): tres rondas corregidas y registradas, tope alcanzado; plan aprobado el 2026-10-27.
- [001 — `CanonicalKey` exacto (issue #54)](001-canonicalkey-exact.md):
  cerrado e integrado el 2026-10-26 (#57).
- [002 — Comparación ejecutable contra Ucum-java](002-compare-java.md):
  cerrado e integrado el 2026-10-27 (#60).

**Próximo paso:** decide el usuario. La ronda 3 dejó un hallazgo medio (K33,
ya corregido) y es el tope: una ronda 4 acotada o el cierre (paso 9). Rama
`fix/float-range`, sin push: la credencial local no tiene permiso en
`gofhir/ucum`. Después de 003: los issues para
Ucum-java (§6 del plan 002), que se muestran antes de publicarse.
Issues abiertos: #55 (NaN/Inf en `CanonicalKey`), #56 (aliasing de `Exact`
sin test) y #59.
**Integrado:** PR #57, squash `9691ced` en `main` (2026-10-26), cierra #54;
publicado como v4.3.1 (#58). PR #60, squash `30048dc` en `main`
(2026-10-27); es `test:`, no genera release.
**Decisiones pendientes del usuario:** ninguna. Java lo
ejecuta el usuario (`make java-dump`).
**Evidencia:** 001 y 002 en [`evidence/`](evidence/).
