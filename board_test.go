package main

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
				assertNotEqual(t, turn, nil)
				assertNotEqual(t, turn.First, nil)
				assertNotEqual(t, turn.Second, nil)
				assertNotEqual(t, turn.First.String(), "")
				assertNotEqual(t, turn.Second.String(), "")
				assertNotEqual(t, turn.Number, 0)

			}
		})
	}
}

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Errorf("%s != %s", a, b)
	}
}

func assertNotEqual(t *testing.T, a interface{}, b interface{}) {
	if a == b {
		t.Errorf("%s == %s", a, b)
	}
}
