package main

import (
	"fmt"
	"html"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"go.icco.me/gotak/ai"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type reportMoveItem struct {
	Turn       int64
	Player     int
	PlayerName string
	Played     string
	Best       string
	Score      int64
	ScoreAfter int64
	Loss       int64
	Quality    string
	IsCritical bool
	Comment    string
	Error      string
}

type reportCriticalMomentItem struct {
	Turn       int64
	Player     int
	PlayerName string
	Played     string
	Best       string
	Loss       int64
	Quality    string
	Score      int64
	ScoreAfter int64
	BoardSVG   template.HTML
}

type reportViewModel struct {
	Slug            string
	Size            int64
	Player1         string
	Player2         string
	Date            string
	Result          string
	Level           string
	Style           string
	TimeLimit       time.Duration
	MoveCount       int
	AgreedCount     int
	AgreedPercent   int
	BlunderCount    int
	MistakeCount    int
	InaccuracyCount int
	EvalGraphSVG    template.HTML
	CriticalMoments []reportCriticalMomentItem
	Moves           []reportMoveItem
}

var uncachedReportTemplate = template.Must(template.New("uncached").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Analysis Not Found</title>
<style>
body {
  font-family: 'Avenir Next', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
  font-size: 16px;
  line-height: 1.5;
  color: #222222;
  background-color: #ffffff;
  max-width: 800px;
  margin: 40px auto;
  padding: 0 20px;
}
a { color: #265c83; }
code {
  background: #f4f4f4;
  padding: 2px 6px;
  border-radius: 3px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
</head>
<body>
<h1>Analysis Not Found</h1>
<p>No cached analysis found for game <strong>{{.Slug}}</strong>.</p>
<p>Run <code>POST /analyze/game/{{.Slug}}</code> to generate the analysis first.</p>
</body>
</html>
`))

var reportTemplate = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Game Analysis: {{.Slug}}</title>
<style>
body {
  font-family: 'Avenir Next', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
  font-size: 16px;
  line-height: 1.5;
  color: #222222;
  background-color: #ffffff;
  max-width: 800px;
  margin: 0 auto;
  padding: 32px 20px;
}
a {
  color: #265c83;
  text-decoration: underline;
}
code {
  background: #f4f4f4;
  padding: 2px 5px;
  border-radius: 3px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.9em;
}
h1 {
  font-size: 26px;
  margin: 0 0 12px 0;
  font-weight: 600;
  color: #111111;
}
h2 {
  font-size: 19px;
  margin: 32px 0 12px 0;
  font-weight: 600;
  border-bottom: 1px solid #e0e0e0;
  padding-bottom: 6px;
  color: #222222;
}
.header-meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  background: #f8f9fa;
  border: 1px solid #e9ecef;
  border-radius: 6px;
  padding: 16px;
  margin-bottom: 24px;
}
.meta-item {
  font-size: 14px;
}
.meta-label {
  color: #666666;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 2px;
}
.meta-value {
  font-weight: 500;
  color: #111111;
}
.summary-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 12px;
}
.summary-pill {
  font-size: 13px;
  padding: 4px 10px;
  border-radius: 4px;
  background: #f1f3f5;
  color: #333333;
}
.eval-graph-container {
  margin: 16px 0;
  border: 1px solid #e0e0e0;
  border-radius: 6px;
  padding: 12px;
  background: #ffffff;
}
.critical-card {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
  align-items: flex-start;
  padding: 16px;
  margin-bottom: 16px;
  background: #fafafa;
  border: 1px solid #e0e0e0;
  border-left: 4px solid #b71c1c;
  border-radius: 6px;
}
.critical-info {
  flex: 1;
  min-width: 240px;
}
.critical-info h3 {
  margin: 0 0 8px 0;
  font-size: 16px;
  font-weight: 600;
}
.critical-board-container {
  text-align: center;
}
.board-caption {
  font-size: 12px;
  color: #666666;
  margin-top: 6px;
}
.badge {
  display: inline-block;
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  text-transform: capitalize;
}
.badge-best { background: #e8f5e9; color: #2e7d32; }
.badge-good { background: #e3f2fd; color: #1565c0; }
.badge-inaccuracy { background: #fff8e1; color: #b26a00; }
.badge-mistake { background: #fff3e0; color: #e65100; }
.badge-blunder { background: #ffebee; color: #c62828; }
.badge-critical { background: #fce4ec; color: #ad1457; }
.move-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 12px;
  font-size: 14px;
}
.move-table th {
  text-align: left;
  padding: 8px 10px;
  border-bottom: 2px solid #dddddd;
  font-weight: 600;
  background: #fafafa;
}
.move-table td {
  padding: 8px 10px;
  border-bottom: 1px solid #eeeeee;
  vertical-align: top;
}
.move-table tr:hover {
  background: #fbfbfb;
}
.row-critical {
  background: #fff5f5 !important;
}
.move-comment {
  font-size: 12px;
  color: #555555;
  font-style: italic;
  margin-top: 4px;
}
.move-error {
  font-size: 12px;
  color: #c62828;
  margin-top: 2px;
}
.no-print {
  margin-bottom: 16px;
  text-align: right;
}
.print-button {
  background: #265c83;
  color: #ffffff;
  border: none;
  padding: 6px 14px;
  border-radius: 4px;
  font-size: 14px;
  cursor: pointer;
}
.print-button:hover {
  background: #1c4563;
}
@media print {
  body {
    max-width: 100%;
    margin: 0;
    padding: 0;
    font-size: 11pt;
    line-height: 1.35;
    color: #000000;
    background: #ffffff;
  }
  .no-print {
    display: none !important;
  }
  .critical-card, .eval-graph-container, table, tr {
    page-break-inside: avoid;
    break-inside: avoid;
  }
  @page {
    margin: 1.5cm;
    size: portrait;
  }
  a {
    color: #000000;
    text-decoration: none;
  }
}
</style>
</head>
<body>
<div class="no-print">
  <button type="button" onclick="window.print()" class="print-button">Print to PDF</button>
</div>

<h1>Game Analysis: {{.Slug}}</h1>

<div class="header-meta">
  <div class="meta-item">
    <div class="meta-label">Players</div>
    <div class="meta-value">{{.Player1}} (White) vs {{.Player2}} (Black)</div>
  </div>
  <div class="meta-item">
    <div class="meta-label">Board Size</div>
    <div class="meta-value">{{.Size}}x{{.Size}}</div>
  </div>
  {{if .Result}}
  <div class="meta-item">
    <div class="meta-label">Result</div>
    <div class="meta-value">{{.Result}}</div>
  </div>
  {{end}}
  {{if .Date}}
  <div class="meta-item">
    <div class="meta-label">Date</div>
    <div class="meta-value">{{.Date}}</div>
  </div>
  {{end}}
  <div class="meta-item">
    <div class="meta-label">Engine Config</div>
    <div class="meta-value">{{.Level}} / {{.Style}} ({{.TimeLimit}})</div>
  </div>
  <div class="meta-item">
    <div class="meta-label">Engine Agreement</div>
    <div class="meta-value">{{.AgreedCount}} / {{.MoveCount}} ({{.AgreedPercent}}%)</div>
  </div>
</div>

<div class="summary-bar">
  <span class="summary-pill">Total Plies: {{.MoveCount}}</span>
  <span class="summary-pill" style="color: #c62828;">Blunders: {{.BlunderCount}}</span>
  <span class="summary-pill" style="color: #e65100;">Mistakes: {{.MistakeCount}}</span>
  <span class="summary-pill" style="color: #b26a00;">Inaccuracies: {{.InaccuracyCount}}</span>
</div>

<h2>Evaluation Graph</h2>
<div class="eval-graph-container">
  {{.EvalGraphSVG}}
</div>

{{if .CriticalMoments}}
<h2>Critical Moments</h2>
{{range .CriticalMoments}}
<div class="critical-card">
  <div class="critical-info">
    <h3>Turn {{.Turn}} ({{.PlayerName}}): <code>{{.Played}}</code></h3>
    <p>
      <span class="badge badge-{{.Quality}}">{{.Quality}}</span>
      <span style="color: #c62828; font-weight: 600; margin-left: 8px;">-{{.Loss}} pts</span>
    </p>
    <p><strong>Played:</strong> <code>{{.Played}}</code></p>
    <p><strong>Engine Recommended:</strong> <code>{{.Best}}</code></p>
    <p style="font-size: 13px; color: #555555;">Score shifted from {{.Score}} to {{.ScoreAfter}}.</p>
  </div>
  <div class="critical-board-container">
    {{.BoardSVG}}
    <div class="board-caption">Position after {{.Played}}</div>
  </div>
</div>
{{end}}
{{end}}

<h2>Move List</h2>
<table class="move-table">
  <thead>
    <tr>
      <th>#</th>
      <th>Player</th>
      <th>Played</th>
      <th>Quality</th>
      <th>Best Move</th>
      <th>Score</th>
      <th>Loss</th>
    </tr>
  </thead>
  <tbody>
    {{range .Moves}}
    <tr{{if .IsCritical}} class="row-critical"{{end}}>
      <td>{{.Turn}}{{if eq .Player 2}}b{{else}}w{{end}}</td>
      <td>{{.PlayerName}}</td>
      <td>
        <code>{{.Played}}</code>
        {{if .IsCritical}}<span class="badge badge-critical" style="margin-left: 4px;">Critical</span>{{end}}
        {{if .Comment}}<div class="move-comment">{{.Comment}}</div>{{end}}
        {{if .Error}}<div class="move-error">{{.Error}}</div>{{end}}
      </td>
      <td>{{if .Quality}}<span class="badge badge-{{.Quality}}">{{.Quality}}</span>{{else}}-{{end}}</td>
      <td>{{if .Best}}<code>{{.Best}}</code>{{else}}-{{end}}</td>
      <td>{{.Score}} -> {{.ScoreAfter}}</td>
      <td>{{if gt .Loss 0}}-{{.Loss}}{{else}}0{{end}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
</body>
</html>
`))

func getAnalyzeReportHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	db, err := getDB()
	if err != nil {
		l.Errorw("could not get db", zap.Error(err))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := w.Write([]byte("<!DOCTYPE html><html><body><h1>Internal Server Error</h1></body></html>")); writeErr != nil {
			l.Errorw("failed to write 500 response", zap.Error(writeErr))
		}
		return
	}

	getAnalyzeReportWithDB(w, r, db)
}

func getAnalyzeReportWithDB(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	slug := ugcPolicy.Sanitize(chi.URLParamFromCtx(ctx, "slug"))

	game, err := getGame(db, slug)
	if err != nil {
		l.Warnw("game not found for report", "slug", slug, zap.Error(err))
		renderUncachedReport(w, slug)
		return
	}

	q := r.URL.Query()
	var req AnalyzeRequest
	req.Level = q.Get("level")
	req.Style = q.Get("style")
	if tlStr := q.Get("time_limit"); tlStr != "" {
		if d, parseErr := time.ParseDuration(tlStr); parseErr == nil {
			req.TimeLimit = d
		} else if ns, parseErr := strconv.ParseInt(tlStr, 10, 64); parseErr == nil {
			req.TimeLimit = time.Duration(ns)
		}
	}

	cfg, levelName := analyzeConfigFromRequest(req)
	key := analysisCacheKey{
		gameID:      game.ID,
		level:       levelName,
		style:       string(cfg.Style),
		timeLimitNs: int64(cfg.TimeLimit),
		gameVersion: gameCacheVersion(game),
	}

	cached, ok := loadAnalysisCache(db, l, key)
	if !ok {
		renderUncachedReport(w, slug)
		return
	}

	renderAnalyzeReport(w, l, game, cfg, levelName, cached)
}

func renderUncachedReport(w http.ResponseWriter, slug string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	data := struct {
		Slug string
	}{
		Slug: slug,
	}
	if err := uncachedReportTemplate.Execute(w, data); err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

func renderAnalyzeReport(w http.ResponseWriter, l *zap.SugaredLogger, game *gotak.Game, cfg ai.AIConfig, levelName string, cached AnalyzeResponse) {
	player1, err := game.GetMeta("Player1")
	if err != nil || player1 == "" {
		player1 = "White"
	}
	player2, err := game.GetMeta("Player2")
	if err != nil || player2 == "" {
		player2 = "Black"
	}
	date, _ := game.GetMeta("Date")
	result, _ := game.GetMeta("Result")

	moments := criticalMoments(cached.Moves)
	criticalMap := make(map[int]bool, len(moments))
	for _, m := range moments {
		criticalMap[m.Index] = true
	}

	steps, err := buildReplaySteps(game, nil)
	if err != nil {
		l.Warnw("failed to build replay steps for report", "slug", game.Slug, zap.Error(err))
		steps = nil
	}

	criticalItems := make([]reportCriticalMomentItem, 0, len(moments))
	for _, m := range moments {
		pName := player1
		if m.Player == gotak.PlayerBlack {
			pName = player2
		}

		var bSVG template.HTML
		if steps != nil && m.Index < len(steps) {
			bSVG = renderBoardSVG(steps[m.Index].Board, int(game.Board.Size), baseSquare(m.Played))
		}

		var scoreBefore, scoreAfter int64
		if m.Index < len(cached.Moves) {
			scoreBefore = cached.Moves[m.Index].Score
			scoreAfter = cached.Moves[m.Index].ScoreAfter
		}

		criticalItems = append(criticalItems, reportCriticalMomentItem{
			Turn:       m.Turn,
			Player:     m.Player,
			PlayerName: pName,
			Played:     m.Played,
			Best:       m.Best,
			Loss:       m.Loss,
			Quality:    m.Quality,
			Score:      scoreBefore,
			ScoreAfter: scoreAfter,
			BoardSVG:   bSVG,
		})
	}

	turnComments := make(map[int64]string, len(game.Turns))
	for _, t := range game.Turns {
		if t != nil && t.Comment != "" {
			turnComments[t.Number] = t.Comment
		}
	}

	blunders, mistakes, inaccuracies := 0, 0, 0
	moveItems := make([]reportMoveItem, 0, len(cached.Moves))
	for i, m := range cached.Moves {
		pName := player1
		if m.Player == gotak.PlayerBlack {
			pName = player2
		}
		switch m.Quality {
		case QualityBlunder:
			blunders++
		case QualityMistake:
			mistakes++
		case QualityInaccuracy:
			inaccuracies++
		}

		var comment string
		if m.Player == gotak.PlayerBlack || i == len(cached.Moves)-1 {
			comment = turnComments[m.Turn]
		}

		moveItems = append(moveItems, reportMoveItem{
			Turn:       m.Turn,
			Player:     m.Player,
			PlayerName: pName,
			Played:     m.Played,
			Best:       m.Best,
			Score:      m.Score,
			ScoreAfter: m.ScoreAfter,
			Loss:       m.Loss,
			Quality:    m.Quality,
			IsCritical: criticalMap[i],
			Comment:    comment,
			Error:      m.Error,
		})
	}

	agreedPercent := 0
	if cached.MoveCount > 0 {
		agreedPercent = (cached.Agreed * 100) / cached.MoveCount
	}

	graphSVG := renderEvaluationGraphSVG(cached.Moves, criticalMap)

	data := reportViewModel{
		Slug:            game.Slug,
		Size:            game.Board.Size,
		Player1:         player1,
		Player2:         player2,
		Date:            date,
		Result:          result,
		Level:           levelName,
		Style:           string(cfg.Style),
		TimeLimit:       cfg.TimeLimit,
		MoveCount:       cached.MoveCount,
		AgreedCount:     cached.Agreed,
		AgreedPercent:   agreedPercent,
		BlunderCount:    blunders,
		MistakeCount:    mistakes,
		InaccuracyCount: inaccuracies,
		EvalGraphSVG:    graphSVG,
		CriticalMoments: criticalItems,
		Moves:           moveItems,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := reportTemplate.Execute(w, data); err != nil {
		l.Errorw("failed to execute report template", zap.Error(err))
	}
}

func baseSquare(moveStr string) string {
	for i := range len(moveStr) - 1 {
		if moveStr[i] >= 'a' && moveStr[i] <= 'k' && moveStr[i+1] >= '1' && moveStr[i+1] <= '9' {
			return moveStr[i : i+2]
		}
	}
	return ""
}

func clampScore(s int64) int64 {
	if s > 3000 {
		return 3000
	}
	if s < -3000 {
		return -3000
	}
	return s
}

func renderEvaluationGraphSVG(moves []MoveAnalysis, criticalIndices map[int]bool) template.HTML {
	const (
		width      = 760
		height     = 180
		padLeft    = 50.0
		padRight   = 20.0
		padTop     = 20.0
		padBottom  = 30.0
		plotWidth  = width - padLeft - padRight
		plotHeight = height - padTop - padBottom
		midY       = padTop + plotHeight/2.0
	)

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg class="eval-graph" viewBox="0 0 %d %d" width="100%%" height="auto" xmlns="http://www.w3.org/2000/svg">`, width, height)

	gridLines := []struct {
		score int64
		label string
	}{
		{3000, "+3000"},
		{1500, "+1500"},
		{0, "0"},
		{-1500, "-1500"},
		{-3000, "-3000"},
	}
	for _, gl := range gridLines {
		y := midY - (float64(gl.score)/3000.0)*(plotHeight/2.0)
		strokeColor := "#e0e0e0"
		dash := ` stroke-dasharray="4 4"`
		strokeWidth := "1"
		if gl.score == 0 {
			strokeColor = "#999999"
			dash = ""
			strokeWidth = "1.5"
		}
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%s"%s />`,
			padLeft, y, padLeft+plotWidth, y, strokeColor, strokeWidth, dash)
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="end" dominant-baseline="central" font-size="11" fill="#777777" font-family="sans-serif">%s</text>`,
			padLeft-6, y, gl.label)
	}

	if len(moves) == 0 {
		sb.WriteString(`</svg>`)
		return template.HTML(sb.String()) //nolint:gosec // Safe SVG output.
	}

	x0 := padLeft
	y0 := midY - (float64(clampScore(moves[0].Score))/3000.0)*(plotHeight/2.0)
	pts := []string{fmt.Sprintf("%.1f,%.1f", x0, y0)}

	type dot struct {
		x      float64
		y      float64
		color  string
		radius float64
		title  string
	}
	var dots []dot

	n := float64(len(moves))
	for i, m := range moves {
		xi := padLeft + (float64(i+1)/n)*plotWidth
		yi := midY - (float64(clampScore(m.ScoreAfter))/3000.0)*(plotHeight/2.0)
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", xi, yi))

		playerStr := "White"
		if m.Player == gotak.PlayerBlack {
			playerStr = "Black"
		}
		title := fmt.Sprintf("Turn %d %s: %s (score: %d, loss: %d, %s)", m.Turn, playerStr, m.Played, m.ScoreAfter, m.Loss, m.Quality)

		if m.Quality == QualityBlunder || m.Quality == QualityMistake || criticalIndices[i] {
			dots = append(dots, dot{x: xi, y: yi, color: "#d9383a", radius: 4.5, title: title})
		} else if m.Quality == QualityInaccuracy {
			dots = append(dots, dot{x: xi, y: yi, color: "#e67e22", radius: 3.5, title: title})
		}
	}

	fmt.Fprintf(&sb, `<polyline fill="none" stroke="#265c83" stroke-width="2.5" stroke-linejoin="round" points="%s" />`,
		strings.Join(pts, " "))

	for _, d := range dots {
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" stroke="#ffffff" stroke-width="1.5"><title>%s</title></circle>`,
			d.x, d.y, d.radius, d.color, html.EscapeString(d.title))
	}

	lastTurn := moves[len(moves)-1].Turn
	step := int64(1)
	if lastTurn > 10 {
		step = 5
	}
	if lastTurn > 30 {
		step = 10
	}
	for t := int64(1); t <= lastTurn; t += step {
		for i, m := range moves {
			if m.Turn == t {
				xi := padLeft + (float64(i)/n)*plotWidth
				fmt.Fprintf(&sb, `<text x="%.1f" y="%d" text-anchor="middle" font-size="10" fill="#777777" font-family="sans-serif">T%d</text>`,
					xi, height-10, t)
				break
			}
		}
	}

	sb.WriteString(`</svg>`)
	return template.HTML(sb.String()) //nolint:gosec // Safe SVG output.
}

func renderBoardSVG(b map[string][]*gotak.Stone, size int, highlightSquare string) template.HTML {
	if size < 3 || size > 8 {
		size = 5
	}

	const (
		cellSize  = 44
		padLeft   = 24
		padBottom = 24
		padTop    = 8
		padRight  = 8
	)
	svgW := padLeft + size*cellSize + padRight
	svgH := padTop + size*cellSize + padBottom

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg class="tak-board" viewBox="0 0 %d %d" width="%d" height="%d" xmlns="http://www.w3.org/2000/svg">`,
		svgW, svgH, svgW, svgH)

	fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="%d" height="%d" fill="#f5ede0" stroke="#bfa37c" stroke-width="1.5" rx="3" />`,
		padLeft, padTop, size*cellSize, size*cellSize)

	for x := range size {
		for y := range size {
			fileChar := rune('a' + x)
			rankNum := size - y
			coord := fmt.Sprintf("%c%d", fileChar, rankNum)
			cellX := padLeft + x*cellSize
			cellY := padTop + y*cellSize
			cx := cellX + cellSize/2
			cy := cellY + cellSize/2

			sqFill := "#eedbb6"
			if (x+y)%2 == 1 {
				sqFill = "#e2cd9e"
			}
			sqStroke := "#cbb389"
			sqStrokeWidth := "1"
			if coord == highlightSquare {
				sqStroke = "#d9383a"
				sqStrokeWidth = "2.5"
			}

			fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" stroke="%s" stroke-width="%s" />`,
				cellX, cellY, cellSize, cellSize, sqFill, sqStroke, sqStrokeWidth)

			stones := b[coord]
			if len(stones) > 0 {
				topStone := stones[len(stones)-1]
				stackHeight := len(stones)

				isWhite := topStone.Player == gotak.PlayerWhite
				stoneFill := "#ffffff"
				stoneStroke := "#333333"
				textColor := "#111111"
				playerLabel := "White"
				if !isWhite {
					stoneFill = "#262626"
					stoneStroke = "#111111"
					textColor = "#ffffff"
					playerLabel = "Black"
				}

				typeLabel := "Flat"
				switch topStone.Type {
				case gotak.StoneStanding:
					typeLabel = "Standing"
				case gotak.StoneCap:
					typeLabel = "Capstone"
				}

				title := fmt.Sprintf("%s: %s %s (stack: %d)", coord, playerLabel, typeLabel, stackHeight)
				fmt.Fprintf(&sb, `<g><title>%s</title>`, html.EscapeString(title))

				switch topStone.Type {
				case gotak.StoneStanding:
					fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="26" height="8" rx="2" fill="%s" stroke="%s" stroke-width="1.5" />`,
						cx-13, cy-4, stoneFill, stoneStroke)
					if stackHeight > 1 {
						fmt.Fprintf(&sb, `<circle cx="%d" cy="%d" r="6" fill="#ffffff" stroke="#333333" stroke-width="1" />`,
							cx+11, cy-9)
						fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="middle" dominant-baseline="central" font-size="8" font-weight="bold" fill="#111111">%d</text>`,
							cx+11, cy-9, stackHeight)
					}
				case gotak.StoneCap:
					fmt.Fprintf(&sb, `<circle cx="%d" cy="%d" r="15" fill="%s" stroke="%s" stroke-width="2" />`,
						cx, cy, stoneFill, stoneStroke)
					fmt.Fprintf(&sb, `<circle cx="%d" cy="%d" r="8" fill="none" stroke="%s" stroke-width="1.5" />`,
						cx, cy, stoneStroke)
					if stackHeight > 1 {
						fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="middle" dominant-baseline="central" font-size="11" font-weight="bold" fill="%s">%d</text>`,
							cx, cy, textColor, stackHeight)
					}
				default:
					fmt.Fprintf(&sb, `<circle cx="%d" cy="%d" r="14" fill="%s" stroke="%s" stroke-width="1.5" />`,
						cx, cy, stoneFill, stoneStroke)
					if stackHeight > 1 {
						fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="middle" dominant-baseline="central" font-size="11" font-weight="bold" fill="%s">%d</text>`,
							cx, cy, textColor, stackHeight)
					}
				}

				sb.WriteString(`</g>`)
			}
		}
	}

	for y := range size {
		rankNum := size - y
		fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="middle" dominant-baseline="central" font-size="11" fill="#777777" font-family="sans-serif">%d</text>`,
			padLeft-10, padTop+y*cellSize+cellSize/2, rankNum)
	}

	for x := range size {
		fileChar := rune('a' + x)
		fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="middle" dominant-baseline="central" font-size="11" fill="#777777" font-family="sans-serif">%c</text>`,
			padLeft+x*cellSize+cellSize/2, padTop+size*cellSize+13, fileChar)
	}

	sb.WriteString(`</svg>`)
	return template.HTML(sb.String()) //nolint:gosec // Safe SVG output.
}
