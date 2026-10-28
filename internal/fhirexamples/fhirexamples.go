// Package fhirexamples reads the UCUM quantities, and the codes of a
// ValueSet, out of HL7's official FHIR example packages. Only tests use it:
// fhir/examples_test.go checks them, and the case generator of
// docs/plans/002-compare-java.md sends them to Ucum-java.
//
// The packages are downloaded by scripts/fetch-packages.sh, which checks their
// SHA-256 against scripts/packages.sha256.
package fhirexamples

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// System is the FHIR system URI of UCUM. It is fhir.UCUMSystem, repeated here
// because importing fhir from internal/engine's tests would be a cycle;
// fhir/examples_test.go checks that the two are equal.
const System = "http://unitsofmeasure.org"

// Packages are the example packages, by FHIR version.
var Packages = []struct{ Version, File string }{
	{"R4", "hl7.fhir.r4.examples-4.0.1.tgz"},
	{"R4B", "hl7.fhir.r4b.examples-4.3.0.tgz"},
	{"R5", "hl7.fhir.r5.examples-5.0.0.tgz"},
}

// Quantity is a value and a code exactly as they appear in the JSON: the value
// is the text of the number, which is the exact decimal.
type Quantity struct{ Value, Code string }

// Read returns the distinct quantities with system UCUM in every JSON resource
// of the package at path, sorted by code and then value, and the number of JSON
// files that did not decode. A caller should treat that number as an error:
// there are none in R4, R4B or R5 today, and a new one would silently remove
// quantities from the corpus.
func Read(path string) (qs []Quantity, undecodable int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, 0, err
	}
	seen := map[Quantity]bool{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		base := filepath.Base(h.Name)
		if !strings.HasSuffix(base, ".json") || base == "package.json" || strings.HasPrefix(base, ".") {
			continue
		}
		dec := json.NewDecoder(tr)
		dec.UseNumber()
		var doc any
		if err := dec.Decode(&doc); err != nil {
			undecodable++
			continue
		}
		walk(doc, seen)
	}
	qs = make([]Quantity, 0, len(seen))
	for q := range seen {
		qs = append(qs, q)
	}
	sort.Slice(qs, func(i, j int) bool {
		if qs[i].Code != qs[j].Code {
			return qs[i].Code < qs[j].Code
		}
		return qs[i].Value < qs[j].Value
	})
	return qs, undecodable, nil
}

func walk(x any, seen map[Quantity]bool) {
	switch v := x.(type) {
	case map[string]any:
		if v["system"] == System {
			num, okNum := v["value"].(json.Number)
			code, okCode := v["code"].(string)
			if okNum && okCode {
				seen[Quantity{num.String(), code}] = true
			}
		}
		for _, e := range v {
			walk(e, seen)
		}
	case []any:
		for _, e := range v {
			walk(e, seen)
		}
	}
}

// ValueSetCodes returns the codes the ValueSet resource in file (for instance
// "ValueSet-ucum-common.json") lists in compose.include.concept, in the order
// the resource gives them, repetitions included. It reads the package at path
// and fails if the file is not in it.
func ValueSetCodes(path, file string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: no %s", filepath.Base(path), file)
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) != file {
			continue
		}
		var vs struct {
			Compose struct {
				Include []struct {
					Concept []struct {
						Code string `json:"code"`
					} `json:"concept"`
				} `json:"include"`
			} `json:"compose"`
		}
		if err := json.NewDecoder(tr).Decode(&vs); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		var codes []string
		for _, inc := range vs.Compose.Include {
			for _, c := range inc.Concept {
				codes = append(codes, c.Code)
			}
		}
		return codes, nil
	}
}
