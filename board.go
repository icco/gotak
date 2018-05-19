package main

import (
	"bufio"
	"bytes"
	"log"
	"strings"
	"text/scanner"
	"time"
)

type Game struct {
	Date   time.Time
	Result string
	Turns  []*Turn
	Board  *Board
	Meta   []*Tag
}

type Tag struct {
	Key   string
	Value string
}

type Turn struct {
	Number  int
	First   *Move
	Second  *Move
	Comment string
}

type Move string

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
		l := s.Text()
		ta, tu, err := parseLine(l)
		log.Printf("%s : %+v, %+v, %+v", l, ta, tu, err)
	}

	if err := s.Err(); err != nil {
		return ret, err
	}

	return ret, nil
}

func parseLine(line string) (*Turn, *Tag, error) {
	r := strings.NewReader(line)
	s := scanner.Scanner{}
	s.Init(r)

	var tag *Tag
	var turn *Turn

	insideTag := false

	run := s.Peek()
	for run != scanner.EOF {
		switch run {
		case '\n', '\r', '1':
			return turn, tag, nil
		case '[', ']':
			run = s.Next()
			insideTag = !insideTag
		default:
			if insideTag {
				s.Scan()
				key := s.TokenText()
				s.Scan()
				val := s.TokenText()
				tag = &Tag{
					Value: strings.Trim(val, "\""),
					Key:   key,
				}
			} else {
				s.Scan()
			}
		}
		run = s.Peek()
	}
	return turn, tag, nil
}

func parseTurn() {

}
