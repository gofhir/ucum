# Punto de reanudación

**Incremento activo:** [004 — espacio dentro de una anotación (PR #52)](004-annotation-space.md),
cerrado en el paso 9; el review convergió en la ronda 2. Plan aprobado el 2026-10-28, como `fix:`.
Rama local `fix/annotation-space`, sin commit de lo propio. Cerrados:
- [003 — resultados fuera de `float64` (#59)](003-float-range.md):
  cerrado e integrado el 2026-10-28 (#62).
- [001 — `CanonicalKey` exacto (issue #54)](001-canonicalkey-exact.md):
  cerrado e integrado el 2026-10-26 (#57).
- [002 — Comparación ejecutable contra Ucum-java](002-compare-java.md):
  cerrado e integrado el 2026-10-27 (#60).

**Próximo paso:** commit y PR de 004 cuando el usuario lo pida (§13 del plan). Los defectos de Ucum-java quedaron
redactados en [002-java-issues.md](002-java-issues.md), sin publicar.
Issues abiertos: #55 (NaN/Inf en `CanonicalKey`, acotado a entradas no
finitas) y #56 (aliasing de `Exact` sin test).
**Integrado:** PR #57, squash `9691ced` en `main` (2026-10-26), cierra #54;
publicado como v4.3.1 (#58). PR #60, squash `30048dc` en `main`
(2026-10-27); es `test:`, no genera release. PR #62, squash `959d6b1` en
`main` (2026-10-28), cierra #59; publicado como v4.3.2 (#63). PR #49
(actions, con `govulncheck` en Go estable), squash `c178b4c` (2026-10-28).
**Decisiones pendientes del usuario:** ejecutar ucum-lhc 8.0.1 o no. #53 espera un issue de diseño del autor. Java lo
ejecuta el usuario (`make java-dump`).
**Evidencia:** 001 a 004 en [`evidence/`](evidence/).
