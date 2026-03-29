package ucum

import "testing"

func TestLoadEmbeddedDefinitions(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatalf("loadDefinitions: %v", err)
	}

	// Verify version
	if model.Version != "2.2" {
		t.Errorf("version = %q, want %q", model.Version, "2.2")
	}

	// Verify counts
	if len(model.Prefixes) < 20 {
		t.Errorf("prefixes = %d, want >= 20", len(model.Prefixes))
	}
	if len(model.BaseUnits) != 7 {
		t.Errorf("base units = %d, want 7", len(model.BaseUnits))
	}
	if len(model.DefinedUnits) < 200 {
		t.Errorf("defined units = %d, want >= 200", len(model.DefinedUnits))
	}

	// Check prefix lookup: kilo should have value 1e3
	kilo := model.getPrefix("k")
	if kilo == nil {
		t.Fatal("prefix k not found")
	}
	if kilo.Name != "kilo" {
		t.Errorf("prefix k name = %q, want %q", kilo.Name, "kilo")
	}
	if kilo.Value.float64() != 1e3 {
		t.Errorf("prefix k value = %v, want 1000", kilo.Value.float64())
	}

	// Check base unit lookup: meter
	meter := model.getUnit("m")
	if meter == nil {
		t.Fatal("unit m not found")
	}
	if !meter.IsBase {
		t.Error("unit m should be a base unit")
	}
	if meter.Name != "meter" {
		t.Errorf("unit m name = %q, want %q", meter.Name, "meter")
	}

	// Check defined unit lookup: inch
	inch := model.getUnit("[in_i]")
	if inch == nil {
		t.Fatal("unit [in_i] not found")
	}
	if inch.IsBase {
		t.Error("unit [in_i] should not be a base unit")
	}
	if inch.Name != "inch" {
		t.Errorf("unit [in_i] name = %q, want %q", inch.Name, "inch")
	}

	// Check special unit: Celsius
	cel := model.getUnit("Cel")
	if cel == nil {
		t.Fatal("unit Cel not found")
	}
	if !cel.IsSpecial {
		t.Error("unit Cel should be marked as special")
	}
}
