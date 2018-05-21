package gotak

import "fmt"

// Tag is a Key and Value pair stored providing meta about a game.
type Tag struct {
	Key   string
	Value string
}

func (t *Tag) String() string {
	return fmt.Sprintf("%s: %s", t.Key, t.Value)
}
