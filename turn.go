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
	if t.First == nil {
		if t.Comment != "" {
			return fmt.Sprintf("{ %s }", t.Comment)
		}
		return ""
	}

	var prefix string
	if t.Number > 0 {
		prefix = fmt.Sprintf("%d%s.", t.Number, t.Branch)
	}

	first := t.First.Text
	if t.First.Comment != "" {
		first = fmt.Sprintf("%s { %s }", first, t.First.Comment)
	}

	var moves string
	if t.Second != nil {
		second := t.Second.Text
		if t.Second.Comment != "" {
			second = fmt.Sprintf("%s { %s }", second, t.Second.Comment)
		}
		moves = fmt.Sprintf("%s %s", first, second)
	} else {
		moves = first
	}

	var line string
	if prefix != "" {
		line = fmt.Sprintf("%s %s", prefix, moves)
	} else {
		line = moves
	}

	if t.Result != "" {
		line = fmt.Sprintf("%s %s", line, t.Result)
	}

	if t.Comment != "" {
		switch {
		case t.Result != "":
			// Post-result position is unambiguous for turn comments
			line = fmt.Sprintf("%s { %s }", line, t.Comment)
		case prefix != "":
			// Pre-move position is unambiguous when there is no result
			line = fmt.Sprintf("%s { %s } %s", prefix, t.Comment, moves)
		default:
			line = fmt.Sprintf("%s { %s }", line, t.Comment)
		}
	}

	return line
}

// Debug is a verbose dumping of the object and its sub objects.
func (t *Turn) Debug() string {
	return fmt.Sprintf("&{%d 1:%+v 1:%+v Result:%+v  Comment: \"%s\"}", t.Number, t.First, t.Second, t.Result, t.Comment)
}
