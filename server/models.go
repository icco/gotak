package main

import (
	"time"

	"gorm.io/gorm"
)

// Game represents a game in the database
type Game struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Slug          string    `gorm:"type:text;uniqueIndex" json:"slug"`
	Status        string    `gorm:"type:text;default:'active'" json:"status"`
	Winner        int       `gorm:"default:0" json:"winner"`
	CurrentPlayer int       `gorm:"default:1" json:"current_player"`
	CurrentTurn   int       `gorm:"default:1" json:"current_turn"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Associations
	Tags  []Tag  `gorm:"foreignKey:GameID" json:"tags,omitempty"`
	Moves []Move `gorm:"foreignKey:GameID" json:"moves,omitempty"`
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

// AutoMigrate runs the database migrations
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Game{}, &Tag{}, &Move{})
}