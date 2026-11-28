package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"go.icco.me/gotak"
	"gorm.io/gorm"
)

func createTestGameWithTag(t *testing.T, db *gorm.DB, id int64, size int) {
	t.Helper()
	game := Game{
		ID:   id,
		Slug: fmt.Sprintf("test-game-%d", id),
	}
	if err := db.Create(&game).Error; err != nil {
		t.Fatalf("failed to create test game %d: %v", id, err)
	}
	tag := Tag{
		GameID: id,
		Key:    "Size",
		Value:  strconv.Itoa(size),
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("failed to create size tag for game %d: %v", id, err)
	}
}

func TestHeatmap_BadSizeParam(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		param      string
		wantStatus int
		wantErr    bool
	}{
		{"missing size", "", http.StatusBadRequest, true},
		{"whitespace only", "   ", http.StatusBadRequest, true},
		{"non-integer text", "six", http.StatusBadRequest, true},
		{"below minimum 4", "3", http.StatusBadRequest, true},
		{"zero", "0", http.StatusBadRequest, true},
		{"negative number", "-1", http.StatusBadRequest, true},
		{"above maximum 9", "10", http.StatusBadRequest, true},
		{"large number", "100", http.StatusBadRequest, true},
		{"valid min 4", "4", http.StatusOK, false},
		{"valid mid 6", "6", http.StatusOK, false},
		{"valid max 9", "9", http.StatusOK, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "/analyze/heatmap"
			if tt.param != "" {
				target += "?size=" + url.QueryEscape(tt.param)
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
			w := httptest.NewRecorder()

			getHeatmapWithDB(w, req, db)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantErr {
				var errResp ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to parse error response: %v", err)
				}
				if errResp.Error == "" {
					t.Errorf("expected non-empty error message in response")
				}
			}
		})
	}
}

func TestHeatmap_SizeFiltering(t *testing.T) {
	db := setupTestDB(t)

	// Fictional test games:
	// Game 1: size 6 with placements
	// Game 2: size 5 with placements
	// Game 3: size 6 with no moves
	createTestGameWithTag(t, db, 1, 6)
	createTestGameWithTag(t, db, 2, 5)
	createTestGameWithTag(t, db, 3, 6)

	g1Moves := []Move{
		{GameID: 1, Turn: 1, Player: gotak.PlayerWhite, Text: "a1"},
		{GameID: 1, Turn: 1, Player: gotak.PlayerBlack, Text: "f6"},
		{GameID: 1, Turn: 2, Player: gotak.PlayerWhite, Text: "b2"},
		{GameID: 1, Turn: 2, Player: gotak.PlayerBlack, Text: "e5"},
	}
	for _, m := range g1Moves {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to seed game 1 move: %v", err)
		}
	}

	g2Moves := []Move{
		{GameID: 2, Turn: 1, Player: gotak.PlayerWhite, Text: "a1"},
		{GameID: 2, Turn: 1, Player: gotak.PlayerBlack, Text: "e5"},
		{GameID: 2, Turn: 2, Player: gotak.PlayerWhite, Text: "c3"},
	}
	for _, m := range g2Moves {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to seed game 2 move: %v", err)
		}
	}

	tests := []struct {
		name           string
		size           int
		wantGameCount  int
		wantPlacements int
		wantSquares    int
		checkSquare    string
		wantWhiteFlat  int
		wantBlackFlat  int
	}{
		{
			name:           "size 6 includes both size 6 games",
			size:           6,
			wantGameCount:  2,
			wantPlacements: 4,
			wantSquares:    36,
			checkSquare:    "b2",
			wantWhiteFlat:  1,
			wantBlackFlat:  0,
		},
		{
			name:           "size 5 includes only size 5 game",
			size:           5,
			wantGameCount:  1,
			wantPlacements: 3,
			wantSquares:    25,
			checkSquare:    "c3",
			wantWhiteFlat:  1,
			wantBlackFlat:  0,
		},
		{
			name:           "size 4 has no games",
			size:           4,
			wantGameCount:  0,
			wantPlacements: 0,
			wantSquares:    16,
			checkSquare:    "a1",
			wantWhiteFlat:  0,
			wantBlackFlat:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := computeHeatmap(db, tt.size)
			if err != nil {
				t.Fatalf("computeHeatmap(%d) unexpected error: %v", tt.size, err)
			}
			if resp.Size != tt.size {
				t.Errorf("resp.Size = %d, want %d", resp.Size, tt.size)
			}
			if resp.GameCount != tt.wantGameCount {
				t.Errorf("resp.GameCount = %d, want %d", resp.GameCount, tt.wantGameCount)
			}
			if resp.Placements != tt.wantPlacements {
				t.Errorf("resp.Placements = %d, want %d", resp.Placements, tt.wantPlacements)
			}
			if len(resp.Squares) != tt.wantSquares {
				t.Errorf("len(resp.Squares) = %d, want %d", len(resp.Squares), tt.wantSquares)
			}

			sq := resp.Squares[tt.checkSquare]
			if sq.White.Flat != tt.wantWhiteFlat {
				t.Errorf("sq[%s].White.Flat = %d, want %d", tt.checkSquare, sq.White.Flat, tt.wantWhiteFlat)
			}
			if sq.Black.Flat != tt.wantBlackFlat {
				t.Errorf("sq[%s].Black.Flat = %d, want %d", tt.checkSquare, sq.Black.Flat, tt.wantBlackFlat)
			}
		})
	}
}

func TestHeatmap_Turn1ColorSwap(t *testing.T) {
	db := setupTestDB(t)

	// In Tak, turn 1 places the opponent's piece:
	// White turn 1: places Black flat
	// Black turn 1: places White flat
	// Turn 2+: player places own piece
	createTestGameWithTag(t, db, 1, 6)

	moves := []Move{
		{GameID: 1, Turn: 1, Player: gotak.PlayerWhite, Text: "a1"},
		{GameID: 1, Turn: 1, Player: gotak.PlayerBlack, Text: "f6"},
		{GameID: 1, Turn: 2, Player: gotak.PlayerWhite, Text: "a2"},
		{GameID: 1, Turn: 2, Player: gotak.PlayerBlack, Text: "f5"},
	}
	for _, m := range moves {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to seed move: %v", err)
		}
	}

	resp, err := computeHeatmap(db, 6)
	if err != nil {
		t.Fatalf("computeHeatmap unexpected error: %v", err)
	}

	tests := []struct {
		square    string
		wantWhite PieceCounts
		wantBlack PieceCounts
	}{
		{
			square:    "a1",
			wantWhite: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
			wantBlack: PieceCounts{Flat: 1, Standing: 0, Capstone: 0},
		},
		{
			square:    "f6",
			wantWhite: PieceCounts{Flat: 1, Standing: 0, Capstone: 0},
			wantBlack: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
		},
		{
			square:    "a2",
			wantWhite: PieceCounts{Flat: 1, Standing: 0, Capstone: 0},
			wantBlack: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
		},
		{
			square:    "f5",
			wantWhite: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
			wantBlack: PieceCounts{Flat: 1, Standing: 0, Capstone: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.square, func(t *testing.T) {
			got := resp.Squares[tt.square]
			if got.White != tt.wantWhite {
				t.Errorf("square %s White = %+v, want %+v", tt.square, got.White, tt.wantWhite)
			}
			if got.Black != tt.wantBlack {
				t.Errorf("square %s Black = %+v, want %+v", tt.square, got.Black, tt.wantBlack)
			}
		})
	}
}

func TestHeatmap_SlidesIgnored(t *testing.T) {
	db := setupTestDB(t)

	createTestGameWithTag(t, db, 1, 6)

	// Mix of placements and slides:
	// Placements:
	//   Turn 1 White "a1" (opponent Black flat)
	//   Turn 1 Black "f6" (opponent White flat)
	//   Turn 2 White "Sa2" (standing stone)
	//   Turn 2 Black "Ca3" (capstone)
	//   Turn 4 White "Fa4" (explicit flat)
	// Slides (must be ignored):
	//   Turn 3 White "3a1+111"
	//   Turn 3 Black "a2+"
	//   Turn 4 Black "2a3>11"
	moves := []Move{
		{GameID: 1, Turn: 1, Player: gotak.PlayerWhite, Text: "a1"},
		{GameID: 1, Turn: 1, Player: gotak.PlayerBlack, Text: "f6"},
		{GameID: 1, Turn: 2, Player: gotak.PlayerWhite, Text: "Sa2"},
		{GameID: 1, Turn: 2, Player: gotak.PlayerBlack, Text: "Ca3"},
		{GameID: 1, Turn: 3, Player: gotak.PlayerWhite, Text: "3a1+111"},
		{GameID: 1, Turn: 3, Player: gotak.PlayerBlack, Text: "a2+"},
		{GameID: 1, Turn: 4, Player: gotak.PlayerWhite, Text: "Fa4"},
		{GameID: 1, Turn: 4, Player: gotak.PlayerBlack, Text: "2a3>11"},
	}
	for _, m := range moves {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to seed move: %v", err)
		}
	}

	resp, err := computeHeatmap(db, 6)
	if err != nil {
		t.Fatalf("computeHeatmap unexpected error: %v", err)
	}

	// Only the 5 placement moves should be counted.
	if resp.Placements != 5 {
		t.Errorf("resp.Placements = %d, want 5", resp.Placements)
	}

	expectedSquares := map[string]SquareCounts{
		"a1": {
			White: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
			Black: PieceCounts{Flat: 1, Standing: 0, Capstone: 0},
		},
		"f6": {
			White: PieceCounts{Flat: 1, Standing: 0, Capstone: 0},
			Black: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
		},
		"a2": {
			White: PieceCounts{Flat: 0, Standing: 1, Capstone: 0},
			Black: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
		},
		"a3": {
			White: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
			Black: PieceCounts{Flat: 0, Standing: 0, Capstone: 1},
		},
		"a4": {
			White: PieceCounts{Flat: 1, Standing: 0, Capstone: 0},
			Black: PieceCounts{Flat: 0, Standing: 0, Capstone: 0},
		},
	}

	for sqName, want := range expectedSquares {
		got := resp.Squares[sqName]
		if got.White != want.White {
			t.Errorf("square %s White = %+v, want %+v", sqName, got.White, want.White)
		}
		if got.Black != want.Black {
			t.Errorf("square %s Black = %+v, want %+v", sqName, got.Black, want.Black)
		}
	}
}
