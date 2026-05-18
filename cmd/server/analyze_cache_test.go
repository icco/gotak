package main

import (
	"testing"

	"github.com/icco/gotak"
	"go.uber.org/zap"
)

func cacheKey(gameID int64, level, style string, timeNs int64, version string) analysisCacheKey {
	return analysisCacheKey{
		gameID:      gameID,
		level:       level,
		style:       style,
		timeLimitNs: timeNs,
		gameVersion: version,
	}
}

func TestAnalysisCache_roundTrip(t *testing.T) {
	db := setupTestDB(t)
	l := zap.NewNop().Sugar()

	moves := []MoveAnalysis{
		{Turn: 1, Player: gotak.PlayerWhite, Played: "a1", Best: "a1", Agreed: true},
		{Turn: 1, Player: gotak.PlayerBlack, Played: "e5", Best: "d4", Agreed: false},
	}
	key := cacheKey(42, "advanced", "balanced", 2e9, "v1:moves=2")
	saveAnalysisCache(db, l, key, 1, moves)

	got, ok := loadAnalysisCache(db, l, key)
	if !ok {
		t.Fatal("expected cache hit immediately after save")
	}
	if got.Agreed != 1 {
		t.Errorf("agreed = %d, want 1", got.Agreed)
	}
	if got.MoveCount != 2 {
		t.Errorf("move count = %d, want 2", got.MoveCount)
	}
	if len(got.Moves) != 2 || got.Moves[1].Best != "d4" {
		t.Errorf("moves = %+v, want round-trip of original", got.Moves)
	}
}

func TestAnalysisCache_keyVariantsMiss(t *testing.T) {
	db := setupTestDB(t)
	l := zap.NewNop().Sugar()

	base := cacheKey(42, "advanced", "balanced", 2e9, "v1:moves=2")
	saveAnalysisCache(db, l, base, 0, nil)

	cases := []struct {
		name string
		key  analysisCacheKey
	}{
		{"different game", cacheKey(43, "advanced", "balanced", 2e9, "v1:moves=2")},
		{"different level", cacheKey(42, "expert", "balanced", 2e9, "v1:moves=2")},
		{"different style", cacheKey(42, "advanced", "aggressive", 2e9, "v1:moves=2")},
		{"different time limit", cacheKey(42, "advanced", "balanced", 5e9, "v1:moves=2")},
		{"game grew", cacheKey(42, "advanced", "balanced", 2e9, "v1:moves=3")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := loadAnalysisCache(db, l, c.key); ok {
				t.Errorf("%s should miss but hit", c.name)
			}
		})
	}
}

func TestAnalysisCache_concurrentWritesNoError(t *testing.T) {
	db := setupTestDB(t)
	l := zap.NewNop().Sugar()
	key := cacheKey(99, "advanced", "balanced", 2e9, "v1:moves=0")

	// Two writes on the same key would violate the unique index without the
	// ON CONFLICT clause.
	saveAnalysisCache(db, l, key, 0, nil)
	saveAnalysisCache(db, l, key, 0, nil)

	if _, ok := loadAnalysisCache(db, l, key); !ok {
		t.Errorf("cache hit expected after second save")
	}
}

func TestGameCacheVersion_changesWithMoveCount(t *testing.T) {
	g := playGame(t, 5, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
	})
	v1 := gameCacheVersion(g)
	if err := g.DoSingleMove("e5", gotak.PlayerBlack); err != nil {
		t.Fatal(err)
	}
	v2 := gameCacheVersion(g)
	if v1 == v2 {
		t.Errorf("game version did not change after a new move: %q", v1)
	}
}
