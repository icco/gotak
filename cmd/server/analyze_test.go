package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.icco.me/gotak"
	"go.icco.me/gotak/ai"
)

// scriptedMove and the playGame helper live in replay_test.go and are
// shared across the cmd/server test binary.

// stubEvaluator returns the next scripted analysis on each call, so tests
// can assert how game analysis drives the engine. A nil entry is an error.
type stubEvaluator struct {
	results []*ai.PositionAnalysis
	calls   int
}

func (s *stubEvaluator) Evaluate(_ context.Context, _ *gotak.Game) (*ai.PositionAnalysis, error) {
	i := s.calls
	s.calls++
	if i >= len(s.results) || s.results[i] == nil {
		return nil, errors.New("stub has no result")
	}
	return s.results[i], nil
}

func eval(best string, score int64) *ai.PositionAnalysis {
	return &ai.PositionAnalysis{Best: best, PV: []string{best}, Score: score, TPS: "x5/x5/x5/x5/x5 1 1"}
}

func TestGameBeforeMove_prefixesTurns(t *testing.T) {
	g := playGame(t, []scriptedMove{
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

func TestAnalyzeGame_scoresEveryMove(t *testing.T) {
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	// One evaluation before each move plus the final position.
	ev := &stubEvaluator{results: []*ai.PositionAnalysis{
		eval("b2", 0),
		eval("e5", 50),
		eval("c3", 0),
		eval("d4", -1000),
		eval("a2", -900),
	}}

	moves := analyzeGame(context.Background(), ev, g)

	if ev.calls != 5 {
		t.Errorf("evaluator called %d times, want 5", ev.calls)
	}
	want := []struct {
		played     string
		best       string
		agreed     bool
		player     int
		score      int64
		scoreAfter int64
		loss       int64
		quality    string
	}{
		{"a1", "b2", false, gotak.PlayerWhite, 0, 50, 0, QualityGood},
		{"e5", "e5", true, gotak.PlayerBlack, 50, 0, 0, QualityBest},
		{"b2", "c3", false, gotak.PlayerWhite, 0, -1000, 1000, QualityBlunder},
		{"d4", "d4", true, gotak.PlayerBlack, -1000, -900, 0, QualityBest},
	}
	if len(moves) != len(want) {
		t.Fatalf("got %d moves, want %d", len(moves), len(want))
	}
	for i, w := range want {
		got := moves[i]
		if got.Played != w.played || got.Best != w.best || got.Agreed != w.agreed || got.Player != w.player ||
			got.Score != w.score || got.ScoreAfter != w.scoreAfter || got.Loss != w.loss || got.Quality != w.quality {
			t.Errorf("move %d = %+v, want %+v", i, got, w)
		}
		if got.Error != "" {
			t.Errorf("move %d has error %q", i, got.Error)
		}
	}
}

func TestAnalyzeGame_evaluatorErrors(t *testing.T) {
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
	})
	ev := &stubEvaluator{results: []*ai.PositionAnalysis{eval("a1", 0), nil, eval("b2", 0)}}

	moves := analyzeGame(context.Background(), ev, g)
	if len(moves) != 2 {
		t.Fatalf("got %d moves, want 2", len(moves))
	}
	if moves[0].Error == "" || moves[0].Best != "a1" {
		t.Errorf("move 0 should keep its best move but flag the missing after-score: %+v", moves[0])
	}
	if moves[1].Error == "" {
		t.Errorf("move 1 should carry the evaluator error: %+v", moves[1])
	}
}

func TestAnalyzeGame_emptyGame(t *testing.T) {
	g, _ := gotak.NewGame(5, 1, "t")
	ev := &stubEvaluator{}
	moves := analyzeGame(context.Background(), ev, g)
	if len(moves) != 0 {
		t.Errorf("empty game produced %d moves, want 0", len(moves))
	}
	if ev.calls != 0 {
		t.Errorf("evaluator called %d times on empty game, want 0", ev.calls)
	}
}

func TestAnalyzeGame_canceledContext(t *testing.T) {
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ev := &stubEvaluator{results: []*ai.PositionAnalysis{eval("a1", 0), eval("e5", 0), eval("b2", 0)}}
	moves := analyzeGame(ctx, ev, g)

	if len(moves) != 2 {
		t.Fatalf("want 2 entries (one per move), got %d", len(moves))
	}
	for i, m := range moves {
		if m.Error == "" {
			t.Errorf("move %d should have Error set when context is canceled, got %+v", i, m)
		}
	}
	if ev.calls != 0 {
		t.Errorf("evaluator called %d times after cancel, want 0", ev.calls)
	}
}

func TestAnalyzeGame_realEngine(t *testing.T) {
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b1"},
	})
	analyzer, err := ai.NewAnalyzer(5, ai.AIConfig{Level: ai.Intermediate, TimeLimit: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewAnalyzer: %v", err)
	}
	moves := analyzeGame(context.Background(), analyzer, g)
	if len(moves) != 3 {
		t.Fatalf("got %d moves, want 3", len(moves))
	}
	for i, m := range moves {
		if m.Error != "" || m.Best == "" || m.TPS == "" || m.Quality == "" {
			t.Errorf("move %d incomplete: %+v", i, m)
		}
	}
}

func TestAnalyzeGame_unsupportedSize(t *testing.T) {
	g, err := gotak.NewGame(9, 1, "big")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	if err := g.DoSingleMove("a1", gotak.PlayerWhite); err != nil {
		t.Fatalf("DoSingleMove: %v", err)
	}
	ev := &stubEvaluator{}
	moves := analyzeGame(context.Background(), ev, g)
	if len(moves) != 1 || moves[0].Error == "" {
		t.Errorf("9x9 move should carry an error, got %+v", moves)
	}
	if ev.calls != 0 {
		t.Errorf("evaluator called %d times on 9x9, want 0", ev.calls)
	}
}

func TestMoveQuality(t *testing.T) {
	cases := []struct {
		agreed bool
		loss   int64
		want   string
	}{
		{true, 0, QualityBest},
		{false, 0, QualityGood},
		{false, 99, QualityGood},
		{false, 100, QualityInaccuracy},
		{false, 300, QualityMistake},
		{false, 800, QualityBlunder},
		{false, ai.ForcedWinScore, QualityBlunder},
	}
	for _, c := range cases {
		if got := moveQuality(c.agreed, c.loss); got != c.want {
			t.Errorf("moveQuality(%v, %d) = %q, want %q", c.agreed, c.loss, got, c.want)
		}
	}
}

func TestCriticalMoments(t *testing.T) {
	moves := []MoveAnalysis{
		{Played: "a1", Loss: 0, Quality: QualityBest},
		{Played: "b1", Loss: 150, Quality: QualityInaccuracy},
		{Played: "c1", Loss: 2000, Quality: QualityBlunder},
		{Played: "d1", Loss: 50, Quality: QualityGood},
		{Played: "e1", Loss: 500, Quality: QualityMistake},
		{Played: "e2", Loss: 120, Quality: QualityInaccuracy},
		{Played: "e3", Error: "canceled"},
	}
	got := criticalMoments(moves, 3)
	if len(got) != 3 {
		t.Fatalf("got %d moments, want 3: %+v", len(got), got)
	}
	wantIdx := []int{2, 4, 1}
	for i, w := range wantIdx {
		if got[i].Index != w || got[i].Played != moves[w].Played {
			t.Errorf("moment %d = %+v, want index %d", i, got[i], w)
		}
	}
	if none := criticalMoments(moves[:1], 3); none == nil || len(none) != 0 {
		t.Errorf("clean game should give an empty, non-nil list, got %#v", none)
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
