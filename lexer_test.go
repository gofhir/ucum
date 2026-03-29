package ucum

import (
	"fmt"
	"testing"
)

type expectedToken struct {
	token string
	typ   tokenType
}

func collectTokens(source string) ([]expectedToken, error) {
	l, err := newLexer(source)
	if err != nil {
		return nil, err
	}

	var tokens []expectedToken
	for !l.finished() {
		tokens = append(tokens, expectedToken{
			token: l.getToken(),
			typ:   l.getType(),
		})
		if err := l.consume(); err != nil {
			return nil, err
		}
	}
	return tokens, nil
}

func assertTokens(t *testing.T, source string, expected []expectedToken) {
	t.Helper()
	tokens, err := collectTokens(source)
	if err != nil {
		t.Fatalf("lexer error for %q: %v", source, err)
	}

	if len(tokens) != len(expected) {
		t.Fatalf("for %q: expected %d tokens, got %d\n  expected: %v\n  got:      %v",
			source, len(expected), len(tokens), expected, tokens)
	}

	for i, exp := range expected {
		got := tokens[i]
		if got.token != exp.token || got.typ != exp.typ {
			t.Errorf("for %q token[%d]: expected {%q %s}, got {%q %s}",
				source, i, exp.token, exp.typ, got.token, got.typ)
		}
	}
}

func TestLexerSimpleUnit(t *testing.T) {
	assertTokens(t, "m", []expectedToken{
		{"m", tokenSymbol},
	})
}

func TestLexerCompound(t *testing.T) {
	assertTokens(t, "m/s", []expectedToken{
		{"m", tokenSymbol},
		{"/", tokenSolidus},
		{"s", tokenSymbol},
	})
}

func TestLexerWithExponent(t *testing.T) {
	assertTokens(t, "m2", []expectedToken{
		{"m", tokenSymbol},
		{"2", tokenNumber},
	})
}

func TestLexerNegativeExponent(t *testing.T) {
	assertTokens(t, "m-2", []expectedToken{
		{"m", tokenSymbol},
		{"-2", tokenNumber},
	})
}

func TestLexerAnnotation(t *testing.T) {
	assertTokens(t, "{score}", []expectedToken{
		{"{score}", tokenAnnotation},
	})
}

func TestLexerBracketedUnit(t *testing.T) {
	assertTokens(t, "[lb_av]", []expectedToken{
		{"[lb_av]", tokenSymbol},
	})
}

func TestLexerMixedWithBracket(t *testing.T) {
	assertTokens(t, "cm[H2O]", []expectedToken{
		{"cm", tokenSymbol},
		{"[H2O]", tokenSymbol},
	})
}

func TestLexerTenStar(t *testing.T) {
	assertTokens(t, "10*3/uL", []expectedToken{
		{"10*", tokenSymbol},
		{"3", tokenNumber},
		{"/", tokenSolidus},
		{"uL", tokenSymbol},
	})
}

func TestLexerKgMPerS2(t *testing.T) {
	assertTokens(t, "kg.m/s2", []expectedToken{
		{"kg", tokenSymbol},
		{".", tokenPeriod},
		{"m", tokenSymbol},
		{"/", tokenSolidus},
		{"s", tokenSymbol},
		{"2", tokenNumber},
	})
}

func TestLexerPercent(t *testing.T) {
	assertTokens(t, "%", []expectedToken{
		{"%", tokenSymbol},
	})
}

func TestLexerParens(t *testing.T) {
	assertTokens(t, "(m)", []expectedToken{
		{"(", tokenOpen},
		{"m", tokenSymbol},
		{")", tokenClose},
	})
}

func TestLexerLeadingSolidus(t *testing.T) {
	assertTokens(t, "/m", []expectedToken{
		{"/", tokenSolidus},
		{"m", tokenSymbol},
	})
}

func TestLexerMolPerL(t *testing.T) {
	assertTokens(t, "mol/L", []expectedToken{
		{"mol", tokenSymbol},
		{"/", tokenSolidus},
		{"L", tokenSymbol},
	})
}

func TestLexerComplex(t *testing.T) {
	assertTokens(t, "4.[pi].10*-7.N/A2", []expectedToken{
		{"4", tokenNumber},
		{".", tokenPeriod},
		{"[pi]", tokenSymbol},
		{".", tokenPeriod},
		{"10*", tokenSymbol},
		{"-7", tokenNumber},
		{".", tokenPeriod},
		{"N", tokenSymbol},
		{"/", tokenSolidus},
		{"A", tokenSymbol},
		{"2", tokenNumber},
	})
}

func TestLexerPureNumber(t *testing.T) {
	assertTokens(t, "123", []expectedToken{
		{"123", tokenNumber},
	})
}

func TestLexerSignedPositive(t *testing.T) {
	assertTokens(t, "m+3", []expectedToken{
		{"m", tokenSymbol},
		{"+3", tokenNumber},
	})
}

func TestLexerAnnotationAfterUnit(t *testing.T) {
	assertTokens(t, "mg{total}", []expectedToken{
		{"mg", tokenSymbol},
		{"{total}", tokenAnnotation},
	})
}

func TestLexerEmpty(t *testing.T) {
	l, err := newLexer("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !l.finished() {
		t.Error("expected finished for empty source")
	}
}

func TestLexerGetTokenAsInt(t *testing.T) {
	l, err := newLexer("42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, err := l.getTokenAsInt()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestLexerGetTokenAsIntSigned(t *testing.T) {
	l, err := newLexer("m-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// consume "m"
	if err := l.consume(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, err := l.getTokenAsInt()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != -3 {
		t.Errorf("expected -3, got %d", v)
	}
}

func TestLexerUnterminatedAnnotation(t *testing.T) {
	_, err := newLexer("{oops")
	if err == nil {
		t.Error("expected error for unterminated annotation")
	}
}

func TestLexerUnterminatedBracket(t *testing.T) {
	_, err := newLexer("[oops")
	if err == nil {
		t.Error("expected error for unterminated bracket")
	}
}

func TestLexerNestedParens(t *testing.T) {
	assertTokens(t, "((m))", []expectedToken{
		{"(", tokenOpen},
		{"(", tokenOpen},
		{"m", tokenSymbol},
		{")", tokenClose},
		{")", tokenClose},
	})
}

func TestLexerDegreeCelsius(t *testing.T) {
	assertTokens(t, "Cel", []expectedToken{
		{"Cel", tokenSymbol},
	})
}

func TestLexerSquareBracketUnit(t *testing.T) {
	assertTokens(t, "[in_i'H2O]", []expectedToken{
		{"[in_i'H2O]", tokenSymbol},
	})
}

func TestLexerMilliliter(t *testing.T) {
	assertTokens(t, "mL", []expectedToken{
		{"mL", tokenSymbol},
	})
}

func TestLexerPercentAnnotation(t *testing.T) {
	assertTokens(t, "%{vol}", []expectedToken{
		{"%", tokenSymbol},
		{"{vol}", tokenAnnotation},
	})
}

func TestLexerTokenTypeStringer(t *testing.T) {
	tests := []struct {
		typ  tokenType
		want string
	}{
		{tokenNone, "none"},
		{tokenNumber, "number"},
		{tokenSymbol, "symbol"},
		{tokenSolidus, "solidus"},
		{tokenPeriod, "period"},
		{tokenOpen, "open"},
		{tokenClose, "close"},
		{tokenAnnotation, "annotation"},
		{tokenType(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.typ.String()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLexerFinishedAfterAllTokens(t *testing.T) {
	l, err := newLexer("m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.finished() {
		t.Error("should not be finished before consuming")
	}
	if err := l.consume(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !l.finished() {
		t.Error("should be finished after consuming all tokens")
	}
}

// Ensure fmt import is used (by token stringer in assertions).
var _ = fmt.Sprintf
