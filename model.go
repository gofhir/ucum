package ucum

import "strings"

// ucumModel holds the complete set of UCUM definitions.
type ucumModel struct {
	Version      string
	Revision     string
	RevisionDate string
	Prefixes     []*prefixDef
	BaseUnits    []*baseUnit
	DefinedUnits []*definedUnit

	// O(1) lookup indexes (built after loading). UCUM defines two parallel
	// vocabularies and calls them incompatible, so each gets its own index and
	// they are never consulted together. The case-insensitive keys are upper
	// cased, since case carries no meaning in that variant.
	prefixByCode map[string]*prefixDef
	unitByCode   map[string]*unitDef

	// Only units get a case-insensitive index. Prefixes are matched by iterating
	// longest-code-first, which is what longest-match resolution needs, so an
	// index would never be consulted.
	unitByCodeCI map[string]*unitDef
}

// unitDef is the common representation for base and defined units.
type unitDef struct {
	Code        string
	CodeCI      string // the case-insensitive spelling UCUM defines for this atom
	Name        string
	Property    string
	IsMetric    bool
	IsSpecial   bool
	IsBase      bool
	IsArbitrary bool
	Dim         string // dimension symbol, base units only
	Value       *unitConversion
	Class       string
}

// unitConversion holds the conversion definition for a defined unit.
type unitConversion struct {
	unit string // UCUM expression
	Text string

	Value decimal // numeric multiplier, relative to unit

	// Function is set for special units, whose value is a conversion function
	// rather than a plain multiplier.
	Function *functionDef
}

// functionDef is the conversion function a special unit performs, as the
// definitions declare it.
//
// Name selects the behavior — Cel, degF, lg, lgTimes2, tanTimes100, sqrt and so
// on — while Value and unit give the reference quantity it is measured against.
// For example, degF is declared as degf(5 K/9), so Name is "degF", Value is 5 and
// unit is "K/9".
type functionDef struct {
	Name  string
	Value decimal
	unit  string
}

// Reference returns the reference quantity as a UCUM expression, combining the
// multiplier with the unit: "5.K/9" for degF, "2.10*-5.Pa" for B[SPL]. This is
// what a special unit's value scales by.
func (f *functionDef) Reference() string {
	if f == nil {
		return ""
	}
	return f.Value.String() + "." + f.unit
}

// prefixDef represents an SI prefix (kilo, milli, etc.).
type prefixDef struct {
	Code   string
	CodeCI string
	Name   string

	Value decimal
}

// baseUnit represents one of the 7 fundamental SI base units.
type baseUnit struct {
	Code     string
	CodeCI   string
	Name     string
	Property string
	Dim      string // single character dimension symbol
}

// definedUnit represents a non-base UCUM unit.
type definedUnit struct {
	Code        string
	CodeCI      string
	Name        string
	Property    string
	IsMetric    bool
	IsSpecial   bool
	IsArbitrary bool
	Class       string
	Value       *unitConversion
}

// getUnit looks up a unit by code (searches base and defined).
func (m *ucumModel) getUnit(code string) *unitDef {
	return m.unitByCode[code]
}

// getPrefix looks up a prefix by code.
func (m *ucumModel) getPrefix(code string) *prefixDef {
	return m.prefixByCode[code]
}

// getUnitCI looks up a unit by its case-insensitive code, in which case carries
// no meaning.
func (m *ucumModel) getUnitCI(code string) *unitDef {
	return m.unitByCodeCI[strings.ToUpper(code)]
}

// buildIndexes populates the lookup maps from the loaded lists.
func (m *ucumModel) buildIndexes() {
	m.prefixByCode = make(map[string]*prefixDef, len(m.Prefixes))
	for _, p := range m.Prefixes {
		m.prefixByCode[p.Code] = p
	}

	m.unitByCode = make(map[string]*unitDef, len(m.BaseUnits)+len(m.DefinedUnits))
	m.unitByCodeCI = make(map[string]*unitDef, len(m.BaseUnits)+len(m.DefinedUnits))
	for _, bu := range m.BaseUnits {
		u := &unitDef{
			Code: bu.Code, CodeCI: bu.CodeCI, Name: bu.Name, Property: bu.Property,
			IsBase: true, Dim: bu.Dim,
		}
		m.unitByCode[bu.Code] = u
		addCI(m.unitByCodeCI, bu.CodeCI, u)
	}
	for _, du := range m.DefinedUnits {
		u := &unitDef{
			Code: du.Code, CodeCI: du.CodeCI, Name: du.Name, Property: du.Property,
			IsMetric: du.IsMetric, IsSpecial: du.IsSpecial,
			IsArbitrary: du.IsArbitrary, Class: du.Class,
			Value: du.Value,
		}
		m.unitByCode[du.Code] = u
		addCI(m.unitByCodeCI, du.CodeCI, u)
	}
}

// addCI indexes an atom under its case-insensitive code, keeping the first entry
// when two case-sensitive atoms share one.
//
// That happens twice in UCUM 2.2: "l" and "L" both become "L", and "[iU]" and
// "[IU]" both become "[IU]". The first pair are synonyms, so nothing is lost. The
// second are arbitrary units and therefore not comparable with each other, so the
// case-insensitive vocabulary genuinely cannot tell them apart — a consequence of
// it being, as the specification puts it, the greatest common denominator.
func addCI[T any](index map[string]*T, code string, atom *T) {
	if code == "" {
		return
	}
	key := strings.ToUpper(code)
	if _, seen := index[key]; !seen {
		index[key] = atom
	}
}
