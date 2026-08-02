package ucum

import (
	"math"
	"testing"
)

// A special unit reads differently depending on whether it names a unit or a
// quantity, and the two APIs are asked different questions. This pins both, so
// the difference is a decision rather than an accident.
//
// Convert is given a *unit*: "Cel/min" is a compound unit, and a special unit in
// an algebraic term denotes a difference — a gradient of 1 Cel/min is a rate of
// change of one degree per minute, which is one kelvin per minute. The offset
// cancels.
//
// Multiply and Divide are given *quantities*: Pair{1, "Cel"} is a temperature,
// and a temperature is a point on its scale — 274.15 K. Dividing it by one
// minute is dividing that temperature by that time.
//
// Both readings are right for the question asked. UCUM §22.1-2 says special
// units "cannot take part in any algebraic operations" at all, so neither is
// mandated by the specification; what matters is that each is consistent.
func TestGradientReadingDependsOnTheQuestion(t *testing.T) {
	svc := newTestService(t)

	// As a unit: the offset cancels, so a degree per minute is a kelvin per
	// minute.
	got, err := svc.Convert(1, "Cel/min", "K/min")
	if err != nil {
		t.Fatalf("Convert(1, \"Cel/min\", \"K/min\"): %v", err)
	}
	if math.Abs(got-1) > 1e-12 {
		t.Errorf("Convert(1, \"Cel/min\", \"K/min\") = %v, want 1: a difference of one degree is one kelvin", got)
	}

	// The canonical form of the unit says the same thing.
	can, err := svc.Canonical(1, "Cel/min")
	if err != nil {
		t.Fatalf("Canonical(1, \"Cel/min\"): %v", err)
	}
	if want := 1.0 / 60.0; math.Abs(can.Value-want) > 1e-12 || can.Code != "K.s-1" {
		t.Errorf("Canonical(1, \"Cel/min\") = %v %s, want %v K.s-1", can.Value, can.Code, want)
	}

	// As a quantity: 1 Cel is a temperature, which is 274.15 K, and dividing it
	// by a minute divides that.
	q, err := svc.Divide(Pair{Value: 1, Code: "Cel"}, Pair{Value: 1, Code: "min"})
	if err != nil {
		t.Fatalf("Divide: %v", err)
	}
	if want := 274.15 / 60.0; math.Abs(q.Value-want) > 1e-9 || q.Code != "K.s-1" {
		t.Errorf("Divide(1 Cel, 1 min) = %v %s, want %v K.s-1: a temperature divided by a time",
			q.Value, q.Code, want)
	}

	// Multiplying two temperatures is the same reading: each operand is a point
	// on its scale.
	q, err = svc.Multiply(Pair{Value: 1, Code: "Cel"}, Pair{Value: 1, Code: "Cel"})
	if err != nil {
		t.Fatalf("Multiply: %v", err)
	}
	if want := 274.15 * 274.15; math.Abs(q.Value-want) > 1e-6 || q.Code != "K2" {
		t.Errorf("Multiply(1 Cel, 1 Cel) = %v %s, want %v K2", q.Value, q.Code, want)
	}

	// A caller who means the difference asks for the difference, by naming the
	// compound unit rather than combining two quantities.
	q, err = svc.Canonical(1, "Cel/min")
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if math.Abs(q.Value-1.0/60.0) > 1e-12 {
		t.Errorf("Canonical(1, \"Cel/min\") = %v, want %v", q.Value, 1.0/60.0)
	}
}

// TestNonLinearScalesRefuseTheAlgebraicReading checks the limit of the delta
// rule: it only makes sense where a difference has one. On a logarithmic scale
// 10^-pH does not decompose into a factor times a value, so there is no
// difference to speak of and the code is refused rather than converted wrongly.
func TestNonLinearScalesRefuseTheAlgebraicReading(t *testing.T) {
	svc := newTestService(t)

	for _, code := range []string{"[pH]/min", "B/s", "[p'diop].m"} {
		if _, err := svc.Canonical(1, code); err == nil {
			t.Errorf("Canonical(1, %q) = nil error, want a refusal: the scale is not linear", code)
		}
	}

	// The linear ones are allowed, since a difference on them is meaningful.
	for _, code := range []string{"Cel/min", "[degF]/h", "mCel/min"} {
		if _, err := svc.Canonical(1, code); err != nil {
			t.Errorf("Canonical(1, %q): %v", code, err)
		}
	}
}
