package gotak

import (
	"strings"
	"testing"
)

type testPair struct {
	Stone  *Stone
	Square string
}

func TestDrop(t *testing.T) {
	tests := map[string]testPair{
		"a1":  {Stone: &Stone{Type: StoneFlat, Player: 1}, Square: "a1"},
		"c2":  {Stone: &Stone{Type: StoneFlat, Player: 1}, Square: "c2"},
		"Ca1": {Stone: &Stone{Type: StoneCap, Player: 1}, Square: "a1"},
		"Sa1": {Stone: &Stone{Type: StoneStanding, Player: 1}, Square: "a1"},
	}

	for mv, data := range tests {
		t.Run(mv, func(t *testing.T) {
			b := &Board{
				Size: 4,
			}
			_ = b.Init()

			move, err := NewMove(mv)
			if err != nil {
				t.Errorf("error creating move: %+v", err)
			}

			err = b.DoMove(move, 1)
			if err != nil {
				t.Errorf("error doing move: %+v", err)
			}

			if b.Squares[data.Square] == nil {
				t.Errorf("placement failed at %+v: %+v", data.Square, b.Squares)
			}

			if b.Squares[data.Square][0].Player != data.Stone.Player {
				t.Errorf("player != %d: %s", data.Stone.Player, b.Squares[data.Square][0])
			}

			if b.Squares[data.Square][0].Type != data.Stone.Type {
				t.Errorf("stone != FLAT: %s", b.Squares[data.Square][0])
			}
		})
	}
}

func TestMoving(t *testing.T) {
	b := &Board{
		Size: 6,
	}
	_ = b.Init()

	tests := []string{
		"a1",
		"a2",
		"a3",
		"a4",
		"a1+",
		"2a2+2",
		"3a3+3",
		"4a4>121",
	}

	for _, mv := range tests {
		t.Run(mv, func(t *testing.T) {

			move, err := NewMove(mv)
			if err != nil {
				t.Errorf("error creating move: %+v", err)
			}

			err = b.DoMove(move, 1)
			if err != nil {
				t.Errorf("error doing move: %+v", err)
			}
		})
	}

	//t.Logf("squares post moves: %+v", b.Squares)

	if len(b.Squares["b4"]) != 1 || len(b.Squares["c4"]) != 2 || len(b.Squares["d4"]) != 1 {
		t.Errorf("pieces are not in the correct place: %+v", b.Squares)
	}

	if len(b.Squares["a1"]) != 0 || len(b.Squares["a2"]) != 0 || len(b.Squares["a3"]) != 0 || len(b.Squares["a4"]) != 0 {
		t.Errorf("not all pieces were removed when moving: %+v", b.Squares)
	}
}

func TestMovingOnce(t *testing.T) {
	b := &Board{
		Size: 6,
	}
	_ = b.Init()

	tests := []string{
		"a1",
		"1a1+1",
	}

	for _, mv := range tests {
		t.Run(mv, func(t *testing.T) {

			move, err := NewMove(mv)
			if err != nil {
				t.Errorf("error creating move: %+v", err)
			}

			err = b.DoMove(move, 1)
			if err != nil {
				t.Errorf("error doing move: %+v", err)
			}
		})
	}

	//t.Logf("squares post moves: %+v", b.Squares)

	if len(b.Squares["a2"]) != 1 {
		t.Errorf("pieces are not in the correct place: %+v", b.Squares)
	}

	if len(b.Squares["a1"]) != 0 {
		t.Errorf("not all pieces were removed when moving: %+v", b.Squares)
	}
}

func TestTranslate(t *testing.T) {
	for _, r := range []string{
		"a1,>,b1",
		"c3,-,c2",
		"c3,+,c4",
		"c3,<,b3",
	} {
		t.Run(r, func(t *testing.T) {
			row := strings.Split(r, ",")
			a := Translate(row[0], row[1])
			if a != row[2] {
				t.Errorf("%s %s != %s: %+v", row[0], row[1], row[2], a)
			}
		})
	}
}

func TestIsEdge(t *testing.T) {
	b := &Board{
		Size: 6,
	}
	_ = b.Init()
	for s, e := range map[string]bool{
		"a1": true,
		"a3": true,
		"c3": false,
		"c6": true,
		"f1": true,
		"f6": true,
		"e2": false,
	} {
		t.Run(s, func(t *testing.T) {
			a := b.IsEdge(s)
			if a != e {
				t.Errorf("%v != %v", a, e)
			}
		})
	}
}

func TestFindRoad(t *testing.T) {
	// TODO: Write a test using TPS
}
