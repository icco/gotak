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

		})
	}
}
