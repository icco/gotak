package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"
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
	Number  int64
	First   *Move
	Second  *Move
	Comment string
}

func (t *Turn) String() string {
	var move string
	if t.First != nil && t.Second != nil {
		move = fmt.Sprintf("%d. %s %s", t.Number, *t.First, *t.Second)
	}
	if t.Comment != "" {
		if move != "" {
			move = fmt.Sprintf("%s { %s }", move, t.Comment)
		} else {
			move = fmt.Sprintf("{ %s }", t.Comment)
		}
	}
	return move
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
		ta, err := parseTag(l)
		tu, err := parseTurn(l)
		log.Printf("%s : %+v, %+v, %+v", l, ta, tu, err)
	}

	if err := s.Err(); err != nil {
		return ret, err
	}

	return ret, nil
}

func parseTag(line string) (*Tag, error) {
	r := strings.NewReader(line)
	s := scanner.Scanner{}
	s.Init(r)

	var tag *Tag

	insideTag := false

	run := s.Peek()
	for run != scanner.EOF {
		switch run {
		case '\n', '\r', '1':
			return tag, nil
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
	return tag, nil
}

func parseTurn(line string) (*Turn, error) {
	turn := &Turn{}
	r := strings.NewReader(line)
	s := scanner.Scanner{}
	s.Init(r)

	commentRegex := regexp.MustCompile("{.+}")
	turn.Comment = strings.TrimSpace(strings.Join(commentRegex.FindAllString(line, -1), " "))
	cleanLine := strings.TrimSpace(commentRegex.ReplaceAllString(line, ""))

	if cleanLine != "" {
		fields := strings.Fields(cleanLine)
		if len(fields) != 3 {
			return turn, fmt.Errorf("Line doesn't have three parts: %+v", fields)
		}

		// TODO: Support branches. Right now we discard things that are not ints.
		numberVal := fields[0]
		numberVal = strings.TrimRight(numberVal, ".")
		num, err := strconv.ParseInt(numberVal, 10, 64)
		if err != nil {
			return nil, err
		}
		turn.Number = num

		p1 := Move(fields[1])
		p2 := Move(fields[2])

		turn.First = &p1
		turn.Second = &p2
	}

	if turn.Comment != "" || (turn.Number > 0 && (turn.First != nil || turn.Second != nil)) {
		return turn, nil
	}

	return nil, nil
}
