package gotak

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestParse(t *testing.T) {
	files, err := ioutil.ReadDir("./test_games")
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

func assertNotEqual(t *testing.T, context string, a interface{}, b interface{}) {
	if a == b {
		t.Errorf("%s: %+v == %+v", context, a, b)
	}
}
