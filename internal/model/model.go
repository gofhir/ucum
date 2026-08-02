// Package model holds the UCUM definitions in memory: the prefixes, base units
// and defined units a service resolves codes against, with the indexes that make
// resolution O(1).
//
// It is a leaf package. It knows what a unit is, and nothing about parsing,
// canonicalizing or converting one.
package model

import (
	"strings"

	"github.com/gofhir/ucum/v4/internal/decimal"
)

// Model holds the complete set of UCUM definitions.
type Model struct {
	Version      string
	Revision     string
	RevisionDate string
	Prefixes     []*Prefix
	BaseUnits    []*BaseUnit
	DefinedUnits []*DefinedUnit

	// O(1) lookup indexes (built after loading). UCUM defines two parallel
	// vocabularies and calls them incompatible, so each gets its own index and
	// they are never consulted together. The case-insensitive keys are upper
	// cased, since case carries no meaning in that variant.
	prefixByCode map[string]*Prefix
	unitByCode   map[string]*Unit

	// Only units get a case-insensitive index. Prefixes are matched by iterating
	// longest-code-first, which is what longest-match resolution needs, so an
	// index would never be consulted.
	unitByCodeCI map[string]*Unit
}

// Unit is the common representation for base and defined units.
type Unit struct {
	Code        string
	CodeCI      string // the case-insensitive spelling UCUM defines for this atom
	Name        string
	Property    string
	IsMetric    bool
	IsSpecial   bool
	IsBase      bool
	IsArbitrary bool
	Dim         string // dimension symbol, base units only
	Value       *Conversion
	Class       string
}

// Conversion holds the conversion definition for a defined unit.
type Conversion struct {
	Unit string // UCUM expression
	Text string

	Value decimal.Decimal // numeric multiplier, relative to unit

	// Function is set for special units, whose value is a conversion function
	// rather than a plain multiplier.
	Function *Function
}

// Function is the conversion function a special unit performs, as the
// definitions declare it.
//
// Name selects the behavior — Cel, degF, lg, lgTimes2, tanTimes100, sqrt and so
// on — while Value and unit give the reference quantity it is measured against.
// For example, degF is declared as degf(5 K/9), so Name is "degF", Value is 5 and
// unit is "K/9".
type Function struct {
	Name  string
	Value decimal.Decimal
	Unit  string
}

// Reference returns the reference quantity as a UCUM expression, combining the
// multiplier with the unit: "5.K/9" for degF, "2.10*-5.Pa" for B[SPL]. This is
// what a special unit's value scales by.
func (f *Function) Reference() string {
	if f == nil {
		return ""
	}
	return f.Value.String() + "." + f.Unit
}

// Prefix represents an SI prefix (kilo, milli, etc.).
type Prefix struct {
	Code   string
	CodeCI string
	Name   string

	Value decimal.Decimal
}

// BaseUnit represents one of the 7 fundamental SI base units.
type BaseUnit struct {
	Code     string
	CodeCI   string
	Name     string
	Property string
	Dim      string // single character dimension symbol
}

// DefinedUnit represents a non-base UCUM unit.
type DefinedUnit struct {
	Code        string
	CodeCI      string
	Name        string
	Property    string
	IsMetric    bool
	IsSpecial   bool
	IsArbitrary bool
	Class       string
	Value       *Conversion
}

// Lookup looks up a unit by code (searches base and defined).
func (m *Model) Lookup(code string) *Unit {
	return m.unitByCode[code]
}

// LookupPrefix looks up a prefix by code.
func (m *Model) LookupPrefix(code string) *Prefix {
	return m.prefixByCode[code]
}

// LookupCI looks up a unit by its case-insensitive code, in which case carries
// no meaning.
func (m *Model) LookupCI(code string) *Unit {
	return m.unitByCodeCI[strings.ToUpper(code)]
}

// BuildIndexes populates the lookup maps from the loaded lists.
func (m *Model) BuildIndexes() {
	m.prefixByCode = make(map[string]*Prefix, len(m.Prefixes))
	for _, p := range m.Prefixes {
		m.prefixByCode[p.Code] = p
	}

	m.unitByCode = make(map[string]*Unit, len(m.BaseUnits)+len(m.DefinedUnits))
	m.unitByCodeCI = make(map[string]*Unit, len(m.BaseUnits)+len(m.DefinedUnits))
	for _, bu := range m.BaseUnits {
		u := &Unit{
			Code: bu.Code, CodeCI: bu.CodeCI, Name: bu.Name, Property: bu.Property,
			IsBase: true, Dim: bu.Dim,
		}
		m.unitByCode[bu.Code] = u
		addCI(m.unitByCodeCI, bu.CodeCI, u)
	}
	for _, du := range m.DefinedUnits {
		u := &Unit{
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
