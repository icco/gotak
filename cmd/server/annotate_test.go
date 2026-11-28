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

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"gorm.io/gorm"
)

func createNamedUser(t *testing.T, db *gorm.DB, providerID, email, name string) *User {
	t.Helper()
	user := &User{
		Provider:    "local",
		ProviderID:  providerID,
		Email:       email,
		Name:        name,
		Preferences: emptyJSONObject,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	return user
}

func setupAnnotateTestGame(t *testing.T) (*User, *User, string) {
	t.Helper()
	db := setupTestDB(t)
	dbOverride = db
	t.Cleanup(func() {
		dbOverride = nil
	})

	alice := createNamedUser(t, db, "alice-1", "alice@example.com", "Alice")
	bob := createNamedUser(t, db, "bob-1", "bob@example.com", "Bob")

	slug, err := createGame(db, 5, alice.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	if err := joinGame(db, slug, bob.ID); err != nil {
		t.Fatalf("joinGame: %v", err)
	}

	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}

	if err := insertMove(db, game.ID, gotak.PlayerWhite, "a1", 1); err != nil {
		t.Fatalf("insertMove white: %v", err)
	}
	if err := insertMove(db, game.ID, gotak.PlayerBlack, "e5", 1); err != nil {
		t.Fatalf("insertMove black: %v", err)
	}

	return alice, bob, slug
}

func callAnnotate(t *testing.T, user *User, slug string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/game/"+slug+"/annotate", bytes.NewReader(payload))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	if user != nil {
		req = req.WithContext(context.WithValue(req.Context(), userContextKey, user))
	}

	w := httptest.NewRecorder()
	annotateHandler(w, req)
	return w
}

func TestAnnotateHandler_authRules(t *testing.T) {
	alice, bob, slug := setupAnnotateTestGame(t)

	charlie := &User{
		ID:         999,
		Provider:   "local",
		ProviderID: "charlie-1",
		Email:      "charlie@example.com",
		Name:       "Charlie",
	}

	// Non-participant cannot annotate
	w := callAnnotate(t, charlie, slug, AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerWhite,
		Comment: "sneaky",
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("non-participant got status %d, want 403", w.Code)
	}

	// Alice cannot annotate Bob's move
	w = callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerBlack,
		Comment: "criticism",
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("annotating other player's move got %d, want 403", w.Code)
	}

	// Bob cannot annotate Alice's move
	w = callAnnotate(t, bob, slug, AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerWhite,
		Comment: "complaint",
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("annotating other player's move got %d, want 403", w.Code)
	}
}

func TestAnnotateHandler_validationAndNotFound(t *testing.T) {
	alice, _, slug := setupAnnotateTestGame(t)

	// Invalid player
	w := callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    1,
		Player:  3,
		Comment: "test",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid player got %d, want 400", w.Code)
	}

	// Invalid turn (0)
	w = callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    0,
		Player:  gotak.PlayerWhite,
		Comment: "test",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("zero turn got %d, want 400", w.Code)
	}

	// Negative turn
	w = callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    -1,
		Player:  gotak.PlayerWhite,
		Comment: "test",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("negative turn got %d, want 400", w.Code)
	}

	// Non-existent game
	w = callAnnotate(t, alice, "non-existent-game", AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerWhite,
		Comment: "test",
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("missing game got %d, want 404", w.Code)
	}

	// Non-existent move
	w = callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    42,
		Player:  gotak.PlayerWhite,
		Comment: "test",
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("missing move got %d, want 404", w.Code)
	}
}

func TestAnnotateHandler_successAndSanitization(t *testing.T) {
	alice, bob, slug := setupAnnotateTestGame(t)

	// Alice annotates her move with braces and HTML tags
	w := callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerWhite,
		Comment: "<b>good</b> {bracketed} move",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("annotate move got status %d, want 200, body: %s", w.Code, w.Body.String())
	}

	var step ReplayStep
	if err := json.Unmarshal(w.Body.Bytes(), &step); err != nil {
		t.Fatalf("unmarshal ReplayStep: %v", err)
	}
	if step.Comment != "good bracketed move" {
		t.Errorf("step.Comment = %q, want 'good bracketed move'", step.Comment)
	}

	// Bob annotates his move
	w = callAnnotate(t, bob, slug, AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerBlack,
		Comment: "strong response",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bob annotate got %d, body: %s", w.Code, w.Body.String())
	}

	// Verify replay exposes comments
	reqReplay := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/game/"+slug+"/replay", nil)
	rctxReplay := chi.NewRouteContext()
	rctxReplay.URLParams.Add("slug", slug)
	reqReplay = reqReplay.WithContext(context.WithValue(reqReplay.Context(), chi.RouteCtxKey, rctxReplay))
	wReplay := httptest.NewRecorder()
	getReplayHandler(wReplay, reqReplay)
	if wReplay.Code != http.StatusOK {
		t.Fatalf("replay handler got %d", wReplay.Code)
	}

	var replayResp ReplayResponse
	if err := json.Unmarshal(wReplay.Body.Bytes(), &replayResp); err != nil {
		t.Fatalf("unmarshal replay: %v", err)
	}
	if len(replayResp.Steps) != 2 {
		t.Fatalf("got %d replay steps, want 2", len(replayResp.Steps))
	}
	if replayResp.Steps[0].Comment != "good bracketed move" {
		t.Errorf("step 0 comment = %q", replayResp.Steps[0].Comment)
	}
	if replayResp.Steps[1].Comment != "strong response" {
		t.Errorf("step 1 comment = %q", replayResp.Steps[1].Comment)
	}

	// Verify PTN export includes move comments
	reqPTN := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/game/"+slug+"/ptn", nil)
	rctxPTN := chi.NewRouteContext()
	rctxPTN.URLParams.Add("slug", slug)
	reqPTN = reqPTN.WithContext(context.WithValue(reqPTN.Context(), chi.RouteCtxKey, rctxPTN))
	wPTN := httptest.NewRecorder()
	getPTNHandler(wPTN, reqPTN)
	if wPTN.Code != http.StatusOK {
		t.Fatalf("ptn handler got %d", wPTN.Code)
	}
	ptnText := wPTN.Body.String()
	wantPTNLine := "1. a1 { good bracketed move } e5 { strong response }"
	if !strings.Contains(ptnText, wantPTNLine) {
		t.Errorf("PTN missing line %q, got:\n%s", wantPTNLine, ptnText)
	}

	// Clearing comment with empty string
	w = callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerWhite,
		Comment: "",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("clear comment got %d", w.Code)
	}
	var clearedStep ReplayStep
	if err := json.Unmarshal(w.Body.Bytes(), &clearedStep); err != nil {
		t.Fatalf("unmarshal cleared step: %v", err)
	}
	if clearedStep.Comment != "" {
		t.Errorf("cleared step comment = %q, want empty", clearedStep.Comment)
	}

	// Long comment truncated at 500 characters
	longComment := strings.Repeat("x", 600)
	w = callAnnotate(t, alice, slug, AnnotateRequest{
		Turn:    1,
		Player:  gotak.PlayerWhite,
		Comment: longComment,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("long comment got %d", w.Code)
	}
	var longStep ReplayStep
	if err := json.Unmarshal(w.Body.Bytes(), &longStep); err != nil {
		t.Fatalf("unmarshal step: %v", err)
	}
	if len(longStep.Comment) != 500 {
		t.Errorf("len(longStep.Comment) = %d, want 500", len(longStep.Comment))
	}
}

func TestNewMoveHandler_withComment(t *testing.T) {
	db := setupTestDB(t)
	dbOverride = db
	t.Cleanup(func() {
		dbOverride = nil
	})

	alice := createNamedUser(t, db, "alice-move-1", "alice-move@example.com", "Alice")

	slug, err := createGame(db, 5, alice.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}

	moveBody := MoveRequest{
		Player:  gotak.PlayerWhite,
		Text:    "a1",
		Turn:    1,
		Comment: "initial move comment",
	}
	payload, err := json.Marshal(moveBody)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/game/"+slug+"/move", bytes.NewReader(payload))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, alice))

	w := httptest.NewRecorder()
	newMoveHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("newMoveHandler got %d, body: %s", w.Code, w.Body.String())
	}

	// Verify move has comment in DB
	var move Move
	if err := db.Where("player = ? AND text = ?", gotak.PlayerWhite, "a1").First(&move).Error; err != nil {
		t.Fatalf("find move in db: %v", err)
	}
	if move.Comment != "initial move comment" {
		t.Errorf("move.Comment = %q, want 'initial move comment'", move.Comment)
	}

	// Verify getGame / getTurns loads comment into gotak.Game
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}
	if len(game.Turns) != 1 || game.Turns[0].First == nil {
		t.Fatalf("turns invalid: %+v", game.Turns)
	}
	if game.Turns[0].First.Comment != "initial move comment" {
		t.Errorf("turn first comment = %q", game.Turns[0].First.Comment)
	}
}

func TestDatabase_insertMoveWithComment(t *testing.T) {
	db := setupTestDB(t)

	alice := createNamedUser(t, db, "alice-db-1", "alice-db@example.com", "Alice")
	slug, err := createGame(db, 5, alice.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}

	err = insertMoveWithComment(db, game.ID, gotak.PlayerWhite, "b2", 1, "test annotation")
	if err != nil {
		t.Fatalf("insertMoveWithComment: %v", err)
	}

	var move Move
	if err := db.Where("game_id = ? AND player = ?", game.ID, gotak.PlayerWhite).First(&move).Error; err != nil {
		t.Fatalf("query move: %v", err)
	}
	if move.Comment != "test annotation" {
		t.Errorf("move.Comment = %q, want 'test annotation'", move.Comment)
	}
}

func TestSanitizeComment(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello world", "hello world"},
		{"{comment in braces}", "comment in braces"},
		{"<script>alert(1)</script>safe", "safe"},
		{"  padded  ", "padded"},
		{"A & B", "A & B"},
		{"Alice's move", "Alice's move"},
		{"line one\nline two", "line one line two"},
		{"  multiple \t whitespace \n lines ", "multiple whitespace lines"},
		{strings.Repeat("a", 600), strings.Repeat("a", 500)},
	}

	for _, tc := range cases {
		if got := sanitizeComment(tc.input); got != tc.want {
			t.Errorf("sanitizeComment(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRouter_annotateRouteAuthMiddleware(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret-annotate")
	db := setupTestDB(t)
	dbOverride = db
	t.Cleanup(func() {
		dbOverride = nil
	})

	alice := createNamedUser(t, db, "alice-router-1", "alice-router@example.com", "Alice")

	token, err := generateJWTForUser(alice)
	if err != nil {
		t.Fatalf("generateJWTForUser: %v", err)
	}

	slug, err := createGame(db, 5, alice.ID, "human")
	if err != nil {
		t.Fatalf("createGame: %v", err)
	}
	game, err := getGame(db, slug)
	if err != nil {
		t.Fatalf("getGame: %v", err)
	}
	if err := insertMove(db, game.ID, gotak.PlayerWhite, "a1", 1); err != nil {
		t.Fatalf("insertMove: %v", err)
	}

	router := buildRouter(routerOptions{IsDev: true})

	// Request without auth header should be rejected with 401
	body, _ := json.Marshal(AnnotateRequest{Turn: 1, Player: 1, Comment: "test"})
	reqWithoutAuth := httptest.NewRequestWithContext(t.Context(), http.MethodPost, fmt.Sprintf("/game/%s/annotate", slug), bytes.NewReader(body))
	wWithoutAuth := httptest.NewRecorder()
	router.ServeHTTP(wWithoutAuth, reqWithoutAuth)
	if wWithoutAuth.Code != http.StatusUnauthorized {
		t.Errorf("without auth header got %d, want 401", wWithoutAuth.Code)
	}

	// Request with valid auth header should succeed with 200
	reqWithAuth := httptest.NewRequestWithContext(t.Context(), http.MethodPost, fmt.Sprintf("/game/%s/annotate", slug), bytes.NewReader(body))
	reqWithAuth.Header.Set("Authorization", "Bearer "+token)
	wWithAuth := httptest.NewRecorder()
	router.ServeHTTP(wWithAuth, reqWithAuth)
	if wWithAuth.Code != http.StatusOK {
		t.Errorf("with auth header got %d, want 200, body: %s", wWithAuth.Code, wWithAuth.Body.String())
	}
}
