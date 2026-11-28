package gotak

import (
	"strings"
	"testing"
)

func TestTurnText_moveComments(t *testing.T) {
	first, err := NewMove("a1")
	if err != nil {
		t.Fatalf("NewMove(a1): %v", err)
	}
	second, err := NewMove("e5")
	if err != nil {
		t.Fatalf("NewMove(e5): %v", err)
	}

	turn := &Turn{
		Number: 1,
		First:  first,
		Second: second,
	}

	if got, want := turn.Text(), "1. a1 e5"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}

	first.Comment = "opening flat"
	if got, want := turn.Text(), "1. a1 { opening flat } e5"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}

	second.Comment = "counter"
	if got, want := turn.Text(), "1. a1 { opening flat } e5 { counter }"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}

	turn.Comment = "turn note"
	if got, want := turn.Text(), "1. a1 { opening flat } e5 { counter } { turn note }"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}

	turn.Second = nil
	if got, want := turn.Text(), "1. a1 { opening flat } { turn note }"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}

func TestGamePTN_moveComments(t *testing.T) {
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

	g.Turns[0].First.Comment = "nice"
	g.Turns[0].Second.Comment = "blunder"

	got := g.PTN()
	want := "1. a1 { nice } e5 { blunder }\n"
	if !testing.Short() {
		// Verify expected PTN turn line
		if turnText := g.Turns[0].Text() + "\n"; turnText != want {
			t.Errorf("Turns[0].Text() = %q, want %q", turnText, want)
		}
	}
	if !containsLine(got, "1. a1 { nice } e5 { blunder }") {
		t.Errorf("PTN missing commented move line, got:\n%s", got)
	}
}

func containsLine(text, line string) bool {
	for _, l := range splitLines(text) {
		if l == line {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}
