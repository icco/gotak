package main

import (
	"context"
	"testing"
	"time"

	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
)

// scriptedMove and the playGame helper live in replay_test.go and are
// shared across the cmd/server test binary.

// stubEngine returns the next hard-coded move on each call, so tests can
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

func TestGameBeforeMove_prefixesTurns(t *testing.T) {
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	cases := []struct {
		name          string
		turnIdx       int
		isSecond      bool
		wantTurns     int
		wantSecondNil bool // wantSecondNil applies to the last turn in the prefix
	}{
		{"before turn-1 first move", 0, false, 0, false},
		{"before turn-1 second move", 0, true, 1, true},
		{"before turn-2 first move", 1, false, 1, false},
		{"before turn-2 second move", 1, true, 2, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pre, err := gameBeforeMove(g, tc.turnIdx, tc.isSecond)
			if err != nil {
				t.Fatalf("gameBeforeMove: %v", err)
			}
			if len(pre.Turns) != tc.wantTurns {
				t.Fatalf("got %d turns, want %d (%+v)", len(pre.Turns), tc.wantTurns, pre.Turns)
			}
			if tc.wantTurns > 0 {
				last := pre.Turns[tc.wantTurns-1]
				if tc.wantSecondNil && last.Second != nil {
					t.Errorf("last turn should have Second=nil, got %+v", last)
				}
				if !tc.wantSecondNil && last.Second == nil {
					t.Errorf("last turn should have Second set, got %+v", last)
				}
			}
		})
	}

	// Original game unchanged.
	if len(g.Turns) != 2 {
		t.Errorf("original game mutated: len=%d, want 2", len(g.Turns))
	}
}

func TestGameBeforeMove_nilGame(t *testing.T) {
	if _, err := gameBeforeMove(nil, 0, false); err == nil {
		t.Errorf("nil game should return error")
	}
}

func TestAnalyzeGame_recordsAgreement(t *testing.T) {
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	// Engine claims b2 for move 0 (disagrees with a1) and matches the
	// player on the other three.
	engine := &stubEngine{moves: []string{"b2", "e5", "b2", "d4"}}

	moves := analyzeGame(context.Background(), engine, g, ai.AIConfig{TimeLimit: 100 * time.Millisecond})

	if len(moves) != 4 {
		t.Fatalf("got %d moves, want 4", len(moves))
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
		got := moves[i]
		if got.Played != w.played || got.Best != w.best || got.Agreed != w.agreed || got.Player != w.player {
			t.Errorf("move %d = %+v, want %+v", i, got, w)
		}
	}

	if engine.calls != 4 {
		t.Errorf("engine called %d times, want 4", engine.calls)
	}
}

func TestAnalyzeGame_emptyGame(t *testing.T) {
	g, _ := gotak.NewGame(5, 1, "t")
	engine := &stubEngine{}
	moves := analyzeGame(context.Background(), engine, g, ai.AIConfig{})
	if len(moves) != 0 {
		t.Errorf("empty game produced %d moves, want 0", len(moves))
	}
	if engine.calls != 0 {
		t.Errorf("engine called %d times on empty game, want 0", engine.calls)
	}
}

func TestAnalyzeGame_canceledContext(t *testing.T) {
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before first call

	engine := &stubEngine{moves: []string{"a1", "e5"}}
	moves := analyzeGame(ctx, engine, g, ai.AIConfig{})

	if len(moves) != 2 {
		t.Fatalf("want 2 entries (one per move), got %d", len(moves))
	}
	for i, m := range moves {
		if m.Error == "" {
			t.Errorf("move %d should have Error set when context is canceled, got %+v", i, m)
		}
	}
}

func TestAnalyzeConfigFromRequest_defaults(t *testing.T) {
	cfg, name := analyzeConfigFromRequest(AnalyzeRequest{})
	if cfg.Level != ai.Advanced {
		t.Errorf("default level = %v, want Advanced", cfg.Level)
	}
	if name != "advanced" {
		t.Errorf("default level name = %q, want advanced", name)
	}
	if cfg.Style != ai.Balanced {
		t.Errorf("default style = %v, want Balanced", cfg.Style)
	}
	if cfg.TimeLimit != defaultAnalyzeTimeLimit {
		t.Errorf("default time limit = %v, want %v", cfg.TimeLimit, defaultAnalyzeTimeLimit)
	}
}

func TestAnalyzeConfigFromRequest_overrides(t *testing.T) {
	cfg, name := analyzeConfigFromRequest(AnalyzeRequest{
		Level:     "beginner",
		Style:     "aggressive",
		TimeLimit: 5 * time.Second,
	})
	if cfg.Level != ai.Beginner {
		t.Errorf("level = %v, want Beginner", cfg.Level)
	}
	if name != "beginner" {
		t.Errorf("level name = %q, want beginner", name)
	}
	if cfg.Style != ai.Aggressive {
		t.Errorf("style = %v, want Aggressive", cfg.Style)
	}
	if cfg.TimeLimit != 5*time.Second {
		t.Errorf("time = %v, want 5s", cfg.TimeLimit)
	}
}

func TestAnalyzeConfigFromRequest_unknownLevelDefaults(t *testing.T) {
	cfg, name := analyzeConfigFromRequest(AnalyzeRequest{Level: "godlike"})
	if cfg.Level != ai.Advanced || name != "advanced" {
		t.Errorf("unknown level should fall back to advanced, got %v/%q", cfg.Level, name)
	}
}
