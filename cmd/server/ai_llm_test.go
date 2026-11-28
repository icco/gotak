package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"gorm.io/gorm"
)

func TestPostAIMoveHandlerUnconfiguredLLM(t *testing.T) {
	origLLM := llmEngine
	llmEngine = nil
	t.Cleanup(func() {
		llmEngine = origLLM
	})

	testDB := setupTestDB(t)
	origGetDB := getDBHook
	getDBHook = func() (*gorm.DB, error) {
		return testDB, nil
	}
	t.Cleanup(func() {
		getDBHook = origGetDB
	})

	user := createTestUser(t, testDB)

	slug, err := createGame(testDB, 4, user.ID, "ai")
	if err != nil {
		t.Fatalf("createGame failed: %v", err)
	}

	// Update game to make it AI's turn (Black)
	if err := testDB.Model(&Game{}).Where("slug = ?", slug).Update("current_player", gotak.PlayerBlack).Error; err != nil {
		t.Fatalf("update current player: %v", err)
	}

	body, err := json.Marshal(AIRequest{
		Engine: "llm",
		Level:  "intermediate",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, userContextKey, user)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/game/"+slug+"/ai-move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	PostAIMoveHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error != "LLM opponent is not configured" {
		t.Errorf("error = %q, want 'LLM opponent is not configured'", errResp.Error)
	}
}
