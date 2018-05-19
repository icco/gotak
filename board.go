package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"regexp"
	"strconv"
	"strings"
)

// Game is the datastructure for a single game. Most data is stored in the meta
// field.
type Game struct {
	Turns []*Turn
	Board *Board
	Meta  []*Tag
}

func (g *Game) GetMeta(key string) (string, error) {
	for _, t := range g.Meta {
		if t.Key == key {
			return t.Value, nil
		}
	}

	return "", fmt.Errorf("No such meta key '%s'", key)
}

// Tag is a Key and Value pair stored providing meta about a game.
type Tag struct {
	Key   string
	Value string
}

func (t *Tag) String() string {
	return fmt.Sprintf("%s: %s", t.Key, t.Value)
}

// Turn is a single turn played in a game.
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

// Move is a single move in Tak.
//
// TODO: Turn into a struct and add functions for modifying a board.
type Move string

func (m *Move) String() string {
	if m == nil {
		return ""
	}
	return string(*m)
}

// Board is a current state of a game of Tak.
type Board struct {
	Size   int64
	Square map[string][]*Stone
}

// Stone is a single Tak stone.
type Stone struct {
	Type   string
	Player int
}

// ParsePTN parses a .ptn file and returns a Game.
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
			if tu.Number > 0 {
				ret.Turns = append(ret.Turns, tu)
				continue
			}
		}
	}

	if err := s.Err(); err != nil {
		return ret, err
	}

	// Get Board size
	size, err := ret.GetMeta("Size")
	if err != nil {
		return nil, err
	}
	num, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return nil, err
	}
	ret.Board = &Board{
		Size: num,
	}

	return ret, nil
}

func parseTag(line string) (*Tag, error) {
	var tag *Tag

	// Example: [Tag_Name "Tag Data"]
	tagRegex := regexp.MustCompile(`\[([0-9A-Za-z_]+) "(.*)"\]`)
	parts := tagRegex.FindStringSubmatch(line)

	if len(parts) >= 3 {
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

func main() {
	file, err := ioutil.ReadFile("test_games/sample.ptn")
	if err != nil {
		log.Panicf("%+v", err)
	}

	g, err := ParsePTN(file)
	if err != nil {
		log.Panicf("%+v", err)
	}
	log.Printf("Game: %+v", g)
}
