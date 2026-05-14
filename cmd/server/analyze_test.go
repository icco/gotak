package main

import (
	"context"
	"testing"
	"time"

	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
)

// stubEngine returns a hard-coded move per call, in order, so tests can
// assert how the analyzer drives the engine.
type stubEngine struct {
	moves []string
	calls int
}

func (s *stubEngine) GetMove(_ context.Context, _ *gotak.Game, _ ai.AIConfig) (string, error) {
	if s.calls >= len(s.moves) {
		s.calls++
		return "", nil
	}
	m := s.moves[s.calls]
	s.calls++
	return m, nil
}

func (s *stubEngine) ExplainMove(_ context.Context, _ *gotak.Game, _ ai.AIConfig) (string, error) {
	return "", nil
}

func mustGame(t *testing.T, size int64, moves []struct {
	player int
	move   string
}) *gotak.Game {
	t.Helper()
	g, err := gotak.NewGame(size, 1, "t")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range moves {
		if err := g.DoSingleMove(m.move, m.player); err != nil {
			t.Fatalf("DoSingleMove(%q, %d): %v", m.move, m.player, err)
		}
	}
	return g
}

func TestGameBeforePly_prefixesTurns(t *testing.T) {
	g := mustGame(t, 5, []struct {
		player int
		move   string
	}{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	// Before ply 0 (turn 1 first): no turns.
	pre, err := gameBeforePly(g, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre.Turns) != 0 {
		t.Errorf("before ply 0: got %d turns, want 0", len(pre.Turns))
	}

	// Before ply 1 (turn 1 second): turn 1 with First only.
	pre, err = gameBeforePly(g, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre.Turns) != 1 || pre.Turns[0].First == nil || pre.Turns[0].Second != nil {
		t.Errorf("before ply 1: got turns %+v, want 1 turn with First only", pre.Turns)
	}

	// Before ply 2 (turn 2 first): turn 1 complete.
	pre, err = gameBeforePly(g, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre.Turns) != 1 || pre.Turns[0].Second == nil {
		t.Errorf("before ply 2: got turns %+v, want 1 complete turn", pre.Turns)
	}

	// Before ply 3 (turn 2 second): turn 1 complete + turn 2 First only.
	pre, err = gameBeforePly(g, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre.Turns) != 2 || pre.Turns[1].Second != nil {
		t.Errorf("before ply 3: got turns %+v, want 2 turns with second partial", pre.Turns)
	}

	// Original game unchanged.
	if len(g.Turns) != 2 {
		t.Errorf("original game mutated: len=%d, want 2", len(g.Turns))
	}
}

func TestAnalyzeGame_recordsAgreement(t *testing.T) {
	g := mustGame(t, 5, []struct {
		player int
		move   string
	}{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	// Engine claims b2 was best for plies 0 and 2 (so ply 2 agrees, ply 0
	// disagrees), and matches the played move on plies 1 and 3.
	engine := &stubEngine{moves: []string{"b2", "e5", "b2", "d4"}}

	plies := analyzeGame(context.Background(), engine, g, ai.AIConfig{TimeLimit: 100 * time.Millisecond})

	if len(plies) != 4 {
		t.Fatalf("got %d plies, want 4", len(plies))
	}

	want := []struct {
		played string
		best   string
		agreed bool
		player int
	}{
		{"a1", "b2", false, gotak.PlayerWhite},
		{"e5", "e5", true, gotak.PlayerBlack},
		{"b2", "b2", true, gotak.PlayerWhite},
		{"d4", "d4", true, gotak.PlayerBlack},
	}
	for i, w := range want {
		if plies[i].Played != w.played || plies[i].Best != w.best || plies[i].Agreed != w.agreed || plies[i].Player != w.player {
			t.Errorf("ply %d = %+v, want %+v", i, plies[i], w)
		}
	}

	if engine.calls != 4 {
		t.Errorf("engine called %d times, want 4", engine.calls)
	}
}

func TestAnalyzeGame_emptyGame(t *testing.T) {
	g, _ := gotak.NewGame(5, 1, "t")
	engine := &stubEngine{}
	plies := analyzeGame(context.Background(), engine, g, ai.AIConfig{})
	if len(plies) != 0 {
		t.Errorf("empty game produced %d plies, want 0", len(plies))
	}
	if engine.calls != 0 {
		t.Errorf("engine called %d times on empty game, want 0", engine.calls)
	}
}

func TestAnalyzeConfigFromRequest_defaults(t *testing.T) {
	cfg := analyzeConfigFromRequest(AnalyzeRequest{})
	if cfg.Level != ai.Advanced {
		t.Errorf("default level = %v, want Advanced", cfg.Level)
	}
	if cfg.Style != ai.Balanced {
		t.Errorf("default style = %v, want Balanced", cfg.Style)
	}
	if cfg.TimeLimit != 2*time.Second {
		t.Errorf("default time limit = %v, want 2s", cfg.TimeLimit)
	}
}

func TestAnalyzeConfigFromRequest_overrides(t *testing.T) {
	cfg := analyzeConfigFromRequest(AnalyzeRequest{
		Level:     "beginner",
		Style:     "aggressive",
		TimeLimit: 5 * time.Second,
	})
	if cfg.Level != ai.Beginner {
		t.Errorf("level = %v, want Beginner", cfg.Level)
	}
	if cfg.Style != ai.Aggressive {
		t.Errorf("style = %v, want Aggressive", cfg.Style)
	}
	if cfg.TimeLimit != 5*time.Second {
		t.Errorf("time = %v, want 5s", cfg.TimeLimit)
	}
}
