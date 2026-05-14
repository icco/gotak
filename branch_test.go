package gotak

import "testing"

func TestParseTurnLabel(t *testing.T) {
	cases := []struct {
		in     string
		num    int64
		branch string
		ok     bool
	}{
		{"1", 1, "", true},
		{"12", 12, "", true},
		{"1a", 1, "a", true},
		{"42c", 42, "c", true},
		{"", 0, "", false},
		{"a1", 0, "", false},
		{"1ab", 0, "", false},
		{"1A", 0, "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			n, br, ok := parseTurnLabel(c.in)
			if ok != c.ok || n != c.num || br != c.branch {
				t.Errorf("parseTurnLabel(%q) = (%d,%q,%v), want (%d,%q,%v)", c.in, n, br, ok, c.num, c.branch, c.ok)
			}
		})
	}
}

func TestParsePTN_branches(t *testing.T) {
	ptn := []byte(`[Size "5"]
[Player1 "alice"]
[Player2 "bob"]

1. a1 e5
2. b2 d4
3. c3 c4

{Alternate continuation}
1a. a1 e5
2a. b2 d4
3a. e1 a5
`)

	g, err := ParsePTN(ptn)
	if err != nil {
		t.Fatalf("ParsePTN: %v", err)
	}

	main := g.TurnsInBranch("")
	if len(main) != 3 {
		t.Errorf("main branch has %d turns, want 3", len(main))
	}

	a := g.TurnsInBranch("a")
	if len(a) != 3 {
		t.Errorf("branch 'a' has %d turns, want 3", len(a))
	}

	branches := g.Branches()
	if len(branches) != 1 || branches[0] != "a" {
		t.Errorf("Branches() = %v, want [a]", branches)
	}

	// Branch turns should keep their original numbering on parse.
	for i, turn := range a {
		if turn.Number != int64(i+1) {
			t.Errorf("branch a turn %d has Number=%d, want %d", i, turn.Number, i+1)
		}
		if turn.Branch != "a" {
			t.Errorf("branch a turn %d has Branch=%q, want \"a\"", i, turn.Branch)
		}
	}
}

func TestTurnText_branch(t *testing.T) {
	turn := &Turn{
		Number: 2,
		Branch: "b",
		First:  &Move{Text: "a1"},
		Second: &Move{Text: "e5"},
	}
	got := turn.Text()
	want := "2b. a1 e5"
	if got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}
