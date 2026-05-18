package main

import (
	"testing"

	"github.com/icco/gotak"
)

func TestParseOpeningPrefix(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"", nil, false},
		{"   ", nil, false},
		{"a1", []string{"a1"}, false},
		{"a1,e5,Sb2", []string{"a1", "e5", "Sb2"}, false},
		{" a1 , e5 ", []string{"a1", "e5"}, false},
		{"a1,,e5", []string{"a1", "e5"}, false},
		{"not-a-move", nil, true},
		{"a1,nonsense", nil, true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := parseOpeningPrefix(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestComputeOpenings(t *testing.T) {
	db := setupTestDB(t)

	// Three games:
	//   1: a1, e5, b2, d4
	//   2: a1, e5, c3, d4
	//   3: a1, a5, b2 (diverges immediately on move 2)
	seed := []struct {
		gameID  int64
		turn    int64
		player  int
		text    string
	}{
		{1, 1, gotak.PlayerWhite, "a1"},
		{1, 1, gotak.PlayerBlack, "e5"},
		{1, 2, gotak.PlayerWhite, "b2"},
		{1, 2, gotak.PlayerBlack, "d4"},
		{2, 1, gotak.PlayerWhite, "a1"},
		{2, 1, gotak.PlayerBlack, "e5"},
		{2, 2, gotak.PlayerWhite, "c3"},
		{2, 2, gotak.PlayerBlack, "d4"},
		{3, 1, gotak.PlayerWhite, "a1"},
		{3, 1, gotak.PlayerBlack, "a5"},
		{3, 2, gotak.PlayerWhite, "b2"},
	}
	for _, s := range seed {
		if err := db.Create(&Move{GameID: s.gameID, Turn: s.turn, Player: s.player, Text: s.text}).Error; err != nil {
			t.Fatal(err)
		}
	}

	t.Run("empty prefix: all games, first-move distribution", func(t *testing.T) {
		count, conts, err := computeOpenings(db, nil)
		if err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Errorf("game count = %d, want 3", count)
		}
		// All three games start with a1.
		if len(conts) != 1 || conts[0].Move != "a1" || conts[0].Count != 3 {
			t.Errorf("continuations = %+v, want [{a1 3}]", conts)
		}
	})

	t.Run("prefix [a1]: 3 games, next-move split", func(t *testing.T) {
		count, conts, err := computeOpenings(db, []string{"a1"})
		if err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Errorf("game count = %d, want 3", count)
		}
		// 2x e5, 1x a5 — sorted by count desc, then alphabetical.
		want := []OpeningContinuation{{Move: "e5", Count: 2}, {Move: "a5", Count: 1}}
		if len(conts) != len(want) {
			t.Fatalf("got %+v, want %+v", conts, want)
		}
		for i, w := range want {
			if conts[i] != w {
				t.Errorf("conts[%d] = %+v, want %+v", i, conts[i], w)
			}
		}
	})

	t.Run("prefix [a1, e5]: 2 games, next-move split", func(t *testing.T) {
		count, conts, err := computeOpenings(db, []string{"a1", "e5"})
		if err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Errorf("game count = %d, want 2", count)
		}
		want := []OpeningContinuation{{Move: "b2", Count: 1}, {Move: "c3", Count: 1}}
		if len(conts) != len(want) {
			t.Fatalf("got %+v, want %+v", conts, want)
		}
		for i, w := range want {
			if conts[i] != w {
				t.Errorf("conts[%d] = %+v, want %+v", i, conts[i], w)
			}
		}
	})

	t.Run("no matching games", func(t *testing.T) {
		count, conts, err := computeOpenings(db, []string{"e3"})
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("game count = %d, want 0", count)
		}
		if len(conts) != 0 {
			t.Errorf("continuations = %+v, want empty", conts)
		}
	})

	t.Run("prefix longer than any game produces zero matches", func(t *testing.T) {
		count, _, err := computeOpenings(db, []string{"a1", "e5", "b2", "d4", "extra"})
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("game count = %d, want 0", count)
		}
	})

	t.Run("prefix matching a game exactly produces no continuations", func(t *testing.T) {
		count, conts, err := computeOpenings(db, []string{"a1", "e5", "b2", "d4"})
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("game count = %d, want 1", count)
		}
		if len(conts) != 0 {
			t.Errorf("continuations = %+v, want empty (game ended at the prefix)", conts)
		}
	})
}
