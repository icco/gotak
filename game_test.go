package gotak

import (
	"fmt"
	"os"
	"path"
	"testing"
)

func assertNotEqual(t *testing.T, context string, a, b interface{}) {
	if a == b {
		t.Errorf("%s: %+v == %+v", context, a, b)
	}
}

func TestParse(t *testing.T) {
	dir := path.Join(".", "test_games")
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("%+v", err)
	}

	for _, fi := range files {
		// Run the following test for each file
		t.Run(fi.Name(), func(t *testing.T) {
			file, err := os.ReadFile(fmt.Sprintf("test_games/%s", fi.Name()))
			if err != nil {
				t.Errorf("%s: %+v", fi.Name(), err)
			}

			g, err := ParsePTN(file)
			if err != nil {
				t.Errorf("%+v", err)
				return
			}

			if g == nil {
				t.Errorf("ParsePTN returned nil game")
				return
			}

			// Required tags
			for _, k := range []string{"Player1", "Player2", "Date", "Size", "Result"} {
				_, err := g.GetMeta(k)
				if err != nil {
					t.Errorf("%+v", err)
				}
			}

			if g == nil {
				t.Errorf("Game is nil")
				return
			}

			if g.Turns == nil {
				// This is valid for games with no moves (e.g., games with only TPS)
				return
			}

			for i, turn := range g.Turns {
				if turn == nil {
					t.Errorf("Turn %d is nil", i)
					continue
				}
				context := fmt.Sprintf("Turn: %+v", turn)
				assertNotEqual(t, context, turn, nil)
				if turn.First == nil {
					t.Errorf("Turn %d First move is nil", i)
					continue
				}
				assertNotEqual(t, context, turn.First.Text, "")
				assertNotEqual(t, context, turn.Number, 0)
				
				// Second move can be nil for incomplete turns (e.g., at game end)
				if turn.Second != nil {
					assertNotEqual(t, context, turn.Second.Text, "")
				}
			}
		})
	}
}

func TestGameOver(t *testing.T) {
	game, err := NewGame(6, 1, "test")
	if err != nil {
		t.Errorf("%+v", err)
	}

	_, over := game.GameOver()
	if over {
		t.Errorf("Game over on empty board")
	}
}
