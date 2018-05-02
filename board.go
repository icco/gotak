package main

import (
	"bufio"
	"bytes"
	"time"
)

type Game struct {
	Player1 string
	Player2 string
	Date    time.Time
	Result  string
	Moves   []*Move
	Board   *Board
	Meta    []*Tag
}

type Tag struct {
	Key   string
	Value string
}

type Move struct {
	Player1 string
	Player2 string
	Comment string
}

type Board struct {
	Size   int
	Square map[string][]*Stone
}

type Stone struct {
	Type   string
	Player int
}

func ParsePTN(ptn []byte) (*Game, error) {
	ret := &Game{}

	s := bufio.NewScanner(bytes.NewReader(ptn))
	for s.Scan() {
		//_ := s.Text()
	}

	if err := s.Err(); err != nil {
		return ret, err
	}

	return ret, nil
}
