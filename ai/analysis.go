package ai

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	taktician "github.com/nelhage/taktician/ai"
	"github.com/nelhage/taktician/ptn"
	"github.com/nelhage/taktician/tak"
	"go.icco.me/gotak"
)

// ForcedWinScore is the score reported for a position with a forced win,
// signed toward the winner. Ordinary evaluations are clamped inside it so
// a chart of scores stays readable.
const ForcedWinScore int64 = 100000

// DefaultAnalysisTimeLimit is the per-position budget when AIConfig has none.
const DefaultAnalysisTimeLimit = 2 * time.Second

// MaxCompareMoves caps how many candidates CompareMoves will search.
const MaxCompareMoves = 16

// analysisTableMem keeps the transposition table well below taktician's
// 100MB default, since one Analyzer lives for a whole request.
const analysisTableMem = 16 << 20

// ErrNoMove is returned when the search ends before finishing even one ply.
var ErrNoMove = errors.New("engine found no move within the time limit")

// PositionAnalysis is the engine's verdict on one position. Score is from
// White's point of view: positive favours White, negative favours Black,
// and roughly 400 is worth one flat on top.
type PositionAnalysis struct {
	TPS       string      `json:"tps"`
	ToMove    int         `json:"to_move"`
	Best      string      `json:"best,omitempty"`
	PV        []string    `json:"pv,omitempty"`
	Score     int64       `json:"score"`
	ForcedWin int         `json:"forced_win,omitempty"`
	GameOver  bool        `json:"game_over"`
	Winner    int         `json:"winner,omitempty"`
	Depth     int         `json:"depth"`
	Threats   RoadThreats `json:"threats"`
}

// RoadThreats lists, per color, the empty squares where a flat placed now
// would complete a road. Either side's list can be non-empty regardless
// of who is to move.
type RoadThreats struct {
	White []string `json:"white"`
	Black []string `json:"black"`
}

// MoveScore is one candidate in a Comparison. Score is White's point of
// view after the move; Loss is how much worse it is than the engine's
// best move for the player making it, never negative.
type MoveScore struct {
	Move      string   `json:"move"`
	Score     int64    `json:"score"`
	Loss      int64    `json:"loss"`
	ForcedWin int      `json:"forced_win,omitempty"`
	Reply     []string `json:"reply,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Comparison scores several candidate moves from the same position.
// Moves are sorted best first; candidates with an Error sort last.
type Comparison struct {
	Position PositionAnalysis `json:"position"`
	Moves    []MoveScore      `json:"moves"`
}

// Analyzer scores positions with a minimax search. It keeps its
// transposition table between calls, so walking one game through a
// single Analyzer is much cheaper than starting fresh at every ply.
// An Analyzer is not safe for concurrent use.
type Analyzer struct {
	size      int
	timeLimit time.Duration
	mm        *taktician.MinimaxAI
}

// NewAnalyzer returns an Analyzer for boards of the given size. The search
// depth comes from cfg.Level and each position gets at most cfg.TimeLimit,
// or DefaultAnalysisTimeLimit when that is zero.
func NewAnalyzer(size int, cfg AIConfig) (*Analyzer, error) {
	if size < 3 || size > 8 {
		return nil, fmt.Errorf("invalid board size %d: the engine supports 3 to 8", size)
	}
	timeLimit := cfg.TimeLimit
	if timeLimit <= 0 {
		timeLimit = DefaultAnalysisTimeLimit
	}
	return &Analyzer{
		size:      size,
		timeLimit: timeLimit,
		mm: taktician.NewMinimax(taktician.MinimaxConfig{
			Size:     size,
			Depth:    analysisDepth(cfg.Level),
			TableMem: analysisTableMem,
		}),
	}, nil
}

// analysisDepth maps a level to a search depth. Beginner still searches,
// since a random mover says nothing about move quality. Zero lets
// taktician deepen until the time limit.
func analysisDepth(level DifficultyLevel) int {
	switch level {
	case Beginner:
		return 2
	case Intermediate:
		return 3
	case Advanced:
		return 5
	default:
		return 0
	}
}

// Size reports the board size this Analyzer was built for.
func (a *Analyzer) Size() int {
	return a.size
}

// Evaluate analyzes the position reached after every move in g.
func (a *Analyzer) Evaluate(ctx context.Context, g *gotak.Game) (*PositionAnalysis, error) {
	if g == nil || g.Board == nil {
		return nil, errors.New("game has no board")
	}
	if g.Board.Size != int64(a.size) {
		return nil, fmt.Errorf("game is %dx%d but analyzer is %dx%d", g.Board.Size, g.Board.Size, a.size, a.size)
	}
	p, err := convertGameToPosition(g)
	if err != nil {
		return nil, err
	}
	return a.analyze(ctx, p)
}

// EvaluateTPS analyzes a position given in Tak Positional System notation.
func (a *Analyzer) EvaluateTPS(ctx context.Context, tps string) (*PositionAnalysis, error) {
	p, err := a.parseTPS(tps)
	if err != nil {
		return nil, err
	}
	return a.analyze(ctx, p)
}

// CompareMoves scores each PTN move in moves when played from the TPS
// position. Illegal or unparsable moves come back with Error set rather
// than failing the whole comparison.
func (a *Analyzer) CompareMoves(ctx context.Context, tps string, moves []string) (*Comparison, error) {
	if len(moves) == 0 {
		return nil, errors.New("no moves to compare")
	}
	if len(moves) > MaxCompareMoves {
		return nil, fmt.Errorf("at most %d moves can be compared, got %d", MaxCompareMoves, len(moves))
	}
	p, err := a.parseTPS(tps)
	if err != nil {
		return nil, err
	}
	if over, _ := p.GameOver(); over {
		return nil, errors.New("game is already over in this position")
	}
	root, err := a.analyze(ctx, p)
	if err != nil {
		return nil, err
	}

	mover := colorToPlayer(p.ToMove())
	out := &Comparison{Position: *root, Moves: make([]MoveScore, 0, len(moves))}
	for _, text := range moves {
		ms := MoveScore{Move: strings.TrimSpace(text)}
		child, err := a.play(p, ms.Move)
		if err != nil {
			ms.Error = err.Error()
			out.Moves = append(out.Moves, ms)
			continue
		}
		res, err := a.analyze(ctx, child)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			ms.Error = err.Error()
			out.Moves = append(out.Moves, ms)
			continue
		}
		ms.Score = res.Score
		ms.ForcedWin = res.ForcedWin
		ms.Reply = res.PV
		if !SameMove(ms.Move, root.Best, a.size) {
			ms.Loss = Loss(root.Score, res.Score, mover)
		}
		out.Moves = append(out.Moves, ms)
	}

	// Loss is clamped at zero, so it cannot order moves that beat the root
	// score; the mover-relative score can.
	sort.SliceStable(out.Moves, func(i, j int) bool {
		mi, mj := out.Moves[i], out.Moves[j]
		if (mi.Error == "") != (mj.Error == "") {
			return mi.Error == ""
		}
		if mover == gotak.PlayerBlack {
			return mi.Score < mj.Score
		}
		return mi.Score > mj.Score
	})
	return out, nil
}

// Loss is how many points the player gave up moving from a position
// scored before to one scored after, both from White's point of view.
// Search noise can make the played move look better than the best one;
// that reads as zero.
func Loss(before, after int64, player int) int64 {
	d := before - after
	if player == gotak.PlayerBlack {
		d = -d
	}
	return max(d, 0)
}

// SameMove reports whether two PTN strings describe the same move on a
// board of the given size, so "a1>" matches "1a1>1".
func SameMove(a, b string, size int) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	ma, err := parseUserMove(a, size)
	if err != nil {
		return false
	}
	mb, err := parseUserMove(b, size)
	if err != nil {
		return false
	}
	return ma.Equal(mb)
}

// parseUserMove parses PTN with taktician's parser, which caps a carry at
// 8, then checks the square is on the board. gotak accepts a stone marker
// after a slide (a1>C) that taktician does not, so it is dropped.
func parseUserMove(text string, size int) (tak.Move, error) {
	clean := strings.TrimSpace(text)
	if strings.ContainsAny(clean, "<>+-") {
		clean = strings.TrimRight(clean, "CSF")
	}
	m, err := ptn.ParseMove(clean)
	if err != nil {
		return tak.Move{}, fmt.Errorf("invalid move %q: %w", text, err)
	}
	if int(m.X) >= size || int(m.Y) >= size {
		return tak.Move{}, fmt.Errorf("move %q is off a %dx%d board", text, size, size)
	}
	carry := 0
	for it := m.Slides.Iterator(); it.Ok(); it = it.Next() {
		carry += it.Elem()
	}
	if carry > size {
		return tak.Move{}, fmt.Errorf("move %q carries more than %d stones", text, size)
	}
	return m, nil
}

// parseTPS validates with gotak's stricter parser first, since taktician's
// indexes into empty cells and bare piece markers and panics on them.
func (a *Analyzer) parseTPS(tps string) (p *tak.Position, err error) {
	norm := strings.Join(strings.Fields(tps), " ")
	if _, _, _, err := gotak.ParseTPS(norm); err != nil {
		return nil, fmt.Errorf("invalid tps: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			p, err = nil, fmt.Errorf("invalid tps: %v", r)
		}
	}()
	p, err = ptn.ParseTPS(norm)
	if err != nil {
		return nil, fmt.Errorf("invalid tps: %w", err)
	}
	if p.Size() != a.size {
		return nil, fmt.Errorf("tps is %dx%d but analyzer is %dx%d", p.Size(), p.Size(), a.size, a.size)
	}
	return p, nil
}

func (a *Analyzer) play(p *tak.Position, text string) (*tak.Position, error) {
	tm, err := parseUserMove(text, a.size)
	if err != nil {
		return nil, err
	}
	child, err := p.Move(tm)
	if err != nil {
		return nil, fmt.Errorf("illegal move %s: %w", text, err)
	}
	return child, nil
}

func (a *Analyzer) analyze(ctx context.Context, p *tak.Position) (*PositionAnalysis, error) {
	res := &PositionAnalysis{
		TPS:     ptn.FormatTPS(p),
		ToMove:  colorToPlayer(p.ToMove()),
		Threats: roadThreats(p),
	}

	if over, winner := p.GameOver(); over {
		res.GameOver = true
		res.Winner = colorToPlayer(winner)
		switch res.Winner {
		case gotak.PlayerWhite:
			res.Score = ForcedWinScore
		case gotak.PlayerBlack:
			res.Score = -ForcedWinScore
		}
		return res, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sctx, cancel := context.WithTimeout(ctx, a.timeLimit)
	pv, v, st := a.mm.Analyze(sctx, p)
	cancel()
	if len(pv) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrNoMove
	}

	for _, m := range pv {
		res.PV = append(res.PV, ptn.FormatMove(m))
	}
	res.Best = res.PV[0]
	res.Depth = st.Depth

	if p.ToMove() == tak.Black {
		v = -v
	}
	switch {
	case v > taktician.WinThreshold:
		res.Score, res.ForcedWin = ForcedWinScore, gotak.PlayerWhite
	case v < -taktician.WinThreshold:
		res.Score, res.ForcedWin = -ForcedWinScore, gotak.PlayerBlack
	default:
		res.Score = max(-ForcedWinScore+1, min(ForcedWinScore-1, v))
	}
	return res, nil
}

func roadThreats(p *tak.Position) RoadThreats {
	out := RoadThreats{White: []string{}, Black: []string{}}
	if over, _ := p.GameOver(); over {
		return out
	}
	n := p.Size()
	board := make([][]tak.Square, n)
	for y := range n {
		board[y] = make([]tak.Square, n)
		for x := range n {
			board[y][x] = p.At(x, y)
		}
	}
	for y := range n {
		for x := range n {
			if len(board[y][x]) != 0 {
				continue
			}
			for _, c := range []tak.Color{tak.White, tak.Black} {
				board[y][x] = tak.Square{tak.MakePiece(c, tak.Flat)}
				q, err := tak.FromSquares(p.Config(), board, p.MoveNumber())
				board[y][x] = nil
				if err != nil {
					continue
				}
				if d := q.WinDetails(); d.Over && d.Reason == tak.RoadWin && d.Winner == c {
					sq := fmt.Sprintf("%c%d", 'a'+x, y+1)
					if c == tak.White {
						out.White = append(out.White, sq)
					} else {
						out.Black = append(out.Black, sq)
					}
				}
			}
		}
	}
	return out
}

func colorToPlayer(c tak.Color) int {
	switch c {
	case tak.White:
		return gotak.PlayerWhite
	case tak.Black:
		return gotak.PlayerBlack
	default:
		return 0
	}
}

// AnalyzeTPS is a one-shot EvaluateTPS that sizes the Analyzer from the
// TPS itself.
func AnalyzeTPS(ctx context.Context, tps string, cfg AIConfig) (*PositionAnalysis, error) {
	a, err := analyzerForTPS(tps, cfg)
	if err != nil {
		return nil, err
	}
	return a.EvaluateTPS(ctx, tps)
}

// CompareMovesTPS is a one-shot CompareMoves that sizes the Analyzer from
// the TPS itself.
func CompareMovesTPS(ctx context.Context, tps string, moves []string, cfg AIConfig) (*Comparison, error) {
	a, err := analyzerForTPS(tps, cfg)
	if err != nil {
		return nil, err
	}
	return a.CompareMoves(ctx, tps, moves)
}

func analyzerForTPS(tps string, cfg AIConfig) (*Analyzer, error) {
	fields := strings.Fields(tps)
	if len(fields) != 3 {
		return nil, fmt.Errorf("invalid tps: want 3 space-separated fields, got %d", len(fields))
	}
	return NewAnalyzer(strings.Count(fields[0], "/")+1, cfg)
}
