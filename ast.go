package ucum

// operator represents a binary operator in a UCUM expression.
type operator int

const (
	opMultiplication operator = iota
	opDivision
)

func (o operator) String() string {
	if o == opDivision {
		return "/"
	}
	return "."
}

// component is the interface for AST nodes.
type component interface {
	isComponent()
}

// term represents a binary operation: comp op term.
type term struct {
	comp component
	op   operator
	term *term
}

func (term) isComponent() {}

// symbol represents a unit reference with optional prefix and exponent.
type symbol struct {
	unit     *Unit
	prefix   *Prefix
	exponent int
}

func (symbol) isComponent() {}

// factor represents a numeric literal.
type factor struct {
	value int
}

func (factor) isComponent() {}
