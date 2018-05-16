import (
	"bufio"
	"bytes"
	"io"
)

// Represents a lexical token
type Token int

// Represent the kinds of tokens we can have
const (
	ILLEGAL Token = iota
	EOF
	WS
	NEWLINE

	SPACE
	COUNT
	DIRECTION
	STONE

	INFORMATION
	COMMENT_START
	TEXT
	COMMENT_END
)

// Represents a token type and the literal characters it was parsed from.
type TokenInstance struct {
	Type    Token
	Literal string
}

// Returns true if ch is a space or a tab.
func isWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t'
}

// Returns true if ch is a upper or lowercase letter.
func isLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// Returns true if ch is a numeric digit
func isDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9')
}

var eof = rune(0)

// Scanner represents a lexical scanner.
type Scanner struct {
	r *bufio.Reader
}

func NewScanner(r io.Reader) *Scanner {
	return &Scanner{r: bufio.NewReader(r)}
}

// read reads the next rune from the bufferred reader.
// Returns the rune(0) if an error occurs (or io.EOF is returned).
func (s *Scanner) read() rune {
	ch, _, err := s.r.ReadRune()
	if err != nil {
		return eof
	}
	return ch
}

// unread places the previously read rune back on the reader.
func (s *Scanner) unread() { _ = s.r.UnreadRune() }

// scanWhitespace consumes the current rune and all contiguous whitespace.
func (s *Scanner) scanWhitespace() (tok Token, lit string) {
	// Create a buffer and read the current character into it.
	var buf bytes.Buffer
	buf.WriteRune(s.read())

	// Read every subsequent whitespace character into the buffer.
	// Non-whitespace characters and EOF will cause the loop to exit.
	for {
		if ch := s.read(); ch == eof {
			break
		} else if !isWhitespace(ch) {
			s.unread()
			break
		} else {
			buf.WriteRune(ch)
		}
	}

	return WS, buf.String()
}

// scanIdent consumes the current rune and all contiguous ident runes.
func (s *Scanner) scanDigit() (tok Token, lit string) {
	// Create a buffer and read the current character into it.
	var buf bytes.Buffer
	buf.WriteRune(s.read())

	// Read every subsequent ident character into the buffer.
	// Non-ident characters and EOF will cause the loop to exit.
	for {
		if ch := s.read(); ch == eof {
			break
		} else if !isDigit(ch) {
			s.unread()
			break
		} else {
			_, _ = buf.WriteRune(ch)
		}
	}

	// Otherwise return as a regular identifier.
	return INTEGER, buf.String()
}

// scanIdent consumes the current rune and all contiguous ident runes.
func (s *Scanner) scanString() (tok Token, lit string) {
	// Create a buffer and read the current character into it.
	var buf bytes.Buffer
	buf.WriteRune(s.read())

	// Read every subsequent ident character into the buffer.
	// Non-ident characters and EOF will cause the loop to exit.
	for {
		if ch := s.read(); ch == eof {
			break
		} else if !isLetter(ch) {
			s.unread()
			break
		} else {
			_, _ = buf.WriteRune(ch)
		}
	}

	// If the string matches a keyword then return that keyword.
	switch buf.String() {
	case "t":
		return KW_TEASPOON, buf.String()
	case "T":
		return KW_TABLESPOON, buf.String()
	case "c":
		return KW_CUP, buf.String()
	case "q":
		return KW_QUART, buf.String()
	}

	// Otherwise return as a regular identifier.
	return WORD, buf.String()
}

// Scan returns the next token and literal value.
func (s *Scanner) Scan() (tok Token, lit string) {
	// Read the next rune.
	ch := s.read()

	// If we see whitespace then consume all contiguous whitespace.
	// If we see a letter then consume as a word or reserved word.
	// If we see a digit then consume as a number.
	if isWhitespace(ch) {
		s.unread()
		return s.scanWhitespace()
	} else if isLetter(ch) {
		s.unread()
		return s.scanString()
	} else if isDigit(ch) {
		s.unread()
		return s.scanDigit()
	}

	// Otherwise read the individual character.
	switch ch {
	case eof:
		return EOF, ""
	case '\n':
		return NEWLINE, string(ch)
	case '.':
		return PERIOD, string(ch)
	}

	return ILLEGAL, string(ch)
}
