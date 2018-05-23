package gotak

import (
	"testing"
)

func TestParseMove(t *testing.T) {
	tests := []string{
		"a1+",
		"1a1+",
		"1a1+1",
		"a1+1",
		"2a2+2",
		"3a3+3",
		"4a4>121",
	}

	for _, mv := range tests {
		t.Run(mv, func(t *testing.T) {

			m, err := NewMove(mv)
			if err != nil {
				t.Errorf("error creating move: %+v", err)
			}

			if m.Stone == "" {
				t.Errorf("stone is empty")
			}

			if m.Square == "" {
				t.Errorf("square is empty")
			}

			if m.MoveCount == 0 {
				t.Errorf("move count is zero")
			}

			if m.MoveDirection == "" {
				t.Errorf("direction is empty")
			}

			if len(m.MoveDropCounts) < 1 {
				t.Errorf("drop counts is empty")
			}
		})
	}
}
