package main

import (
	"log"

	"github.com/icco/gotak"
)

func main() {
	g := &gotak.Game{}
	g.Board = &gotak.Board{
		Size: 6,
	}
	err := g.Board.Init()
	if err != nil {
		log.Fatalf("Creating board: %+v", err)
	}

	mv, err := gotak.NewMove("c3")
	if err != nil {
		log.Fatalf("Error moving: %+v", err)
	}
	if err := g.Board.DoMove(mv, gotak.PlayerWhite); err != nil {
		log.Fatalf("Error making move: %+v", err)
	}
	g.PrintCurrentState()
}
