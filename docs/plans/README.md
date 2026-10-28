# Punto de reanudación

**Incremento activo:** ninguno. Último cerrado:
- [003 — resultados fuera de `float64` (#59)](003-float-range.md):
  cerrado e integrado el 2026-10-28 (#62).
- [001 — `CanonicalKey` exacto (issue #54)](001-canonicalkey-exact.md):
  cerrado e integrado el 2026-10-26 (#57).
- [002 — Comparación ejecutable contra Ucum-java](002-compare-java.md):
  cerrado e integrado el 2026-10-27 (#60).

**Próximo paso:** ninguno definido. Los defectos de Ucum-java quedaron
redactados en [002-java-issues.md](002-java-issues.md), sin publicar.
Issues abiertos: #55 (NaN/Inf en `CanonicalKey`, acotado a entradas no
finitas) y #56 (aliasing de `Exact` sin test).
**Integrado:** PR #57, squash `9691ced` en `main` (2026-10-26), cierra #54;
publicado como v4.3.1 (#58). PR #60, squash `30048dc` en `main`
(2026-10-27); es `test:`, no genera release. PR #62, squash `959d6b1` en
`main` (2026-10-28), cierra #59; release-please abrirá el release.
**Decisiones pendientes del usuario:** ninguna. Java lo
ejecuta el usuario (`make java-dump`).
**Evidencia:** 001, 002 y 003 en [`evidence/`](evidence/).
