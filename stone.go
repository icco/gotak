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

// StoneFlat is a constant for a string representation of a flat stone.
const StoneFlat string = "F"

// StoneStanding is a constant for a string representation of a standing stone.
const StoneStanding string = "S"

// StoneCap is a constant for a string representation of a cap stone.
const StoneCap string = "C"
