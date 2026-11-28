package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nelhage/taktician/ptn"
	"github.com/nelhage/taktician/tak"
	"go.icco.me/gotak"
	"go.icco.me/gutil/vertex"
)

type fakeGenerator struct {
	responses []func(r vertex.Request, v any) (*vertex.Response, error)
	calls     []vertex.Request
	callCount int
}

func (f *fakeGenerator) GenerateJSON(_ context.Context, r vertex.Request, v any) (*vertex.Response, error) {
	f.calls = append(f.calls, r)
	idx := f.callCount
	f.callCount++
	if idx >= len(f.responses) {
		return nil, errors.New("fakeGenerator: no more programmed responses")
	}
	return f.responses[idx](r, v)
}

func newSimpleGame() *gotak.Game {
	return &gotak.Game{
		Board: &gotak.Board{
			Size: 3,
		},
		Turns: []*gotak.Turn{},
	}
}

func TestLLMEngineLegalReply(t *testing.T) {
	game := newSimpleGame()
	fake := &fakeGenerator{
		responses: []func(r vertex.Request, v any) (*vertex.Response, error){
			func(_ vertex.Request, v any) (*vertex.Response, error) {
				resp := v.(*llmMoveResponse)
				resp.Move = "a1"
				resp.Reason = "claim corner flat"
				return &vertex.Response{Usage: vertex.Usage{In: 100, Out: 20}}, nil
			},
		},
	}

	engine := NewLLMEngine(fake, "gemini-test")
	cfg := AIConfig{Personality: "cautious"}
	move, reason, err := engine.MoveWithReason(context.Background(), game, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if move != "a1" {
		t.Errorf("got move %q, want %q", move, "a1")
	}
	if reason != "claim corner flat" {
		t.Errorf("got reason %q, want %q", reason, "claim corner flat")
	}
	if fake.callCount != 1 {
		t.Errorf("callCount = %d, want 1", fake.callCount)
	}

	// Verify GetMove and ExplainMove delegates
	m, err := engine.GetMove(context.Background(), game, cfg)
	if err != nil || m != "a1" {
		t.Errorf("GetMove = (%q, %v), want a1", m, err)
	}
}

func TestLLMEngineRetryOnIllegalMove(t *testing.T) {
	game := newSimpleGame()
	fake := &fakeGenerator{
		responses: []func(r vertex.Request, v any) (*vertex.Response, error){
			func(_ vertex.Request, v any) (*vertex.Response, error) {
				resp := v.(*llmMoveResponse)
				resp.Move = "z9"
				resp.Reason = "invalid square"
				return &vertex.Response{}, nil
			},
			func(_ vertex.Request, v any) (*vertex.Response, error) {
				resp := v.(*llmMoveResponse)
				resp.Move = "b2"
				resp.Reason = "center placement"
				return &vertex.Response{}, nil
			},
		},
	}

	engine := NewLLMEngine(fake, "gemini-test")
	move, reason, err := engine.MoveWithReason(context.Background(), game, AIConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if move != "b2" {
		t.Errorf("got move %q, want %q", move, "b2")
	}
	if reason != "center placement" {
		t.Errorf("got reason %q, want %q", reason, "center placement")
	}
	if fake.callCount != 2 {
		t.Errorf("callCount = %d, want 2", fake.callCount)
	}

	if len(fake.calls) >= 2 {
		userPrompt := fake.calls[1].Parts[0].Text
		if !strings.Contains(userPrompt, `Your previous move "z9" was not legal`) {
			t.Errorf("expected retry prompt to mention previous move, got %s", userPrompt)
		}
	}
}

func TestLLMEngineTwiceIllegalFallback(t *testing.T) {
	game := newSimpleGame()
	fake := &fakeGenerator{
		responses: []func(r vertex.Request, v any) (*vertex.Response, error){
			func(_ vertex.Request, v any) (*vertex.Response, error) {
				resp := v.(*llmMoveResponse)
				resp.Move = "invalid1"
				resp.Reason = "bad1"
				return &vertex.Response{}, nil
			},
			func(_ vertex.Request, v any) (*vertex.Response, error) {
				resp := v.(*llmMoveResponse)
				resp.Move = "invalid2"
				resp.Reason = "bad2"
				return &vertex.Response{}, nil
			},
		},
	}

	engine := NewLLMEngine(fake, "gemini-test")
	move, reason, err := engine.MoveWithReason(context.Background(), game, AIConfig{})
	if err != nil {
		t.Fatalf("fallback should succeed, got error: %v", err)
	}
	if move == "" {
		t.Errorf("expected non-empty move from fallback")
	}
	if !strings.Contains(reason, "fell back to Taktician") {
		t.Errorf("expected fallback explanation, got %q", reason)
	}
	if fake.callCount != 2 {
		t.Errorf("callCount = %d, want 2", fake.callCount)
	}
}

func TestLLMEngineGeneratorErrorFallback(t *testing.T) {
	game := newSimpleGame()
	fake := &fakeGenerator{
		responses: []func(r vertex.Request, v any) (*vertex.Response, error){
			func(_ vertex.Request, _ any) (*vertex.Response, error) {
				return nil, errors.New("upstream timeout")
			},
			func(_ vertex.Request, _ any) (*vertex.Response, error) {
				return nil, errors.New("upstream timeout again")
			},
		},
	}

	engine := NewLLMEngine(fake, "gemini-test")
	move, reason, err := engine.MoveWithReason(context.Background(), game, AIConfig{})
	if err != nil {
		t.Fatalf("fallback should succeed after generator errors, got: %v", err)
	}
	if move == "" {
		t.Errorf("expected non-empty move from fallback")
	}
	if !strings.Contains(reason, "fell back to Taktician") {
		t.Errorf("expected fallback in reason, got %q", reason)
	}
}

func TestLLMEnginePersonalityCapped(t *testing.T) {
	game := newSimpleGame()
	longPersonality := strings.Repeat("A very long personality description that should be capped. ", 20)
	fake := &fakeGenerator{
		responses: []func(r vertex.Request, v any) (*vertex.Response, error){
			func(_ vertex.Request, v any) (*vertex.Response, error) {
				resp := v.(*llmMoveResponse)
				resp.Move = "a1"
				resp.Reason = "reason"
				return &vertex.Response{}, nil
			},
		},
	}

	engine := NewLLMEngine(fake, "gemini-test")
	_, _, err := engine.MoveWithReason(context.Background(), game, AIConfig{Personality: longPersonality})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) == 0 {
		t.Fatalf("expected at least one call")
	}
	system := fake.calls[0].System
	if !strings.Contains(system, "Personality flavor") {
		t.Errorf("expected personality flavor header in system prompt")
	}
	lines := strings.Split(system, "\n")
	lastLine := lines[len(lines)-1]
	if len([]rune(lastLine)) > maxPersonalityRunes {
		t.Errorf("personality length %d exceeds max %d", len([]rune(lastLine)), maxPersonalityRunes)
	}
}

func TestBuildGamePositionLegalMoves(t *testing.T) {
	game := &gotak.Game{
		Board: &gotak.Board{Size: 3},
		Turns: []*gotak.Turn{
			{
				Number: 1,
				First:  &gotak.Move{Text: "a1"},
				Second: &gotak.Move{Text: "b2"},
			},
		},
	}

	pos, legalMoves, legalMap, err := buildGamePosition(game)
	if err != nil {
		t.Fatalf("buildGamePosition failed: %v", err)
	}
	if pos.Size() != 3 {
		t.Errorf("size = %d, want 3", pos.Size())
	}
	if pos.MoveNumber() != 2 {
		t.Errorf("move number = %d, want 2", pos.MoveNumber())
	}

	// Turn 1 first move places Black flat on a1.
	// Turn 1 second move places White flat on b2.
	// Now White to move on turn 2. a1 and b2 should be occupied flats.
	if sq := pos.At(0, 0); len(sq) == 0 || sq[0].Color() != tak.Black {
		t.Errorf("expected black flat at a1, got %v", sq)
	}
	if sq := pos.At(1, 1); len(sq) == 0 || sq[0].Color() != tak.White {
		t.Errorf("expected white flat at b2, got %v", sq)
	}

	// Verify legal moves: flat placements on empty squares (7 squares left),
	// standing placements on empty squares (7 squares left), capstone placements if any,
	// plus slides of the friendly b2 stack if possible.
	for _, m := range legalMoves {
		if _, ok := legalMap[m]; !ok {
			t.Errorf("move %v not in legalMap", m)
		}
		formatted := ptn.FormatMove(m)
		parsed, err := ptn.ParseMove(formatted)
		if err != nil {
			t.Errorf("ptn.FormatMove(%v) produced unparseable %q: %v", m, formatted, err)
		}
		if parsed != m {
			t.Errorf("roundtrip mismatch: got %v, want %v", parsed, m)
		}
	}
}

func TestNewLLMEngineFromEnv(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "")
	engine, err := NewLLMEngineFromEnv(context.Background())
	if err != nil || engine != nil {
		t.Errorf("unset key should yield (nil, nil), got (%v, %v)", engine, err)
	}

	t.Setenv("GEMINI_API_KEY", "secret-virtual-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "")
	engine, err = NewLLMEngineFromEnv(context.Background())
	if err == nil || engine != nil {
		t.Errorf("key without base URL must return error, got (%v, %v)", engine, err)
	}

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"move\":\"a1\",\"reason\":\"corner\"}"}]}}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":10}}`
		_, _ = w.Write([]byte(resp))
	}))
	defer proxy.Close()

	t.Setenv("GEMINI_API_KEY", "alice-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", proxy.URL)
	t.Setenv("GEMINI_MODEL", "gemini-test-flash")

	engine, err = NewLLMEngineFromEnv(context.Background())
	if err != nil || engine == nil {
		t.Fatalf("NewLLMEngineFromEnv with proxy failed: %v", err)
	}
	if engine.model != "gemini-test-flash" {
		t.Errorf("model = %q, want gemini-test-flash", engine.model)
	}

	game := newSimpleGame()
	move, reason, err := engine.MoveWithReason(context.Background(), game, AIConfig{})
	if err != nil {
		t.Fatalf("MoveWithReason via proxy failed: %v", err)
	}
	if move != "a1" {
		t.Errorf("got move %q, want a1", move)
	}
	if reason != "corner" {
		t.Errorf("got reason %q, want corner", reason)
	}
}
