package gotak

import "fmt"

// Stone is a single Tak stone.
type Stone struct {
	Type   string
	Player int
}

func (s *Stone) String() string {
	return fmt.Sprintf("%d(%s)", s.Player, s.Type)
}
