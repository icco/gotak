package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.icco.me/gotak"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
)

func createNamedTestUser(t *testing.T, db *gorm.DB, name, email string) *User {
	t.Helper()
	u := &User{
		Provider:    "local",
		ProviderID:  fmt.Sprintf("provider-%s", email),
		Email:       email,
		Name:        name,
		Preferences: emptyJSONObject,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func TestSpectate_PresenceCounting(t *testing.T) {
	db := setupTestDB(t)

	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	bob := createNamedTestUser(t, db, "Bob Smith", "bob@example.com")
	charlie := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}
	game.BlackPlayerID = &bob.ID
	if err := db.Save(&game).Error; err != nil {
		t.Fatalf("save game: %v", err)
	}

	now := time.Now()

	specs := []Spectator{
		{GameID: game.ID, Session: "session-anon-active", UserID: nil, LastSeen: now, CreatedAt: now},
		{GameID: game.ID, Session: "session-charlie-active", UserID: &charlie.ID, LastSeen: now.Add(-10 * time.Second), CreatedAt: now.Add(-10 * time.Second)},
		{GameID: game.ID, Session: "session-charlie-tab2", UserID: &charlie.ID, LastSeen: now.Add(-20 * time.Second), CreatedAt: now.Add(-20 * time.Second)},
		{GameID: game.ID, Session: "session-ada-player", UserID: &ada.ID, LastSeen: now, CreatedAt: now},
		{GameID: game.ID, Session: "session-bob-player", UserID: &bob.ID, LastSeen: now, CreatedAt: now},
		{GameID: game.ID, Session: "session-anon-stale", UserID: nil, LastSeen: now.Add(-60 * time.Second), CreatedAt: now.Add(-60 * time.Second)},
	}
	for i := range specs {
		if err := db.Create(&specs[i]).Error; err != nil {
			t.Fatalf("create spectator: %v", err)
		}
	}

	count, err := countSpectators(db, game.ID, game.WhitePlayerID, game.BlackPlayerID)
	if err != nil {
		t.Fatalf("countSpectators: %v", err)
	}
	if count != 3 {
		t.Fatalf("got spectator count %d, want 3", count)
	}
}

func TestSpectate_HubFanOutAndPollerStop(t *testing.T) {
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}

	logger := zaptest.NewLogger(t).Sugar()
	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	hub, c1 := mgr.registerSpectator(slug, game.ID, db, logger)
	_, c2 := mgr.registerSpectator(slug, game.ID, db, logger)

	if err := insertMove(db, game.ID, gotak.PlayerWhite, "a1", 1); err != nil {
		t.Fatalf("insertMove: %v", err)
	}

	readStateMsg := func(c *spectatorClient) WSStateMessage {
		select {
		case raw := <-c.send:
			var msg WSStateMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			return msg
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for hub broadcast")
			return WSStateMessage{}
		}
	}

	msg1 := readStateMsg(c1)
	msg2 := readStateMsg(c2)

	if msg1.Type != "state" || msg2.Type != "state" {
		t.Fatalf("unexpected message types: %q, %q", msg1.Type, msg2.Type)
	}
	if msg1.Game.Slug != slug || msg2.Game.Slug != slug {
		t.Fatalf("unexpected game slugs in messages")
	}

	mgr.unregisterSpectator(slug, hub, c1)

	// Poller continues running while c2 remains subscribed.
	if err := insertMove(db, game.ID, gotak.PlayerBlack, "b2", 1); err != nil {
		t.Fatalf("insertMove 2: %v", err)
	}
	msg3 := readStateMsg(c2)
	if msg3.Type != "state" {
		t.Fatalf("expected state message on c2, got %q", msg3.Type)
	}

	mgr.unregisterSpectator(slug, hub, c2)

	select {
	case <-hub.pollerDone:
	case <-time.After(1 * time.Second):
		t.Fatal("poller failed to stop after last subscriber disconnected")
	}
}

func TestSpectate_LaggingClientDisconnected(t *testing.T) {
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}

	logger := zaptest.NewLogger(t).Sugar()
	mgr := newSpectateHubManager(10*time.Millisecond, 1*time.Second)
	hub, client := mgr.registerSpectator(slug, game.ID, db, logger)
	defer mgr.unregisterSpectator(slug, hub, client)

	// Fill the buffer to capacity (32 messages).
	for i := range cap(client.send) {
		client.send <- []byte(fmt.Sprintf(`{"dummy":%d}`, i))
	}

	// Trigger broadcast on full buffer; lagging client should be signaled to disconnect.
	hub.broadcast([]byte(`{"type":"overflow"}`))

	select {
	case <-client.done:
		// Client was signaled to disconnect rather than silently dropping messages.
	case <-time.After(1 * time.Second):
		t.Fatal("lagging client was not signaled to disconnect on buffer overflow")
	}
}

func TestSpectate_HTTPEndpoint(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	mgr := newSpectateHubManager(50*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()

	// 404 for unknown game
	req404, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/game/nonexistent-game/spectate", nil)
	if err != nil {
		t.Fatalf("new request 404: %v", err)
	}
	resp404, err := http.DefaultClient.Do(req404)
	if err != nil {
		t.Fatalf("GET unknown game: %v", err)
	}
	defer func() { _ = resp404.Body.Close() }()
	if resp404.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", resp404.StatusCode)
	}
	var errPayload ErrorResponse
	if err := json.NewDecoder(resp404.Body).Decode(&errPayload); err != nil {
		t.Fatalf("decode 404 json: %v", err)
	}
	if errPayload.Error != "game not found" {
		t.Fatalf("got error message %q, want 'game not found'", errPayload.Error)
	}

	// 200 for valid game
	req200, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/game/"+slug+"/spectate", nil)
	if err != nil {
		t.Fatalf("new request 200: %v", err)
	}
	resp200, err := http.DefaultClient.Do(req200)
	if err != nil {
		t.Fatalf("GET spectate: %v", err)
	}
	defer func() { _ = resp200.Body.Close() }()
	if resp200.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp200.StatusCode)
	}
	var spectateResp SpectateResponse
	if err := json.NewDecoder(resp200.Body).Decode(&spectateResp); err != nil {
		t.Fatalf("decode 200 json: %v", err)
	}
	if spectateResp.Slug != slug {
		t.Fatalf("got slug %q, want %q", spectateResp.Slug, slug)
	}
	if spectateResp.WebSocketURL != "/game/"+slug+"/spectate" {
		t.Fatalf("got websocket_url %q, want %q", spectateResp.WebSocketURL, "/game/"+slug+"/spectate")
	}
	if spectateResp.Game == nil || spectateResp.Game.Slug != slug {
		t.Fatalf("expected game payload populated with slug %q", slug)
	}
}

func TestSpectate_WebSocketE2E(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Verify initial state message
	_, initialBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read initial message: %v", err)
	}
	var initMsg WSStateMessage
	if err := json.Unmarshal(initialBytes, &initMsg); err != nil {
		t.Fatalf("unmarshal initial: %v", err)
	}
	if initMsg.Type != "state" || initMsg.Game.Slug != slug {
		t.Fatalf("unexpected initial state: %+v", initMsg)
	}
	if initMsg.Spectators != 1 {
		t.Fatalf("got initial spectator count %d, want 1", initMsg.Spectators)
	}

	// Insert move and verify real-time update
	if err := insertMove(db, game.ID, gotak.PlayerWhite, "a1", 1); err != nil {
		t.Fatalf("insert move: %v", err)
	}

	_, updateBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read update message: %v", err)
	}
	var updateMsg WSStateMessage
	if err := json.Unmarshal(updateBytes, &updateMsg); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if updateMsg.Type != "state" {
		t.Fatalf("expected state message, got %q", updateMsg.Type)
	}
	if len(updateMsg.Game.Turns) == 0 || updateMsg.Game.Turns[0].First == nil {
		t.Fatalf("expected move in updated state turns: %+v", updateMsg.Game.Turns)
	}
}

func TestSpectate_DeadlineSurvival(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	mgr := newSpectateHubManager(50*time.Millisecond, 2*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	// Short timeouts simulate main.go's 15s timeouts without slow test sleeps.
	shortTimeoutServer := &http.Server{
		Handler:      handler,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	}

	go func() {
		_ = shortTimeoutServer.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = shortTimeoutServer.Close()
	})

	wsURL := fmt.Sprintf("ws://%s/game/%s/spectate", listener.Addr().String(), slug)

	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Read initial message to verify connection is open.
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("initial read: %v", err)
	}

	// CloseRead on client enables pong processing for conn.Ping.
	clientCtx := conn.CloseRead(ctx)

	// Sleep 250ms, longer than both ReadTimeout and WriteTimeout (100ms).
	time.Sleep(250 * time.Millisecond)

	// Connection survives because deadlines were cleared before accepting.
	pingCtx, pingCancel := context.WithTimeout(clientCtx, 500*time.Millisecond)
	defer pingCancel()

	if err := conn.Ping(pingCtx); err != nil {
		t.Fatalf("ping failed after server timeout duration: %v", err)
	}
}
