package fhir

import (
	"bufio"
	"embed"
	"sort"
	"strings"
	"sync"
)

//go:embed ucum-common.tsv
var valueSetFiles embed.FS

// CommonUnitsValueSet identifies the value set this package carries.
const (
	CommonUnitsValueSet        = "http://hl7.org/fhir/ValueSet/ucum-common"
	CommonUnitsValueSetVersion = "5.0.0"

	// CommonUnitsCodeCount is the number of distinct codes in the value set.
	// The published resource lists 848 concepts but only 840 distinct codes:
	// eight are repeated. See loadCommon.
	CommonUnitsCodeCount = 840

	// UCUMSystem is the code system URI FHIR uses for UCUM codes, the value a
	// Quantity.system must carry for its code to be a UCUM code.
	UCUMSystem = "http://unitsofmeasure.org"
)

var (
	commonOnce sync.Once
	commonMap  map[string]string
)

// loadCommon parses the embedded value set once. The file ships with the
// package, so a parse failure would be a build-time mistake, not a runtime
// condition: malformed lines are skipped rather than reported, and the
// accompanying test asserts the expected concept count.
func loadCommon() {
	commonMap = make(map[string]string, 900)
	f, err := valueSetFiles.Open("ucum-common.tsv")
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		code, display, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		// The published value set repeats eight codes, sometimes with different
		// display strings ("/[HPF]" appears as both "per high power field" and
		// "per hpf"). Keep the first, which is the more descriptive one in every
		// case, and treat the rest as the duplicates they are.
		if _, seen := commonMap[code]; !seen {
			commonMap[code] = display
		}
	}
}

// InCommonUnits reports whether code is a member of the FHIR ucum-common value
// set.
//
// The value set is marked extensible in FHIR, so a code outside it is not
// thereby invalid — it may still be perfectly good UCUM. Use it to decide
// whether a code is one FHIR expects to see commonly, not to reject input. To
// check validity, use the parent package's Validate.
func InCommonUnits(code string) bool {
	commonOnce.Do(loadCommon)
	_, ok := commonMap[code]
	return ok
}

// CommonDisplay returns the display string FHIR publishes for a code in the
// ucum-common value set, and whether the code is a member.
//
// These are FHIR's own display strings, which differ from the unit names UCUM
// declares and from the output of the parent package's Analyze: FHIR gives
// "10*3/uL" the display "thousand per microliter", while Analyze renders it
// structurally.
func CommonDisplay(code string) (string, bool) {
	commonOnce.Do(loadCommon)
	display, ok := commonMap[code]
	return display, ok
}

// CommonCodes returns every code in the value set, sorted, as a fresh slice.
func CommonCodes() []string {
	commonOnce.Do(loadCommon)
	codes := make([]string, 0, len(commonMap))
	for code := range commonMap {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}
