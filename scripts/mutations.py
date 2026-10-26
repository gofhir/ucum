#!/usr/bin/env python3
"""Mutaciones del plan 001 (docs/plans/001-canonicalkey-exact.md, §7).

Cada mutación se aplica en una copia del repo y se corre el test indicado.
"kill" exige que el test falle; "survive" exige que pase, para las que
comprueban que un oráculo debilitado de verdad deja de ver M1.

Uso: python3 -I scripts/mutations.py [carpeta de packages]
La carpeta de packages (por omisión .cache/packages) es la de
make check-packages; sin ella no corre TestOfficialExamples.
"""
import os
import shutil
import subprocess
import sys
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PACKAGES = sys.argv[1] if len(sys.argv) > 1 else os.path.join(REPO, ".cache", "packages")

Q = "fhir/quantity.go"
T = "fhir/canonicalkey_test.go"

M1 = (Q, "if q.Exact != nil {\n\t\tp, ok, err", "if false && q.Exact != nil {\n\t\tp, ok, err")

# (id, descripción, [(archivo, viejo, nuevo)], regex de tests, esperado)
MUTATIONS = [
    ("M1", "back to Canonical(q.float())", [M1],
     "CanonicalKeyExactByHand|CanonicalKeyScaleCommonUnits|CanonicalKeyMatchesCompareCommonUnits|OfficialExamples", "kill"),
    ("M1/hand", "M1 against the hand cases alone", [M1], "CanonicalKeyExactByHand$", "kill"),
    ("M1/scale", "M1 against the scale oracle alone", [M1], "CanonicalKeyScaleCommonUnits$", "kill"),
    ("M1/compare", "M1 against the Compare oracle alone", [M1], "CanonicalKeyMatchesCompareCommonUnits$", "kill"),
    ("M1/examples", "M1 against the official examples alone", [M1], "OfficialExamples$", "kill"),
    ("M2", "ignore Exact, use Value",
     [(Q, "c.canonicalRat(q.Exact, q.Code)", "c.canonicalRat(new(big.Rat).SetFloat64(q.Value), q.Code)")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M3", "return ErrNotRational instead of falling back",
     [(Q, "if errors.Is(err, ucum.ErrNotRational) {\n\t\treturn ucum.RatPair{}, false, nil", "if false {\n\t\treturn ucum.RatPair{}, false, nil")],
     "CanonicalKeyNonRationalUsesExact$", "kill"),
    ("M4", "round toward zero",
     [(Q, "f, _ := p.Value.Float64()", "f, _ := new(big.Float).SetPrec(53).SetMode(big.ToZero).SetRat(p.Value).Float64()")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M9", "break ties away from zero",
     [(Q, "f, _ := p.Value.Float64()", "f, _ := new(big.Float).SetPrec(53).SetMode(big.ToNearestAway).SetRat(p.Value).Float64()")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M10", "round through a 64-bit big.Float",
     [(Q, "f, _ := p.Value.Float64()", "f, _ := new(big.Float).SetPrec(64).SetRat(p.Value).Float64()")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M11", "round through a 56-bit big.Float",
     [(Q, "f, _ := p.Value.Float64()", "f, _ := new(big.Float).SetPrec(56).SetRat(p.Value).Float64()")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M12", "round through a 200-bit big.Float",
     [(Q, "f, _ := p.Value.Float64()", "f, _ := new(big.Float).SetPrec(200).SetRat(p.Value).Float64()")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M13", "honour Exact only when Value is zero",
     [(Q, "if q.Exact != nil {\n\t\tp, ok, err", "if q.Exact != nil && q.Value == 0 {\n\t\tp, ok, err")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M14", "round through a big.Float as wide as the rational",
     [(Q, "f, _ := p.Value.Float64()", "f, _ := new(big.Float).SetRat(p.Value).Float64()")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M15", "drop the sign of the exact value",
     [(Q, "f, _ := p.Value.Float64()", "f, _ := new(big.Rat).Abs(p.Value).Float64()")],
     "CanonicalKeyExactByHand$", "kill"),
    ("M16", "keep -0 when the exact value underflows",
     [(Q, "if f == 0 {\n\t\t\t\t// Un negativo", "if false {\n\t\t\t\t// Un negativo")],
     "CanonicalKeyEdges$", "kill"),
    ("M5", "fall back to float64 on any error",
     [(Q, "if err != nil {\n\t\treturn ucum.RatPair{}, false, err", "if err != nil {\n\t\treturn ucum.RatPair{}, false, nil")],
     "CanonicalKeyPropagatesErrors$", "kill"),
    ("M6", "scale oracle compares keys only: M1 must survive it",
     [M1, (T, "if gotCode != code || got != want {", "if gotCode != code {")],
     "CanonicalKeyScaleCommonUnits$", "survive"),
    ("M7", "fallback uses Value instead of float()",
     [(Q, "p, err := c.svc.Canonical(q.float(), q.Code)\n\tif err != nil {\n\t\treturn \"\", 0, err\n\t}\n\treturn p.Code",
       "p, err := c.svc.Canonical(q.Value, q.Code)\n\tif err != nil {\n\t\treturn \"\", 0, err\n\t}\n\treturn p.Code")],
     "CanonicalKeyNonRationalUsesExact$", "kill"),
    ("M8", "Compare oracle with same-unit pairs: M1 must survive it",
     [M1,
      (T, "pairs = append(pairs, [2]string{g[0], g[i]})", "pairs = append(pairs, [2]string{g[i], g[i]})"),
      (T, "pairs = append(pairs, [2]string{g[i-1], g[i]})", "pairs = append(pairs, [2]string{g[i], g[i]})")],
     "CanonicalKeyMatchesCompareCommonUnits$", "survive"),
]


def run(work, env, tests):
    """Corre los tests y devuelve "kill", "survive", "no tests" o "build error"."""
    r = subprocess.run(["go", "test", "-count=1", "-v", "-run", tests, "./fhir/"],
                       cwd=work, env=env, capture_output=True, text=True)
    out = r.stdout + r.stderr
    if "[build failed]" in out or "[setup failed]" in out:
        return "build error"
    # Un regex que no encuentra tests pasaría y haría «sobrevivir» en vano.
    if "no tests to run" in out:
        return "no tests"
    if r.returncode == 0:
        return "survive"
    # Solo cuenta como muerto si falló un test, no el arranque.
    return "kill" if "--- FAIL: Test" in out else "build error"


def main():
    ok = True
    env = dict(os.environ, UCUM_PACKAGES=PACKAGES)
    tests = {m[3] for m in MUTATIONS}
    # Sin mutación todos tienen que pasar; si no, un "kill" no prueba nada.
    work = tempfile.mkdtemp(prefix="ucum-mut-")
    try:
        shutil.copytree(REPO, work, dirs_exist_ok=True,
                        ignore=shutil.ignore_patterns(".git", ".cache"))
        for t in sorted(tests):
            got = run(work, env, t)
            if got != "survive":
                print(f"baseline     UNEXPECTED want survive got {got:11} {t}")
                sys.exit(1)
    finally:
        shutil.rmtree(work)
    print("baseline     ok         every test passes unmutated")
    for mid, desc, edits, tests, want in MUTATIONS:
        work = tempfile.mkdtemp(prefix="ucum-mut-")
        try:
            shutil.copytree(REPO, work, dirs_exist_ok=True,
                            ignore=shutil.ignore_patterns(".git", ".cache"))
            for path, old, new in edits:
                full = os.path.join(work, path)
                src = open(full).read()
                if src.count(old) != 1:
                    print(f"{mid}: pattern not found exactly once in {path}")
                    ok = False
                    break
                open(full, "w").write(src.replace(old, new))
            else:
                got = run(work, env, tests)
                mark = "ok" if got == want else "UNEXPECTED"
                if got != want:
                    ok = False
                print(f"{mid:12} {mark:10} want {want:7} got {got:11} {desc}")
        finally:
            shutil.rmtree(work)
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
