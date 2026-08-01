package ucum

import "math/big"

// Model holds the complete set of UCUM definitions.
type Model struct {
	Version      string
	Revision     string
	RevisionDate string
	Prefixes     []*Prefix
	BaseUnits    []*BaseUnit
	DefinedUnits []*DefinedUnit

	// O(1) lookup indexes (built after loading)
	prefixByCode map[string]*Prefix
	unitByCode   map[string]*Unit
}

// Unit is the common representation for base and defined units.
type Unit struct {
	Code        string
	Name        string
	Property    string
	IsMetric    bool
	IsSpecial   bool
	IsBase      bool
	IsArbitrary bool
	Dim         string // dimension symbol, base units only
	Value       *UnitValue
	Class       string
}

// UnitValue holds the conversion definition for a defined unit.
type UnitValue struct {
	Unit string // UCUM expression
	Text string

	// Deprecated: Value is typed with an unexported type, so a caller outside
	// the package can read it but do almost nothing with it — its only exported
	// method, String, formats to ten decimal places and loses precision. Use
	// Rat, which returns the same number exactly. Value will be unexported in
	// the next major version.
	Value decimal // numeric multiplier
}

// Rat returns the numeric multiplier as an exact rational, or nil if the
// UnitValue is nil.
//
// This is the definition as UCUM states it, with no rounding, and it is relative
// to UnitValue.Unit rather than to a base unit: [in_i] is defined as 254e-2 cm,
// so Rat returns 127/50 and Unit returns "cm". For converting values rather than
// inspecting definitions, use ExactService, which resolves the whole chain.
func (v *UnitValue) Rat() *big.Rat {
	if v == nil {
		return nil
	}
	return v.Value.rat()
}

// Prefix represents an SI prefix (kilo, milli, etc.).
type Prefix struct {
	Code string
	Name string

	// Deprecated: Value is typed with an unexported type, so a caller outside
	// the package can read it but do almost nothing with it — its only exported
	// method, String, formats to ten decimal places, rendering milli as
	// "0.0010000000". Use Rat, which returns the same number exactly. Value will
	// be unexported in the next major version.
	Value decimal
}

// Rat returns the prefix multiplier as an exact rational, or nil if the Prefix
// is nil. Milli is 1/1000 exactly.
func (p *Prefix) Rat() *big.Rat {
	if p == nil {
		return nil
	}
	return p.Value.rat()
}

// BaseUnit represents one of the 7 fundamental SI base units.
type BaseUnit struct {
	Code     string
	Name     string
	Property string
	Dim      string // single character dimension symbol
}

// DefinedUnit represents a non-base UCUM unit.
type DefinedUnit struct {
	Code        string
	Name        string
	Property    string
	IsMetric    bool
	IsSpecial   bool
	IsArbitrary bool
	Class       string
	Value       *UnitValue
}

// getUnit looks up a unit by code (searches base and defined).
func (m *Model) getUnit(code string) *Unit {
	return m.unitByCode[code]
}

// getPrefix looks up a prefix by code.
func (m *Model) getPrefix(code string) *Prefix {
	return m.prefixByCode[code]
}

// buildIndexes populates the lookup maps from the loaded lists.
func (m *Model) buildIndexes() {
	m.prefixByCode = make(map[string]*Prefix, len(m.Prefixes))
	for _, p := range m.Prefixes {
		m.prefixByCode[p.Code] = p
	}

	m.unitByCode = make(map[string]*Unit, len(m.BaseUnits)+len(m.DefinedUnits))
	for _, bu := range m.BaseUnits {
		m.unitByCode[bu.Code] = &Unit{
			Code: bu.Code, Name: bu.Name, Property: bu.Property,
			IsBase: true, Dim: bu.Dim,
		}
	}
	for _, du := range m.DefinedUnits {
		m.unitByCode[du.Code] = &Unit{
			Code: du.Code, Name: du.Name, Property: du.Property,
			IsMetric: du.IsMetric, IsSpecial: du.IsSpecial,
			IsArbitrary: du.IsArbitrary, Class: du.Class,
			Value: du.Value,
		}
	}
}
