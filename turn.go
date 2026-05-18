package gotak

import "fmt"

// Turn is a single turn played in a game.
type Turn struct {
	Number  int64
	First   *Move
	Second  *Move
	Result  string
	Comment string
	// Branch is the optional PTN branch label appended to the turn
	// number (e.g. `1a.` -> "a"). Main-line turns leave it empty.
	Branch string
}

// Text returns a PTN-formatted string of the turn. An incomplete final
// turn (no Second move yet) renders without the second field, which the
// PTN parser accepts.
//
// A Second-only turn cannot be rendered because the parser has no
// placeholder syntax for a missing First move; such a turn would silently
// disappear from round-tripped output.
func (t *Turn) Text() string {
	var line string
	switch {
	case t.First != nil && t.Second != nil:
		line = fmt.Sprintf("%d%s. %s %s", t.Number, t.Branch, t.First.Text, t.Second.Text)
	case t.First != nil:
		line = fmt.Sprintf("%d%s. %s", t.Number, t.Branch, t.First.Text)
	}

	if t.Comment != "" {
		if line != "" {
			line = fmt.Sprintf("%s { %s }", line, t.Comment)
		} else {
			line = fmt.Sprintf("{ %s }", t.Comment)
		}
	}

	return line
}

// Debug is a verbose dumping of the object and its sub objects.
func (t *Turn) Debug() string {
	return fmt.Sprintf("&{%d 1:%+v 1:%+v Result:%+v  Comment: \"%s\"}", t.Number, t.First, t.Second, t.Result, t.Comment)
}
