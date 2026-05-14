package main

import (
	"testing"

	"github.com/icco/gotak"
)

// playGame builds a game by applying the given (player, moveText) pairs
// in order.
func playGame(t *testing.T, size int64, moves []struct {
	player int
	move   string
}) *gotak.Game {
	t.Helper()
	g, err := gotak.NewGame(size, 1, "test")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	for _, m := range moves {
		if err := g.DoSingleMove(m.move, m.player); err != nil {
			t.Fatalf("DoSingleMove(%q, %d): %v", m.move, m.player, err)
		}
	}
	return g
}

func TestBuildReplaySteps_emptyGame(t *testing.T) {
	g, err := gotak.NewGame(5, 1, "t")
	if err != nil {
		t.Fatal(err)
	}
	steps, err := buildReplaySteps(g)
	if err != nil {
		t.Fatalf("buildReplaySteps: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(steps))
	}
}

func TestBuildReplaySteps_firstTurnInversion(t *testing.T) {
	// Turn 1: White plays a1, Black plays e5. Per Tak rules each player
	// places the OPPONENT's stone on turn 1, so a1 ends up black and e5
	// ends up white.
	g := playGame(t, 5, []struct {
		player int
		move   string
	}{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
	})

	steps, err := buildReplaySteps(g)
	if err != nil {
		t.Fatalf("buildReplaySteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	a1 := steps[0].Board["a1"]
	if len(a1) != 1 || a1[0].Player != gotak.PlayerBlack {
		t.Errorf("step 0 a1 = %+v, want single black stone", a1)
	}
	e5 := steps[1].Board["e5"]
	if len(e5) != 1 || e5[0].Player != gotak.PlayerWhite {
		t.Errorf("step 1 e5 = %+v, want single white stone", e5)
	}

	if steps[0].Player != gotak.PlayerWhite || steps[0].Move != "a1" {
		t.Errorf("step 0 metadata = %+v, want White/a1", steps[0])
	}
	if steps[1].Player != gotak.PlayerBlack || steps[1].Move != "e5" {
		t.Errorf("step 1 metadata = %+v, want Black/e5", steps[1])
	}
}

func TestBuildReplaySteps_snapshotsAreIndependent(t *testing.T) {
	// Each snapshot should be a deep copy so mutations to the live board
	// after step N don't show up in step N's snapshot.
	g := playGame(t, 5, []struct {
		player int
		move   string
	}{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
	})
	steps, err := buildReplaySteps(g)
	if err != nil {
		t.Fatal(err)
	}

	// Step 0 (after move 1) should have a1 occupied but b2 empty.
	if len(steps[0].Board["a1"]) != 1 {
		t.Errorf("step 0 a1 should be occupied")
	}
	if len(steps[0].Board["b2"]) != 0 {
		t.Errorf("step 0 b2 should be empty, got %+v", steps[0].Board["b2"])
	}

	// Step 2 (after b2) should have b2 occupied.
	if len(steps[2].Board["b2"]) != 1 {
		t.Errorf("step 2 b2 should be occupied, got %+v", steps[2].Board["b2"])
	}
}

func TestBoardAtTurn(t *testing.T) {
	g := playGame(t, 5, []struct {
		player int
		move   string
	}{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	// Turn 0: empty board.
	at0, err := boardAtTurn(g, 0)
	if err != nil {
		t.Fatal(err)
	}
	for sq, stones := range at0 {
		if len(stones) != 0 {
			t.Errorf("turn 0 square %s should be empty, got %+v", sq, stones)
		}
	}

	// Turn 1: a1 (black per Tak rule) + e5 (white per Tak rule).
	at1, err := boardAtTurn(g, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(at1["a1"]) != 1 || at1["a1"][0].Player != gotak.PlayerBlack {
		t.Errorf("turn 1 a1 = %+v, want black", at1["a1"])
	}
	if len(at1["b2"]) != 0 {
		t.Errorf("turn 1 b2 should still be empty, got %+v", at1["b2"])
	}

	// Turn 2: a1 + e5 + b2 (white) + d4 (black).
	at2, err := boardAtTurn(g, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(at2["b2"]) != 1 || at2["b2"][0].Player != gotak.PlayerWhite {
		t.Errorf("turn 2 b2 = %+v, want white", at2["b2"])
	}
	if len(at2["d4"]) != 1 || at2["d4"][0].Player != gotak.PlayerBlack {
		t.Errorf("turn 2 d4 = %+v, want black", at2["d4"])
	}

	// Turn way-past-end: same as final state.
	atBig, err := boardAtTurn(g, 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(atBig["d4"]) != 1 {
		t.Errorf("turn 99 d4 should be occupied (same as final)")
	}
}

func TestSnapshotSquares_isDeepCopy(t *testing.T) {
	g := playGame(t, 5, []struct {
		player int
		move   string
	}{
		{gotak.PlayerWhite, "a1"},
	})
	snap := snapshotSquares(g.Board)
	g.Board.Squares["a1"][0].Player = -999
	if snap["a1"][0].Player == -999 {
		t.Errorf("snapshot was a shallow copy; mutation leaked through")
	}
}
