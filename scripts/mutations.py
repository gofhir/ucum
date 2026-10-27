#!/usr/bin/env python3
"""Mutaciones de los planes 001 (docs/plans/001-canonicalkey-exact.md, §7) y
002 (docs/plans/002-compare-java.md, §7).

Cada mutación se aplica en una copia del repo y se corre el test indicado.
"kill" exige que el test falle; "survive" exige que pase, para las que
comprueban que un oráculo debilitado de verdad deja de ver M1.

Uso: python3 -I scripts/mutations.py [carpeta de packages]
La carpeta de packages (por omisión .cache/packages) es la de
make check-packages; sin ella no corre TestOfficialExamples.
"""
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PACKAGES = sys.argv[1] if len(sys.argv) > 1 else os.path.join(REPO, ".cache", "packages")

Q = "fhir/quantity.go"
T = "fhir/canonicalkey_test.go"

E = "internal/engine"
JC = E + "/javaclasses_test.go"
JR = E + "/testdata/java/ucum-java-1.0.10.tsv"
JCASES = E + "/testdata/java/cases.tsv"
JAVA_TESTS = "TestCompareJava$|TestJavaCasesFresh$"

M1 = (Q, "if q.Exact != nil {\n\t\tp, ok, err", "if false && q.Exact != nil {\n\t\tp, ok, err")

# (id, descripción, [(archivo, viejo, nuevo)], regex de tests, esperado
#  [, paquete, resellar]). Resellar reescribe en el manifiesto de 002 el
# SHA-256 de los casos y del resultado, para que la mutación llegue a la
# comparación en vez de frenarse en la integridad.
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

    # Plan 002: la comparación contra Ucum-java.
    ("J1", "ignore the exponent of a prefix on a base unit",
     [(E + "/canonicalize.go", "val := prefixVal.Pow(sym.Exponent)", "val := prefixVal")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J2", "accept the empty expression as unity, as Java does",
     [("internal/expr/parser.go", 'if code == "" {\n\t\treturn nil, fmt.Errorf("UCUM expression is empty")',
       'if code == "" {\n\t\treturn &Term{Comp: &Factor{Value: 1}}, nil')],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J3", "double the factor of [ft_i] and everything defined from it",
     [(E + "/canonicalize.go", "can.Value = prefixVal.Mul(unitValue).Mul(can.Value)\n",
       "can.Value = prefixVal.Mul(unitValue).Mul(can.Value)\n\tif u.Code == \"[ft_i]\" {\n\t\tcan.Value = can.Value.Add(can.Value)\n\t}\n")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J4", "a class predicate that accepts everything",
     [(JC, "_, listed := javaSyntaxBugs[d.Case.Args[0]]", "listed := true"),
      (JC, 'return d.Case.Op == "validate" && d.What == "status" && d.Java.Status == "ok" &&\n\t\t\t\td.Ours.Status == "invalid" && listed',
       "return listed")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J5", "one digit of Java's result changed (10th significant digit of 1 deg), resealed",
     [(JR, "xml-atom:00075\tok\t0.0174532925199432957692369", "xml-atom:00075\tok\t0.0174532926199432957692369")],
     JAVA_TESTS, "kill", "./internal/engine/", True),
    ("J5/sha", "the same digit changed without resealing",
     [(JR, "xml-atom:00075\tok\t0.0174532925199432957692369", "xml-atom:00075\tok\t0.0174532926199432957692369")],
     "TestCompareJava$", "kill", "./internal/engine/", False),
    ("J6", "cases.tsv changed without regenerating the result",
     [(JCASES, "xml-atom:00000\tvalidate\txml-atom\tm\n", "xml-atom:00000\tvalidate\txml-atom\tcm\n")],
     "TestCompareJava$", "kill", "./internal/engine/", False),
    ("J6/fresh", "cases.tsv changed and resealed: the generator no longer produces it",
     [(JCASES, "xml-atom:00000\tvalidate\txml-atom\tm\n", "xml-atom:00000\tvalidate\txml-atom\tcm\n")],
     "TestJavaCasesFresh$", "kill", "./internal/engine/", True),
    ("J7", "a class that explains no case",
     [(JC, "var javaClasses = []javaClass{\n",
       'var javaClasses = []javaClass{\n\t{Name: "java-dead", Kind: "deliberate", match: func(*javaContext, javaDiff) bool { return false }},\n')],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J8", "drop [pH] from Java's HoldingHandler list",
     [(JC, '"[pH]": true,\n\t\t"Np"', '\n\t\t"Np"')],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J9", "wrong pH formula in the specification oracle",
     [(JC, "\"pH\":          func(x float64) float64 { return math.Pow(10, -x) },         // §45■3",
       "\"pH\":          func(x float64) float64 { return math.Pow(10, x) },         // §45■3")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J10", "a result line deleted, resealed",
     [(JR, "xml-atom:00075\tok\t0.0174532925199432957692369\trad\t24\n", "")],
     JAVA_TESTS, "kill", "./internal/engine/", True),
    ("J11", "our pH handler without the minus sign",
     [("internal/special/special.go", '"pH":       func(c string) Handler { return logHandler{unitCode: c, base: 10, expDivisor: 1, negate: true} },',
       '"pH":       func(c string) Handler { return logHandler{unitCode: c, base: 10, expDivisor: 1} },')],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J12", "double the factor of t, an integer in the XML (Java declares one digit for it)",
     [(E + "/canonicalize.go", "can.Value = prefixVal.Mul(unitValue).Mul(can.Value)\n",
       "can.Value = prefixVal.Mul(unitValue).Mul(can.Value)\n\tif u.Code == \"t\" {\n\t\tcan.Value = can.Value.Add(can.Value)\n\t}\n")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J13", "a wrong conversion result with every atom right",
     [(E + "/exact.go", "\treturn s.convertRatCore(value, convertParts{\n\t\tsrcTerm: srcTerm, dstTerm: dstTerm,",
       "\tif from == \"[bu_us]\" && to == \"[pk_us]\" {\n\t\tvalue = new(big.Rat).Mul(value, big.NewRat(10, 1))\n\t}\n"
       "\treturn s.convertRatCore(value, convertParts{\n\t\tsrcTerm: srcTerm, dstTerm: dstTerm,")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J14", "conversion into [degRe] off by one, from a linear unit too",
     [(E + "/exact.go", "\treturn s.fromCanonicalRat(p.dstTerm, p.to, result)\n}",
       "\tr, err := s.fromCanonicalRat(p.dstTerm, p.to, result)\n\tif err == nil && p.to == \"[degRe]\" {\n"
       "\t\tr.Add(r, big.NewRat(1, 1))\n\t}\n\treturn r, err\n}")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J15", "the loader doubles the value of [foz_m]",
     [("internal/essence/essence.go", "\t\t\tunitVal = &model.Conversion{Unit: xu.Value.Unit, Text: xu.Value.Text, Value: v}\n",
       "\t\t\tif xu.Code == \"[foz_m]\" {\n\t\t\t\tv = v.Add(v)\n\t\t\t}\n"
       "\t\t\tunitVal = &model.Conversion{Unit: xu.Value.Unit, Text: xu.Value.Text, Value: v}\n")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
    ("J16", "ConvertRat converts between two arbitrary units, like Java",
     [(E + "/exact.go", "ErrNilValue)\n\t}\n\n\tsrcTerm, srcCan, err := s.canonicalParts(from)\n",
       "ErrNilValue)\n\t}\n\tif from == \"[iU]\" && to == \"[IU]\" {\n\t\treturn new(big.Rat).Set(value), nil\n\t}\n"
       "\n\tsrcTerm, srcCan, err := s.canonicalParts(from)\n")],
     JAVA_TESTS, "kill", "./internal/engine/", False),
]


def reseal(work):
    """Reescribe en el manifiesto el SHA-256 de los casos y del resultado."""
    path = os.path.join(work, E, "testdata/java/manifest.json")
    m = json.load(open(path))
    for key, f in (("cases_sha256", JCASES), ("result_sha256", JR)):
        m[key] = hashlib.sha256(open(os.path.join(work, f), "rb").read()).hexdigest()
    json.dump(m, open(path, "w"), indent=2)


def spec(m):
    """Completa una mutación con su paquete y si hay que resellar."""
    return m if len(m) == 7 else m + ("./fhir/", False)


def run(work, env, tests, pkg):
    """Corre los tests y devuelve "kill", "survive", "no tests" o "build error"."""
    r = subprocess.run(["go", "test", "-count=1", "-v", "-run", tests, pkg],
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
    tests = {(spec(m)[3], spec(m)[5]) for m in MUTATIONS}
    # Sin mutación todos tienen que pasar; si no, un "kill" no prueba nada.
    work = tempfile.mkdtemp(prefix="ucum-mut-")
    try:
        shutil.copytree(REPO, work, dirs_exist_ok=True,
                        ignore=shutil.ignore_patterns(".git", ".cache"))
        for t, pkg in sorted(tests):
            got = run(work, env, t, pkg)
            if got != "survive":
                print(f"baseline     UNEXPECTED want survive got {got:11} {pkg} {t}")
                sys.exit(1)
    finally:
        shutil.rmtree(work)
    print("baseline     ok         every test passes unmutated")
    for mid, desc, edits, tests, want, pkg, resealed in map(spec, MUTATIONS):
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
                if resealed:
                    reseal(work)
                got = run(work, env, tests, pkg)
                mark = "ok" if got == want else "UNEXPECTED"
                if got != want:
                    ok = False
                print(f"{mid:12} {mark:10} want {want:7} got {got:11} {desc}")
        finally:
            shutil.rmtree(work)
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
