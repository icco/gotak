package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
)

type Game struct {
	Turns []*Turn
	Board *Board
	Meta  []*Tag
}

type Tag struct {
	Key   string
	Value string
}

func (t *Tag) String() string {
	return fmt.Sprintf("%s: %s", t.Key, t.Value)
}

type Turn struct {
	Number  int64
	First   *Move
	Second  *Move
	Result  string
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
		if err != nil {
			return nil, err
		}

		if ta != nil {
			ret.Meta = append(ret.Meta, ta)
			continue
		}

		tu, err := parseTurn(l)
		if err != nil {
			return nil, err
		}
		if tu != nil {
			ret.Turns = append(ret.Turns, tu)
			continue
		}

	}

	if err := s.Err(); err != nil {
		return ret, err
	}

	//log.Printf("Parsed Game: %+v", ret)
	return ret, nil
}

func parseTag(line string) (*Tag, error) {
	var tag *Tag

	// Example: [Tag_Name "Tag Data"]
	tagRegex := regexp.MustCompile(`\[([0-9A-Za-z_]+) "(.*)"\]`)
	parts := tagRegex.FindStringSubmatch(line)

	if parts != nil && len(parts) >= 3 {
		tag = &Tag{
			Key:   parts[1],
			Value: parts[2],
		}
	}

	return tag, nil
}

func parseTurn(line string) (*Turn, error) {
	turn := &Turn{}

	// Parse out comments
	commentRegex := regexp.MustCompile("{.+}")
	cmnt := strings.TrimSpace(strings.Join(commentRegex.FindAllString(line, -1), " "))
	cmnt = strings.Trim(cmnt, "{}")
	turn.Comment = cmnt

	cleanLine := strings.TrimSpace(commentRegex.ReplaceAllString(line, ""))

	if cleanLine != "" {
		fields := strings.Fields(cleanLine)
		if len(fields) < 3 || len(fields) > 4 {
			return turn, fmt.Errorf("Line doesn't have correct number of parts: %+v", fields)
		}

		// TODO: Support branches. Right now we discard things that are not ints.
		numberVal := fields[0]
		numberVal = strings.TrimRight(numberVal, ".")
		if regexp.MustCompile("[^0-9]+").MatchString(numberVal) {
			log.Printf("%+v is not a number, ignoring line.", numberVal)
			return nil, nil
		}
		num, err := strconv.ParseInt(numberVal, 10, 64)
		if err != nil {
			return nil, err
		}
		turn.Number = num

		p1 := Move(fields[1])
		p2 := Move(fields[2])

		turn.First = &p1
		turn.Second = &p2

		if len(fields) == 4 {
			turn.Result = fields[3]
		}
	}

	if turn.Comment != "" || (turn.Number > 0 && (turn.First != nil || turn.Second != nil)) {
		return turn, nil
	}

	return nil, nil
}
