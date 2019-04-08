package gotak

import (
	"fmt"
	"io/ioutil"
	"path"
	"testing"
)

func assertNotEqual(t *testing.T, context string, a interface{}, b interface{}) {
	if a == b {
		t.Errorf("%s: %+v == %+v", context, a, b)
	}
}

func TestParse(t *testing.T) {
	dir := path.Join(".", "test_games")
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Errorf("%+v", err)
	}

	for _, fi := range files {
		// Run the following test for each file
		t.Run(fi.Name(), func(t *testing.T) {
			file, err := ioutil.ReadFile(fmt.Sprintf("test_games/%s", fi.Name()))
			if err != nil {
				t.Errorf("%s: %+v", fi.Name(), err)
			}

			g, err := ParsePTN(file)
			if err != nil {
				t.Errorf("%+v", err)
			}

			// Required tags
			for _, k := range []string{"Player1", "Player2", "Date", "Size", "Result"} {
				_, err := g.GetMeta(k)
				if err != nil {
					t.Errorf("%+v", err)
				}
			}

			for _, turn := range g.Turns {
				context := fmt.Sprintf("Turn: %+v", turn)
				assertNotEqual(t, context, turn, nil)
				assertNotEqual(t, context, turn.First, nil)
				assertNotEqual(t, context, turn.Second, nil)
				assertNotEqual(t, context, turn.First.Text, "")
				assertNotEqual(t, context, turn.Second.Text, "")
				assertNotEqual(t, context, turn.Number, 0)
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
