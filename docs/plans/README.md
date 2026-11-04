# Punto de reanudación

**Incremento activo:** [016 — un prefijo o una unidad de valor 0 dividen por cero](016-zero-prefix.md),
paso 9 (cierre, sin commit), rama `fix/zero-prefix`. Cerrados:
- [015 — un factor canónico negativo en la vía log10](015-negative-factor.md):
  cerrado e integrado el 2026-11-03 (#95).
- [014 — un ángulo fuera de (-π/2, π/2) hacia una escala de tangente](014-tan-domain.md):
  cerrado e integrado el 2026-11-02 (#92).
- [013 — un valor negativo en una escala de raíz cuadrada](013-sqrt-negative.md):
  cerrado e integrado el 2026-11-02 (#88).
- [012 — las escalas de temperatura en la vía float64](012-offset-float.md):
  cerrado e integrado el 2026-11-02 (#85).
- [011 — un solo redondeo con el factor de la unidad (#75, opción B)](011-log-factor.md):
  cerrado e integrado el 2026-11-02 (#82).
- [010 — potencia exacta cuando el intermedio no es normal (#75)](010-log-nonnormal.md):
  cerrado e integrado el 2026-10-31 (#79).
- [009 — potencia exacta en las escalas logarítmicas (#75)](009-log-pow.md):
  cerrado e integrado el 2026-10-30 (#76).
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

**Próximo paso:** commit `fix:` de 016 y PR (el usuario hace el push); después, release 4.4.8. Verificación al cierre en verde (evidence/016-2026-11-03-mutations.txt). Un PR de release-please no dispara el CI (lo abre `GITHUB_TOKEN`) y el ruleset lo bloquea: cerrarlo y reabrirlo lo dispara (#93). Pendientes sin issue: el signo del cero (010 §10; 011 §12); los de 014 §6 (tangente a tangente, ángulo diminuto, `Validate` de una unidad cíclica, DAG exponencial); las citas de temperatura de 012 dicen §21-22 y los orígenes están en §30 ■4 y §43 (013 §6); los límites de 012 §6. v4.4.7 publicada el 2026-11-03 (#96). CI y `make check` usan golangci-lint 2.15.0 (`GOLANGCI_LINT_VERSION`). Los defectos de Ucum-java quedaron
redactados en [002-java-issues.md](002-java-issues.md), sin publicar.
Issues abiertos: ninguno propio (#75 cerrado por 011; PR #53 espera a su autor).
**Integrado:** PR #88, *rebase and merge* en `main` (2026-11-02): `fix:` (plan 013, negativos en `sqrt`) y `ci:` (golangci-lint 2.15.0);
sale en v4.4.5 (#89, publicada el 2026-11-02). PR #85, *rebase and merge* en `main` (2026-11-02): `fix:` (plan 012, temperatura en la vía float64);
sale en v4.4.4 (#86, publicada el 2026-11-02). PR #82, *rebase and merge* en `main` (2026-11-02): `fix:`; cierra #75 (opción B);
sale en v4.4.3 (#83, publicada el 2026-11-02). PR #79, *rebase and merge* en `main` (2026-10-31): `fix:`; refiere #75 (tramo no normal);
sale en v4.4.2 (#80, publicada el 2026-10-31). PR #76, *rebase and merge* en `main` (2026-10-30): `fix:` y
`docs:`; refiere #75 (opción A); sale en v4.4.1. PR #72, *rebase and merge* en `main` (2026-10-29): `feat(fhir)`
y `docs:`; cierra #55; sale en v4.4.0 (#73, publicada el 2026-10-29). PR #70, *rebase and merge* en `main` (2026-10-28): `d73d3b9`
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
