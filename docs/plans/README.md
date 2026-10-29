# Punto de reanudación

**Incremento activo:** [009 — potencia exacta en las escalas logarítmicas (#75)](009-log-pow.md),
cerrado, pendiente de commit y PR (`fix:`). Cerrados:
- [008 — `CanonicalKey` acepta NaN y ±Inf (#55)](008-nonfinite-key.md):
  cerrado e integrado el 2026-10-29 (#72).
- [007 — aliasing de `*big.Rat` sin test (#56)](007-exact-aliasing.md):
  cerrado e integrado el 2026-10-28 (#70).
- [006 — la caché retiene la entrada (#61)](006-cache-key-retention.md):
  cerrado e integrado el 2026-10-28 (#68).
- [005 — llaves anidadas en una anotación (#65)](005-nested-braces.md):
  cerrado e integrado el 2026-10-28 (#67).
- [004 — espacio dentro de una anotación (PR #52)](004-annotation-space.md):
  cerrado e integrado el 2026-10-28 (#64).
- [003 — resultados fuera de `float64` (#59)](003-float-range.md):
  cerrado e integrado el 2026-10-28 (#62).
- [001 — `CanonicalKey` exacto (issue #54)](001-canonicalkey-exact.md):
  cerrado e integrado el 2026-10-26 (#57).
- [002 — Comparación ejecutable contra Ucum-java](002-compare-java.md):
  cerrado e integrado el 2026-10-27 (#60).

**Próximo paso:** commit y PR de 009 cuando el usuario lo pida. La release 4.4.0 (#73) la integra el usuario. Los defectos de Ucum-java quedaron
redactados en [002-java-issues.md](002-java-issues.md), sin publicar.
Issues abiertos: #75 (redondeo de las escalas logarítmicas; 009 cubre la
opción A).
**Integrado:** PR #72, *rebase and merge* en `main` (2026-10-29): `feat(fhir)`
y `docs:`; cierra #55; sale en v4.4.0. PR #70, *rebase and merge* en `main` (2026-10-28): `d73d3b9`
(`fix:`), `94f1415` (`docs:`) y `2368b6f`; cierra #56; sale en v4.3.5 (#71,
publicada el 2026-10-28). PR #68, squash `672635f` en `main` (2026-10-28), cierra #61;
sale en v4.3.4 (#69, publicada el 2026-10-28). PR #67, squash `cd0d2bd` en `main` (2026-10-28), cierra #65;
sale con #64 en v4.3.3 (#66, publicada el 2026-10-28). PR #64, *rebase and merge* en `main` (2026-10-28): `bd014cb`
(kaptinlin), `41e983d` y `c93f36d`; cierra #52. PR #57, squash `9691ced` en `main` (2026-10-26), cierra #54;
publicado como v4.3.1 (#58). PR #60, squash `30048dc` en `main`
(2026-10-27); es `test:`, no genera release. PR #62, squash `959d6b1` en
`main` (2026-10-28), cierra #59; publicado como v4.3.2 (#63). PR #49
(actions, con `govulncheck` en Go estable), squash `c178b4c` (2026-10-28).
**Decisiones pendientes del usuario:** ejecutar ucum-lhc 8.0.1 o no. #53 espera un issue de diseño del autor. Java lo
ejecuta el usuario (`make java-dump`).
**Evidencia:** 001 a 005 en [`evidence/`](evidence/).
