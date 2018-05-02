package main

import (
	"log"
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

	log.Printf("%s", ptn)

	return ret, nil
}
