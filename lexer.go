package ucum

import (
	"fmt"
	"strconv"
	"unicode"
)

type tokenType int

const (
	tokenNone       tokenType = iota
	tokenNumber               // digits only (possibly signed for exponents)
	tokenSymbol               // unit symbols like kg, m, [lb_av], 10*
	tokenSolidus              // /
	tokenPeriod               // .
	tokenOpen                 // (
	tokenClose                // )
	tokenAnnotation           // {annotation}
)

func (t tokenType) String() string {
	switch t {
	case tokenNone:
		return "none"
	case tokenNumber:
		return "number"
	case tokenSymbol:
		return "symbol"
	case tokenSolidus:
		return "solidus"
	case tokenPeriod:
		return "period"
	case tokenOpen:
		return "open"
	case tokenClose:
		return "close"
	case tokenAnnotation:
		return "annotation"
	default:
		return "unknown"
	}
}

// lexer is a hand-written tokenizer for UCUM expressions.
// It is a port of the Java Lexer.java from FHIR/Ucum-java.
type lexer struct {
	source string
	index  int
	token  string
	typ    tokenType
}

// newLexer creates a new lexer for the given UCUM expression and consumes
// the first token.
func newLexer(source string) (*lexer, error) {
	l := &lexer{
		source: source,
		index:  0,
	}
	if err := l.consume(); err != nil {
		return nil, err
	}
	return l, nil
}

// consume advances the lexer to the next token.
func (l *lexer) consume() error {
	l.token = ""
	l.typ = tokenNone

	if l.index >= len(l.source) {
		return nil
	}

	ch := l.source[l.index]

	// Single character tokens
	switch ch {
	case '/':
		l.token = "/"
		l.typ = tokenSolidus
		l.index++
		return nil
	case '.':
		l.token = "."
		l.typ = tokenPeriod
		l.index++
		return nil
	case '(':
		l.token = "("
		l.typ = tokenOpen
		l.index++
		return nil
	case ')':
		l.token = ")"
		l.typ = tokenClose
		l.index++
		return nil
	}

	// Annotation: {content}
	if ch == '{' {
		return l.readAnnotation()
	}

	// Signed number (exponent): + or - followed by digits
	if ch == '+' || ch == '-' {
		return l.readSignedNumber()
	}

	// General token: symbol or number
	return l.readGeneralToken()
}

// readAnnotation reads a {annotation} token. The braces are included in the
// token value. Only ASCII characters are permitted inside the annotation.
func (l *lexer) readAnnotation() error {
	start := l.index
	l.index++ // skip opening {

	for l.index < len(l.source) {
		ch := l.source[l.index]
		if ch == '}' {
			l.index++ // skip closing }
			l.token = l.source[start:l.index]
			l.typ = tokenAnnotation
			return nil
		}
		if ch > 0x7E || ch < 0x20 {
			return fmt.Errorf("lexer error at position %d: invalid character in annotation", l.index)
		}
		l.index++
	}

	return fmt.Errorf("lexer error at position %d: unterminated annotation", start)
}

// readSignedNumber reads a signed number token (e.g. +3 or -2).
func (l *lexer) readSignedNumber() error {
	start := l.index
	l.index++ // skip sign

	if l.index >= len(l.source) || !isDigit(l.source[l.index]) {
		return fmt.Errorf("lexer error at position %d: sign must be followed by a digit", start)
	}

	for l.index < len(l.source) && isDigit(l.source[l.index]) {
		l.index++
	}

	l.token = l.source[start:l.index]
	l.typ = tokenNumber
	return nil
}

// readGeneralToken reads a symbol or number token. The token type is determined
// by whether any non-digit character was encountered.
//
// Key rules:
//   - A bracketed group like [lb_av] is a complete symbol token.
//   - If a '[' appears after other characters have been consumed, it starts a
//     new token (e.g. "cm[H2O]" -> "cm" + "[H2O]").
//   - Once any non-digit character has been seen, a subsequent digit ends the
//     token so the digit(s) become a separate number token (exponent). This
//     handles "m2" -> "m" + "2" and "10*3" -> "10*" + "3".
func (l *lexer) readGeneralToken() error {
	start := l.index
	hasNonDigit := false
	inBracket := false

	for l.index < len(l.source) {
		ch := l.source[l.index]

		if inBracket {
			// Inside brackets, consume until closing bracket.
			if ch == ']' {
				l.index++
				inBracket = false
				continue
			}
			l.index++
			continue
		}

		if ch == '[' {
			// If we've already consumed characters, the bracket starts a
			// new token.
			if l.index > start {
				break
			}
			inBracket = true
			hasNonDigit = true
			l.index++
			continue
		}

		if isDigit(ch) {
			// If we've already seen a non-digit, the digit is an exponent
			// and belongs to the next token.
			if hasNonDigit {
				break
			}
			l.index++
			continue
		}

		if isSymbolChar(ch) {
			hasNonDigit = true
			l.index++
			continue
		}

		// Not a valid token character; stop here.
		break
	}

	if inBracket {
		return fmt.Errorf("lexer error at position %d: unterminated bracket", start)
	}

	if l.index == start {
		return fmt.Errorf("lexer error at position %d: unexpected character '%c'", l.index, l.source[l.index])
	}

	l.token = l.source[start:l.index]

	if hasNonDigit {
		l.typ = tokenSymbol
	} else {
		l.typ = tokenNumber
	}

	return nil
}

// getToken returns the current token value.
func (l *lexer) getToken() string {
	return l.token
}

// getType returns the current token type.
func (l *lexer) getType() tokenType {
	return l.typ
}

// finished returns true if the lexer has consumed all input.
func (l *lexer) finished() bool {
	return l.index >= len(l.source) && l.typ == tokenNone
}

// getTokenAsInt returns the current token as an integer. An error is returned
// if the current token is not a valid integer.
func (l *lexer) getTokenAsInt() (int, error) {
	v, err := strconv.Atoi(l.token)
	if err != nil {
		return 0, fmt.Errorf("lexer error: token %q is not a valid integer", l.token)
	}
	return v, nil
}

// isDigit returns true if ch is an ASCII digit.
func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isSymbolChar returns true if ch is a valid non-digit symbol character.
// Valid symbol characters are: letters, %, *, ^, ', ", _
func isSymbolChar(ch byte) bool {
	if unicode.IsLetter(rune(ch)) {
		return true
	}
	switch ch {
	case '%', '*', '^', '\'', '"', '_':
		return true
	}
	return false
}
