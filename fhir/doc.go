// Package fhir carries the FHIR-specific rules that a UCUM engine cannot infer
// on its own.
//
// The parent package implements UCUM and nothing else, which is correct: UCUM is
// a standard in its own right and most of it has no FHIR opinion. But a consumer
// wiring UCUM into FHIR needs three things UCUM does not provide — the value-set
// subsets FHIR narrows to, the calendar-duration semantics FHIRPath layers on top
// of the time units, and canonical comparison of Quantity values. Those live
// here, so the engine stays free of them.
//
// Everything in this package is traceable to a published document, cited at the
// declaration it justifies. Where FHIR and UCUM disagree, the disagreement is
// documented rather than resolved in favor of either: see the note on
// CalendarEquivalentOf about the length of a month.
//
// # Sources
//
//   - FHIR R5, "Using FHIRPath with FHIR", the Quantity mapping table:
//     https://hl7.org/fhir/R5/fhirpath.html
//   - FHIRPath N1, "Time-valued Quantities" and "Date/Time Arithmetic":
//     https://hl7.org/fhirpath/N1/
//   - FHIR ValueSet ucum-common, version 5.0.0:
//     https://hl7.org/fhir/valueset-ucum-common.json
//
// # Regenerating the value set
//
// ucum-common.tsv is extracted from the published ValueSet resource. To refresh
// it against a newer FHIR release:
//
//	curl -sSL https://hl7.org/fhir/valueset-ucum-common.json -o vs.json
//	python3 - <<'EOF'
//	import json
//	d = json.load(open('vs.json'))
//	inc = d['compose']['include'][0]
//	cs = inc['concept']
//	hdr = [
//	    '# FHIR ucum-common value set, extracted from the published ValueSet resource.',
//	    '# source:  https://hl7.org/fhir/valueset-ucum-common.json',
//	    '# url:     %s' % d['url'],
//	    '# version: %s' % d['version'],
//	    '# system:  %s' % inc['system'],
//	    '# concepts: %d' % len(cs),
//	    '# format: <code>\t<display>. Regenerate with the command in doc.go.',
//	]
//	rows = ['%s\t%s' % (c['code'], c.get('display', '')) for c in cs]
//	open('fhir/ucum-common.tsv', 'w').write('\n'.join(hdr + rows) + '\n')
//	EOF
//
// TestCommonUnitsAreValidUCUM then checks every code in the refreshed file
// against the parent package, which is how a bad extraction shows up.
package fhir
