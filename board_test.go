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

	// t.Logf("squares post moves: %+v", b.Squares)

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

	// t.Logf("squares post moves: %+v", b.Squares)

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
	cases := []struct {
		name       string
		tps        string
		startEdges []string
		endEdges   []string
		player     int
		expectRoad bool
	}{
		{
			name:       "horizontal-white-road",
			tps:        "x5/x5/x5/x5/1,1,1,1,1 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"e1"},
			player:     PlayerWhite,
			expectRoad: true,
		},
		{
			name:       "vertical-black-road",
			tps:        "2,x4/2,x4/2,x4/2,x4/2,x4 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"a5"},
			player:     PlayerBlack,
			expectRoad: true,
		},
		{
			name:       "broken-road-with-standing",
			tps:        "x5/x5/x5/x5/1,1,1S,1,1 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"e1"},
			player:     PlayerWhite,
			expectRoad: false,
		},
		{
			name:       "capstone-completes-road",
			tps:        "x5/x5/x5/x5/1,1,1C,1,1 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"e1"},
			player:     PlayerWhite,
			expectRoad: true,
		},
		{
			name:       "stack-with-opponent-on-top-blocks-road",
			tps:        "x5/x5/x5/x5/1,1,12,1,1 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"e1"},
			player:     PlayerWhite,
			expectRoad: false,
		},
		{
			name:       "L-shaped-road",
			tps:        "x5/x5/x5/1,1,1,1,1/1,x4 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"e2"},
			player:     PlayerWhite,
			expectRoad: true,
		},
		{
			name:       "no-stones-no-road",
			tps:        "x5/x5/x5/x5/x5 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"e1"},
			player:     PlayerWhite,
			expectRoad: false,
		},
		{
			name:       "isolated-stones-do-not-connect",
			tps:        "x5/x5/x5/x5/1,x3,1 1 1",
			startEdges: []string{"a1"},
			endEdges:   []string{"e1"},
			player:     PlayerWhite,
			expectRoad: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, _, _, err := ParseTPS(tc.tps)
			if err != nil {
				t.Fatalf("parse tps: %v", err)
			}

			found := false
			for _, start := range tc.startEdges {
				if b.Color(start) != tc.player {
					continue
				}
				top := b.TopStone(start)
				if top == nil || top.Type == StoneStanding {
					continue
				}
				if b.FindRoad(start, tc.endEdges) {
					found = true
					break
				}
			}

			if found != tc.expectRoad {
				t.Errorf("FindRoad from %v to %v: got %v, want %v", tc.startEdges, tc.endEdges, found, tc.expectRoad)
			}
		})
	}
}

func TestValidSquareIntegerOverflowPrevention(t *testing.T) {
	b := &Board{
		Size: 256,
	}
	_ = b.Init()

	if b.isValidSquare("a1") {
		t.Errorf("isValidSquare should return false for board size > 255 to prevent integer overflow")
	}

	b.Size = 159
	if b.isValidSquare("a1") {
		t.Errorf("isValidSquare should return false for board size that would cause overflow ('a' + size > 255)")
	}

	b.Size = 158
	if !b.isValidSquare("a1") {
		t.Errorf("isValidSquare should return true for board size that doesn't cause overflow")
	}

	b.Size = 9
	if !b.isValidSquare("a1") {
		t.Errorf("isValidSquare should return true for normal board size")
	}
	if !b.isValidSquare("i9") {
		t.Errorf("isValidSquare should return true for valid square at max board size")
	}
	if b.isValidSquare("j1") {
		t.Errorf("isValidSquare should return false for column beyond board size")
	}
}
