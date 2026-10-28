# Punto de reanudación

**Incremento activo:** [005 — llaves anidadas en una anotación (#65)](005-nested-braces.md),
cerrado en el paso 9 (el review convergió en la ronda 1), sin commit. Cerrados:
- [004 — espacio dentro de una anotación (PR #52)](004-annotation-space.md):
  cerrado e integrado el 2026-10-28 (#64).
- [003 — resultados fuera de `float64` (#59)](003-float-range.md):
  cerrado e integrado el 2026-10-28 (#62).
- [001 — `CanonicalKey` exacto (issue #54)](001-canonicalkey-exact.md):
  cerrado e integrado el 2026-10-26 (#57).
- [002 — Comparación ejecutable contra Ucum-java](002-compare-java.md):
  cerrado e integrado el 2026-10-27 (#60).

**Próximo paso:** commit y PR de 005 cuando el usuario lo pida. Los defectos de Ucum-java quedaron
redactados en [002-java-issues.md](002-java-issues.md), sin publicar.
Issues abiertos: #55 (NaN/Inf en `CanonicalKey`, acotado a entradas no
finitas), #56 (aliasing de `Exact` sin test), #61 (la caché retiene la
entrada) y #65 (005).
**Integrado:** PR #64, *rebase and merge* en `main` (2026-10-28): `bd014cb`
(kaptinlin), `41e983d` y `c93f36d`; cierra #52. PR #57, squash `9691ced` en `main` (2026-10-26), cierra #54;
publicado como v4.3.1 (#58). PR #60, squash `30048dc` en `main`
(2026-10-27); es `test:`, no genera release. PR #62, squash `959d6b1` en
`main` (2026-10-28), cierra #59; publicado como v4.3.2 (#63). PR #49
(actions, con `govulncheck` en Go estable), squash `c178b4c` (2026-10-28).
**Decisiones pendientes del usuario:** ejecutar ucum-lhc 8.0.1 o no. #53 espera un issue de diseño del autor. Java lo
ejecuta el usuario (`make java-dump`).
**Evidencia:** 001 a 005 en [`evidence/`](evidence/).
