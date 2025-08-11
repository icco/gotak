package main

import (
	"log"
	"os"

	"github.com/icco/gotak"
	"github.com/jessevdk/go-flags"
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
		//log.Printf("%+v", t.Debug())
		if i == 0 {
			_ = g.Board.DoMove(t.First, gotak.PlayerBlack)
			_ = g.Board.DoMove(t.Second, gotak.PlayerWhite)

		} else {
			_ = g.Board.DoMove(t.First, gotak.PlayerWhite)
			_ = g.Board.DoMove(t.Second, gotak.PlayerBlack)
		}
	}
	log.Printf("Game: %+v", g)
}
