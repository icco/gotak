package main

import (
	"io/ioutil"
	"testing"
)

func TestParse(t *testing.T) {
	file, err := ioutil.ReadFile("test.ptn")
	if err != nil {
		t.Errorf("%+v", err)
	}

	_, err = ParsePTN(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
}
