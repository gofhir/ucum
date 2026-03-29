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
func loadDefinitions(r io.Reader) (*UcumModel, error) {
	if r == nil {
		f, err := embeddedDefinitions.Open("ucum-essence.xml")
		if err != nil {
			return nil, fmt.Errorf("open embedded definitions: %w", err)
		}
		defer f.Close()
		r = f
	}
	return parseDefinitions(r)
}

// XML structures for unmarshaling ucum-essence.xml

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
	Unit  string `xml:"Unit,attr"`
	UNIT  string `xml:"UNIT,attr"`
	Value string `xml:"value,attr"`
	Text  string `xml:",chardata"`
}

func parseDefinitions(r io.Reader) (*UcumModel, error) {
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

	model := &UcumModel{
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
		model.Prefixes = append(model.Prefixes, &Prefix{
			Code: xp.Code, Name: xp.Name, Value: val,
		})
	}

	// Parse base units
	for _, xb := range root.BaseUnits {
		model.BaseUnits = append(model.BaseUnits, &BaseUnit{
			Code: xb.Code, Name: xb.Name, Property: xb.Property, Dim: xb.Dim,
		})
	}

	// Parse defined units
	for _, xu := range root.Units {
		var unitVal *UnitValue
		if xu.Value.Value != "" || xu.Value.Unit != "" {
			v, err := decimalFromString(xu.Value.Value)
			if err != nil {
				// Some special units have empty value; default to 1
				v = decimalFromInt(1)
			}
			unitVal = &UnitValue{Unit: xu.Value.Unit, Text: xu.Value.Text, Value: v}
		}
		model.DefinedUnits = append(model.DefinedUnits, &DefinedUnit{
			Code: xu.Code, Name: xu.Name, Property: xu.Property,
			IsMetric: xu.IsMetric == "yes", IsSpecial: xu.IsSpecial == "yes",
			IsArbitrary: xu.IsArbitrary == "yes", Class: xu.Class,
			Value: unitVal,
		})
	}

	model.buildIndexes()
	return model, nil
}
