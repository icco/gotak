package main

import (
	"io/ioutil"
	"log"

	"github.com/icco/gotak"
)

func main() {
	file, err := ioutil.ReadFile("test_games/sample.ptn")
	if err != nil {
		log.Panicf("%+v", err)
	}

	g, err := gotak.ParsePTN(file)
	if err != nil {
		log.Panicf("%+v", err)
	}

	for _, t := range g.Turns {
		g.Board.DoMove(t.First, 1)
		g.Board.DoMove(t.Second, 2)
	}
	log.Printf("Game: %+v", g)
}
