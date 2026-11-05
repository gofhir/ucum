package ucum_test

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 017: una definición compartida se expande una vez por servicio. UCUM
// §18 ■5 da a una unidad el valor de «a real number scalar» por la expresión
// que la define; el valor de una unidad es una función de su definición, no de
// dónde aparece, así que dos caminos hasta la misma unidad dan el mismo valor y
// pueden compartir el trabajo.
//
// Los DAG de prueba son [e_i] = 2 [e_{i+1}]/[e_{i+1}] (valor 2, adimensional
// en cada nivel) y [d_i] = 3 [d_{i+1}].[d_{i+1}] (valor distinto por rama),
// ambos con la hoja en 1 m. Sin memoria, expandir [e_0] visita 2^n caminos.
//
// Ningún esperado sale de la librería: salen de la aritmética (3^(2^n − 1) con
// math/big) y de §18 ■5.
//
// Medir el costo: sin reloj. Los tiempos de pared dependen de la máquina y de
// la carga; las asignaciones (runtime.MemStats.Mallocs) son deterministas y
// crecen con el trabajo. Cada medición toma un servicio recién construido
// (construirlo no cuenta) y mide solo su primera llamada; así el caché está
// frío, que es lo que cuesta. Con el caché el trabajo es lineal en n, y la
// razón entre n = 16 y n = 8 queda cerca de 2; sin él es 2^8 = 256.
//
// Los tamaños (8 y 16) los fija el código de hoy: a n = 20 expandir cuesta
// 0.7 s sin caché, n = 30 serían 12 minutos, y n = 16 cuesta unos 40 ms. Por
// eso los valores de n = 30 y n = 200 solo se piden cuando la medición dio
// lineal (dagSize); si no, esos tests bajan a n = 8 para fallar por razón y no
// colgar la corrida.

const (
	dagSmall    = 8
	dagLarge    = 16
	dagMaxRatio = 3.0
)

// dagEUnits escribe [e_0] … [e_n]: cada nivel vale factor por el cociente de
// sí mismo con el siguiente, y la hoja vale 1 m.
func dagEUnits(n int, factor string) string { return dagEUnitsOn(n, factor, "m") }

// dagEUnitsOn es dagEUnits con la hoja en 1 de la unidad base dada (el XML
// mínimo de tanCustomXML solo tiene rad).
func dagEUnitsOn(n int, factor, leaf string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, ` <unit Code="[e_%d]" CODE="[E_%d]" isMetric="no" class="dimless">
  <name>e</name><printSymbol>e</printSymbol><property>test</property>
  <value Unit="[e_%d]/[e_%d]" UNIT="[E_%d]/[E_%d]" value="%s">e</value>
 </unit>
`, i, i, i+1, i+1, i+1, i+1, factor)
	}
	fmt.Fprintf(&b, ` <unit Code="[e_%d]" CODE="[E_%d]" isMetric="no" class="dimless">
  <name>e</name><printSymbol>e</printSymbol><property>test</property>
  <value Unit="%s" UNIT="%s" value="1">e</value>
 </unit>
`, n, n, leaf, strings.ToUpper(leaf))
	return b.String()
}

// dagDUnits escribe [d_0] … [d_n]: cada nivel vale 3 por la multiplicación de dos
// copias del siguiente, y la hoja vale 1 m.
func dagDUnits(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, ` <unit Code="[d_%d]" CODE="[D_%d]" isMetric="no" class="dimless">
  <name>d</name><printSymbol>d</printSymbol><property>test</property>
  <value Unit="[d_%d].[d_%d]" UNIT="[D_%d].[D_%d]" value="3">d</value>
 </unit>
`, i, i, i+1, i+1, i+1, i+1)
	}
	fmt.Fprintf(&b, ` <unit Code="[d_%d]" CODE="[D_%d]" isMetric="no" class="dimless">
  <name>d</name><printSymbol>d</printSymbol><property>test</property>
  <value Unit="m" UNIT="M" value="1">d</value>
 </unit>
`, n, n)
	return b.String()
}

// dagOfficialXML es el XML embebido con unidades propias agregadas.
func dagOfficialXML(t testing.TB, extra string) string {
	t.Helper()
	raw, err := os.ReadFile("internal/essence/ucum-essence.xml")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(string(raw), "</root>", extra+"</root>", 1)
}

func dagService(t testing.TB, xml string) ucum.Service {
	t.Helper()
	svc, err := ucum.NewFromReader(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("NewFromReader: %v", err)
	}
	return svc
}

func dagExact(t testing.TB, svc ucum.Service) ucum.ExactService {
	t.Helper()
	ex, ok := svc.(ucum.ExactService)
	if !ok {
		t.Fatal("service is not an ExactService")
	}
	return ex
}

// dagMallocs cuenta las asignaciones de f. No hay tests paralelos corriendo
// mientras un test secuencial está activo, así que el contador global solo ve
// a f.
func dagMallocs(f func()) uint64 {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.Mallocs - a.Mallocs
}

// dagMinMallocs mide f sobre `runs` servicios recién construidos y devuelve el
// menor (el ruido de fondo solo suma).
func dagMinMallocs(runs int, build func() func()) uint64 {
	best := uint64(math.MaxUint64)
	for i := 0; i < runs; i++ {
		f := build()
		if m := dagMallocs(f); m < best {
			best = m
		}
	}
	return best
}

type dagCost struct {
	canonSmall, canonLarge uint64
	newSmall, newLarge     uint64
}

func (c dagCost) canonRatio() float64 { return dagRatio(c.canonLarge, c.canonSmall) }
func (c dagCost) newRatio() float64   { return dagRatio(c.newLarge, c.newSmall) }

// dagRatio es large/small. Una medición vacía (small = 0: un Fatal dentro del
// sync.Once dejó el resultado en cero) da +Inf y no NaN, para que falle en vez
// de pasar: NaN > dagMaxRatio es falso.
func dagRatio(large, small uint64) float64 {
	if small == 0 {
		return math.Inf(1)
	}
	return float64(large) / float64(small)
}
func (c dagCost) linear() bool {
	return c.canonRatio() <= dagMaxRatio && c.newRatio() <= dagMaxRatio
}

var (
	dagCostOnce sync.Once
	dagCostRes  dagCost
)

// dagMeasure mide una sola vez, para todos los tests del archivo.
func dagMeasure(t testing.TB) dagCost {
	t.Helper()
	dagCostOnce.Do(func() {
		canon := func(n int) uint64 {
			xml := dagOfficialXML(t, dagEUnits(n, "2"))
			return dagMinMallocs(2, func() func() {
				svc := dagService(t, xml)
				return func() { _, _ = svc.Canonical(1, "[e_0]") }
			})
		}
		// [pi] como raíz: New canonicaliza [pi] al cargar, así que la carga
		// misma expande el DAG.
		newSvc := func(n int) uint64 {
			xml := tanCustomXML("[e_0]", "1", false, dagEUnitsOn(n, "2", "rad"))
			return dagMinMallocs(2, func() func() {
				return func() { _, _ = ucum.NewFromReader(strings.NewReader(xml)) }
			})
		}
		dagCostRes = dagCost{
			canonSmall: canon(dagSmall), canonLarge: canon(dagLarge),
			newSmall: newSvc(dagSmall), newLarge: newSvc(dagLarge),
		}
	})
	return dagCostRes
}

// dagSize devuelve want si el costo es lineal y, si no, un tamaño que la
// corrida de hoy resuelve en un instante.
func dagSize(t testing.TB, want int) int {
	t.Helper()
	if dagMeasure(t).linear() {
		return want
	}
	return min(want, dagSmall)
}

// 4.1: el costo de la primera canonicalización es lineal en n.
func TestDagFirstCanonicalCostIsLinear(t *testing.T) {
	c := dagMeasure(t)
	t.Logf("Canonical first call: n=%d %d mallocs, n=%d %d mallocs, ratio %.2f",
		dagSmall, c.canonSmall, dagLarge, c.canonLarge, c.canonRatio())
	if r := c.canonRatio(); r > dagMaxRatio {
		t.Fatalf("first Canonical(1, [e_0]) allocations n=%d vs n=%d: ratio %.1f, want at most %.0f (linear); the work doubles per level",
			dagLarge, dagSmall, r, dagMaxRatio)
	}
	// Con el costo lineal ya medido, n = 50 y n = 100 son baratos.
	m := func(n int) uint64 {
		xml := dagOfficialXML(t, dagEUnits(n, "2"))
		return dagMinMallocs(2, func() func() {
			svc := dagService(t, xml)
			return func() { _, _ = svc.Canonical(1, "[e_0]") }
		})
	}
	a, b := m(50), m(100)
	if r := float64(b) / float64(a); r > dagMaxRatio {
		t.Errorf("first Canonical allocations n=100 vs n=50: ratio %.2f, want at most %.0f", r, dagMaxRatio)
	}
}

// 4.1: con [pi] como raíz del DAG, New termina y su costo es lineal.
func TestDagPiRootLoadCostIsLinear(t *testing.T) {
	c := dagMeasure(t)
	t.Logf("New with [pi] root: n=%d %d mallocs, n=%d %d mallocs, ratio %.2f",
		dagSmall, c.newSmall, dagLarge, c.newLarge, c.newRatio())
	if r := c.newRatio(); r > dagMaxRatio {
		t.Fatalf("New allocations with [pi] = [e_0], n=%d vs n=%d: ratio %.1f, want at most %.0f (linear)",
			dagLarge, dagSmall, r, dagMaxRatio)
	}
	for _, n := range []int{30, 200} {
		svc := dagService(t, tanCustomXML("[e_0]", "1", false, dagEUnitsOn(n, "2", "rad")))
		// [pi] vale 2: basta con que la carga termine y el DAG responda.
		if got, err := svc.Canonical(1, "[e_0]"); err != nil || got != (ucum.Pair{Value: 2, Code: "1"}) {
			t.Errorf("n=%d: Canonical(1, [e_0]) = (%v, %v), want 2 and dimensionless", n, got, err)
		}
	}
}

// 4.1: el valor es 2 y adimensional (2·(v/v) = 2 por nivel) en todas las vías.
func TestDagValuesAreTwoAndDimensionless(t *testing.T) {
	for _, want := range []int{30, 200} {
		n := dagSize(t, want)
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			xml := dagOfficialXML(t, dagEUnits(n, "2"))
			svc := dagService(t, xml)
			ex := dagExact(t, svc)
			ci, err := ucum.NewCaseInsensitiveFromReader(strings.NewReader(xml))
			if err != nil {
				t.Fatal(err)
			}
			two := ucum.Pair{Value: 2, Code: "1"}

			if got, err := svc.Canonical(1, "[e_0]"); err != nil || got != two {
				t.Errorf("Canonical(1, [e_0]) = (%v, %v), want %v", got, err, two)
			}
			if got, err := svc.Canonical(5, "[e_0]"); err != nil || got != (ucum.Pair{Value: 10, Code: "1"}) {
				t.Errorf("Canonical(5, [e_0]) = (%v, %v), want 10 dimensionless", got, err)
			}
			if got, err := svc.Convert(1, "[e_0]", "1"); err != nil || got != 2 {
				t.Errorf("Convert(1, [e_0], 1) = (%v, %v), want 2", got, err)
			}
			if got, err := svc.Convert(3, "[e_0]", "[e_1]"); err != nil || got != 3 {
				t.Errorf("Convert(3, [e_0], [e_1]) = (%v, %v), want 3", got, err)
			}
			rp, err := ex.CanonicalRat(big.NewRat(3, 2), "[e_0]")
			if err != nil || rp.Code != "1" || rp.Value.Cmp(big.NewRat(3, 1)) != 0 {
				t.Errorf("CanonicalRat(3/2, [e_0]) = (%v, %v), want 3 dimensionless", rp, err)
			}
			if got, err := ci.Canonical(1, "[E_0]"); err != nil || got != two {
				t.Errorf("case-insensitive Canonical(1, [E_0]) = (%v, %v), want %v", got, err, two)
			}
			if err := svc.ValidateInProperty("[e_0]", "test"); err != nil {
				t.Errorf("ValidateInProperty([e_0], test) = %v, want nil", err)
			}
			if err := svc.ValidateInProperty("[e_0]", "mass"); err == nil {
				t.Error("ValidateInProperty([e_0], mass) = nil, want an error")
			}
			// 1 [e_0] · 3 [e_1] = 3·2·2 = 12; 6 [e_0] / 3 [e_1] = 12/6 = 2.
			// La representación del resultado es del servicio; su valor
			// canónico no.
			p, err := svc.Multiply(ucum.Pair{Value: 1, Code: "[e_0]"}, ucum.Pair{Value: 3, Code: "[e_1]"})
			if err != nil {
				t.Fatalf("Multiply: %v", err)
			}
			if got, err := svc.Canonical(p.Value, p.Code); err != nil || got != (ucum.Pair{Value: 12, Code: "1"}) {
				t.Errorf("Multiply result %v canonicalizes to (%v, %v), want 12 dimensionless", p, got, err)
			}
			q, err := svc.Divide(ucum.Pair{Value: 6, Code: "[e_0]"}, ucum.Pair{Value: 3, Code: "[e_1]"})
			if err != nil {
				t.Fatalf("Divide: %v", err)
			}
			if got, err := svc.Canonical(q.Value, q.Code); err != nil || got != two {
				t.Errorf("Divide result %v canonicalizes to (%v, %v), want %v", q, got, err, two)
			}
		})
	}
}

// 4.1: un caché por servicio, no uno global: el mismo texto [e_0] vale 2 en un
// servicio y 5 en otro.
func TestDagCacheIsPerService(t *testing.T) {
	a := dagService(t, dagOfficialXML(t, dagEUnits(5, "2")))
	b := dagService(t, dagOfficialXML(t, dagEUnits(5, "5")))
	for i := 0; i < 2; i++ {
		if got, err := a.Canonical(1, "[e_0]"); err != nil || got.Value != 2 {
			t.Errorf("service A: Canonical(1, [e_0]) = (%v, %v), want 2", got, err)
		}
		if got, err := b.Canonical(1, "[e_0]"); err != nil || got.Value != 5 {
			t.Errorf("service B: Canonical(1, [e_0]) = (%v, %v), want 5", got, err)
		}
	}
}

// dagPow3 es 3^e.
func dagPow3(e int64) *big.Int { return new(big.Int).Exp(big.NewInt(3), big.NewInt(e), nil) }

// 4.2: un DAG con valores distintos por rama. [d_0] = 3^(2^n − 1) m^(2^n).
func TestDagDistinctBranchValues(t *testing.T) {
	const n = 6
	exp := int64(1)<<n - 1 // 63
	unit := fmt.Sprintf("m%d", int64(1)<<n)
	xml := dagOfficialXML(t, dagDUnits(n))
	svc := dagService(t, xml)
	ex := dagExact(t, svc)
	want := new(big.Rat).SetInt(dagPow3(exp))

	rp, err := ex.CanonicalRat(big.NewRat(1, 1), "[d_0]")
	if err != nil || rp.Code != unit || rp.Value.Cmp(want) != 0 {
		t.Errorf("CanonicalRat(1, [d_0]) = (%v, %v), want 3^%d %s", rp, err, exp, unit)
	}
	// Cada nivel i vale 3^(2^(n−i) − 1) m^(2^(n−i)).
	for i := 0; i <= n; i++ {
		e := int64(1)<<(n-i) - 1
		u := fmt.Sprintf("m%d", int64(1)<<(n-i))
		if i == n {
			u = "m"
		}
		rp, err := ex.CanonicalRat(big.NewRat(1, 1), fmt.Sprintf("[d_%d]", i))
		if err != nil || rp.Code != u || rp.Value.Cmp(new(big.Rat).SetInt(dagPow3(e))) != 0 {
			t.Errorf("CanonicalRat(1, [d_%d]) = (%v, %v), want 3^%d %s", i, rp, err, e, u)
		}
	}
	// El float64 redondea 3^63; la tolerancia es la del redondeo.
	wantF, _ := new(big.Float).SetInt(dagPow3(exp)).Float64()
	if got, err := svc.Canonical(1, "[d_0]"); err != nil || got.Code != unit || math.Abs(got.Value-wantF) > wantF*1e-14 {
		t.Errorf("Canonical(1, [d_0]) = (%v, %v), want %g %s", got, err, wantF, unit)
	}
	// [d_0] = 3 [d_1]^2, exacto.
	if got, err := ex.ConvertRat(big.NewRat(1, 1), "[d_0]", "[d_1]2"); err != nil || got.Cmp(big.NewRat(3, 1)) != 0 {
		t.Errorf("ConvertRat(1, [d_0], [d_1]2) = (%v, %v), want 3", got, err)
	}
	ci, err := ucum.NewCaseInsensitiveFromReader(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ci.Canonical(1, "[D_0]"); err != nil || got.Code != unit || math.Abs(got.Value-wantF) > wantF*1e-14 {
		t.Errorf("case-insensitive Canonical(1, [D_0]) = (%v, %v), want %g %s", got, err, wantF, unit)
	}
}

// 4.3: aislamiento. Un acierto del caché que no copia Units le entregaría al
// segundo llamador el exponente ya multiplicado por el primero.
func TestDagCacheHitDoesNotShareUnits(t *testing.T) {
	const n = 6
	xml := dagOfficialXML(t, dagDUnits(n))
	type call struct {
		in   string
		code string
		exp  int64 // el valor es 3^exp
		neg  bool  // 3^-exp
	}
	// [d_0]2 = 3^126 m128; [d_0]-1 = 3^-63 m-64; [d_0] = 3^63 m64.
	calls := []call{
		{"[d_0]2", "m128", 126, false},
		{"[d_0]-1", "m-64", 63, true},
		{"[d_0]", "m64", 63, false},
	}
	orders := map[string][]int{"forward": {0, 1, 2}, "reverse": {2, 1, 0}, "repeat": {1, 1, 0, 0, 2, 2}}
	for name, order := range orders {
		ex := dagExact(t, dagService(t, xml))
		for _, i := range order {
			c := calls[i]
			want := new(big.Rat).SetInt(dagPow3(c.exp))
			if c.neg {
				want.Inv(want)
			}
			rp, err := ex.CanonicalRat(big.NewRat(1, 1), c.in)
			if err != nil || rp.Code != c.code || rp.Value.Cmp(want) != 0 {
				t.Errorf("%s: CanonicalRat(1, %q) = (%v, %v), want %s", name, c.in, rp, err, c.code)
			}
		}
	}
	// El mismo servicio por la vía float, en ambos órdenes.
	for name, order := range orders {
		svc := dagService(t, xml)
		for _, i := range order {
			c := calls[i]
			wantF, _ := new(big.Float).SetInt(dagPow3(c.exp)).Float64()
			if c.neg {
				wantF = 1 / wantF
			}
			got, err := svc.Canonical(1, c.in)
			if err != nil || got.Code != c.code || math.Abs(got.Value-wantF) > wantF*1e-14 {
				t.Errorf("%s: Canonical(1, %q) = (%v, %v), want %g %s", name, c.in, got, err, wantF, c.code)
			}
		}
	}
}

// dagOfficialCodes son los códigos de las unidades del XML oficial.
func dagOfficialCodes(t testing.TB) []string {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	codes := make([]string, 0, len(m.DefinedUnits))
	for _, u := range m.DefinedUnits {
		codes = append(codes, u.Code)
	}
	return codes
}

type dagRes struct {
	pair ucum.Pair
	err  string
}

func dagCanon(svc ucum.Service, code string) dagRes {
	p, err := svc.Canonical(1, code)
	r := dagRes{pair: p}
	if err != nil {
		r.err = err.Error()
	}
	return r
}

func dagSame(a, b dagRes) bool {
	if a.err != b.err || a.pair.Code != b.pair.Code {
		return false
	}
	return a.pair.Value == b.pair.Value || (math.IsNaN(a.pair.Value) && math.IsNaN(b.pair.Value))
}

// 4.4: equivalencia con el XML oficial. Para cada unidad, el caché frío (un
// servicio recién construido, una sola consulta) y el caliente (otro servicio
// ya recorrido en orden inverso) dan el mismo par.
func TestDagOfficialUnitsColdEqualsHot(t *testing.T) {
	codes := dagOfficialCodes(t)
	if len(codes) < 200 {
		t.Fatalf("only %d official units", len(codes))
	}
	hot := dagService(t, dagOfficialXML(t, ""))
	for i := len(codes) - 1; i >= 0; i-- {
		dagCanon(hot, codes[i])
	}
	progressive := dagService(t, dagOfficialXML(t, ""))
	for i, code := range codes {
		h := dagCanon(hot, code)
		if p := dagCanon(progressive, code); !dagSame(p, h) {
			t.Errorf("%q: progressive cache %v, hot cache %v", code, p, h)
		}
		// Un servicio frío por unidad es caro (cada uno carga el XML): uno de
		// cada siete, con otro desplazamiento en cada pasada.
		if i%7 == 0 {
			cold := dagService(t, dagOfficialXML(t, ""))
			if c := dagCanon(cold, code); !dagSame(c, h) {
				t.Errorf("%q: cold cache %v, hot cache %v", code, c, h)
			}
		}
	}
	// Unidades con prefijo, para que la expansión de un átomo compartido pase
	// por la misma ruta.
	for _, code := range []string{"km", "mg", "[ft_i]", "[in_i]2", "[lb_av]/[in_i]2", "cel", "[pH]", "mmol/L", "kg.m/s2"} {
		cold := dagService(t, dagOfficialXML(t, ""))
		if c, h := dagCanon(cold, code), dagCanon(hot, code); !dagSame(c, h) {
			t.Errorf("%q: cold cache %v, hot cache %v", code, c, h)
		}
	}
}

// 4.5: muchas goroutines canonicalizan el DAG y las unidades del XML a la vez
// sobre un servicio compartido y frío. Con -race detecta un caché sin sincronía.
func TestDagConcurrentCanonicalization(t *testing.T) {
	n := dagSize(t, 30)
	const d = 6
	xml := dagOfficialXML(t, dagEUnits(n, "2")+dagDUnits(d))
	codes := dagOfficialCodes(t)

	ref := dagService(t, dagOfficialXML(t, ""))
	want := make([]dagRes, len(codes))
	for i, c := range codes {
		want[i] = dagCanon(ref, c)
	}
	wantD := new(big.Rat).SetInt(dagPow3(1<<d - 1))
	wantDCode := fmt.Sprintf("m%d", 1<<d)

	svc := dagService(t, xml)
	ex := dagExact(t, svc)
	const workers = 16
	var (
		start sync.WaitGroup
		done  sync.WaitGroup
	)
	start.Add(1)
	for w := 0; w < workers; w++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			for k := 0; k < len(codes); k++ {
				i := (k + w*17) % len(codes)
				if got := dagCanon(svc, codes[i]); !dagSame(got, want[i]) {
					t.Errorf("worker %d: Canonical(1, %q) = %v, want %v", w, codes[i], got, want[i])
				}
				if k%25 == 0 {
					if got, err := svc.Canonical(1, "[e_0]"); err != nil || got != (ucum.Pair{Value: 2, Code: "1"}) {
						t.Errorf("worker %d: Canonical(1, [e_0]) = (%v, %v)", w, got, err)
					}
					rp, err := ex.CanonicalRat(big.NewRat(1, 1), "[d_0]")
					if err != nil || rp.Code != wantDCode || rp.Value.Cmp(wantD) != 0 {
						t.Errorf("worker %d: CanonicalRat(1, [d_0]) = (%v, %v)", w, rp, err)
					}
					// Cada llamador recibe sus propias Units: un exponente
					// compartido se multiplicaría entre goroutines.
					if rp, err := ex.CanonicalRat(big.NewRat(1, 1), "[d_0]2"); err != nil || rp.Code != fmt.Sprintf("m%d", 2<<d) {
						t.Errorf("worker %d: CanonicalRat(1, [d_0]2) = (%v, %v)", w, rp, err)
					}
				}
			}
		}()
	}
	start.Done()
	done.Wait()
}

// 4.6: los ciclos de 014 siguen dando el error de ciclo con el caché caliente.
// [ok] y [cy_a] comparten [sh]; [cy_a] = [sh].[cy_b] y [cy_b] = [cy_a] forman
// un ciclo. Una unidad sana canonicalizada antes no debe tapar el ciclo, y el
// ciclo no debe envenenar a la unidad sana.
func TestDagCycleStillAnErrorWithWarmCache(t *testing.T) {
	unit := func(code, of string, value string) string {
		return fmt.Sprintf(` <unit Code=%q CODE=%q isMetric="no" class="dimless">
  <name>cycle</name><printSymbol>c</printSymbol><property>number</property>
  <value Unit=%q UNIT=%q value=%q>c</value>
 </unit>
`, code, strings.ToUpper(code), of, strings.ToUpper(of), value)
	}
	extra := unit("[sh]", "rad", "3") + unit("[ok]", "[sh]", "2") +
		unit("[cy_a]", "[sh].[cy_b]", "1") + unit("[cy_b]", "[cy_a]", "1") +
		unit("[self]", "[self]", "1")
	svc := dagService(t, tanCustomXML("[pi]", "3.141592653589793", false, extra))

	// Calentar: [sh] y [ok] primero.
	for _, c := range []string{"[sh]", "[ok]"} {
		if _, err := svc.Canonical(1, c); err != nil {
			t.Fatalf("warm Canonical(1, %q): %v", c, err)
		}
	}
	for round := 0; round < 3; round++ {
		for _, c := range []string{"[cy_a]", "[cy_b]", "[self]", "[ok].[cy_a]", "[sh].[cy_b]"} {
			_, err := svc.Canonical(1, c)
			if err == nil {
				t.Errorf("round %d: Canonical(1, %q) succeeded, want a cycle error", round, c)
				continue
			}
			if !strings.Contains(err.Error(), "definition cycle") {
				t.Errorf("round %d: Canonical(1, %q) error = %v, want a definition cycle error", round, c, err)
			}
		}
		// Una unidad sana sigue valiendo lo mismo: 2 [sh] = 6 rad.
		if got, err := svc.Canonical(1, "[ok]"); err != nil || got.Value != 6 || got.Code != "rad" {
			t.Errorf("round %d: Canonical(1, [ok]) = (%v, %v), want 6 rad", round, got, err)
		}
	}
	// Y a la inversa: el ciclo primero, en un servicio sin calentar.
	svc2 := dagService(t, tanCustomXML("[pi]", "3.141592653589793", false, extra))
	if _, err := svc2.Canonical(1, "[cy_a]"); err == nil || !strings.Contains(err.Error(), "definition cycle") {
		t.Errorf("cold Canonical(1, [cy_a]) error = %v, want a definition cycle error", err)
	}
	if got, err := svc2.Canonical(1, "[ok]"); err != nil || got.Value != 6 || got.Code != "rad" {
		t.Errorf("after a cycle, Canonical(1, [ok]) = (%v, %v), want 6 rad", got, err)
	}
}

// El error de un ciclo nombra una unidad del ciclo, en cualquier orden de
// llamadas. Sin caché, la profundidad se agotaba al bajar por un subárbol sano
// compartido ([x] → [y] → [z]) y el error nombraba [z], que no es parte de
// ningún ciclo; con la guarda antes del caché nombra [x], que tampoco lo es. El
// tercer orden calienta primero el subárbol sano.
func TestDagCycleErrorNamesAUnitOfTheCycle(t *testing.T) {
	unit := func(code, of string) string {
		return fmt.Sprintf(` <unit Code=%q CODE=%q isMetric="no" class="dimless">
  <name>cycle</name><printSymbol>c</printSymbol><property>number</property>
  <value Unit=%q UNIT=%q value="1">c</value>
 </unit>
`, code, strings.ToUpper(code), of, strings.ToUpper(of))
	}
	extra := unit("[a]", "[x].[a]") + unit("[x]", "[y]") + unit("[y]", "[z]") + unit("[z]", "rad") +
		unit("[b]", "[x]/[a]") + unit("[c]", "[x].[x].[c]")
	xml := tanCustomXML("[pi]", "3.141592653589793", false, extra)
	// Cada código y la unidad del ciclo que su error debe nombrar.
	want := map[string]string{"[a]": "[a]", "[b]": "[a]", "[c]": "[c]"}
	orders := [][]string{
		{"[a]", "[b]", "[c]"},
		{"[c]", "[b]", "[a]"},
		{"[x]", "[c]", "[a]", "[b]"}, // el subárbol sano ya en el caché
	}
	for _, order := range orders {
		svc := dagService(t, xml)
		for _, code := range order {
			_, err := svc.Canonical(1, code)
			name, inCycle := want[code]
			if !inCycle {
				if err != nil {
					t.Errorf("order %v: Canonical(1, %q) = %v, want a value", order, code, err)
				}
				continue
			}
			msg := fmt.Sprintf("unit %q is part of or leads to a definition cycle", name)
			if err == nil || !strings.Contains(err.Error(), msg) {
				t.Errorf("order %v: Canonical(1, %q) error = %v, want it to contain %q", order, code, err, msg)
			}
		}
	}
}
