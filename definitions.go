package ucum

import (
	"embed"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

//go:embed ucum-essence.xml
var embeddedDefinitions embed.FS

// loadDefinitions parses ucum-essence.xml from the given reader, or from embedded if nil.
func loadDefinitions(r io.Reader) (*ucumModel, error) {
	if r == nil {
		f, err := embeddedDefinitions.Open("ucum-essence.xml")
		if err != nil {
			return nil, fmt.Errorf("open embedded definitions: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	return parseDefinitions(r)
}

// XML structures for unmarshaling ucum-essence.xml.

type xmlRoot struct {
	XMLName      xml.Name         `xml:"root"`
	Version      string           `xml:"version,attr"`
	Revision     string           `xml:"revision,attr"`
	RevisionDate string           `xml:"revision-date,attr"`
	Prefixes     []xmlPrefix      `xml:"prefix"`
	BaseUnits    []xmlBaseUnit    `xml:"base-unit"`
	Units        []xmlDefinedUnit `xml:"unit"`
}

type xmlPrefix struct {
	Code        string   `xml:"Code,attr"`
	CodeUC      string   `xml:"CODE,attr"`
	Name        string   `xml:"name"`
	PrintSymbol string   `xml:"printSymbol"`
	Value       xmlValue `xml:"value"`
}

type xmlBaseUnit struct {
	Code        string `xml:"Code,attr"`
	CodeUC      string `xml:"CODE,attr"`
	Dim         string `xml:"dim,attr"`
	Name        string `xml:"name"`
	PrintSymbol string `xml:"printSymbol"`
	Property    string `xml:"property"`
}

type xmlDefinedUnit struct {
	Code        string   `xml:"Code,attr"`
	CodeUC      string   `xml:"CODE,attr"`
	IsMetric    string   `xml:"isMetric,attr"`
	IsSpecial   string   `xml:"isSpecial,attr"`
	IsArbitrary string   `xml:"isArbitrary,attr"`
	Class       string   `xml:"class,attr"`
	Name        string   `xml:"name"`
	PrintSymbol string   `xml:"printSymbol"`
	Property    string   `xml:"property"`
	Value       xmlValue `xml:"value"`
}

type xmlValue struct {
	Unit     string       `xml:"Unit,attr"`
	UNIT     string       `xml:"UNIT,attr"`
	Value    string       `xml:"value,attr"`
	Text     string       `xml:",chardata"`
	Function *xmlFunction `xml:"function"`
}

// xmlFunction is the <function> element a special unit carries in place of a
// plain numeric value:
//
//	<value Unit="degf(5 K/9)"><function name="degF" value="5" Unit="K/9"/></value>
//
// Name selects the conversion the unit performs; Value and Unit give its
// reference quantity — 5 K/9 for degF, 2 10*-5.Pa for B[SPL], 10 nV for
// B[10.nV]. Reading them keeps those numbers in the definitions rather than
// duplicated in code.
type xmlFunction struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
	Unit  string `xml:"Unit,attr"`
}

const xmlYes = "yes"

func parseDefinitions(r io.Reader) (*ucumModel, error) {
	var root xmlRoot
	dec := xml.NewDecoder(r)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "ascii") || strings.EqualFold(charset, "us-ascii") {
			return input, nil
		}
		return nil, fmt.Errorf("unsupported charset: %s", charset)
	}
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode ucum-essence.xml: %w", err)
	}

	model := &ucumModel{
		Version:      root.Version,
		Revision:     root.Revision,
		RevisionDate: root.RevisionDate,
	}

	// Parse prefixes
	for _, xp := range root.Prefixes {
		val, err := decimalFromString(xp.Value.Value)
		if err != nil {
			return nil, fmt.Errorf("prefix %s value: %w", xp.Code, err)
		}
		model.Prefixes = append(model.Prefixes, &prefixDef{
			Code: xp.Code, CodeCI: xp.CodeUC, Name: xp.Name, Value: val,
		})
	}

	// Parse base units
	for _, xb := range root.BaseUnits {
		model.BaseUnits = append(model.BaseUnits, &baseUnit{
			Code: xb.Code, CodeCI: xb.CodeUC, Name: xb.Name,
			Property: xb.Property, Dim: xb.Dim,
		})
	}

	// Parse defined units
	for _, xu := range root.Units {
		var unitVal *unitConversion
		if xu.Value.Value != "" || xu.Value.Unit != "" {
			v, err := decimalFromString(xu.Value.Value)
			if err != nil {
				// Some special units have empty value; default to 1
				v = decimalFromInt(1)
			}
			unitVal = &unitConversion{unit: xu.Value.Unit, Text: xu.Value.Text, Value: v}

			if xf := xu.Value.Function; xf != nil {
				fv, err := decimalFromString(xf.Value)
				if err != nil {
					return nil, fmt.Errorf("unit %s function value %q: %w", xu.Code, xf.Value, err)
				}
				unitVal.Function = &functionDef{Name: xf.Name, Value: fv, unit: xf.Unit}
			}
		}
		model.DefinedUnits = append(model.DefinedUnits, &definedUnit{
			Code: xu.Code, CodeCI: xu.CodeUC, Name: xu.Name, Property: xu.Property,
			IsMetric: xu.IsMetric == xmlYes, IsSpecial: xu.IsSpecial == xmlYes,
			IsArbitrary: xu.IsArbitrary == xmlYes, Class: xu.Class,
			Value: unitVal,
		})
	}

	model.buildIndexes()
	return model, nil
}
