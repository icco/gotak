package main

import (
	"io/ioutil"
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

	file, err := ioutil.ReadFile(string(opts.Filename))
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
