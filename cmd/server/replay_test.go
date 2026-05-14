package main

import (
	"testing"

	"github.com/icco/gotak"
)

// scriptedMove is one entry in a hand-crafted game used by tests.
type scriptedMove struct {
	player int
	move   string
}

// playGame builds a game by applying the given (player, move) pairs in
// order. It fails the test if any move is rejected.
func playGame(t *testing.T, size int64, moves []scriptedMove) *gotak.Game {
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
		t.Errorf("got %d steps, want 0", len(steps))
	}
}

func TestBuildReplaySteps_nilGame(t *testing.T) {
	steps, err := buildReplaySteps(nil)
	if err != nil {
		t.Errorf("nil game should return (nil, nil), got err=%v", err)
	}
	if steps != nil {
		t.Errorf("nil game should return (nil, nil), got %d steps", len(steps))
	}
}

func TestBuildReplaySteps_firstTurnInversion(t *testing.T) {
	// On turn 1 each player places the opponent's stone: White's a1
	// lands as a black stone, Black's e5 lands as a white stone.
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
	})

	steps, err := buildReplaySteps(g)
	if err != nil {
		t.Fatalf("buildReplaySteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}

	if a1 := steps[0].Board["a1"]; len(a1) != 1 || a1[0].Player != gotak.PlayerBlack {
		t.Errorf("step 0 a1 = %+v, want single black stone", a1)
	}
	if e5 := steps[1].Board["e5"]; len(e5) != 1 || e5[0].Player != gotak.PlayerWhite {
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
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
	})
	steps, err := buildReplaySteps(g)
	if err != nil {
		t.Fatal(err)
	}

	// Step 0 (after move 1) has a1 set, b2 empty.
	if len(steps[0].Board["a1"]) != 1 {
		t.Errorf("step 0 a1 should be occupied")
	}
	if len(steps[0].Board["b2"]) != 0 {
		t.Errorf("step 0 b2 should be empty, got %+v", steps[0].Board["b2"])
	}
	// Step 2 (after b2) has b2 occupied. If step 0's snapshot shared
	// state with the live board, b2 would already be set here.
	if len(steps[2].Board["b2"]) != 1 {
		t.Errorf("step 2 b2 should be occupied, got %+v", steps[2].Board["b2"])
	}
}

func TestBoardAtTurn(t *testing.T) {
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	cases := []struct {
		name string
		turn int64
		// occupied lists (square, expectedPlayer) pairs; everything else
		// should be empty.
		occupied map[string]int
	}{
		{
			name:     "turn 0: empty board",
			turn:     0,
			occupied: map[string]int{},
		},
		{
			name: "turn 1: opening placements (colors inverted)",
			turn: 1,
			occupied: map[string]int{
				"a1": gotak.PlayerBlack,
				"e5": gotak.PlayerWhite,
			},
		},
		{
			name: "turn 2: own-color placements",
			turn: 2,
			occupied: map[string]int{
				"a1": gotak.PlayerBlack,
				"e5": gotak.PlayerWhite,
				"b2": gotak.PlayerWhite,
				"d4": gotak.PlayerBlack,
			},
		},
		{
			name: "turn way past end: same as final",
			turn: 99,
			occupied: map[string]int{
				"a1": gotak.PlayerBlack,
				"e5": gotak.PlayerWhite,
				"b2": gotak.PlayerWhite,
				"d4": gotak.PlayerBlack,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			board, err := boardAtTurn(g, tc.turn)
			if err != nil {
				t.Fatalf("boardAtTurn(%d): %v", tc.turn, err)
			}
			for sq, stones := range board {
				want, isOccupied := tc.occupied[sq]
				if !isOccupied {
					if len(stones) != 0 {
						t.Errorf("square %s should be empty, got %+v", sq, stones)
					}
					continue
				}
				if len(stones) != 1 {
					t.Errorf("square %s should have one stone, got %+v", sq, stones)
					continue
				}
				if stones[0].Player != want {
					t.Errorf("square %s should be player %d, got %d", sq, want, stones[0].Player)
				}
			}
		})
	}
}

func TestBoardAtTurn_nilGame(t *testing.T) {
	board, err := boardAtTurn(nil, 0)
	if err != nil {
		t.Errorf("nil game should return (nil, nil), got err=%v", err)
	}
	if board != nil {
		t.Errorf("nil game should return (nil, nil), got %d squares", len(board))
	}
}

func TestSnapshotSquares_isDeepCopy(t *testing.T) {
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
	})
	snap := snapshotSquares(g.Board)
	g.Board.Squares["a1"][0].Player = -999
	if snap["a1"][0].Player == -999 {
		t.Errorf("snapshot was a shallow copy; mutation leaked through")
	}
}

func TestApplyHalfTurn_turnOneInversion(t *testing.T) {
	b := &gotak.Board{Size: 5}
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	turn := &gotak.Turn{
		Number: 1,
		First:  &gotak.Move{Stone: gotak.StoneFlat, Square: "a1", Text: "a1"},
		Second: &gotak.Move{Stone: gotak.StoneFlat, Square: "e5", Text: "e5"},
	}
	if err := applyHalfTurn(b, turn, false); err != nil {
		t.Fatalf("apply first: %v", err)
	}
	if err := applyHalfTurn(b, turn, true); err != nil {
		t.Fatalf("apply second: %v", err)
	}
	if b.Squares["a1"][0].Player != gotak.PlayerBlack {
		t.Errorf("turn-1 First should place black stone, got %+v", b.Squares["a1"])
	}
	if b.Squares["e5"][0].Player != gotak.PlayerWhite {
		t.Errorf("turn-1 Second should place white stone, got %+v", b.Squares["e5"])
	}
}

func TestApplyHalfTurn_normalTurn(t *testing.T) {
	b := &gotak.Board{Size: 5}
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	turn := &gotak.Turn{
		Number: 4,
		First:  &gotak.Move{Stone: gotak.StoneFlat, Square: "c3", Text: "c3"},
		Second: &gotak.Move{Stone: gotak.StoneFlat, Square: "d4", Text: "d4"},
	}
	if err := applyHalfTurn(b, turn, false); err != nil {
		t.Fatalf("apply first: %v", err)
	}
	if err := applyHalfTurn(b, turn, true); err != nil {
		t.Fatalf("apply second: %v", err)
	}
	if b.Squares["c3"][0].Player != gotak.PlayerWhite {
		t.Errorf("First should place white stone on non-opening turn")
	}
	if b.Squares["d4"][0].Player != gotak.PlayerBlack {
		t.Errorf("Second should place black stone on non-opening turn")
	}
}

func TestApplyHalfTurn_missingMoveIsNoop(t *testing.T) {
	b := &gotak.Board{Size: 5}
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	turn := &gotak.Turn{Number: 2} // no First, no Second
	if err := applyHalfTurn(b, turn, false); err != nil {
		t.Errorf("missing First should be a no-op, got %v", err)
	}
	if err := applyHalfTurn(b, turn, true); err != nil {
		t.Errorf("missing Second should be a no-op, got %v", err)
	}
}
