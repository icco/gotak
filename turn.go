package gotak

import "fmt"

// Turn is a single turn played in a game.
type Turn struct {
	Number  int64
	First   *Move
	Second  *Move
	Result  string
	Comment string
}

// Text returns a PTN formated string of the turn.
func (t *Turn) Text() string {
	var move string
	if t.First != nil && t.Second != nil {
		move = fmt.Sprintf("%d. %s %s", t.Number, t.First.Text, t.Second.Text)
	}

	if t.Comment != "" {
		if move != "" {
			move = fmt.Sprintf("%s { %s }", move, t.Comment)
		} else {
			move = fmt.Sprintf("{ %s }", t.Comment)
		}
	}

	return move
}

// Debug is a verbose dumping of the object and its sub objects.
func (t *Turn) Debug() string {
	return fmt.Sprintf("&{%d 1:%+v 1:%+v Result:%+v  Comment: \"%s\"}", t.Number, t.First, t.Second, t.Result, t.Comment)
}
