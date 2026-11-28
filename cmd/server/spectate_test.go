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

	count, err := countSpectators(context.Background(), db, game.ID, game.WhitePlayerID, game.BlackPlayerID)
	if err != nil {
		t.Fatalf("countSpectators: %v", err)
	}
	if count != 3 {
		t.Fatalf("got spectator count %d, want 3", count)
	}
}

func TestSpectate_CleanExpiredSpectators(t *testing.T) {
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

	now := time.Now()
	expired := Spectator{
		GameID:    game.ID,
		Session:   "session-expired",
		LastSeen:  now.Add(-10 * time.Minute),
		CreatedAt: now.Add(-10 * time.Minute),
	}
	active := Spectator{
		GameID:    game.ID,
		Session:   "session-active",
		LastSeen:  now.Add(-1 * time.Minute),
		CreatedAt: now.Add(-1 * time.Minute),
	}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatalf("create expired: %v", err)
	}
	if err := db.Create(&active).Error; err != nil {
		t.Fatalf("create active: %v", err)
	}

	if err := cleanExpiredSpectators(db, 5*time.Minute); err != nil {
		t.Fatalf("cleanExpiredSpectators: %v", err)
	}

	var remaining []Spectator
	if err := db.Where("game_id = ?", game.ID).Find(&remaining).Error; err != nil {
		t.Fatalf("find remaining: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Session != "session-active" {
		t.Fatalf("expected only session-active remaining, got %v", remaining)
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
	hub, c1 := mgr.registerSpectator(slug, game, nil, db, logger)
	_, c2 := mgr.registerSpectator(slug, game, nil, db, logger)

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
	hub, client := mgr.registerSpectator(slug, game, nil, db, logger)
	defer mgr.unregisterSpectator(slug, hub, client)

	// Fill the buffer to capacity.
	for len(client.send) < cap(client.send) {
		client.send <- []byte(`{"dummy":1}`)
	}

	// Trigger broadcast on full buffer; lagging client should be signaled to disconnect.
	hub.broadcast([]byte(`{"type":"overflow"}`))

	select {
	case <-client.done:
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

	var updateMsg WSStateMessage
	for {
		_, updateBytes, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read update message: %v", err)
		}
		var peek struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(updateBytes, &peek)
		if peek.Type == "state" {
			var sm WSStateMessage
			_ = json.Unmarshal(updateBytes, &sm)
			if sm.Game != nil && len(sm.Game.Turns) > 0 && sm.Game.Turns[0].First != nil {
				updateMsg = sm
				break
			}
		}
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

	// Drain frames in background to process pong responses for conn.Ping.
	go func() {
		for {
			if _, _, rerr := conn.Read(ctx); rerr != nil {
				return
			}
		}
	}()

	// Sleep 250ms, longer than both ReadTimeout and WriteTimeout (100ms).
	time.Sleep(250 * time.Millisecond)

	// Connection survives because deadlines were cleared before accepting.
	pingCtx, pingCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer pingCancel()

	if err := conn.Ping(pingCtx); err != nil {
		t.Fatalf("ping failed after server timeout duration: %v", err)
	}
}

func TestSpectate_ChatAuthRequired(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Anonymous connection without token.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate"
	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Read initial state message.
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("initial read: %v", err)
	}

	// Attempt to post chat anonymously.
	chatFrame := `{"type":"chat","body":"hello anonymous"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(chatFrame)); err != nil {
		t.Fatalf("write chat frame: %v", err)
	}

	// Receive error response.
	_, errBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var errResp map[string]string
	if err := json.Unmarshal(errBytes, &errResp); err != nil {
		t.Fatalf("unmarshal error frame: %v", err)
	}
	if errResp["type"] != "error" || !strings.Contains(errResp["error"], "sign-in required") {
		t.Fatalf("expected sign-in required error, got %v", errResp)
	}

	var count int64
	if err := db.Model(&SpectatorMessage{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 messages in DB, got %d", count)
	}
}

func TestSpectate_ChatSanitizationAndLengthCap(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	viewer := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	token, err := generateJWTForUser(viewer)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate?token=" + token
	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Read initial state message.
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("initial read: %v", err)
	}

	// Send message exceeding 500 characters.
	tooLong := strings.Repeat("x", 501)
	if err := conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"chat","body":%q}`, tooLong))); err != nil {
		t.Fatalf("write long: %v", err)
	}

	_, errBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	var errResp map[string]string
	if err := json.Unmarshal(errBytes, &errResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if errResp["type"] != "error" || !strings.Contains(errResp["error"], "too long") {
		t.Fatalf("expected too long error, got %v", errResp)
	}

	// Send HTML/script content to test ugcPolicy sanitization.
	scriptMsg := `<script>alert('hack')</script>hello <b>world</b>`
	if err := conn.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"chat","body":%q}`, scriptMsg))); err != nil {
		t.Fatalf("write script: %v", err)
	}

	var chatMsg WSChatMessage
	for {
		_, chatBytes, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read sanitized chat: %v", err)
		}
		var peek struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(chatBytes, &peek)
		if peek.Type == "chat" {
			_ = json.Unmarshal(chatBytes, &chatMsg)
			break
		}
	}
	if chatMsg.Body != "hello world" {
		t.Fatalf("expected body 'hello world', got %q", chatMsg.Body)
	}
	if chatMsg.AuthorName != "Charlie Bucket" {
		t.Fatalf("expected author name 'Charlie Bucket', got %q", chatMsg.AuthorName)
	}
}

func TestSpectate_ChatRateLimit(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	viewer := createNamedTestUser(t, db, "Bob Smith", "bob@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	token, err := generateJWTForUser(viewer)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate?token=" + token
	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Read initial state message.
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("initial read: %v", err)
	}

	// Send 5 permitted messages.
	for i := 1; i <= 5; i++ {
		msg := fmt.Sprintf(`{"type":"chat","body":"msg %d"}`, i)
		if err := conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
			t.Fatalf("write msg %d: %v", i, err)
		}
		// Read the broadcasted chat frame.
		_, _, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read msg %d broadcast: %v", i, err)
		}
	}

	// 6th message should be rate-limited.
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"chat","body":"msg 6"}`)); err != nil {
		t.Fatalf("write msg 6: %v", err)
	}

	var errResp map[string]any
	for {
		_, msgBytes, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read rate limit error: %v", err)
		}
		var peek map[string]any
		if err := json.Unmarshal(msgBytes, &peek); err == nil && peek["type"] == "error" {
			errResp = peek
			break
		}
	}
	if !strings.Contains(fmt.Sprint(errResp["error"]), "rate limit exceeded") {
		t.Fatalf("expected rate limit error, got %v", errResp)
	}
}

func TestSpectate_ChatPlayersExcludedWhileActive(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	bob := createNamedTestUser(t, db, "Bob Smith", "bob@example.com")
	viewer := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}
	game.BlackPlayerID = &bob.ID
	game.Status = "active"
	if err := db.Save(&game).Error; err != nil {
		t.Fatalf("save game: %v", err)
	}

	adaToken, err := generateJWTForUser(ada)
	if err != nil {
		t.Fatalf("ada token: %v", err)
	}
	viewerToken, err := generateJWTForUser(viewer)
	if err != nil {
		t.Fatalf("viewer token: %v", err)
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect Ada (player of active game).
	adaURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate?token=" + adaToken
	adaConn, respA, err := websocket.Dial(ctx, adaURL, nil)
	if respA != nil && respA.Body != nil {
		_ = respA.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial ada: %v", err)
	}
	defer func() { _ = adaConn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Connect Charlie (spectator).
	viewerURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate?token=" + viewerToken
	viewerConn, respV, err := websocket.Dial(ctx, viewerURL, nil)
	if respV != nil && respV.Body != nil {
		_ = respV.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial viewer: %v", err)
	}
	defer func() { _ = viewerConn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Both receive initial state.
	_, _, _ = adaConn.Read(ctx)
	_, _, _ = viewerConn.Read(ctx)

	// Ada attempts to chat while active; should be rejected.
	if err := adaConn.Write(ctx, websocket.MessageText, []byte(`{"type":"chat","body":"player chat"}`)); err != nil {
		t.Fatalf("write ada chat: %v", err)
	}
	var errResp map[string]string
	for {
		_, errBytes, err := adaConn.Read(ctx)
		if err != nil {
			t.Fatalf("ada read error: %v", err)
		}
		var peek struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(errBytes, &peek)
		if peek.Type == "error" {
			_ = json.Unmarshal(errBytes, &errResp)
			break
		}
	}
	if !strings.Contains(errResp["error"], "players cannot chat") {
		t.Fatalf("expected players cannot chat error, got %v", errResp)
	}

	// Charlie sends a chat message.
	if err := viewerConn.Write(ctx, websocket.MessageText, []byte(`{"type":"chat","body":"spectator cheers"}`)); err != nil {
		t.Fatalf("write viewer chat: %v", err)
	}

	// Charlie receives the broadcast.
	var vChat WSChatMessage
	for {
		_, viewerChatBytes, err := viewerConn.Read(ctx)
		if err != nil {
			t.Fatalf("viewer read chat: %v", err)
		}
		var peek struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(viewerChatBytes, &peek)
		if peek.Type == "chat" {
			_ = json.Unmarshal(viewerChatBytes, &vChat)
			break
		}
	}
	if vChat.Type != "chat" || vChat.Body != "spectator cheers" {
		t.Fatalf("viewer unexpected msg: %+v", vChat)
	}

	// Ada must NOT receive this chat message while the game is active.
	adaReadCtx, adaReadCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer adaReadCancel()
	for {
		_, adaBytes, err := adaConn.Read(adaReadCtx)
		if err != nil {
			// Expected timeout or error without chat message.
			break
		}
		var peek struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(adaBytes, &peek)
		if peek.Type == "chat" {
			t.Fatal("Ada (player) unexpectedly received spectator chat while game is active")
		}
	}
}

func TestSpectate_ChatHistoryOnConnect(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	viewer := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}

	// Pre-populate 3 chat messages.
	t1 := time.Now().Add(-3 * time.Minute)
	t2 := time.Now().Add(-2 * time.Minute)
	t3 := time.Now().Add(-1 * time.Minute)
	msgs := []SpectatorMessage{
		{GameID: game.ID, UserID: viewer.ID, Body: "first message", CreatedAt: t1},
		{GameID: game.ID, UserID: viewer.ID, Body: "second message", CreatedAt: t2},
		{GameID: game.ID, UserID: viewer.ID, Body: "third message", CreatedAt: t3},
	}
	for i := range msgs {
		if err := db.Create(&msgs[i]).Error; err != nil {
			t.Fatalf("create msg: %v", err)
		}
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate"
	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// First message is the initial game state.
	_, initBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var stateMsg WSStateMessage
	if err := json.Unmarshal(initBytes, &stateMsg); err != nil {
		t.Fatalf("unmarshal init: %v", err)
	}
	if stateMsg.Type != "state" {
		t.Fatalf("expected state message, got %q", stateMsg.Type)
	}

	// Next 3 messages should be the chat history in chronological order.
	expectedBodies := []string{"first message", "second message", "third message"}
	for _, wantBody := range expectedBodies {
		_, chatBytes, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read history: %v", err)
		}
		var cMsg WSChatMessage
		if err := json.Unmarshal(chatBytes, &cMsg); err != nil {
			t.Fatalf("unmarshal chat: %v", err)
		}
		if cMsg.Type != "chat" || cMsg.Body != wantBody {
			t.Fatalf("got chat body %q, want %q", cMsg.Body, wantBody)
		}
	}
}

func TestSpectate_ChatCrossInstancePoller(t *testing.T) {
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	viewer := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}

	logger := zaptest.NewLogger(t).Sugar()
	// Manager simulating Node 2.
	mgr2 := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	hub2, client2 := mgr2.registerSpectator(slug, game, nil, db, logger)
	defer mgr2.unregisterSpectator(slug, hub2, client2)

	// Consume initial state snapshot on client2.
	select {
	case <-client2.send:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for initial state on client2")
	}

	// Insert message directly into DB (simulating Node 1 writing it).
	msg := SpectatorMessage{
		GameID:    game.ID,
		UserID:    viewer.ID,
		Body:      "message from instance 1",
		CreatedAt: time.Now(),
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create msg: %v", err)
	}

	// Poller on Node 2 should detect the new maxChatID and broadcast to client2.
	var chatMsg WSChatMessage
	timeout := time.After(2 * time.Second)
	found := false
	for !found {
		select {
		case raw := <-client2.send:
			var peek struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &peek); err == nil && peek.Type == "chat" {
				_ = json.Unmarshal(raw, &chatMsg)
				found = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for node 2 poller to pick up chat message from DB")
		}
	}
	if chatMsg.Body != "message from instance 1" {
		t.Fatalf("unexpected message on client2: %+v", chatMsg)
	}
	if chatMsg.AuthorName != "Charlie Bucket" {
		t.Fatalf("unexpected author: %q", chatMsg.AuthorName)
	}
}

func TestSpectate_ChatHTTPEndpoint(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	bob := createNamedTestUser(t, db, "Bob Smith", "bob@example.com")
	viewer := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}
	game.BlackPlayerID = &bob.ID
	game.Status = "active"
	if err := db.Save(&game).Error; err != nil {
		t.Fatalf("save game: %v", err)
	}

	msg := SpectatorMessage{
		GameID:    game.ID,
		UserID:    viewer.ID,
		Body:      "great match!",
		CreatedAt: time.Now(),
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create msg: %v", err)
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
	req404, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/game/nonexistent/chat", nil)
	resp404, err := http.DefaultClient.Do(req404)
	if err != nil {
		t.Fatalf("GET 404: %v", err)
	}
	defer func() { _ = resp404.Body.Close() }()
	if resp404.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", resp404.StatusCode)
	}

	// 403 for active player Ada
	adaToken, _ := generateJWTForUser(ada)
	req403, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/game/"+slug+"/chat", nil)
	req403.Header.Set("Authorization", "Bearer "+adaToken)
	resp403, err := http.DefaultClient.Do(req403)
	if err != nil {
		t.Fatalf("GET 403: %v", err)
	}
	defer func() { _ = resp403.Body.Close() }()
	if resp403.StatusCode != http.StatusForbidden {
		t.Fatalf("got status %d, want 403", resp403.StatusCode)
	}

	// 200 for public/spectator
	req200, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/game/"+slug+"/chat", nil)
	resp200, err := http.DefaultClient.Do(req200)
	if err != nil {
		t.Fatalf("GET 200: %v", err)
	}
	defer func() { _ = resp200.Body.Close() }()
	if resp200.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp200.StatusCode)
	}
	var chatHist ChatHistoryResponse
	if err := json.NewDecoder(resp200.Body).Decode(&chatHist); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if chatHist.Slug != slug || len(chatHist.Messages) != 1 || chatHist.Messages[0].Body != "great match!" {
		t.Fatalf("unexpected chat history: %+v", chatHist)
	}
}

func TestSpectate_ChatWebSocketE2E(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	viewer := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	token, err := generateJWTForUser(viewer)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate?token=" + token
	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Read initial state message.
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("initial read: %v", err)
	}

	// Send chat message over WebSocket.
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"chat","body":"live chat test"}`)); err != nil {
		t.Fatalf("write chat: %v", err)
	}

	// Read broadcasted chat frame.
	var receivedChat WSChatMessage
	for {
		_, msgBytes, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var peek struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(msgBytes, &peek)
		if peek.Type == "chat" {
			_ = json.Unmarshal(msgBytes, &receivedChat)
			break
		}
	}

	if receivedChat.Body != "live chat test" {
		t.Fatalf("expected body 'live chat test', got %q", receivedChat.Body)
	}
	if receivedChat.AuthorName != "Charlie Bucket" {
		t.Fatalf("expected author 'Charlie Bucket', got %q", receivedChat.AuthorName)
	}
	if receivedChat.UserID != viewer.ID {
		t.Fatalf("expected user id %d, got %d", viewer.ID, receivedChat.UserID)
	}
}

func TestSpectate_ChatFull50HistoryOnConnect(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "test-secret")
	db := setupTestDB(t)
	ada := createNamedTestUser(t, db, "Ada Lovelace", "ada@example.com")
	viewer := createNamedTestUser(t, db, "Charlie Bucket", "charlie@example.com")

	slug, err := createGame(db, 5, ada.ID, "human")
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	var game Game
	if err := db.Where("slug = ?", slug).First(&game).Error; err != nil {
		t.Fatalf("find game: %v", err)
	}

	// Insert 50 messages.
	now := time.Now()
	for i := 1; i <= 50; i++ {
		m := SpectatorMessage{
			GameID:    game.ID,
			UserID:    viewer.ID,
			Body:      fmt.Sprintf("history-%02d", i),
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("create msg %d: %v", i, err)
		}
	}

	mgr := newSpectateHubManager(20*time.Millisecond, 1*time.Second)
	handler := buildRouter(routerOptions{
		IsDev:      true,
		DB:         db,
		HubManager: mgr,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/game/" + slug + "/spectate"
	conn, dialResp, err := websocket.Dial(ctx, wsURL, nil)
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test finished") }()

	// Read initial state message.
	_, initBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var stateMsg WSStateMessage
	if err := json.Unmarshal(initBytes, &stateMsg); err != nil {
		t.Fatalf("unmarshal init: %v", err)
	}
	if stateMsg.Type != "state" {
		t.Fatalf("expected state message, got %q", stateMsg.Type)
	}

	// Read all 50 history messages in order.
	for i := 1; i <= 50; i++ {
		_, msgBytes, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read history msg %d: %v", i, err)
		}
		var chat WSChatMessage
		if err := json.Unmarshal(msgBytes, &chat); err != nil {
			t.Fatalf("unmarshal history msg %d: %v", i, err)
		}
		wantBody := fmt.Sprintf("history-%02d", i)
		if chat.Body != wantBody {
			t.Fatalf("msg %d: got body %q, want %q", i, chat.Body, wantBody)
		}
	}
}
