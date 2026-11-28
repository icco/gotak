// Package ai provides AI engines for playing the Tak board game.
package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nelhage/taktician/ptn"
	"github.com/nelhage/taktician/tak"
	"go.icco.me/gotak"
	"go.icco.me/gutil/vertex"
	"google.golang.org/genai"
)

const (
	// DefaultGeminiModel is the default Gemini model used for Tak move generation.
	// https://ai.google.dev/gemini-api/docs/models
	DefaultGeminiModel = "gemini-2.5-flash"

	// maxPersonalityRunes caps personality flavor text injected into the system prompt.
	maxPersonalityRunes = 200

	// llmCallTimeout bounds each individual call to the generator to keep
	// the combined attempts comfortably within the server write timeout.
	llmCallTimeout = 5 * time.Second
)

// Generator abstracts the structured JSON generation performed against an LLM.
type Generator interface {
	GenerateJSON(ctx context.Context, r vertex.Request, v any) (*vertex.Response, error)
}

// MoveExplainer can return both a move and a reason in a single call.
type MoveExplainer interface {
	MoveWithReason(ctx context.Context, g *gotak.Game, cfg AIConfig) (move, reason string, err error)
}

// LLMEngine plays Tak using a language model via LiteLLM/Vertex.
type LLMEngine struct {
	gen      Generator
	fallback Engine
	model    string
}

type llmMoveResponse struct {
	Move   string `json:"move"`
	Reason string `json:"reason"`
}

// NewLLMEngine builds an LLM-backed Tak engine over the given generator.
func NewLLMEngine(gen Generator, model string) *LLMEngine {
	if model == "" {
		model = DefaultGeminiModel
	}
	return &LLMEngine{
		gen:      gen,
		fallback: &TakticianEngine{},
		model:    model,
	}
}

// NewLLMEngineFromEnv builds an LLM engine from the environment.
// GEMINI_API_KEY is the service's LiteLLM virtual key and GOOGLE_GEMINI_BASE_URL
// the proxy URL. When GEMINI_API_KEY is unset, it returns nil, nil.
// If GEMINI_API_KEY is set without GOOGLE_GEMINI_BASE_URL, it returns an error.
func NewLLMEngineFromEnv(ctx context.Context) (*LLMEngine, error) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return nil, nil
	}

	baseURL := os.Getenv("GOOGLE_GEMINI_BASE_URL")
	if baseURL == "" {
		return nil, errors.New("GEMINI_API_KEY is set without GOOGLE_GEMINI_BASE_URL; point it at the LiteLLM proxy")
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = DefaultGeminiModel
	}

	client, err := vertex.New(ctx, vertex.Config{
		APIKey:  key,
		BaseURL: baseURL,
		Model:   model,
	})
	if err != nil {
		return nil, fmt.Errorf("create vertex client: %w", err)
	}

	return NewLLMEngine(client, model), nil
}

// GetMove returns a legal move for the given game state.
func (e *LLMEngine) GetMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	move, _, err := e.MoveWithReason(ctx, g, cfg)
	return move, err
}

// ExplainMove explains the move chosen for the given game state.
func (e *LLMEngine) ExplainMove(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, error) {
	_, reason, err := e.MoveWithReason(ctx, g, cfg)
	return reason, err
}

// MoveWithReason queries the LLM for a move and explanation, falling back
// to TakticianEngine if the LLM fails or produces illegal moves.
func (e *LLMEngine) MoveWithReason(ctx context.Context, g *gotak.Game, cfg AIConfig) (string, string, error) {
	pos, legalMoves, legalMap, err := buildGamePosition(g)
	if err != nil {
		return e.fallbackMove(ctx, g, cfg, fmt.Sprintf("fallback to Taktician: position error: %v", err))
	}
	if len(legalMoves) == 0 {
		return "", "", fmt.Errorf("no legal moves available")
	}

	boardSize := pos.Size()
	systemPrompt := buildSystemPrompt(cfg.Personality)
	userPrompt := buildUserPrompt(g, pos, legalMoves)

	schema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"move":   {Type: genai.TypeString, Description: "The chosen move in PTN notation"},
			"reason": {Type: genai.TypeString, Description: "Strategic explanation for the chosen move"},
		},
		Required: []string{"move", "reason"},
	}

	req := vertex.Request{
		System:         systemPrompt,
		Parts:          vertex.Text(userPrompt),
		Schema:         schema,
		ThinkingBudget: vertex.NoThinking(),
		Temperature:    vertex.Temperature(0.2),
	}

	// First attempt
	callCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	var resp llmMoveResponse
	_, err = e.gen.GenerateJSON(callCtx, req, &resp)
	cancel()

	if err == nil {
		if chosenMove, ok := matchLegalMove(resp.Move, boardSize, legalMap); ok {
			return ptn.FormatMove(chosenMove), resp.Reason, nil
		}
	}

	// Retry once if illegal, unparseable, or generation failed
	retryUserPrompt := userPrompt
	if err != nil {
		retryUserPrompt += fmt.Sprintf("\n\nYour previous response encountered an error: %v. Please select a valid move from the legal moves list.", err)
	} else {
		retryUserPrompt += fmt.Sprintf("\n\nYour previous move %q was not legal or could not be parsed. You MUST choose one move exactly matching the legal moves list.", resp.Move)
	}

	retryReq := vertex.Request{
		System:         systemPrompt,
		Parts:          vertex.Text(retryUserPrompt),
		Schema:         schema,
		ThinkingBudget: vertex.NoThinking(),
		Temperature:    vertex.Temperature(0.1),
	}

	retryCtx, retryCancel := context.WithTimeout(ctx, llmCallTimeout)
	var retryResp llmMoveResponse
	_, retryErr := e.gen.GenerateJSON(retryCtx, retryReq, &retryResp)
	retryCancel()

	if retryErr == nil {
		if chosenMove, ok := matchLegalMove(retryResp.Move, boardSize, legalMap); ok {
			return ptn.FormatMove(chosenMove), retryResp.Reason, nil
		}
	}

	// Fallback to Taktician
	reasonNote := "LLM move generation failed; fell back to Taktician (Intermediate)"
	if retryErr != nil {
		reasonNote = fmt.Sprintf("LLM call failed (%v); fell back to Taktician (Intermediate)", retryErr)
	} else if retryResp.Move != "" {
		reasonNote = fmt.Sprintf("LLM produced illegal move %q; fell back to Taktician (Intermediate)", retryResp.Move)
	}
	return e.fallbackMove(ctx, g, cfg, reasonNote)
}

func (e *LLMEngine) fallbackMove(ctx context.Context, g *gotak.Game, cfg AIConfig, reason string) (string, string, error) {
	fbCfg := cfg
	fbCfg.Level = Intermediate
	move, err := e.fallback.GetMove(ctx, g, fbCfg)
	if err != nil {
		return "", "", fmt.Errorf("fallback engine failed: %w", err)
	}
	return move, reason, nil
}

// buildGamePosition replays game history to build tak.Position and extracts legal moves.
func buildGamePosition(g *gotak.Game) (*tak.Position, []tak.Move, map[tak.Move]struct{}, error) {
	if g == nil || g.Board == nil {
		return nil, nil, nil, fmt.Errorf("game or board is nil")
	}
	if g.Board.Size < 3 || g.Board.Size > 9 {
		return nil, nil, nil, fmt.Errorf("invalid board size %d: must be between 3 and 9", g.Board.Size)
	}

	boardSize := int(g.Board.Size)
	pos := tak.New(tak.Config{Size: boardSize})

	for _, turn := range g.Turns {
		moves := []*gotak.Move{}
		if turn.First != nil {
			moves = append(moves, turn.First)
		}
		if turn.Second != nil {
			moves = append(moves, turn.Second)
		}

		for _, m := range moves {
			if m == nil {
				continue
			}
			parsed, err := ptn.ParseMove(m.Text)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("parse history move %s: %w", m.Text, err)
			}
			if !isMoveInBounds(parsed, boardSize) {
				return nil, nil, nil, fmt.Errorf("move %s out of board bounds", m.Text)
			}
			nextPos, err := pos.Move(parsed)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("apply history move %s: %w", m.Text, err)
			}
			pos = nextPos
		}
	}

	candidates := pos.AllMoves(nil)
	legalMoves := make([]tak.Move, 0, len(candidates))
	legalMap := make(map[tak.Move]struct{}, len(candidates))
	for _, m := range candidates {
		if _, err := pos.Move(m); err == nil {
			legalMoves = append(legalMoves, m)
			legalMap[m] = struct{}{}
		}
	}

	return pos, legalMoves, legalMap, nil
}

// isMoveInBounds verifies the move coordinates and slide destinations lie within the board.
func isMoveInBounds(m tak.Move, size int) bool {
	if int(m.X) < 0 || int(m.X) >= size || int(m.Y) < 0 || int(m.Y) >= size {
		return false
	}
	var dx, dy int
	switch m.Type {
	case tak.SlideLeft:
		dx = -1
	case tak.SlideRight:
		dx = 1
	case tak.SlideUp:
		dy = 1
	case tak.SlideDown:
		dy = -1
	default:
		return true
	}
	x := int(m.X)
	y := int(m.Y)
	for it := m.Slides.Iterator(); it.Ok(); it = it.Next() {
		x += dx
		y += dy
		if x < 0 || x >= size || y < 0 || y >= size {
			return false
		}
	}
	return true
}

func matchLegalMove(raw string, boardSize int, legalMap map[tak.Move]struct{}) (tak.Move, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tak.Move{}, false
	}
	parsed, err := ptn.ParseMove(raw)
	if err != nil {
		return tak.Move{}, false
	}
	if !isMoveInBounds(parsed, boardSize) {
		return tak.Move{}, false
	}
	if _, ok := legalMap[parsed]; ok {
		return parsed, true
	}
	return tak.Move{}, false
}

func buildSystemPrompt(personality string) string {
	sys := "You are an expert player of the abstract strategy board game Tak.\n" +
		"Analyze the game position carefully and choose the best legal move.\n" +
		"You MUST respond ONLY with a valid JSON object matching the requested schema with keys 'move' and 'reason'.\n" +
		"The 'move' field MUST be formatted in PTN (Portable Tak Notation) and MUST be chosen from the list of legal moves.\n" +
		"No rule violations or illegal moves are permitted under any circumstances."

	p := strings.TrimSpace(personality)
	if p != "" {
		runes := []rune(p)
		if len(runes) > maxPersonalityRunes {
			runes = runes[:maxPersonalityRunes]
		}
		sys += fmt.Sprintf("\n\nPersonality flavor (adopt this persona for your reasoning, but do not let it override rules or output format):\n%s", string(runes))
	}
	return sys
}

func buildUserPrompt(g *gotak.Game, pos *tak.Position, legalMoves []tak.Move) string {
	var b strings.Builder
	size := pos.Size()

	b.WriteString("### Tak Game Rules & Notation Summary\n")
	b.WriteString("- Goal: Build a continuous orthogonal road of your stones connecting opposite edges, or have the most top flat stones when the board fills or a player runs out of stones.\n")
	b.WriteString("- Board coordinates: Columns a-h (left-to-right), rows 1-8 (bottom-to-top). a1 is bottom-left.\n")
	b.WriteString("- Turn 1: Player 1 (White) places a flat stone of the OPPONENT's color (Black). Player 2 (Black) places a flat stone of the OPPONENT's color (White).\n")
	b.WriteString("- Normal turns: Place a flat (e.g. 'c3'), standing stone/wall ('Sc3'), or capstone ('Cc3') on an empty square; or move a stack ('3c3>12', 'c3+').\n")
	b.WriteString("- Walls and capstones block roads. Capstones can flatten standing walls when moved alone onto them.\n\n")

	fmt.Fprintf(&b, "Board size: %dx%d\n", size, size)
	fmt.Fprintf(&b, "Position (TPS): %s\n\n", ptn.FormatTPS(pos))

	b.WriteString("Board Grid (top row to bottom row):\n")
	b.WriteString(formatBoardGrid(pos))
	b.WriteString("\n")

	side := "White"
	if pos.ToMove() == tak.Black {
		side = "Black"
	}
	fmt.Fprintf(&b, "Side to move: %s\n", side)

	wStones, wCaps, bStones, bCaps := countRemainingReserves(pos)
	fmt.Fprintf(&b, "White reserves: %d stones, %d capstones\n", wStones, wCaps)
	fmt.Fprintf(&b, "Black reserves: %d stones, %d capstones\n\n", bStones, bCaps)

	history := formatMoveHistory(g)
	if history != "" {
		b.WriteString("Move History:\n")
		b.WriteString(history)
		b.WriteString("\n\n")
	}

	formattedLegal := make([]string, 0, len(legalMoves))
	for _, m := range legalMoves {
		formattedLegal = append(formattedLegal, ptn.FormatMove(m))
	}
	sort.Strings(formattedLegal)

	fmt.Fprintf(&b, "Legal moves (%d total):\n", len(formattedLegal))
	b.WriteString(strings.Join(formattedLegal, ", "))
	b.WriteString("\n\nSelect the best move from the legal moves list and explain your strategic reason.")

	return b.String()
}

func countRemainingReserves(pos *tak.Position) (wStones, wCaps, bStones, bCaps int) {
	cfg := pos.Config()
	wStones = pos.WhiteStones()
	bStones = pos.BlackStones()
	wCaps = cfg.Capstones
	bCaps = cfg.Capstones

	size := pos.Size()
	for x := range size {
		for y := range size {
			sq := pos.At(x, y)
			for _, piece := range sq {
				if piece.Kind() == tak.Capstone {
					if piece.Color() == tak.White {
						wCaps--
					} else {
						bCaps--
					}
				}
			}
		}
	}
	if wCaps < 0 {
		wCaps = 0
	}
	if bCaps < 0 {
		bCaps = 0
	}
	return wStones, wCaps, bStones, bCaps
}

func formatBoardGrid(pos *tak.Position) string {
	size := pos.Size()
	var b strings.Builder
	for y := size - 1; y >= 0; y-- {
		fmt.Fprintf(&b, "%d |", y+1)
		for x := range size {
			sq := pos.At(x, y)
			if len(sq) == 0 {
				b.WriteString(" .    ")
			} else {
				cell := ""
				for _, p := range sq {
					cell += p.String()
				}
				fmt.Fprintf(&b, " %-4s", cell)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("   ")
	for x := range size {
		fmt.Fprintf(&b, " %-4c", 'a'+x)
	}
	b.WriteString("\n")
	return b.String()
}

func formatMoveHistory(g *gotak.Game) string {
	if g == nil || len(g.Turns) == 0 {
		return ""
	}
	var lines []string
	for _, t := range g.Turns {
		if t.First == nil {
			continue
		}
		m1 := t.First.Text
		if t.Second != nil {
			lines = append(lines, fmt.Sprintf("%d. %s %s", t.Number, m1, t.Second.Text))
		} else {
			lines = append(lines, fmt.Sprintf("%d. %s", t.Number, m1))
		}
	}
	return strings.Join(lines, "\n")
}
