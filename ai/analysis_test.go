package ai

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"go.icco.me/gotak"
)

var fastCfg = AIConfig{Level: Intermediate, TimeLimit: 500 * time.Millisecond}

func TestAnalyzeTPS_findsRoadInOne(t *testing.T) {
	cases := []struct {
		name      string
		tps       string
		wantWin   int
		wantScore int64
		wantMover int
	}{
		{"white to move", "x5/x5/x5/2,2,x3/1,1,1,1,x 1 5", gotak.PlayerWhite, ForcedWinScore, gotak.PlayerWhite},
		{"black to move", "x5/x5/x5/1,1,1,x2/2,2,2,2,x 2 5", gotak.PlayerBlack, -ForcedWinScore, gotak.PlayerBlack},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := AnalyzeTPS(context.Background(), tc.tps, fastCfg)
			if err != nil {
				t.Fatalf("AnalyzeTPS: %v", err)
			}
			if !strings.HasSuffix(res.Best, "e1") {
				t.Errorf("best = %q, want a stone on e1 (pv %v)", res.Best, res.PV)
			}
			if res.ForcedWin != tc.wantWin {
				t.Errorf("forced win = %d, want %d", res.ForcedWin, tc.wantWin)
			}
			if res.Score != tc.wantScore {
				t.Errorf("score = %d, want %d", res.Score, tc.wantScore)
			}
			if res.ToMove != tc.wantMover {
				t.Errorf("to move = %d, want %d", res.ToMove, tc.wantMover)
			}
			if res.GameOver {
				t.Error("position is not over yet")
			}
		})
	}
}

func TestAnalyzeTPS_finishedGame(t *testing.T) {
	res, err := AnalyzeTPS(context.Background(), "x5/x5/x5/2,2,x3/1,1,1,1,1 2 5", fastCfg)
	if err != nil {
		t.Fatalf("AnalyzeTPS: %v", err)
	}
	if !res.GameOver || res.Winner != gotak.PlayerWhite {
		t.Errorf("game over = %v winner = %d, want white road", res.GameOver, res.Winner)
	}
	if res.Score != ForcedWinScore || res.Best != "" {
		t.Errorf("score = %d best = %q, want %d and no move", res.Score, res.Best, ForcedWinScore)
	}
}

func TestAnalyzeTPS_rejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		tps  string
	}{
		{"empty", ""},
		{"two fields", "x5/x5/x5/x5/x5 1"},
		{"nine by nine", "x9/x9/x9/x9/x9/x9/x9/x9/x9 1 1"},
		{"empty row", "/x3/x3 1 1"},
		{"empty cell", "x5/x5/x5/x5/,x4 1 2"},
		{"bare standing marker", "x5/x5/x5/x5/S,x4 1 2"},
		{"bare capstone marker", "x5/x5/x5/x5/C,x4 1 2"},
		{"oversized stack", "x5/x5/x5/x5/1111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111,x4 1 2"},
		{"too many white stones", "x5/x5/x5/x5/1111111111111111111111,x4 1 2"},
		{"two capstones on 5x5", "x5/x5/x5/x5/1C,1C,x3 1 2"},
		{"stack taller than the engine tracks", "x8/x8/x8/x8/x8/x8/x8/212121212121212121212121212121212121212121212121212121212121212121,x7 1 40"},
		{"ragged row", "x5/x5/x5/x5/x4 1 1"},
		{"bad player", "x5/x5/x5/x5/x5 3 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AnalyzeTPS(context.Background(), tc.tps, fastCfg); err == nil {
				t.Errorf("AnalyzeTPS(%q) should fail", tc.tps)
			}
		})
	}
}

func TestAnalyzer_sizeMismatch(t *testing.T) {
	a, err := NewAnalyzer(6, fastCfg)
	if err != nil {
		t.Fatalf("NewAnalyzer: %v", err)
	}
	if _, err := a.EvaluateTPS(context.Background(), "x5/x5/x5/x5/x5 1 1"); err == nil {
		t.Error("5x5 tps on a 6x6 analyzer should fail")
	}
	g, err := gotak.NewGame(5, 1, "alice-vs-bob")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	if _, err := a.Evaluate(context.Background(), g); err == nil {
		t.Error("5x5 game on a 6x6 analyzer should fail")
	}
}

func TestAnalyzer_evaluateGame(t *testing.T) {
	g, err := gotak.NewGame(5, 1, "alice-vs-bob")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	if err := g.DoTurn("a1", "e5"); err != nil {
		t.Fatalf("DoTurn: %v", err)
	}
	a, err := NewAnalyzer(5, fastCfg)
	if err != nil {
		t.Fatalf("NewAnalyzer: %v", err)
	}
	res, err := a.Evaluate(context.Background(), g)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Best == "" || len(res.PV) == 0 || res.Depth == 0 {
		t.Errorf("want a searched move, got %+v", res)
	}
	if res.ToMove != gotak.PlayerWhite {
		t.Errorf("to move = %d, want white", res.ToMove)
	}
	if res.Score <= -ForcedWinScore || res.Score >= ForcedWinScore {
		t.Errorf("opening score %d should not be a forced result", res.Score)
	}
	if res.TPS != "x4,1/x5/x5/x5/2,x4 1 2" {
		t.Errorf("tps = %q", res.TPS)
	}
	if _, _, _, err := gotak.ParseTPS(res.TPS); err != nil {
		t.Errorf("gotak cannot read the engine's tps %q: %v", res.TPS, err)
	}
}

func TestCompareMovesTPS(t *testing.T) {
	cmp, err := CompareMovesTPS(context.Background(), "x5/x5/x5/2,2,x3/1,1,1,1,x 1 5",
		[]string{"a3", "nonsense", "e1", "a1"}, fastCfg)
	if err != nil {
		t.Fatalf("CompareMovesTPS: %v", err)
	}
	if len(cmp.Moves) != 4 {
		t.Fatalf("got %d moves, want 4", len(cmp.Moves))
	}
	if got := cmp.Moves[0]; got.Move != "e1" || got.Loss != 0 || got.Score != ForcedWinScore {
		t.Errorf("first = %+v, want e1 with no loss", got)
	}
	if got := cmp.Moves[1]; got.Move != "a3" || got.Loss <= 0 || got.Error != "" {
		t.Errorf("second = %+v, want a3 with a loss", got)
	}
	for _, m := range cmp.Moves[2:] {
		if m.Error == "" {
			t.Errorf("%q should carry an error: %+v", m.Move, m)
		}
	}
	if !strings.HasSuffix(cmp.Position.Best, "e1") {
		t.Errorf("position best = %q, want a stone on e1", cmp.Position.Best)
	}
}

func TestCompareMovesTPS_limits(t *testing.T) {
	tps := "x5/x5/x5/x5/x5 1 1"
	if _, err := CompareMovesTPS(context.Background(), tps, nil, fastCfg); err == nil {
		t.Error("no moves should fail")
	}
	many := make([]string, MaxCompareMoves+1)
	for i := range many {
		many[i] = "a1"
	}
	if _, err := CompareMovesTPS(context.Background(), tps, many, fastCfg); err == nil {
		t.Error("too many moves should fail")
	}
	if _, err := CompareMovesTPS(context.Background(), "x5/x5/x5/2,2,x3/1,1,1,1,1 2 5", []string{"a3"}, fastCfg); err == nil {
		t.Error("finished position should fail")
	}
}

func TestAnalyzeTPS_roadThreats(t *testing.T) {
	cases := []struct {
		name       string
		tps        string
		wantWhite  []string
		wantBlacks []string
	}{
		{"both sides one away", "x5/x5/x5/2,2,2,2,x/1,1,1,1,x 1 5", []string{"e1"}, []string{"e2"}},
		{"two gaps for white", "x5/x5/x5/x5/1,1,x,1,x 2 4", []string{}, []string{}},
		{"vertical road", "1,x4/1,x4/x5/1,x4/1,x4 2 5", []string{"a3"}, []string{}},
		{"empty board", "x5/x5/x5/x5/x5 1 1", []string{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := AnalyzeTPS(context.Background(), tc.tps, fastCfg)
			if err != nil {
				t.Fatalf("AnalyzeTPS: %v", err)
			}
			if strings.Join(res.Threats.White, ",") != strings.Join(tc.wantWhite, ",") {
				t.Errorf("white threats = %v, want %v", res.Threats.White, tc.wantWhite)
			}
			if strings.Join(res.Threats.Black, ",") != strings.Join(tc.wantBlacks, ",") {
				t.Errorf("black threats = %v, want %v", res.Threats.Black, tc.wantBlacks)
			}
		})
	}
}

func TestAnalyzeTPS_endgamePatterns(t *testing.T) {
	cases := []struct {
		name         string
		tps          string
		wantPatterns []string
		omitPatterns []string
	}{
		{
			name:         "tak present and double tak absent",
			tps:          "x5/x5/x5/2,2,2,2,x/1,1,1,1,x 1 5",
			wantPatterns: []string{PatternTak, PatternWinningLine},
			omitPatterns: []string{PatternDoubleTak, PatternTinue, PatternFlatRace},
		},
		{
			name:         "double tak and tinue present",
			tps:          "x4/1,1,1,x/1,1,1,x/x4 2 5",
			wantPatterns: []string{PatternTak, PatternDoubleTak, PatternTinue},
			omitPatterns: []string{PatternWinningLine, PatternFlatRace},
		},
		{
			name:         "winning line present",
			tps:          "x4/1,1,1,x/1,1,1,x/x4 1 5",
			wantPatterns: []string{PatternTak, PatternDoubleTak, PatternWinningLine},
			omitPatterns: []string{PatternTinue, PatternFlatRace},
		},
		{
			name:         "flat race present via low empty squares",
			tps:          "1,2,1,2/2,1,2,1/1,2,1,2/2,x3 1 10",
			wantPatterns: []string{PatternFlatRace},
			omitPatterns: []string{PatternTak, PatternDoubleTak, PatternTinue, PatternWinningLine},
		},
		{
			name:         "flat race present via low reserves",
			tps:          "x4/121,21,12,x/21,12,21,12/12,21,12,21 1 15",
			wantPatterns: []string{PatternFlatRace},
			omitPatterns: []string{PatternTak, PatternDoubleTak, PatternTinue, PatternWinningLine},
		},
		{
			name:         "quiet opening position has no patterns",
			tps:          "x5/x5/x5/x5/x5 1 1",
			wantPatterns: []string{},
			omitPatterns: []string{PatternTak, PatternDoubleTak, PatternTinue, PatternWinningLine, PatternFlatRace},
		},
		{
			name:         "completed game has no patterns",
			tps:          "x5/x5/x5/2,2,x3/1,1,1,1,1 2 5",
			wantPatterns: []string{},
			omitPatterns: []string{PatternTak, PatternDoubleTak, PatternTinue, PatternWinningLine, PatternFlatRace},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := AnalyzeTPS(context.Background(), tc.tps, fastCfg)
			if err != nil {
				t.Fatalf("AnalyzeTPS: %v", err)
			}
			if res.Patterns == nil {
				t.Fatalf("patterns should not be nil")
			}
			for _, p := range tc.wantPatterns {
				if !slices.Contains(res.Patterns, p) {
					t.Errorf("patterns %v missing expected %q", res.Patterns, p)
				}
			}
			for _, p := range tc.omitPatterns {
				if slices.Contains(res.Patterns, p) {
					t.Errorf("patterns %v should not contain %q", res.Patterns, p)
				}
			}
		})
	}
}

func TestAnalyzer_repeatedPositionKeepsScore(t *testing.T) {
	for _, level := range []DifficultyLevel{Beginner, Intermediate} {
		a, err := NewAnalyzer(5, AIConfig{Level: level, TimeLimit: time.Second})
		if err != nil {
			t.Fatalf("NewAnalyzer: %v", err)
		}
		tps := "x5/x5/x2,1,x2/x5/2,x4 2 2"
		first, err := a.EvaluateTPS(context.Background(), tps)
		if err != nil {
			t.Fatalf("first: %v", err)
		}
		second, err := a.EvaluateTPS(context.Background(), tps)
		if err != nil {
			t.Fatalf("second: %v", err)
		}
		if first.Score == 0 {
			t.Fatalf("level %d: want a nonzero score to compare, got 0", level)
		}
		if second.Score != first.Score {
			t.Errorf("level %d: repeated position scored %d then %d", level, first.Score, second.Score)
		}
	}
}

func TestAnalyzer_canceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AnalyzeTPS(ctx, "x5/x5/x5/x5/x5 1 1", fastCfg); err == nil {
		t.Error("canceled context should fail")
	}
}

func TestLoss(t *testing.T) {
	cases := []struct {
		name          string
		before, after int64
		player        int
		want          int64
	}{
		{"white loses ground", 300, -100, gotak.PlayerWhite, 400},
		{"white gains reads as zero", 100, 300, gotak.PlayerWhite, 0},
		{"black loses ground", -300, 100, gotak.PlayerBlack, 400},
		{"black gains reads as zero", 100, -300, gotak.PlayerBlack, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Loss(tc.before, tc.after, tc.player); got != tc.want {
				t.Errorf("Loss(%d, %d, %d) = %d, want %d", tc.before, tc.after, tc.player, got, tc.want)
			}
		})
	}
}

func TestParseUserMove(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"a1", false},
		{"Ce5", false},
		{"3b2>12", false},
		{"5a1+", false},
		{"a6", true},
		{"f1", true},
		{"6a1>", true},
		{"9a1>", true},
		{"99999999a1>", true},
		{"", true},
	}
	for _, tc := range cases {
		_, err := parseUserMove(tc.in, 5)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseUserMove(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
	}
}

func TestSameMove(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"a1", "a1", true},
		{"a1>", "1a1>1", true},
		{"2a1>", "2a1>2", true},
		{"2a1>", "2a1>11", false},
		{"3b2>12C", "3b2>12", true},
		{"a1", "Sa1", false},
		{"a1", "", false},
		{"junk", "junk2", false},
	}
	for _, tc := range cases {
		if got := SameMove(tc.a, tc.b, 5); got != tc.want {
			t.Errorf("SameMove(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
