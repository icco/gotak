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

func (t *Turn) String() string {
	var move string
	if t.First != nil && t.Second != nil {
		move = fmt.Sprintf("%d. %s %s", t.Number, *t.First, *t.Second)
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
