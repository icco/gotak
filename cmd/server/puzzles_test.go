package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"go.icco.me/gotak/ai"
	"go.uber.org/zap"
)

func TestMinePuzzles_identifiesWinningLineAndMissedWin(t *testing.T) {
	// Game with 4 plies:
	// ply 0: Turn 1 White "a1"
	// ply 1: Turn 1 Black "e5"
	// ply 2: Turn 2 White "b2"
	// ply 3: Turn 2 Black "d4"
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b2"},
		{gotak.PlayerBlack, "d4"},
	})

	// evals has len(plies) + 1 = 5
	// ply 0 (White to move): no forced win
	// ply 1 (Black to move): ForcedWin = PlayerBlack, PV = ["e5", "b2", "d4"]. Next ply (ply 2): ForcedWin = 0 -> missed_win!
	// ply 2 (White to move): no forced win
	// ply 3 (Black to move): ForcedWin = PlayerBlack, PV = ["d4", "c3", "e1"]. Next ply (ply 4): ForcedWin = PlayerBlack -> winning_line
	// ply 4 (final): ForcedWin = PlayerBlack
	ev := &stubEvaluator{
		results: []*ai.PositionAnalysis{
			{Best: "a1", PV: []string{"a1"}, Score: 0, TPS: "tps0"},
			{Best: "e5", PV: []string{"e5", "b2", "d4"}, Score: -ai.ForcedWinScore, ForcedWin: gotak.PlayerBlack, Depth: 3, TPS: "tps1"},
			{Best: "b2", PV: []string{"b2"}, Score: 0, TPS: "tps2"},
			{Best: "d4", PV: []string{"d4", "c3", "e1"}, Score: -ai.ForcedWinScore, ForcedWin: gotak.PlayerBlack, Depth: 4, TPS: "tps3"},
			{Best: "c3", PV: []string{"c3"}, Score: -ai.ForcedWinScore, ForcedWin: gotak.PlayerBlack, Depth: 4, TPS: "tps4"},
		},
	}

	puzzles := minePuzzles(context.Background(), ev, g)
	if len(puzzles) != 2 {
		t.Fatalf("got %d puzzles, want 2: %+v", len(puzzles), puzzles)
	}

	// First puzzle: ply 1, missed_win
	p1 := puzzles[0]
	if p1.Ply != 1 || p1.Player != gotak.PlayerBlack || p1.Source != "missed_win" {
		t.Errorf("p1 unexpected: %+v", p1)
	}
	var sol1 []string
	if err := json.Unmarshal([]byte(p1.Solution), &sol1); err != nil {
		t.Fatalf("p1 solution unmarshal: %v", err)
	}
	if len(sol1) != 3 || sol1[0] != "e5" || sol1[2] != "d4" {
		t.Errorf("p1 solution = %v, want [e5 b2 d4]", sol1)
	}

	// Second puzzle: ply 3, winning_line
	p2 := puzzles[1]
	if p2.Ply != 3 || p2.Player != gotak.PlayerBlack || p2.Source != "winning_line" {
		t.Errorf("p2 unexpected: %+v", p2)
	}
	var sol2 []string
	if err := json.Unmarshal([]byte(p2.Solution), &sol2); err != nil {
		t.Fatalf("p2 solution unmarshal: %v", err)
	}
	if len(sol2) != 3 || sol2[0] != "d4" {
		t.Errorf("p2 solution = %v, want [d4 c3 e1]", sol2)
	}
}

func TestMinePuzzles_deduplicatesSequence(t *testing.T) {
	// Game where White has ForcedWin across White (ply 0), Black (ply 1), and White (ply 2).
	// It should emit only ONE puzzle (for ply 0) and not a second puzzle for ply 2.
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
		{gotak.PlayerWhite, "b1"},
	})

	// ply 0 (White): ForcedWin = White, PV = ["a1", "e5", "b1"]
	// ply 1 (Black): ForcedWin = White
	// ply 2 (White): ForcedWin = White, PV = ["b1"]
	// ply 3 (final): GameOver = true, Winner = White
	ev := &stubEvaluator{
		results: []*ai.PositionAnalysis{
			{Best: "a1", PV: []string{"a1", "e5", "b1"}, Score: ai.ForcedWinScore, ForcedWin: gotak.PlayerWhite, Depth: 3, TPS: "tps0"},
			{Best: "e5", PV: []string{"e5"}, Score: ai.ForcedWinScore, ForcedWin: gotak.PlayerWhite, Depth: 2, TPS: "tps1"},
			{Best: "b1", PV: []string{"b1"}, Score: ai.ForcedWinScore, ForcedWin: gotak.PlayerWhite, Depth: 1, TPS: "tps2"},
			{Best: "", PV: []string{}, Score: ai.ForcedWinScore, GameOver: true, Winner: gotak.PlayerWhite, TPS: "tps3"},
		},
	}

	puzzles := minePuzzles(context.Background(), ev, g)
	if len(puzzles) != 1 {
		t.Fatalf("got %d puzzles, want 1: %+v", len(puzzles), puzzles)
	}
	if puzzles[0].Ply != 0 {
		t.Errorf("expected puzzle at ply 0, got %d", puzzles[0].Ply)
	}
	if puzzles[0].Source != "winning_line" {
		t.Errorf("expected source winning_line for terminal win, got %q", puzzles[0].Source)
	}
}

func TestMinePuzzles_realEngineRoadInOne(t *testing.T) {
	// Build a game where the position before the final move is a road-in-one for White.
	// We can replay moves to reach a road-in-one or create a game that sets up a winning move.
	// For example:
	// Turn 1: White a1 (placed on a1), Black e5 (placed on e5)
	// Turn 2: White b1, Black e4
	// Turn 3: White c1, Black e3
	// Turn 4: White d1, Black e2
	// Before Turn 5 White plays e1 to complete road a1-b1-c1-d1-e1!
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a2"}, // White places opponent (Black) stone on a2
		{gotak.PlayerBlack, "a1"}, // Black places opponent (White) stone on a1
		{gotak.PlayerWhite, "b1"}, // White places White on b1
		{gotak.PlayerBlack, "b2"}, // Black places Black on b2
		{gotak.PlayerWhite, "c1"}, // White places White on c1
		{gotak.PlayerBlack, "c2"}, // Black places Black on c2
		{gotak.PlayerWhite, "d1"}, // White places White on d1
		{gotak.PlayerBlack, "d2"}, // Black places Black on d2
		{gotak.PlayerWhite, "e2"}, // White plays e2 instead of winning with e1 (missed win!)
	})

	analyzer, err := ai.NewAnalyzer(5, ai.AIConfig{Level: ai.Intermediate, TimeLimit: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewAnalyzer: %v", err)
	}

	puzzles := minePuzzles(context.Background(), analyzer, g)
	if len(puzzles) == 0 {
		t.Fatalf("expected at least one puzzle mined, got 0")
	}

	foundRoadInOne := false
	for _, p := range puzzles {
		if p.Player == gotak.PlayerWhite {
			var sol []string
			if err := json.Unmarshal([]byte(p.Solution), &sol); err != nil {
				t.Fatalf("json unmarshal: %v", err)
			}
			if len(sol) > 0 && strings.HasSuffix(sol[0], "e1") {
				foundRoadInOne = true
				if p.Source != "missed_win" {
					t.Errorf("expected source to be missed_win, got %q", p.Source)
				}
				break
			}
		}
	}
	if !foundRoadInOne {
		t.Errorf("did not find expected road-in-one puzzle with e1: %+v", puzzles)
	}
}

func TestPuzzles_HTTPHandlers(t *testing.T) {
	db := setupTestDB(t)
	dbMu.Lock()
	savedDB := dbConn
	dbConn = db
	dbMu.Unlock()
	t.Cleanup(func() {
		dbMu.Lock()
		dbConn = savedDB
		dbMu.Unlock()
	})
	l := zap.NewNop().Sugar()

	user1 := createTestUser(t, db)
	user2 := &User{
		Provider:    "local",
		ProviderID:  "test-user-puzzles-2",
		Email:       "user2@example.com",
		Name:        "Test User 2",
		Preferences: emptyJSONObject,
	}
	if err := db.Create(user2).Error; err != nil {
		t.Fatalf("Create user2: %v", err)
	}

	slug, err := createGame(db, 5, user1.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/puzzles/tactical", getTacticalPuzzleHandler)
	r.Post("/puzzles/{id}/attempt", postPuzzleAttemptHandler)
	r.With(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			// Set user from header if specified for testing
			if auth := req.Header.Get("X-Test-User"); auth == "user2" {
				ctx = context.WithValue(ctx, userContextKey, user2)
			} else {
				ctx = context.WithValue(ctx, userContextKey, user1)
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}).Post("/puzzles/mine/{slug}", postMinePuzzlesHandler)

	// 1. Initially GET /puzzles/tactical -> 404
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/puzzles/tactical", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /puzzles/tactical empty = %d, want 404", w.Code)
	}

	// 2. Insert a puzzle manually to test GET /puzzles/tactical & attempt
	p := Puzzle{
		GameID:    1,
		Ply:       8,
		Turn:      5,
		Player:    gotak.PlayerWhite,
		Size:      5,
		TPS:       "x5/x5/x5/2,2,x3/1,1,1,1,x 1 5",
		Solution:  `["e1"]`,
		Depth:     3,
		Source:    "missed_win",
		CreatedAt: time.Now(),
	}
	if _, err := savePuzzles(db, l, []Puzzle{p}); err != nil {
		t.Fatalf("savePuzzles: %v", err)
	}

	// 3. GET /puzzles/tactical -> 200 without solution
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/puzzles/tactical?size=5", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /puzzles/tactical?size=5 = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	var puzzleResp TacticalPuzzleResponse
	if err := json.NewDecoder(w.Body).Decode(&puzzleResp); err != nil {
		t.Fatalf("decode puzzle response: %v", err)
	}
	if puzzleResp.TPS != p.TPS || puzzleResp.Size != 5 {
		t.Errorf("puzzleResp mismatch: %+v", puzzleResp)
	}

	// 4. GET /puzzles/tactical?size=6 -> 404
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/puzzles/tactical?size=6", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /puzzles/tactical?size=6 = %d, want 404", w.Code)
	}

	// 5. POST /puzzles/{id}/attempt with incorrect move without reveal
	attemptBody, _ := json.Marshal(PuzzleAttemptRequest{Move: "a2", Reveal: false})
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("/puzzles/%d/attempt", puzzleResp.ID), bytes.NewReader(attemptBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /puzzles/{id}/attempt = %d, want 200", w.Code)
	}
	var attemptResp PuzzleAttemptResponse
	if err := json.NewDecoder(w.Body).Decode(&attemptResp); err != nil {
		t.Fatalf("decode attempt response: %v", err)
	}
	if attemptResp.Correct {
		t.Errorf("expected incorrect move, got correct")
	}
	if len(attemptResp.Solution) != 0 {
		t.Errorf("expected no solution revealed, got %v", attemptResp.Solution)
	}

	// 6. POST /puzzles/{id}/attempt with incorrect move WITH reveal: true
	attemptBody, _ = json.Marshal(PuzzleAttemptRequest{Move: "a2", Reveal: true})
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("/puzzles/%d/attempt", puzzleResp.ID), bytes.NewReader(attemptBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /puzzles/{id}/attempt = %d, want 200", w.Code)
	}
	if err := json.NewDecoder(w.Body).Decode(&attemptResp); err != nil {
		t.Fatalf("decode attempt response: %v", err)
	}
	if attemptResp.Correct {
		t.Errorf("expected incorrect move, got correct")
	}
	if len(attemptResp.Solution) != 1 || attemptResp.Solution[0] != "e1" {
		t.Errorf("expected solution [e1] revealed, got %v", attemptResp.Solution)
	}

	// 7. POST /puzzles/{id}/attempt with correct move (should match "e1" or "Fe1")
	attemptBody, _ = json.Marshal(PuzzleAttemptRequest{Move: "e1", Reveal: false})
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("/puzzles/%d/attempt", puzzleResp.ID), bytes.NewReader(attemptBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /puzzles/{id}/attempt = %d, want 200", w.Code)
	}
	if err := json.NewDecoder(w.Body).Decode(&attemptResp); err != nil {
		t.Fatalf("decode attempt response: %v", err)
	}
	if !attemptResp.Correct {
		t.Errorf("expected correct move, got false")
	}
	if len(attemptResp.Solution) != 1 || attemptResp.Solution[0] != "e1" {
		t.Errorf("expected solution [e1], got %v", attemptResp.Solution)
	}

	// 8. Test /puzzles/mine/{slug} with unauthorized user (user2)
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/puzzles/mine/"+slug, nil)
	req.Header.Set("X-Test-User", "user2")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("POST /puzzles/mine/{slug} unauthorized = %d, want 403", w.Code)
	}

	// 9. Test /puzzles/mine/{slug} with participant user (user1)
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/puzzles/mine/"+slug, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("POST /puzzles/mine/{slug} authorized = %d, want 200 (%s)", w.Code, w.Body.String())
	}
}
