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

			_, err = ParsePTN(file)
			if err != nil {
				t.Errorf("%+v", err)
			}
		})
	}
}
