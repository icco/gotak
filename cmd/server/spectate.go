package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SpectateResponse is returned by GET /game/{slug}/spectate over HTTP.
type SpectateResponse struct {
	*GameStateResponse
	Slug         string             `json:"slug"`
	Spectators   int                `json:"spectators"`
	WebSocketURL string             `json:"websocket_url"`
	Game         *GameStateResponse `json:"game"`
}

// WSStateMessage is sent when a spectator connects and whenever the game state updates.
type WSStateMessage struct {
	Type       string             `json:"type"`
	Game       *GameStateResponse `json:"game"`
	Spectators int                `json:"spectators"`
	Revision   int64              `json:"revision,omitempty"`
}

// WSSpectatorsMessage is sent when the spectator count updates without a state change.
type WSSpectatorsMessage struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

const (
	spectatorActiveThreshold    = 45 * time.Second
	spectatorCleanupGracePeriod = 5 * time.Minute
)

var defaultSpectateManager = newSpectateHubManager(1*time.Second, 15*time.Second)

// @Summary Spectate game
// @Description Returns the current game state, spectator count, and WebSocket URL for live updates.
// @Description When requested with WebSocket upgrade headers, opens a live spectating stream.
// @Tags game
// @Accept json
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Success 200 {object} SpectateResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /game/{slug}/spectate [get]
func spectateHandler(w http.ResponseWriter, r *http.Request) {
	spectateHandlerWithDB(w, r, nil, defaultSpectateManager)
}

func spectateHandlerWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB, mgr *spectateHubManager) {
	if db == nil {
		var err error
		db, err = getDB()
		if err != nil {
			l := logging.FromContext(r.Context())
			l.Errorw("could not get db", zap.Error(err))
			if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "bad connection to db"}); jerr != nil {
				l.Errorw("failed to render JSON", zap.Error(jerr))
			}
			return
		}
	}
	if mgr == nil {
		mgr = defaultSpectateManager
	}

	if isWebSocketUpgrade(r) {
		spectateWebSocketHandler(w, r, db, mgr)
		return
	}
	spectateHTTPHandler(w, r, db)
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func spectateHTTPHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	var game Game
	if err := db.WithContext(ctx).Where("slug = ?", slug).First(&game).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "game not found"}); jerr != nil {
				l.Errorw("failed to render JSON", zap.Error(jerr))
			}
			return
		}
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "database error"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	state, err := buildGameStateResponse(db, slug)
	if err != nil {
		l.Errorw("could not build game state", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not build game state"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	spectatorCount, err := countSpectators(ctx, db, game.ID, game.WhitePlayerID, game.BlackPlayerID)
	if err != nil {
		l.Errorw("could not count spectators", "slug", slug, zap.Error(err))
		spectatorCount = 0
	}

	resp := SpectateResponse{
		GameStateResponse: state,
		Slug:              slug,
		Spectators:        spectatorCount,
		WebSocketURL:      fmt.Sprintf("/game/%s/spectate", slug),
		Game:              state,
	}

	if err := Renderer.JSON(w, http.StatusOK, resp); err != nil {
		l.Errorw("failed to render JSON", zap.Error(err))
	}
}

func spectateWebSocketHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB, mgr *spectateHubManager) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	mgr.ensureCleanup(db)

	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))
	var game Game
	if err := db.WithContext(ctx).Where("slug = ?", slug).First(&game).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "game not found"}); jerr != nil {
				l.Errorw("failed to render JSON", zap.Error(jerr))
			}
			return
		}
		l.Errorw("could not get game", "slug", slug, zap.Error(err))
		if jerr := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "database error"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	// Deadlines from http.Server apply to hijacked connections and must be cleared.
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		l.Warnw("failed to clear read deadline", zap.Error(err))
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		l.Warnw("failed to clear write deadline", zap.Error(err))
	}

	// InsecureSkipVerify allows all origins because CORS is open and this stream is public.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		l.Errorw("failed to accept websocket connection", zap.Error(err))
		return
	}
	defer func() {
		if cerr := c.Close(websocket.StatusNormalClosure, "connection closed"); cerr != nil {
			l.Debugw("websocket close", zap.Error(cerr))
		}
	}()

	var uid *int64
	if u := getOptionalUser(r, db); u != nil {
		uid = &u.ID
	}

	sessionID := uuid.New().String()
	now := time.Now()
	spectator := Spectator{
		GameID:    game.ID,
		Session:   sessionID,
		UserID:    uid,
		LastSeen:  now,
		CreatedAt: now,
	}
	if err := upsertSpectator(db, &spectator); err != nil {
		l.Errorw("failed to upsert spectator presence", zap.Error(err))
		return
	}
	defer func() {
		if err := removeSpectator(db, game.ID, sessionID); err != nil {
			l.Warnw("failed to remove spectator presence", "session", sessionID, zap.Error(err))
		}
	}()

	hub, client := mgr.registerSpectator(slug, game.ID, db, l)
	defer mgr.unregisterSpectator(slug, hub, client)

	// CloseRead drains frames in background to respond to pings and detects client disconnect.
	connCtx := c.CloseRead(ctx)

	heartbeatTicker := time.NewTicker(mgr.heartbeatInterval)
	defer heartbeatTicker.Stop()

	var lastRevision int64

	for {
		select {
		case <-connCtx.Done():
			return
		case <-client.done:
			l.Warnw("disconnecting lagging spectator client", "slug", slug)
			return
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			var header struct {
				Type     string `json:"type"`
				Revision int64  `json:"revision"`
			}
			if err := json.Unmarshal(msg, &header); err == nil && header.Type == "state" && header.Revision > 0 {
				if header.Revision < lastRevision {
					continue
				}
				lastRevision = header.Revision
			}
			writeCtx, writeCancel := context.WithTimeout(connCtx, 5*time.Second)
			err := c.Write(writeCtx, websocket.MessageText, msg)
			writeCancel()
			if err != nil {
				l.Warnw("failed to write websocket message to spectator", zap.Error(err))
				return
			}
		case <-heartbeatTicker.C:
			spectator.LastSeen = time.Now()
			if err := upsertSpectator(db, &spectator); err != nil {
				l.Warnw("failed to refresh spectator heartbeat", zap.Error(err))
			}
			pingCtx, pingCancel := context.WithTimeout(connCtx, 5*time.Second)
			err := c.Ping(pingCtx)
			pingCancel()
			if err != nil {
				l.Infow("spectator ping failed; terminating connection", zap.Error(err))
				return
			}
		}
	}
}

func getOptionalUser(r *http.Request, db *gorm.DB) *User {
	var tokenString string
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenString = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		tokenString = r.URL.Query().Get("token")
	}
	if tokenString == "" {
		return nil
	}
	secret := os.Getenv("AUTH_JWT_SECRET")
	if secret == "" {
		return nil
	}
	auth := newAuthService()
	claims, err := auth.TokenService().Parse(tokenString)
	if err != nil {
		return nil
	}
	var user User
	if err := db.First(&user, claims.User.ID).Error; err != nil {
		return nil
	}
	return &user
}

func upsertSpectator(db *gorm.DB, s *Spectator) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "game_id"}, {Name: "session"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_seen", "user_id"}),
	}).Create(s).Error
}

func removeSpectator(db *gorm.DB, gameID int64, session string) error {
	return db.Where("game_id = ? AND session = ?", gameID, session).
		Delete(&Spectator{}).Error
}

func cleanExpiredSpectators(db *gorm.DB, gracePeriod time.Duration) error {
	cutoff := time.Now().Add(-gracePeriod)
	return db.Where("last_seen < ?", cutoff).Delete(&Spectator{}).Error
}

func countSpectators(ctx context.Context, db *gorm.DB, gameID int64, whitePlayerID, blackPlayerID *int64) (int, error) {
	cutoff := time.Now().Add(-spectatorActiveThreshold)
	q := db.WithContext(ctx).Model(&Spectator{}).Where("game_id = ? AND last_seen >= ?", gameID, cutoff)
	var excluded []int64
	if whitePlayerID != nil {
		excluded = append(excluded, *whitePlayerID)
	}
	if blackPlayerID != nil {
		excluded = append(excluded, *blackPlayerID)
	}
	if len(excluded) > 0 {
		q = q.Where("user_id IS NULL OR user_id NOT IN ?", excluded)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

type gameFingerprint struct {
	MoveCount     int64
	MaxMoveID     int64
	Status        string
	Winner        int
	CurrentTurn   int
	CurrentPlayer int
	UpdatedAtUnix int64
}

func getGameFingerprint(ctx context.Context, db *gorm.DB, gameID int64) (gameFingerprint, *int64, *int64, error) {
	var fp gameFingerprint
	var g Game
	if err := db.WithContext(ctx).Model(&Game{}).
		Select("status, winner, current_turn, current_player, updated_at, white_player_id, black_player_id").
		Where("id = ?", gameID).
		First(&g).Error; err != nil {
		return fp, nil, nil, err
	}
	fp.Status = g.Status
	fp.Winner = g.Winner
	fp.CurrentTurn = g.CurrentTurn
	fp.CurrentPlayer = g.CurrentPlayer
	fp.UpdatedAtUnix = g.UpdatedAt.UnixNano()

	var moveStats struct {
		Count int64
		MaxID int64
	}
	if err := db.WithContext(ctx).Model(&Move{}).
		Select("COUNT(*) as count, COALESCE(MAX(id), 0) as max_id").
		Where("game_id = ?", gameID).
		Scan(&moveStats).Error; err != nil {
		return fp, g.WhitePlayerID, g.BlackPlayerID, err
	}
	fp.MoveCount = moveStats.Count
	fp.MaxMoveID = moveStats.MaxID

	return fp, g.WhitePlayerID, g.BlackPlayerID, nil
}

type spectatorClient struct {
	send chan []byte
	done chan struct{}
	once sync.Once
}

func newSpectatorClient() *spectatorClient {
	return &spectatorClient{
		send: make(chan []byte, 32),
		done: make(chan struct{}),
	}
}

func (c *spectatorClient) disconnect() {
	c.once.Do(func() {
		close(c.done)
	})
}

type gameHub struct {
	slug       string
	gameID     int64
	db         *gorm.DB
	logger     *zap.SugaredLogger
	pollPeriod time.Duration

	mu           sync.Mutex
	subscribers  map[*spectatorClient]struct{}
	stopPoller   chan struct{}
	pollerDone   chan struct{}
	lastFP       gameFingerprint
	lastCount    int
	lastStateMsg []byte
	revision     int64
}

func newGameHub(slug string, gameID int64, db *gorm.DB, l *zap.SugaredLogger, pollPeriod time.Duration) *gameHub {
	return &gameHub{
		slug:        slug,
		gameID:      gameID,
		db:          db,
		logger:      l,
		pollPeriod:  pollPeriod,
		subscribers: make(map[*spectatorClient]struct{}),
		stopPoller:  make(chan struct{}),
		pollerDone:  make(chan struct{}),
	}
}

func (h *gameHub) runPoller() {
	defer close(h.pollerDone)

	// Build initial state snapshot for the first subscriber.
	h.pollAndBroadcast()

	ticker := time.NewTicker(h.pollPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopPoller:
			return
		case <-ticker.C:
			h.pollAndBroadcast()
		}
	}
}

func (h *gameHub) broadcast(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.subscribers {
		select {
		case client.send <- msg:
		default:
			// Disconnect lagging client so it reconnects for a fresh snapshot rather than staying stale.
			client.disconnect()
		}
	}
}

func (h *gameHub) pollAndBroadcast() {
	queryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fp, whiteID, blackID, err := getGameFingerprint(queryCtx, h.db, h.gameID)
	if err != nil {
		h.logger.Warnw("failed to get game fingerprint", "slug", h.slug, zap.Error(err))
		return
	}

	count, err := countSpectators(queryCtx, h.db, h.gameID, whiteID, blackID)
	if err != nil {
		h.logger.Warnw("failed to count spectators", "slug", h.slug, zap.Error(err))
		return
	}

	h.mu.Lock()
	stateChanged := fp != h.lastFP || h.lastStateMsg == nil
	countChanged := count != h.lastCount
	h.mu.Unlock()

	if !stateChanged && !countChanged {
		return
	}

	if stateChanged {
		state, err := buildGameStateResponse(h.db, h.slug)
		if err != nil {
			h.logger.Warnw("failed to build game state", "slug", h.slug, zap.Error(err))
			return
		}
		h.mu.Lock()
		h.revision++
		rev := h.revision
		h.mu.Unlock()

		msg, err := json.Marshal(WSStateMessage{
			Type:       "state",
			Game:       state,
			Spectators: count,
			Revision:   rev,
		})
		if err != nil {
			h.logger.Warnw("failed to marshal state message", zap.Error(err))
			return
		}
		h.mu.Lock()
		h.lastFP = fp
		h.lastCount = count
		h.lastStateMsg = msg
		h.mu.Unlock()
		h.broadcast(msg)
	} else if countChanged {
		msg, err := json.Marshal(WSSpectatorsMessage{
			Type:  "spectators",
			Count: count,
		})
		if err != nil {
			h.logger.Warnw("failed to marshal spectators message", zap.Error(err))
			return
		}
		h.mu.Lock()
		h.lastCount = count
		h.mu.Unlock()
		h.broadcast(msg)
	}
}

type spectateHubManager struct {
	mu                sync.Mutex
	hubs              map[string]*gameHub
	pollPeriod        time.Duration
	heartbeatInterval time.Duration
	cleanupOnce       sync.Once
	cleanupStop       chan struct{}
}

func newSpectateHubManager(pollPeriod, heartbeatInterval time.Duration) *spectateHubManager {
	return &spectateHubManager{
		hubs:              make(map[string]*gameHub),
		pollPeriod:        pollPeriod,
		heartbeatInterval: heartbeatInterval,
	}
}

func (mgr *spectateHubManager) ensureCleanup(db *gorm.DB) {
	mgr.cleanupOnce.Do(func() {
		mgr.cleanupStop = make(chan struct{})
		go func() {
			_ = cleanExpiredSpectators(db, spectatorCleanupGracePeriod)
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-mgr.cleanupStop:
					return
				case <-ticker.C:
					if err := cleanExpiredSpectators(db, spectatorCleanupGracePeriod); err != nil {
						log.Warnw("failed to clean expired spectators", zap.Error(err))
					}
				}
			}
		}()
	})
}

//nolint:contextcheck // hub poller outlives individual request contexts
func (mgr *spectateHubManager) registerSpectator(slug string, gameID int64, db *gorm.DB, l *zap.SugaredLogger) (*gameHub, *spectatorClient) {
	mgr.mu.Lock()
	hub, ok := mgr.hubs[slug]
	if !ok {
		hub = newGameHub(slug, gameID, db, l, mgr.pollPeriod)
		mgr.hubs[slug] = hub
	}

	client := newSpectatorClient()

	hub.mu.Lock()
	hub.subscribers[client] = struct{}{}
	isFirst := len(hub.subscribers) == 1
	var cachedMsg []byte
	if !isFirst && hub.lastStateMsg != nil {
		cachedMsg = hub.lastStateMsg
	}
	hub.mu.Unlock()
	mgr.mu.Unlock()

	if cachedMsg != nil {
		select {
		case client.send <- cachedMsg:
		default:
			client.disconnect()
		}
	}

	if isFirst {
		go hub.runPoller()
	}

	return hub, client
}

func (mgr *spectateHubManager) unregisterSpectator(slug string, hub *gameHub, client *spectatorClient) {
	mgr.mu.Lock()
	hub.mu.Lock()
	delete(hub.subscribers, client)
	close(client.send)
	shouldStop := len(hub.subscribers) == 0
	if shouldStop && mgr.hubs[slug] == hub {
		delete(mgr.hubs, slug)
	}
	hub.mu.Unlock()
	mgr.mu.Unlock()

	if shouldStop {
		close(hub.stopPoller)
		<-hub.pollerDone
	}
}
