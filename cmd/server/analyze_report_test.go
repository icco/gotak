package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"go.uber.org/zap"
)

func TestAnalyzeReport_RendersWithSeededCache(t *testing.T) {
	db := setupTestDB(t)

	user := createTestUser(t, db)
	slug, err := createGame(db, 5, user.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("getGameID: %v", err)
	}

	if err := db.Create(&Tag{GameID: gameID, Key: "Player1", Value: "Ada Lovelace"}).Error; err != nil {
		t.Fatalf("create Player1 tag: %v", err)
	}
	if err := db.Create(&Tag{GameID: gameID, Key: "Player2", Value: "Alice"}).Error; err != nil {
		t.Fatalf("create Player2 tag: %v", err)
	}
	if err := db.Create(&Tag{GameID: gameID, Key: "Result", Value: "R-0"}).Error; err != nil {
		t.Fatalf("create Result tag: %v", err)
	}

	moves := []struct {
		player int
		text   string
		turn   int64
	}{
		{gotak.PlayerWhite, "a1", 1},
		{gotak.PlayerBlack, "e5", 1},
		{gotak.PlayerWhite, "c3", 2},
		{gotak.PlayerBlack, "d4", 2},
	}
	for _, m := range moves {
		if err := insertMove(db, gameID, m.player, m.text, m.turn); err != nil {
			t.Fatalf("insertMove(%s): %v", m.text, err)
		}
	}

	analysisMoves := []MoveAnalysis{
		{Turn: 1, Player: gotak.PlayerWhite, Played: "a1", Best: "a1", Score: 0, ScoreAfter: 10, Agreed: true, Quality: QualityBest},
		{Turn: 1, Player: gotak.PlayerBlack, Played: "e5", Best: "e5", Score: 10, ScoreAfter: 5, Agreed: true, Quality: QualityBest},
		{Turn: 2, Player: gotak.PlayerWhite, Played: "c3", Best: "c3", Score: 5, ScoreAfter: 40, Agreed: true, Quality: QualityGood},
		{Turn: 2, Player: gotak.PlayerBlack, Played: "d4", Best: "c4", Score: 40, ScoreAfter: 850, Agreed: false, Loss: 810, Quality: QualityBlunder},
	}
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}
	cacheKey := analysisCacheKey{
		gameID:      gameID,
		level:       "advanced",
		style:       "balanced",
		timeLimitNs: int64(defaultAnalyzeTimeLimit),
		gameVersion: gameCacheVersion(game),
	}
	saveAnalysisCache(db, zap.NewNop().Sugar(), cacheKey, 3, analysisMoves)
	if top := criticalMoments(analysisMoves); len(top) != 1 {
		t.Fatalf("criticalMoments returned %d items, want 1", len(top))
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/"+slug+"/report", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	getAnalyzeReportWithDB(w, req, db)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	body := w.Body.String()
	checks := []string{
		"Game Analysis: " + slug,
		"Ada Lovelace",
		"Alice",
		"5x5",
		"R-0",
		"eval-graph",
		"<svg class=\"eval-graph\"",
		"<svg class=\"tak-board\"",
		"badge-blunder",
		"Critical Moments",
		"Print to PDF",
		"@media print",
	}
	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("body missing expected snippet %q", check)
		}
	}
}

func TestAnalyzeReport_HTMLEscapesHostileInput(t *testing.T) {
	db := setupTestDB(t)

	user := createTestUser(t, db)
	slug, err := createGame(db, 5, user.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("getGameID: %v", err)
	}

	if err := insertMove(db, gameID, gotak.PlayerWhite, "a1", 1); err != nil {
		t.Fatalf("insertMove: %v", err)
	}
	if err := insertMove(db, gameID, gotak.PlayerBlack, "e5", 1); err != nil {
		t.Fatalf("insertMove: %v", err)
	}

	hostileMove := "<script>alert('move')</script>"
	hostileBest := "<img src=x onerror=alert('best')>"
	analysisMoves := []MoveAnalysis{
		{Turn: 1, Player: gotak.PlayerWhite, Played: hostileMove, Best: hostileBest, Score: 0, ScoreAfter: 50, Agreed: false, Quality: QualityMistake},
		{Turn: 1, Player: gotak.PlayerBlack, Played: "e5", Best: "e5", Score: 50, ScoreAfter: 20, Agreed: true, Quality: QualityGood},
	}
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}
	cacheKey := analysisCacheKey{
		gameID:      gameID,
		level:       "advanced",
		style:       "balanced",
		timeLimitNs: int64(defaultAnalyzeTimeLimit),
		gameVersion: gameCacheVersion(game),
	}
	saveAnalysisCache(db, zap.NewNop().Sugar(), cacheKey, 1, analysisMoves)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/"+slug+"/report", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	getAnalyzeReportWithDB(w, req, db)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, hostileMove) {
		t.Errorf("body contains unescaped hostile move script tag: %s", body)
	}
	if strings.Contains(body, hostileBest) {
		t.Errorf("body contains unescaped hostile best img tag: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;alert(&#39;move&#39;)&lt;/script&gt;") && !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("body missing escaped script tag: %s", body)
	}

	// Also verify hostile turn comments are escaped when rendered
	game, err = getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}
	turn, err := game.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	hostileComment := "<script>alert('comment')</script>"
	turn.Comment = hostileComment
	game.UpdateTurn(turn)

	rec := httptest.NewRecorder()
	cfg, levelName := analyzeConfigFromRequest(AnalyzeRequest{})
	renderAnalyzeReport(rec, zap.NewNop().Sugar(), game, cfg, levelName, AnalyzeResponse{
		Moves:     analysisMoves,
		MoveCount: len(analysisMoves),
		Agreed:    1,
	})

	renderBody := rec.Body.String()
	if strings.Contains(renderBody, hostileComment) {
		t.Errorf("renderBody contains unescaped hostile comment: %s", renderBody)
	}
	if !strings.Contains(renderBody, "&lt;script&gt;alert(&#39;comment&#39;)&lt;/script&gt;") && !strings.Contains(renderBody, "&lt;script&gt;") {
		t.Errorf("renderBody missing escaped comment tag: %s", renderBody)
	}
}

func TestAnalyzeReport_UncachedReturns404(t *testing.T) {
	db := setupTestDB(t)

	user := createTestUser(t, db)
	slug, err := createGame(db, 5, user.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/"+slug+"/report", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	getAnalyzeReportWithDB(w, req, db)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Analysis Not Found") {
		t.Errorf("body missing 'Analysis Not Found': %s", body)
	}
	if !strings.Contains(body, "POST /analyze/game/"+slug) {
		t.Errorf("body missing recommendation to POST /analyze/game/%s: %s", slug, body)
	}

	// Non-existent game slug
	reqNonExistent := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/non-existent/report", http.NoBody)
	rctx2 := chi.NewRouteContext()
	rctx2.URLParams.Add("slug", "non-existent")
	reqNonExistent = reqNonExistent.WithContext(context.WithValue(reqNonExistent.Context(), chi.RouteCtxKey, rctx2))
	w2 := httptest.NewRecorder()

	getAnalyzeReportWithDB(w2, reqNonExistent, db)

	if w2.Code != http.StatusNotFound {
		t.Fatalf("non-existent status = %d, want %d", w2.Code, http.StatusNotFound)
	}
}

func TestAnalyzeReport_SVGPresent(t *testing.T) {
	db := setupTestDB(t)

	user := createTestUser(t, db)
	slug, err := createGame(db, 5, user.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("getGameID: %v", err)
	}

	moves := []struct {
		player int
		text   string
		turn   int64
	}{
		{gotak.PlayerWhite, "a1", 1},
		{gotak.PlayerBlack, "e5", 1},
	}
	for _, m := range moves {
		if err := insertMove(db, gameID, m.player, m.text, m.turn); err != nil {
			t.Fatalf("insertMove: %v", err)
		}
	}

	analysisMoves := []MoveAnalysis{
		{Turn: 1, Player: gotak.PlayerWhite, Played: "a1", Best: "a1", Score: 0, ScoreAfter: 20, Agreed: true, Quality: QualityBest},
		{Turn: 1, Player: gotak.PlayerBlack, Played: "e5", Best: "d4", Score: 20, ScoreAfter: 700, Agreed: false, Loss: 680, Quality: QualityBlunder},
	}
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}
	cacheKey := analysisCacheKey{
		gameID:      gameID,
		level:       "advanced",
		style:       "balanced",
		timeLimitNs: int64(defaultAnalyzeTimeLimit),
		gameVersion: gameCacheVersion(game),
	}
	saveAnalysisCache(db, zap.NewNop().Sugar(), cacheKey, 1, analysisMoves)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/"+slug+"/report", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	getAnalyzeReportWithDB(w, req, db)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "<svg class=\"eval-graph\"") {
		t.Errorf("missing eval-graph SVG in body")
	}
	if !strings.Contains(body, "+3000") || !strings.Contains(body, "-3000") {
		t.Errorf("missing eval-graph clamped score labels in body")
	}
	if !strings.Contains(body, "<svg class=\"tak-board\"") {
		t.Errorf("missing tak-board SVG in body")
	}
	if !strings.Contains(body, "polyline") {
		t.Errorf("missing polyline in eval graph")
	}

	boardWithCap := map[string][]*gotak.Stone{
		"c3": {
			{Player: gotak.PlayerBlack, Type: gotak.StoneCap},
		},
	}
	capSVG := renderBoardSVG(boardWithCap, 5, "")
	if !strings.Contains(string(capSVG), `stroke="#ffffff"`) {
		t.Errorf("black capstone inner ring should use contrasting white stroke: %s", string(capSVG))
	}
}

func TestAnalyzeReport_QueryParamsMatching(t *testing.T) {
	db := setupTestDB(t)

	user := createTestUser(t, db)
	slug, err := createGame(db, 5, user.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	gameID, err := getGameID(db, slug)
	if err != nil {
		t.Fatalf("getGameID: %v", err)
	}

	if err := insertMove(db, gameID, gotak.PlayerWhite, "a1", 1); err != nil {
		t.Fatalf("insertMove: %v", err)
	}
	if err := insertMove(db, gameID, gotak.PlayerBlack, "e5", 1); err != nil {
		t.Fatalf("insertMove: %v", err)
	}

	analysisMoves := []MoveAnalysis{
		{Turn: 1, Player: gotak.PlayerWhite, Played: "a1", Best: "a1", Score: 0, ScoreAfter: 10, Agreed: true, Quality: QualityBest},
		{Turn: 1, Player: gotak.PlayerBlack, Played: "e5", Best: "e5", Score: 10, ScoreAfter: 0, Agreed: true, Quality: QualityBest},
	}
	customTimeLimit := 500 * time.Millisecond
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}
	cacheKey := analysisCacheKey{
		gameID:      gameID,
		level:       "intermediate",
		style:       "aggressive",
		timeLimitNs: int64(customTimeLimit),
		gameVersion: gameCacheVersion(game),
	}
	saveAnalysisCache(db, zap.NewNop().Sugar(), cacheKey, 2, analysisMoves)

	// Default query params -> miss (404)
	reqMiss := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/"+slug+"/report", http.NoBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	reqMiss = reqMiss.WithContext(context.WithValue(reqMiss.Context(), chi.RouteCtxKey, rctx))
	wMiss := httptest.NewRecorder()
	getAnalyzeReportWithDB(wMiss, reqMiss, db)
	if wMiss.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 on mismatched cache key", wMiss.Code)
	}

	// Custom query params on uncached game -> 404 includes guidance with custom settings
	reqMissCustom := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/"+slug+"/report?level=expert&style=defensive&time_limit=3s", http.NoBody)
	reqMissCustom = reqMissCustom.WithContext(context.WithValue(reqMissCustom.Context(), chi.RouteCtxKey, rctx))
	wMissCustom := httptest.NewRecorder()
	getAnalyzeReportWithDB(wMissCustom, reqMissCustom, db)
	if wMissCustom.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 on custom miss", wMissCustom.Code)
	}
	if !strings.Contains(wMissCustom.Body.String(), `"level":"expert"`) {
		t.Errorf("expected custom miss guidance to contain expert level: %s", wMissCustom.Body.String())
	}

	// Exact query params -> hit (200)
	reqHit := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/analyze/game/"+slug+"/report?level=intermediate&style=aggressive&time_limit=500ms", http.NoBody)
	reqHit = reqHit.WithContext(context.WithValue(reqHit.Context(), chi.RouteCtxKey, rctx))
	wHit := httptest.NewRecorder()
	getAnalyzeReportWithDB(wHit, reqHit, db)
	if wHit.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 on matching cache key; body = %s", wHit.Code, wHit.Body.String())
	}
}
