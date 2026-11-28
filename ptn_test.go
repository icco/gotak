package gotak

import (
	"strings"
	"testing"
)

func TestGamePTN_emptyGame(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	got := g.PTN()
	// The Size meta tag is set by NewGame, so we expect that line at least.
	if !strings.Contains(got, `[Size "5"]`) {
		t.Errorf("PTN missing Size tag: %q", got)
	}
}

func TestGamePTN_completedTurns(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("a1", PlayerWhite); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("e5", PlayerBlack); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("b2", PlayerWhite); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("d4", PlayerBlack); err != nil {
		t.Fatal(err)
	}

	got := g.PTN()
	for _, want := range []string{
		`[Size "5"]`,
		"1. a1 e5",
		"2. b2 d4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PTN missing %q: %q", want, got)
		}
	}
}

func TestGamePTN_incompleteFinalTurn(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("a1", PlayerWhite); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("e5", PlayerBlack); err != nil {
		t.Fatal(err)
	}
	if err := g.DoSingleMove("b2", PlayerWhite); err != nil {
		t.Fatal(err)
	}

	got := g.PTN()
	// Incomplete final turn must render as exactly "2. b2\n", not
	// "2. b2 \n" with a dangling space and not "2. b2 --\n".
	if !strings.Contains(got, "\n2. b2\n") {
		t.Errorf("incomplete turn 2 should render as `2. b2`, got:\n%s", got)
	}
}

func TestGamePTN_tagWithQuoteRoundtrips(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.UpdateMeta("Player1", `"Alice"`); err != nil {
		t.Fatal(err)
	}
	out := g.PTN()
	if strings.Contains(out, `"Alice"`) {
		t.Errorf("PTN should coerce embedded quotes; got %q", out)
	}
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN failed on tag with quote: %v\n%s", err, out)
	}
	v, err := round.GetMeta("Player1")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if !strings.Contains(v, "Alice") {
		t.Errorf("Player1 = %q, want it to still contain Alice", v)
	}
}

func TestGamePTN_roundTrip(t *testing.T) {
	src := []byte(`[Size "5"]
[Player1 "alice"]
[Player2 "bob"]

1. a1 e5
2. b2 d4
3. c3 c4
`)

	g, err := ParsePTN(src)
	if err != nil {
		t.Fatalf("ParsePTN: %v", err)
	}

	out := g.PTN()
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN(PTN(ParsePTN(src))) failed: %v\noutput was:\n%s", err, out)
	}

	if len(round.Turns) != len(g.Turns) {
		t.Errorf("turn count drifted: original=%d, round-tripped=%d", len(g.Turns), len(round.Turns))
	}
	for i := range g.Turns {
		if g.Turns[i].First.Text != round.Turns[i].First.Text {
			t.Errorf("turn %d first move drift: %q vs %q", i, g.Turns[i].First.Text, round.Turns[i].First.Text)
		}
		if (g.Turns[i].Second == nil) != (round.Turns[i].Second == nil) {
			t.Errorf("turn %d second move nil-ness drift", i)
		}
	}
}

func TestGamePTN_nilGame(t *testing.T) {
	var g *Game
	if got := g.PTN(); got != "" {
		t.Errorf("nil game should return empty string, got %q", got)
	}
}

func TestParsePTN_moveCommentsRoundTrip(t *testing.T) {
	src := []byte(`[Size "5"]
[Player1 "alice"]
[Player2 "bob"]

1. a1 { nice } e5 { blunder }
2. b2 { solid } d4
3. c3 e4 { mistake }
4. d3 { win threat } R-0 { resignation }
5. { turn note without final move comment } e1 c2
6. { turn note on incomplete turn } b3
`)

	g, err := ParsePTN(src)
	if err != nil {
		t.Fatalf("ParsePTN: %v", err)
	}

	if len(g.Turns) != 6 {
		t.Fatalf("got %d turns, want 6", len(g.Turns))
	}

	// Turn 1: both moves have comments
	t1 := g.Turns[0]
	if t1.First == nil || t1.First.Text != "a1" || t1.First.Comment != "nice" {
		t.Errorf("turn 1 first move = %+v, want a1 with 'nice'", t1.First)
	}
	if t1.Second == nil || t1.Second.Text != "e5" || t1.Second.Comment != "blunder" {
		t.Errorf("turn 1 second move = %+v, want e5 with 'blunder'", t1.Second)
	}

	// Turn 2: only first move has comment
	t2 := g.Turns[1]
	if t2.First == nil || t2.First.Comment != "solid" {
		t.Errorf("turn 2 first comment = %q, want 'solid'", t2.First.Comment)
	}
	if t2.Second == nil || t2.Second.Comment != "" {
		t.Errorf("turn 2 second comment = %q, want empty", t2.Second.Comment)
	}

	// Turn 3: only second move has comment
	t3 := g.Turns[2]
	if t3.First == nil || t3.First.Comment != "" {
		t.Errorf("turn 3 first comment = %q, want empty", t3.First.Comment)
	}
	if t3.Second == nil || t3.Second.Comment != "mistake" {
		t.Errorf("turn 3 second comment = %q, want 'mistake'", t3.Second.Comment)
	}

	// Turn 4: first move has comment, comment after result goes to turn
	t4 := g.Turns[3]
	if t4.First == nil || t4.First.Comment != "win threat" {
		t.Errorf("turn 4 first comment = %q, want 'win threat'", t4.First.Comment)
	}
	if t4.Result != "R-0" {
		t.Errorf("turn 4 result = %q, want 'R-0'", t4.Result)
	}
	if t4.Comment != "resignation" {
		t.Errorf("turn 4 comment = %q, want 'resignation'", t4.Comment)
	}

	// Turn 5: turn comment with no annotations on moves
	t5 := g.Turns[4]
	if t5.First == nil || t5.First.Comment != "" {
		t.Errorf("turn 5 first comment = %q, want empty", t5.First.Comment)
	}
	if t5.Second == nil || t5.Second.Comment != "" {
		t.Errorf("turn 5 second comment = %q, want empty", t5.Second.Comment)
	}
	if t5.Comment != "turn note without final move comment" {
		t.Errorf("turn 5 comment = %q, want 'turn note without final move comment'", t5.Comment)
	}

	// Turn 6: turn comment on incomplete turn with no move annotation
	t6 := g.Turns[5]
	if t6.First == nil || t6.First.Comment != "" {
		t.Errorf("turn 6 first comment = %q, want empty", t6.First.Comment)
	}
	if t6.Second != nil {
		t.Errorf("turn 6 second should be nil")
	}
	if t6.Comment != "turn note on incomplete turn" {
		t.Errorf("turn 6 comment = %q, want 'turn note on incomplete turn'", t6.Comment)
	}

	out := g.PTN()
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN(PTN): %v\noutput was:\n%s", err, out)
	}

	if len(round.Turns) != len(g.Turns) {
		t.Fatalf("round-trip turns = %d, want %d", len(round.Turns), len(g.Turns))
	}
	for i := range g.Turns {
		orig := g.Turns[i]
		rt := round.Turns[i]
		if orig.First.Text != rt.First.Text || orig.First.Comment != rt.First.Comment {
			t.Errorf("turn %d first move drift: orig=(%q, %q) vs rt=(%q, %q)",
				i+1, orig.First.Text, orig.First.Comment, rt.First.Text, rt.First.Comment)
		}
		if (orig.Second == nil) != (rt.Second == nil) {
			t.Errorf("turn %d second nil drift", i+1)
		} else if orig.Second != nil {
			if orig.Second.Text != rt.Second.Text || orig.Second.Comment != rt.Second.Comment {
				t.Errorf("turn %d second move drift: orig=(%q, %q) vs rt=(%q, %q)",
					i+1, orig.Second.Text, orig.Second.Comment, rt.Second.Text, rt.Second.Comment)
			}
		}
		if orig.Comment != rt.Comment {
			t.Errorf("turn %d turn comment drift: %q vs %q", i+1, orig.Comment, rt.Comment)
		}
		if orig.Result != rt.Result {
			t.Errorf("turn %d result drift: %q vs %q", i+1, orig.Result, rt.Result)
		}
	}
}

func TestParsePTN_tagLikeCommentText(t *testing.T) {
	src := []byte(`[Size "5"]
[Player1 "alice"]
[Player2 "bob"]

1. a1 { reviewed by [Reviewer "Alice"] } e5
`)

	g, err := ParsePTN(src)
	if err != nil {
		t.Fatalf("ParsePTN: %v", err)
	}

	if len(g.Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(g.Turns))
	}
	if g.Turns[0].First == nil || g.Turns[0].First.Text != "a1" {
		t.Errorf("first move = %+v, want a1", g.Turns[0].First)
	}
	if g.Turns[0].First.Comment != `reviewed by [Reviewer "Alice"]` {
		t.Errorf("first move comment = %q", g.Turns[0].First.Comment)
	}
	if g.Turns[0].Second == nil || g.Turns[0].Second.Text != "e5" {
		t.Errorf("second move = %+v, want e5", g.Turns[0].Second)
	}

	// Verify it does not add a bogus metadata tag
	if val, _ := g.GetMeta("Reviewer"); val != "" {
		t.Errorf("unexpected metadata tag Reviewer = %q", val)
	}

	// Round-trip check
	out := g.PTN()
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN(PTN): %v\noutput was:\n%s", err, out)
	}
	if len(round.Turns) != 1 || round.Turns[0].First == nil || round.Turns[0].First.Comment != `reviewed by [Reviewer "Alice"]` {
		t.Fatalf("round-trip comment drift: %+v", round.Turns[0].First)
	}
}

func TestParsePTN_consecutiveCommentBlocks(t *testing.T) {
	src := []byte(`[Size "5"]
[Player1 "alice"]
[Player2 "bob"]

1. a1 { opening } { follow-up } e5 { response } { second note }
`)

	g, err := ParsePTN(src)
	if err != nil {
		t.Fatalf("ParsePTN: %v", err)
	}

	if len(g.Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(g.Turns))
	}
	t1 := g.Turns[0]
	if t1.First == nil || t1.First.Comment != "opening follow-up" {
		t.Errorf("first comment = %q, want 'opening follow-up'", t1.First.Comment)
	}
	if t1.Second == nil || t1.Second.Comment != "response second note" {
		t.Errorf("second comment = %q, want 'response second note'", t1.Second.Comment)
	}
	if t1.Comment != "" {
		t.Errorf("turn comment = %q, want empty", t1.Comment)
	}

	// Round-trip check
	out := g.PTN()
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN(PTN): %v\noutput was:\n%s", err, out)
	}
	if len(round.Turns) != 1 || round.Turns[0].First.Comment != "opening follow-up" || round.Turns[0].Second.Comment != "response second note" {
		t.Fatalf("round-trip comment mismatch: %+v", round.Turns[0])
	}
}

func TestGamePTN_unrenderableTurnsSkipped(t *testing.T) {
	g, err := NewGame(5, 1, "test")
	if err != nil {
		t.Fatal(err)
	}

	// Second-only turn should be skipped, not emit bare label "1."
	g.Turns = []*Turn{
		{
			Number: 1,
			Second: &Move{Text: "e5"},
		},
	}

	out := g.PTN()
	if strings.Contains(out, "1.") {
		t.Errorf("unrenderable second-only turn should not emit label, got:\n%s", out)
	}

	// Game with unrenderable turn should still round-trip through ParsePTN without errors
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN failed on output with skipped turn: %v", err)
	}
	if len(round.Turns) != 0 {
		t.Errorf("got %d turns, want 0", len(round.Turns))
	}
}

func TestParsePTN_annotatedSlideNotTreatedAsResult(t *testing.T) {
	src := []byte(`[Size "5"]
[Player1 "alice"]
[Player2 "bob"]

1. a1 1b3-1! { note }
`)

	g, err := ParsePTN(src)
	if err != nil {
		t.Fatalf("ParsePTN: %v", err)
	}

	if len(g.Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(g.Turns))
	}
	t1 := g.Turns[0]
	if t1.First == nil || t1.First.Text != "a1" {
		t.Errorf("first move = %+v, want a1", t1.First)
	}
	if t1.Second == nil || t1.Second.Text != "1b3-1" {
		t.Errorf("second move = %+v, want 1b3-1", t1.Second)
	}
	if t1.Second.Comment != "note" {
		t.Errorf("second move comment = %q, want 'note'", t1.Second.Comment)
	}
	if t1.Result != "" {
		t.Errorf("result = %q, want empty", t1.Result)
	}
	if t1.Comment != "" {
		t.Errorf("turn comment = %q, want empty", t1.Comment)
	}

	// Round-trip check
	out := g.PTN()
	round, err := ParsePTN([]byte(out))
	if err != nil {
		t.Fatalf("ParsePTN(PTN): %v\noutput was:\n%s", err, out)
	}
	if len(round.Turns) != 1 || round.Turns[0].Second == nil || round.Turns[0].Second.Text != "1b3-1" || round.Turns[0].Second.Comment != "note" {
		t.Fatalf("round-trip drift: %+v", round.Turns[0])
	}
}
