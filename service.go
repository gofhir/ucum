package ucum

import "io"

type service struct{}

func newService(_ io.Reader) (*service, error) {
	return &service{}, nil
}

func (s *service) Validate(code string) error                         { return nil }
func (s *service) ValidateInProperty(code, property string) error     { return nil }
func (s *service) Canonical(value float64, code string) (Pair, error) { return Pair{}, nil }
func (s *service) Convert(value float64, from, to string) (float64, error) {
	return 0, nil
}
func (s *service) IsComparable(code1, code2 string) (bool, error) { return false, nil }
func (s *service) Analyse(code string) (string, error)            { return "", nil }
func (s *service) Multiply(v1, v2 Pair) (Pair, error)             { return Pair{}, nil }
