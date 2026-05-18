package gotak

import (
	"strings"
	"testing"
)

func TestGamePTN_emptyGame(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	got := g.PTN()
	// The Size meta tag is set by NewGame, so we expect that line at least.
	if !strings.Contains(got, `[Size "5"]`) {
		t.Errorf("PTN missing Size tag: %q", got)
	}
}

func TestGamePTN_completedTurns(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("a1", PlayerWhite); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("e5", PlayerBlack); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("b2", PlayerWhite); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("d4", PlayerBlack); err != nil {
		t.Fatal(err)
	}

	got := g.PTN()
	for _, want := range []string{
		`[Size "5"]`,
		"1. a1 e5",
		"2. b2 d4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PTN missing %q: %q", want, got)
		}
	}
}

func TestGamePTN_incompleteFinalTurn(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("a1", PlayerWhite); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("e5", PlayerBlack); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("b2", PlayerWhite); err != nil {
		t.Fatal(err)
	}

	got := g.PTN()
	// Incomplete final turn must render as exactly "2. b2\n", not
	// "2. b2 \n" with a dangling space and not "2. b2 --\n".
	if !strings.Contains(got, "\n2. b2\n") {
		t.Errorf("incomplete turn 2 should render as `2. b2`, got:\n%s", got)
	}
}

func TestGamePTN_tagWithQuoteRoundtrips(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.UpdateMeta("Player1", `"Alice"`); err != nil {
		t.Fatal(err)
	}
	out := g.PTN()
	if strings.Contains(out, `"Alice"`) {
		t.Errorf("PTN should coerce embedded quotes; got %q", out)
	}
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN failed on tag with quote: %v\n%s", err, out)
	}
	v, err := round.GetMeta("Player1")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if !strings.Contains(v, "Alice") {
		t.Errorf("Player1 = %q, want it to still contain Alice", v)
	}
}

func TestGamePTN_roundTrip(t *testing.T) {
	src := []byte(`[Size "5"]
[Player1 "alice"]
[Player2 "bob"]

1. a1 e5
2. b2 d4
3. c3 c4
`)

	g, err := ParsePTN(src)
	if err != nil {
		t.Fatalf("ParsePTN: %v", err)
	}

	out := g.PTN()
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN(PTN(ParsePTN(src))) failed: %v\noutput was:\n%s", err, out)
	}

	if len(round.Turns) != len(g.Turns) {
		t.Errorf("turn count drifted: original=%d, round-tripped=%d", len(g.Turns), len(round.Turns))
	}
	for i := range g.Turns {
		if g.Turns[i].First.Text != round.Turns[i].First.Text {
			t.Errorf("turn %d first move drift: %q vs %q", i, g.Turns[i].First.Text, round.Turns[i].First.Text)
		}
		if (g.Turns[i].Second == nil) != (round.Turns[i].Second == nil) {
			t.Errorf("turn %d second move nil-ness drift", i)
		}
	}
}

func TestGamePTN_nilGame(t *testing.T) {
	var g *Game
	if got := g.PTN(); got != "" {
		t.Errorf("nil game should return empty string, got %q", got)
	}
}
