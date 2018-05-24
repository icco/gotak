package gotak

import "fmt"

// Stone is a single Tak stone.
type Stone struct {
	Type   string
	Player int
}

func (s *Stone) String() string {
	plyrText := ""
	if s.Player == PlayerWhite {
		plyrText = "W"
	} else if s.Player == PlayerBlack {
		plyrText = "B"
	}

	return fmt.Sprintf("%s%s", plyrText, s.Type)
}

// Validate checks that all fields of Stone are correct.
func (s *Stone) Validate() error {
	if s.Player != PlayerWhite && s.Player != PlayerBlack {
		return fmt.Errorf("%d is not a valid player int", s.Player)
	}

	if s.Type == "" {
		return fmt.Errorf("type is empty string")
	}

	if s.Type != StoneFlat && s.Type != StoneStanding && s.Type != StoneCap {
		return fmt.Errorf("type '%s' is not valid", s.Type)
	}

	return nil
}

// StoneFlat is a constant for a string representation of a flat stone.
const StoneFlat string = "F"

// StoneStanding is a constant for a string representation of a standing stone.
const StoneStanding string = "S"

// StoneCap is a constant for a string representation of a cap stone.
const StoneCap string = "C"

// PlayerWhite is the person moving the white or light colored stones.
const PlayerWhite int = 1

// PlayerBlack is the person moving the black or dark colored stones.
const PlayerBlack int = 2
