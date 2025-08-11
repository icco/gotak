package main

import (
	"log"
	"os"

	"github.com/jessevdk/go-flags"

	"github.com/icco/gotak"
)

var opts struct {
	Filename flags.Filename `short:"f" long:"filename" description:"PTN file to parse" required:"true"`
}

func main() {
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	file, err := os.ReadFile(string(opts.Filename))
	if err != nil {
		log.Panicf("%+v", err)
	}

	g, err := gotak.ParsePTN(file)
	if err != nil {
		log.Panicf("%+v", err)
	}

	for i, t := range g.Turns {
		// log.Printf("%+v", t.Debug())
		if i == 0 {
			if err := g.Board.DoMove(t.First, gotak.PlayerBlack); err != nil {
				log.Printf("Error making first move: %v", err)
			}
			if err := g.Board.DoMove(t.Second, gotak.PlayerWhite); err != nil {
				log.Printf("Error making second move: %v", err)
			}
		} else {
			if err := g.Board.DoMove(t.First, gotak.PlayerWhite); err != nil {
				log.Printf("Error making white move: %v", err)
			}
			if err := g.Board.DoMove(t.Second, gotak.PlayerBlack); err != nil {
				log.Printf("Error making black move: %v", err)
			}
		}
	}
	log.Printf("Game: %+v", g)
}
