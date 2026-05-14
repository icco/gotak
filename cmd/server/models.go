package main

import (
	"time"

	"gorm.io/gorm"
)

// ErrorResponse is the standard error payload returned by API handlers.
type ErrorResponse struct {
	Error string `json:"error" example:"Something went wrong"`
}

type MessageResponse struct {
	Message string `json:"message" example:"Operation successful"`
}

type HealthResponse struct {
	Healthy  string `json:"healthy" example:"true"`
	Revision string `json:"revision,omitempty" example:"abc123def"`
	Tag      string `json:"tag,omitempty" example:"v1.0.0"`
	Branch   string `json:"branch,omitempty" example:"main"`
}

// Game represents a game in the database
type Game struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Slug          string    `gorm:"type:text;uniqueIndex" json:"slug"`
	Status        string    `gorm:"type:text;default:'waiting'" json:"status"` // waiting, active, finished
	Winner        int       `gorm:"default:0" json:"winner"`
	CurrentPlayer int       `gorm:"default:1" json:"current_player"`
	CurrentTurn   int       `gorm:"default:1" json:"current_turn"`
	WhitePlayerID *int64    `gorm:"index" json:"white_player_id,omitempty"`
	BlackPlayerID *int64    `gorm:"index" json:"black_player_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Associations
	WhitePlayer *User  `gorm:"foreignKey:WhitePlayerID" json:"white_player,omitempty"`
	BlackPlayer *User  `gorm:"foreignKey:BlackPlayerID" json:"black_player,omitempty"`
	Tags        []Tag  `gorm:"foreignKey:GameID" json:"tags,omitempty"`
	Moves       []Move `gorm:"foreignKey:GameID" json:"moves,omitempty"`
}

// Tag represents game metadata tags
type Tag struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	GameID    int64     `gorm:"index;not null" json:"game_id"`
	Key       string    `gorm:"type:text" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	CreatedAt time.Time `json:"created_at"`

	// Associations
	Game Game `gorm:"foreignKey:GameID" json:"-"`
}

// Move represents a move in a game
type Move struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	GameID    int64     `gorm:"index;not null" json:"game_id"`
	Player    int       `gorm:"not null" json:"player"`
	Turn      int64     `gorm:"not null" json:"turn"`
	Text      string    `gorm:"type:text" json:"text"`
	CreatedAt time.Time `json:"created_at"`

	// Associations
	Game Game `gorm:"foreignKey:GameID" json:"-"`
}

// User represents an authenticated user (local or social)
type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Provider     string    `gorm:"type:varchar(32);not null;uniqueIndex:idx_provider_id" json:"provider"`
	ProviderID   string    `gorm:"type:varchar(128);not null;uniqueIndex:idx_provider_id" json:"provider_id"`
	Email        string    `gorm:"type:varchar(255);uniqueIndex" json:"email,omitempty"`
	Name         string    `gorm:"type:varchar(128)" json:"name,omitempty"`
	AvatarURL    string    `gorm:"type:varchar(512)" json:"avatar_url,omitempty"`
	PasswordHash string    `gorm:"type:varchar(255)" json:"-"` // nullable for social login
	Preferences  string    `gorm:"type:jsonb" json:"preferences,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AnalysisCache stores the JSON-encoded result of a previous
// postAnalyzeHandler call keyed by (game, engine config, game version)
// so repeat requests against a stable game return immediately.
// GameVersion is an opaque string fingerprint; today it encodes the
// half-turn count, but the analyzer is free to add more (UpdatedAt, a
// move-text hash) without a schema change.
// `type:jsonb` is Postgres-native; SQLite (used in unit tests) silently
// degrades it to TEXT, which is fine.
type AnalysisCache struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	GameID      int64     `gorm:"not null;uniqueIndex:idx_analysis_lookup,priority:1" json:"game_id"`
	Level       string    `gorm:"type:varchar(16);not null;uniqueIndex:idx_analysis_lookup,priority:2" json:"level"`
	Style       string    `gorm:"type:varchar(16);not null;uniqueIndex:idx_analysis_lookup,priority:3" json:"style"`
	TimeLimitNs int64     `gorm:"not null;uniqueIndex:idx_analysis_lookup,priority:4" json:"time_limit_ns"`
	GameVersion string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_analysis_lookup,priority:5" json:"game_version"`
	Agreed      int       `json:"agreed"`
	Moves       string    `gorm:"type:jsonb" json:"moves"` // JSON-encoded []MoveAnalysis
	CreatedAt   time.Time `json:"created_at"`
}

// AutoMigrate runs the database migrations
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Game{}, &Tag{}, &Move{}, &User{}, &AnalysisCache{})
}
