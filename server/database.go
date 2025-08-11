package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ifo/sanic"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/icco/gotak"
)

// zapGormLogger adapts zap logger for GORM
type zapGormLogger struct {
	logger *zap.Logger
	level  logger.LogLevel
}

// NewZapGormLogger creates a new GORM logger using zap
func NewZapGormLogger(zapLogger *zap.Logger, level logger.LogLevel) logger.Interface {
	return &zapGormLogger{
		logger: zapLogger,
		level:  level,
	}
}

func (l *zapGormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &zapGormLogger{
		logger: l.logger,
		level:  level,
	}
}

func (l *zapGormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Info {
		l.logger.Sugar().Infof(msg, data...)
	}
}

func (l *zapGormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Warn {
		l.logger.Sugar().Warnf(msg, data...)
	}
}

func (l *zapGormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Error {
		l.logger.Sugar().Errorf(msg, data...)
	}
}

func (l *zapGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []zap.Field{
		zap.String("sql", sql),
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
	}

	if err != nil && l.level >= logger.Error {
		l.logger.Error("database query failed", append(fields, zap.Error(err))...)
	} else if elapsed > 200*time.Millisecond && l.level >= logger.Warn {
		l.logger.Warn("slow database query", fields...)
	} else if l.level >= logger.Info {
		l.logger.Info("database query", fields...)
	}
}

func getDB() (*gorm.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}

	// Configure GORM to use our zap logger
	config := &gorm.Config{
		Logger: NewZapGormLogger(log.Desugar(), logger.Warn),
	}

	db, err := gorm.Open(postgres.Open(dbURL), config)
	if err != nil {
		return nil, err
	}

	// Auto-migrate the schema
	err = AutoMigrate(db)
	if err != nil {
		return nil, fmt.Errorf("failed to run auto-migration: %v", err)
	}

	return db, nil
}

func createGame(db *gorm.DB, size int) (string, error) {
	if size < 4 {
		size = 6
	}

	// Game Slug
	worker := sanic.NewWorker7()
	id := worker.NextID()
	slug := worker.IDString(id)

	game := Game{
		Slug: slug,
	}

	if err := db.Create(&game).Error; err != nil {
		return "", err
	}

	return slug, updateTag(db, slug, "Size", strconv.Itoa(size))
}

func updateTag(db *gorm.DB, slug, key, value string) error {
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		return err
	}

	tag := Tag{
		GameID: game.ID,
		Key:    key,
		Value:  value,
	}

	return db.Create(&tag).Error
}

func insertMove(db *gorm.DB, gameID int64, player int, text string, turnNumber int64) error {
	move := Move{
		GameID: gameID,
		Player: player,
		Text:   text,
		Turn:   turnNumber,
	}

	return db.Create(&move).Error
}

func getGameID(db *gorm.DB, slug string) (int64, error) {
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		return 0, err
	}
	return game.ID, nil
}

func getGame(db *gorm.DB, slug string) (*gotak.Game, error) {
	id, err := getGameID(db, slug)
	if err != nil {
		return nil, err
	}

	// Get Size
	var tag Tag
	if err := db.Where("game_id = ? AND key = ?", id, "Size").First(&tag).Error; err != nil {
		return nil, err
	}

	size, err := strconv.ParseInt(tag.Value, 10, 64)
	if err != nil {
		return nil, err
	}

	// Init game
	game, err := gotak.NewGame(size, id, slug)
	if err != nil {
		return game, err
	}

	err = getTurns(db, game)
	if err != nil {
		return game, err
	}

	err = getMeta(db, game)
	if err != nil {
		return game, err
	}

	return game, nil
}

func getTurns(db *gorm.DB, game *gotak.Game) error {
	var moves []Move
	if err := db.Where("game_id = ?", game.ID).Order("turn, created_at").Find(&moves).Error; err != nil {
		return err
	}

	for _, move := range moves {
		currentTurn, err := game.GetTurn(move.Turn)
		if err != nil {
			return err
		}

		mv, err := gotak.NewMove(move.Text)
		if err != nil {
			return err
		}

		if move.Player == gotak.PlayerWhite {
			if move.Turn > 1 {
				currentTurn.First = mv
			} else {
				currentTurn.Second = mv
			}
		}

		if move.Player == gotak.PlayerBlack {
			if move.Turn > 1 {
				currentTurn.Second = mv
			} else {
				currentTurn.First = mv
			}
		}

		game.UpdateTurn(currentTurn)
	}

	return nil
}

func getMeta(db *gorm.DB, game *gotak.Game) error {
	var tags []Tag
	if err := db.Where("game_id = ?", game.ID).Order("created_at").Find(&tags).Error; err != nil {
		return err
	}

	for _, tag := range tags {
		err := game.UpdateMeta(tag.Key, tag.Value)
		if err != nil {
			return err
		}
	}

	return nil
}

// replayMoves replays all moves in a game to restore board state
func replayMoves(game *gotak.Game) error {
	// Reset board
	err := game.Board.Init()
	if err != nil {
		return err
	}

	// Replay all moves in order
	for _, turn := range game.Turns {
		if turn.First != nil {
			if turn.Number == 1 {
				// First turn: white places black stone
				err = game.Board.DoMove(turn.First, gotak.PlayerBlack)
			} else {
				err = game.Board.DoMove(turn.First, gotak.PlayerWhite)
			}
			if err != nil {
				return fmt.Errorf("error replaying turn %d first move: %v", turn.Number, err)
			}
		}

		if turn.Second != nil {
			if turn.Number == 1 {
				// First turn: black places white stone
				err = game.Board.DoMove(turn.Second, gotak.PlayerWhite)
			} else {
				err = game.Board.DoMove(turn.Second, gotak.PlayerBlack)
			}
			if err != nil {
				return fmt.Errorf("error replaying turn %d second move: %v", turn.Number, err)
			}
		}
	}

	return nil
}

// updateGameStatus updates the game status in the database
func updateGameStatus(db *gorm.DB, slug, status string, winner int) error {
	result := db.Model(&Game{}).Where("slug = ?", slug).Updates(Game{
		Status: status,
		Winner: winner,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
