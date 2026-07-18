package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeGameMode(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "human", false},
		{"human", "human", false},
		{"AI", "ai", false},
		{"ai", "ai", false},
		{"  Human ", "human", false},
		{"bot", "", true},
	}
	for _, tt := range tests {
		got, err := normalizeGameMode(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("normalizeGameMode(%q) expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeGameMode(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("normalizeGameMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWantsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/game/new", nil)
	if wantsJSON(req) {
		t.Error("default request should not want JSON")
	}

	req = httptest.NewRequest(http.MethodPost, "/game/new?format=json", nil)
	if !wantsJSON(req) {
		t.Error("format=json should want JSON")
	}

	req = httptest.NewRequest(http.MethodPost, "/game/new", nil)
	req.Header.Set("Accept", "application/json")
	if !wantsJSON(req) {
		t.Error("Accept: application/json should want JSON")
	}
}

func TestCreateGameModes(t *testing.T) {
	db := setupTestDB(t)
	user := createTestUser(t, db)

	humanSlug, err := createGame(db, 5, user.ID, "human")
	if err != nil {
		t.Fatalf("create human game: %v", err)
	}
	var humanGame Game
	if err := db.Where("slug = ?", humanSlug).First(&humanGame).Error; err != nil {
		t.Fatalf("load human game: %v", err)
	}
	if humanGame.Status != "waiting" {
		t.Errorf("human status = %q, want waiting", humanGame.Status)
	}

	aiSlug, err := createGame(db, 5, user.ID, "ai")
	if err != nil {
		t.Fatalf("create ai game: %v", err)
	}
	var aiGame Game
	if err := db.Where("slug = ?", aiSlug).First(&aiGame).Error; err != nil {
		t.Fatalf("load ai game: %v", err)
	}
	if aiGame.Status != "active" {
		t.Errorf("ai status = %q, want active", aiGame.Status)
	}

	state, err := buildGameStateResponse(db, aiSlug)
	if err != nil {
		t.Fatalf("buildGameStateResponse: %v", err)
	}
	if state.Mode != "ai" {
		t.Errorf("mode = %q, want ai", state.Mode)
	}
	if state.CurrentPlayer != 1 {
		t.Errorf("current_player = %d, want 1", state.CurrentPlayer)
	}
	if state.Status != "active" {
		t.Errorf("status = %q, want active", state.Status)
	}
	if state.Slug == "" || state.Board == nil {
		t.Error("expected embedded game fields populated")
	}

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"Slug", "current_player", "status", "mode", "Board"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("JSON missing key %q; got %v", key, decoded)
		}
	}

	_, err = createGame(db, 5, user.ID, "invalid")
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestCreateGameDefaultMode(t *testing.T) {
	db := setupTestDB(t)
	user := createTestUser(t, db)

	slug, err := createGame(db, 6, user.ID, "")
	if err != nil {
		t.Fatalf("createGame empty mode: %v", err)
	}
	state, err := buildGameStateResponse(db, slug)
	if err != nil {
		t.Fatalf("buildGameStateResponse: %v", err)
	}
	if state.Mode != "human" {
		t.Errorf("mode = %q, want human", state.Mode)
	}
}
