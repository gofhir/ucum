package ucum

// canonical represents a value expressed in canonical (base) units.
type canonical struct {
	value decimal
	units []canonicalUnit
}

// canonicalUnit pairs a base unit with its exponent in a canonical form.
type canonicalUnit struct {
	base     *BaseUnit
	exponent int
}
