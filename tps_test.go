package gotak

import "testing"

func TestParseTPS_empty(t *testing.T) {
	b, player, move, err := ParseTPS("x5/x5/x5/x5/x5 1 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b.Size != 5 {
		t.Errorf("size = %d, want 5", b.Size)
	}
	if player != PlayerWhite {
		t.Errorf("player = %d, want %d", player, PlayerWhite)
	}
	if move != 1 {
		t.Errorf("move = %d, want 1", move)
	}
	_ = b.IterateOverSquares(func(_ string, stones []*Stone) error {
		if len(stones) != 0 {
			t.Errorf("expected empty board, got %d stones", len(stones))
		}
		return nil
	})
}

func TestParseTPS_stacksAndTypes(t *testing.T) {
	b, _, _, err := ParseTPS("x4/x4/x4/1,12S,1C,x 2 3")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b.Size != 4 {
		t.Fatalf("size = %d, want 4", b.Size)
	}

	if len(b.Squares["a1"]) != 1 || b.Squares["a1"][0].Type != StoneFlat || b.Squares["a1"][0].Player != PlayerWhite {
		t.Errorf("a1 = %+v, want single white flat", b.Squares["a1"])
	}

	b1 := b.Squares["b1"]
	if len(b1) != 2 ||
		b1[0].Player != PlayerWhite || b1[0].Type != StoneFlat ||
		b1[1].Player != PlayerBlack || b1[1].Type != StoneStanding {
		t.Errorf("b1 = %+v, want [W flat, B standing]", b1)
	}

	c1 := b.Squares["c1"]
	if len(c1) != 1 || c1[0].Player != PlayerWhite || c1[0].Type != StoneCap {
		t.Errorf("c1 = %+v, want white capstone", c1)
	}

	if len(b.Squares["d1"]) != 0 {
		t.Errorf("d1 = %+v, want empty", b.Squares["d1"])
	}
}

func TestParseTPS_invalid(t *testing.T) {
	cases := []string{
		"",                          // empty
		"x5/x5/x5/x5/x5",            // missing player/move
		"x5/x5/x5/x4 1 1",           // row too short
		"x5/x5/x5/x5/1,1,1,1,1 3 1", // bad player
		"x5/x5/x5/x5/1,1,1,1,1 1 0", // bad move
		"x5/x5/x5/x5/3,1,1,1,1 1 1", // bad stone char
		"x3/x3/x3 1 1",              // size below supported range
	}
	for _, c := range cases {
		if _, _, _, err := ParseTPS(c); err == nil {
			t.Errorf("ParseTPS(%q) want error, got none", c)
		}
	}
}
