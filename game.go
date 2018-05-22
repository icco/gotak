package gotak

import (
	"bufio"
	"bytes"
	"fmt"
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

// GetMeta does a linear search for the key specified and returns the value. It
// returns an error if the key does not exist.
func (g *Game) GetMeta(key string) (string, error) {
	for _, t := range g.Meta {
		if t != nil && t.Key == key {
			return t.Value, nil
		}
	}

	return "", fmt.Errorf("No such meta key '%s'", key)
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

	err = ret.Board.Init()
	if err != nil {
		return nil, err
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

		p1, err := NewMove(fields[1])
		if err != nil {
			return nil, err
		}

		p2, err := NewMove(fields[2])
		if err != nil {
			return nil, err
		}

		turn.First = p1
		turn.Second = p2

		if len(fields) == 4 {
			turn.Result = fields[3]
		}
	}

	if turn.Comment != "" || (turn.Number > 0 && (turn.First != nil || turn.Second != nil)) {
		return turn, nil
	}

	return nil, nil
}
