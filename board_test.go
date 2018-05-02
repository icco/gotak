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
		file, err := ioutil.ReadFile(fmt.Sprintf("test_games/%s", fi.Name()))
		if err != nil {
			t.Errorf("%+v", err)
		}

		_, err = ParsePTN(file)
		if err != nil {
			t.Errorf("%+v", err)
		}
	}
}
