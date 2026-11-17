package essence

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/model"
)

// wrapXML envuelve un fragmento en un <root> mínimo.
func wrapXML(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><root version="2.2" revision="1" revision-date="2026-01-01">` + body + `</root>`
}

type matrixCase struct {
	name string
	kind string // prefix o unit
	body string
}

// matrixCases cruza, para prefijo, unidad y función, el atributo value vacío,
// omitido y el elemento <value> omitido, con y sin Unit (plan 023, §5.2).
func matrixCases() []matrixCase {
	var cs []matrixCase
	prefix := func(name, value string) {
		cs = append(cs, matrixCase{"prefix/" + name, "prefix", `<prefix Code="k" CODE="K"><name>kilo</name>` + value + `</prefix>`})
	}
	prefix("value-empty", `<value value="">1</value>`)
	prefix("value-omitted", `<value>1</value>`)
	prefix("value-element-omitted", ``)
	prefix("value-empty-with-unit", `<value Unit="1" value="">1</value>`)
	prefix("value-omitted-with-unit", `<value Unit="1">1</value>`)

	unit := func(name, value string) {
		cs = append(cs, matrixCase{"unit/" + name, "unit", `<unit Code="[x]" CODE="[X]"><name>x</name>` + value + `</unit>`})
	}
	unit("value-empty", `<value value="">1</value>`)
	unit("value-omitted", `<value>1</value>`)
	unit("value-element-omitted", ``)
	unit("value-empty-with-unit", `<value Unit="m" value="">1</value>`)
	unit("value-omitted-with-unit", `<value Unit="m">1</value>`)

	fn := func(name, value string) {
		cs = append(cs, matrixCase{"function/" + name, "unit", `<unit Code="[x]" CODE="[X]"><name>x</name>` + value + `</unit>`})
	}
	fn("value-empty", `<value Unit="m"><function name="f" value="" Unit="K"/></value>`)
	fn("value-omitted", `<value Unit="m"><function name="f" Unit="K"/></value>`)
	fn("value-empty-without-unit", `<value Unit="m"><function name="f" value=""/></value>`)
	fn("value-omitted-without-unit", `<value Unit="m"><function name="f"/></value>`)
	fn("element-omitted", `<value Unit="m"></value>`)
	fn("value-empty-in-bare-value", `<value><function name="f" value="" Unit="K"/></value>`)
	fn("value-omitted-in-bare-value", `<value><function name="f" Unit="K"/></value>`)
	fn("non-special-function-in-bare-value", `<value><function name="Cel" value="1" Unit="m"/></value>`)
	fn("value-empty-without-unit-in-bare-value", `<value><function name="f" value=""/></value>`)
	return cs
}

// describe resume el resultado de una carga como texto estable.
func describe(m *model.Model, err error, kind string) string {
	if err != nil {
		return "error: " + err.Error()
	}
	if kind == "prefix" {
		return "prefix value=" + m.Prefixes[0].Value.Rat().String()
	}
	u := m.DefinedUnits[0]
	if u.Value == nil {
		return "unit value=nil"
	}
	s := fmt.Sprintf("unit value=%s unit=%q", u.Value.Value.Rat().String(), u.Value.Unit)
	if f := u.Value.Function; f != nil {
		s += fmt.Sprintf(" function name=%q value=%s unit=%q", f.Name, f.Value.Rat().String(), f.Unit)
	} else {
		s += " function=nil"
	}
	return s
}

// matrixWant son los resultados de la matriz. Los de vacío y omitido son los
// de main antes del plan 023, salvo los tres con <function> directamente en un
// <value> sin atributos: hoy el gate value != "" || Unit != "" los descarta
// (unit value=nil), y desde 3.4 la función se lee siempre que el elemento
// exista.
var matrixWant = map[string]string{
	"prefix/value-empty":                              "prefix value=1/1",
	"prefix/value-omitted":                            "prefix value=1/1",
	"prefix/value-element-omitted":                    "prefix value=1/1",
	"prefix/value-empty-with-unit":                    "prefix value=1/1",
	"prefix/value-omitted-with-unit":                  "prefix value=1/1",
	"unit/value-empty":                                "unit value=nil",
	"unit/value-omitted":                              "unit value=nil",
	"unit/value-element-omitted":                      "unit value=nil",
	"unit/value-empty-with-unit":                      `unit value=1/1 unit="m" function=nil`,
	"unit/value-omitted-with-unit":                    `unit value=1/1 unit="m" function=nil`,
	"function/value-empty":                            `unit value=1/1 unit="m" function name="f" value=1/1 unit="K"`,
	"function/value-omitted":                          `unit value=1/1 unit="m" function name="f" value=1/1 unit="K"`,
	"function/value-empty-without-unit":               `unit value=1/1 unit="m" function name="f" value=1/1 unit=""`,
	"function/value-omitted-without-unit":             `unit value=1/1 unit="m" function name="f" value=1/1 unit=""`,
	"function/element-omitted":                        `unit value=1/1 unit="m" function=nil`,
	"function/value-empty-in-bare-value":              "unit value=nil",
	"function/value-omitted-in-bare-value":            "unit value=nil",
	"function/non-special-function-in-bare-value":     "unit value=nil",
	"function/value-empty-without-unit-in-bare-value": "unit value=nil",
}

func TestLoadEmptyAndOmittedValues(t *testing.T) {
	cases := matrixCases()
	if len(cases) != len(matrixWant) {
		t.Fatalf("%d cases, %d expectations", len(cases), len(matrixWant))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Load(strings.NewReader(wrapXML(c.body)))
			if got := describe(m, err, c.kind); got != matrixWant[c.name] {
				t.Errorf("got  %s\nwant %s", got, matrixWant[c.name])
			}
		})
	}
}

func TestLoadInvalidValueMessages(t *testing.T) {
	unit := func(inner string) string {
		return `<unit Code="[x]" CODE="[X]"><name>x</name>` + inner + `</unit>`
	}
	cases := []struct {
		name, body, want string
	}{
		{"prefix", `<prefix Code="k" CODE="K"><name>kilo</name><value value="abc">1</value></prefix>`,
			`prefix k value: invalid decimal "abc"`},
		{"prefix-padded", `<prefix Code="k" CODE="K"><name>kilo</name><value value=" 7 ">1</value></prefix>`,
			`prefix k value: invalid decimal " 7 "`},
		{"unit", unit(`<value Unit="m" value="abc">1</value>`),
			`unit [x] value: invalid decimal "abc"`},
		{"unit-without-unit", unit(`<value value="1e">1</value>`),
			`unit [x] value: invalid decimal "1e"`},
		{"unit-plus-exponent", unit(`<value Unit="m" value="1e+3">1</value>`),
			`unit [x] value: invalid decimal "1e+3"`},
		{"unit-exponent-bound", unit(`<value Unit="m" value="1e10000">1</value>`),
			`unit [x] value: exponent of "1e10000" is beyond ±9999`},
		{"function", unit(`<value Unit="m"><function name="f" value="abc" Unit="K"/></value>`),
			`unit [x] function value "abc": invalid decimal "abc"`},
		{"function-in-bare-value", unit(`<value><function name="f" value="abc"/></value>`),
			`unit [x] function value "abc": invalid decimal "abc"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(wrapXML(c.body)))
			if err == nil || err.Error() != c.want {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}
