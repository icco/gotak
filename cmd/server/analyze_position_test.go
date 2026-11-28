package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"go.icco.me/gotak/ai"
	"go.icco.me/gutil/logging"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

const roadInOneTPS = "x5/x5/x5/2,2,x3/1,1,1,1,x 1 5"

func postJSON(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestPostAnalyzePosition(t *testing.T) {
	rec := postJSON(t, postAnalyzePositionHandler,
		`{"tps":"`+roadInOneTPS+`","level":"intermediate","time_limit":300000000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got PositionAnalysisResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasSuffix(got.Best, "e1") || got.ForcedWin != gotak.PlayerWhite || got.Score != ai.ForcedWinScore {
		t.Errorf("want a forced white win on e1, got %+v", got)
	}
	if got.Level != "intermediate" {
		t.Errorf("level = %q, want intermediate", got.Level)
	}
	if len(got.Threats.White) != 1 || got.Threats.White[0] != "e1" {
		t.Errorf("white threats = %v, want [e1]", got.Threats.White)
	}
}

func TestPostAnalyzePosition_badRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"not json", `{`},
		{"missing tps", `{}`},
		{"blank tps", `{"tps":"   "}`},
		{"bad tps", `{"tps":"x5/x5 1 1"}`},
		{"nine by nine", `{"tps":"x9/x9/x9/x9/x9/x9/x9/x9/x9 1 1"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := postJSON(t, postAnalyzePositionHandler, c.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
			}
		})
	}
}

func TestPostCompareMoves(t *testing.T) {
	rec := postJSON(t, postCompareMovesHandler,
		`{"tps":"`+roadInOneTPS+`","moves":["a3","e1","Sz9"],"level":"intermediate","time_limit":300000000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got CompareMovesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Moves) != 3 {
		t.Fatalf("got %d moves, want 3", len(got.Moves))
	}
	if got.Moves[0].Move != "e1" || got.Moves[0].Loss != 0 {
		t.Errorf("first = %+v, want e1 with no loss", got.Moves[0])
	}
	if got.Moves[1].Move != "a3" || got.Moves[1].Loss == 0 {
		t.Errorf("second = %+v, want a3 with a loss", got.Moves[1])
	}
	if got.Moves[2].Error == "" {
		t.Errorf("Sz9 should be rejected: %+v", got.Moves[2])
	}
}

func TestPostAnalyzeTree(t *testing.T) {
	rec := postJSON(t, postAnalyzeTreeHandler,
		`{"tps":"`+roadInOneTPS+`","breadth":3,"depth":2,"level":"intermediate","time_limit":300000000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got TreeAnalysisResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Level != "intermediate" {
		t.Errorf("level = %q, want intermediate", got.Level)
	}
	if got.Move != "" {
		t.Errorf("root move = %q, want empty", got.Move)
	}
	if len(got.Children) == 0 {
		t.Fatal("tree has no children")
	}
	top := got.Children[0]
	if top.Move != "e1" {
		t.Errorf("top move = %q, want e1", top.Move)
	}
	if top.Score != ai.ForcedWinScore {
		t.Errorf("top score = %d, want %d", top.Score, ai.ForcedWinScore)
	}
	if top.ForcedWin != gotak.PlayerWhite {
		t.Errorf("top forced_win = %d, want %d", top.ForcedWin, gotak.PlayerWhite)
	}
	if !top.GameOver || top.Winner != gotak.PlayerWhite {
		t.Errorf("top game_over = %v winner = %d, want white win", top.GameOver, top.Winner)
	}
}

func TestPostAnalyzeTree_shape(t *testing.T) {
	rec := postJSON(t, postAnalyzeTreeHandler,
		`{"tps":"x5/x5/x5/x5/x5 1 1","breadth":2,"depth":2,"level":"intermediate","time_limit":300000000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got TreeAnalysisResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(got.Children))
	}
	if len(got.PV) != 0 {
		t.Errorf("root PV = %v, want empty on intermediate node", got.PV)
	}
	for i, c := range got.Children {
		if c.Move == "" {
			t.Errorf("child %d move is empty", i)
		}
		if len(c.PV) != 0 {
			t.Errorf("child %d PV = %v, want empty on intermediate node", i, c.PV)
		}
		if len(c.Children) != 2 {
			t.Fatalf("child %d children = %d, want 2", i, len(c.Children))
		}
		for j, gc := range c.Children {
			if gc.Move == "" {
				t.Errorf("leaf %d.%d move is empty", i, j)
			}
			if len(gc.Children) != 0 {
				t.Errorf("leaf %d.%d children = %d, want 0", i, j, len(gc.Children))
			}
			if len(gc.PV) == 0 {
				t.Errorf("leaf %d.%d PV is empty", i, j)
			}
		}
	}
}

func TestPostAnalyzeTree_badRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"not json", `{`},
		{"missing tps", `{"breadth":2,"depth":2}`},
		{"blank tps", `{"tps":"   ","breadth":2,"depth":2}`},
		{"breadth zero", `{"tps":"x5/x5/x5/x5/x5 1 1","breadth":0,"depth":2}`},
		{"breadth negative", `{"tps":"x5/x5/x5/x5/x5 1 1","breadth":-1,"depth":2}`},
		{"breadth too large", `{"tps":"x5/x5/x5/x5/x5 1 1","breadth":5,"depth":2}`},
		{"depth zero", `{"tps":"x5/x5/x5/x5/x5 1 1","breadth":2,"depth":0}`},
		{"depth negative", `{"tps":"x5/x5/x5/x5/x5 1 1","breadth":2,"depth":-1}`},
		{"depth too large", `{"tps":"x5/x5/x5/x5/x5 1 1","breadth":2,"depth":4}`},
		{"bad tps", `{"tps":"x5/x5 1 1","breadth":2,"depth":2}`},
		{"nine by nine", `{"tps":"x9/x9/x9/x9/x9/x9/x9/x9/x9 1 1","breadth":2,"depth":2}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := postJSON(t, postAnalyzeTreeHandler, c.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
			}
		})
	}
}

func TestMaxTreeSearchesFor(t *testing.T) {
	cases := []struct {
		breadth int
		depth   int
		want    int
	}{
		{1, 1, 3},
		{1, 3, 7},
		{2, 1, 4},
		{2, 2, 10},
		{2, 3, 22},
		{3, 2, 17},
		{3, 3, 53},
		{4, 2, 26},
		{4, 3, 106},
	}
	for _, c := range cases {
		got := maxTreeSearchesFor(c.breadth, c.depth)
		if got != c.want {
			t.Errorf("maxTreeSearchesFor(%d, %d) = %d, want %d", c.breadth, c.depth, got, c.want)
		}
	}
}

func TestPostCompareMoves_badRequests(t *testing.T) {
	many, _ := json.Marshal(make([]string, ai.MaxCompareMoves+1))
	cases := []struct {
		name string
		body string
	}{
		{"not json", `[`},
		{"missing tps", `{"moves":["a1"]}`},
		{"no moves", `{"tps":"x5/x5/x5/x5/x5 1 1","moves":[]}`},
		{"too many moves", `{"tps":"x5/x5/x5/x5/x5 1 1","moves":` + string(many) + `}`},
		{"finished position", `{"tps":"x5/x5/x5/2,2,x3/1,1,1,1,1 2 5","moves":["a3"]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := postJSON(t, postCompareMovesHandler, c.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
			}
		})
	}
}

func TestPostAnalyzePosition_bodyTooLarge(t *testing.T) {
	body := `{"tps":"` + strings.Repeat("x", maxPositionBody) + `"}`
	rec := postJSON(t, postAnalyzePositionHandler, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAnalysisErrorStatus(t *testing.T) {
	if got := analysisErrorStatus(ai.ErrNoMove); got != http.StatusServiceUnavailable {
		t.Errorf("ErrNoMove -> %d, want 503", got)
	}
	if got := analysisErrorStatus(io.ErrUnexpectedEOF); got != http.StatusBadRequest {
		t.Errorf("other error -> %d, want 400", got)
	}
}

// TestExtendWriteDeadline_outlivesServerTimeout runs through the same
// wrappers as production (gutil logging and otelhttp), since the deadline
// only moves if every wrapper lets ResponseController reach the conn.
func TestExtendWriteDeadline_outlivesServerTimeout(t *testing.T) {
	const serverTimeout = 100 * time.Millisecond

	r := chi.NewRouter()
	r.Use(logging.Middleware(zap.NewNop()))
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		extendWriteDeadline(w, zap.NewNop().Sugar(), 5*time.Second)
		time.Sleep(3 * serverTimeout)
		if _, err := w.Write([]byte("done")); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	srv := httptest.NewUnstartedServer(otelhttp.NewHandler(r, "test"))
	srv.Config.WriteTimeout = serverTimeout
	srv.Start()
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed, deadline was not extended: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(body, []byte("done")) {
		t.Errorf("body = %q, want done", body)
	}
}
