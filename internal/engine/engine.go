// Package engine canonicalizes and converts UCUM codes.
//
// It is where the arithmetic lives: expanding a unit through its definition,
// reducing an expression to base units, applying the conversion a special unit
// performs, and doing all of it in exact rational arithmetic so that a float64
// result is rounded once at the end.
//
// The root package wraps it as Service and ExactService and adds nothing but the
// interfaces and constructors.
package engine

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/gofhir/ucum/v4/internal/essence"
	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"
	"github.com/gofhir/ucum/v4/internal/special"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Service is the concrete implementation of Service.
type Service struct {
	model *model.Model

	// parser resolves codes supplied by a caller, in whichever of UCUM's two
	// vocabularies this service was built for.
	parser *expr.Parser
	cache  *termCache

	// defParser resolves the expressions inside the definitions, which are always
	// written in case-sensitive codes: the year is defined as "a_j", not as its
	// case-insensitive spelling "ANN_J". Expanding a definition therefore never
	// uses the caller's vocabulary.
	//
	// defCache is unbounded, unlike cache, and can be: its keys come from the
	// definitions, so there are as many as ucum-essence.xml has unit expressions
	// and no caller can add to them.
	defParser *expr.Parser
	defCache  sync.Map // map[string]*expr.Term

	// baseByCode indexes the seven base units, and arbitraryBases gives every
	// arbitrary unit its own dimension. Both are populated once at construction
	// and read-only afterwards.
	baseByCode     map[string]*model.BaseUnit
	arbitraryBases map[string]*model.BaseUnit

	// codesByProperty maps a lower-cased property name to the codes of the units
	// that declare it. Built at construction; the canonical forms behind them
	// are resolved lazily and memoized in propertyForms.
	codesByProperty map[string][]string
	propertyForms   sync.Map // map[string]map[string]bool

	// handlers holds one conversion per special unit, built from the function
	// each definition names. Read-only after construction.
	handlers map[string]special.Handler
}

// New creates a fully wired service.
//
// It reads the definitions from r, or the embedded ucum-essence.xml if r is nil,
// and resolves codes in one of UCUM's two vocabularies: the case-sensitive one,
// which is what FHIR uses, or the case-insensitive one.
func New(r io.Reader, insensitive bool) (*Service, error) {
	m, err := essence.Load(r)
	if err != nil {
		return nil, fmt.Errorf("ucum: load definitions: %w", err)
	}
	bases := make(map[string]*model.BaseUnit, len(m.BaseUnits))
	for _, bu := range m.BaseUnits {
		bases[bu.Code] = bu
	}

	// UCUM makes arbitrary units commensurable with nothing, so each one gets a
	// synthetic base unit standing for its own dimension.
	arbitrary := make(map[string]*model.BaseUnit)
	for _, du := range m.DefinedUnits {
		if du.IsArbitrary {
			arbitrary[du.Code] = &model.BaseUnit{
				Code:     du.Code,
				Name:     du.Name,
				Property: du.Property,
			}
		}
	}

	// Index the declared properties. Canonicalizing every unit here would slow
	// construction down for a feature most callers never use, so only the codes
	// are collected now.
	codesByProperty := make(map[string][]string)
	addProperty := func(prop, code string) {
		if prop == "" {
			return
		}
		key := strings.ToLower(prop)
		codesByProperty[key] = append(codesByProperty[key], code)
	}
	for _, bu := range m.BaseUnits {
		addProperty(bu.Property, bu.Code)
	}
	for _, du := range m.DefinedUnits {
		addProperty(du.Property, du.Code)
	}

	handlers, err := special.BuildHandlers(m)
	if err != nil {
		return nil, fmt.Errorf("ucum: %w", err)
	}

	return &Service{
		model:           m,
		parser:          expr.NewParserFor(m, insensitive),
		cache:           newTermCache(),
		defParser:       expr.NewParser(m),
		baseByCode:      bases,
		arbitraryBases:  arbitrary,
		codesByProperty: codesByProperty,
		handlers:        handlers,
	}, nil
}

// parseCached parses a UCUM code, caching the result.
func (s *Service) parseCached(code string) (*expr.Term, error) {
	if t, ok := s.cache.load(code); ok {
		return t, nil
	}
	t, err := s.parser.Parse(code)
	if err != nil {
		return nil, err
	}
	s.cache.store(code, t)
	return t, nil
}

// Definitions reports which UCUM release these definitions declare themselves
// to be.
func (s *Service) Definitions() Definitions {
	return Definitions{
		Version:      s.model.Version,
		Revision:     s.model.Revision,
		RevisionDate: s.model.RevisionDate,
	}
}

// parseDefinition parses an expression taken from the definitions, always in the
// case-sensitive vocabulary, and caches the result separately from caller input.
func (s *Service) parseDefinition(text string) (*expr.Term, error) {
	if v, ok := s.defCache.Load(text); ok {
		t, ok := v.(*expr.Term)
		if !ok {
			return nil, fmt.Errorf("ucum: unexpected cache entry type %T", v)
		}
		return t, nil
	}
	t, err := s.defParser.Parse(text)
	if err != nil {
		return nil, err
	}
	s.defCache.Store(text, t)
	return t, nil
}

// Validate checks if the given code is a valid UCUM expression.
func (s *Service) Validate(code string) error {
	_, err := s.parseCached(code)
	return ucumerr.Validation(code, err)
}

// specialContext says how a special unit in a term is to be read.
//
// Standalone it denotes a point on its own scale, and its handler applies the
// whole mapping including any offset: 1 Cel is 274.15 K. Inside an algebraic expr.Term
// it denotes a difference — a gradient of 1 [degF]/min is a rate of change, not a
// temperature — so the offset cancels while the scale, which the definitions
// give, still applies.
type specialContext int

const (
	specialStandalone specialContext = iota
	specialAsDelta
)

// Canonical arithmetic helpers.

// Human-readable analysis.

// unityDisplayName is what the official suite expects for an empty expression.
const unityDisplayName = "(unity)"

// Special unit detection.
