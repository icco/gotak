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
	ID    int64
	Slug  string
	Turns []*Turn
	Board *Board
	Meta  []*Tag
}

// PrintCurrentState is an attempt to render a tak game as text.
func (g *Game) PrintCurrentState() {
	letters := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	for x := int64(0); x < g.Board.Size; x++ {
		for y := int64(1); y <= g.Board.Size; y++ {
			location := letters[x] + strconv.FormatInt(y, 10)
			fmt.Printf("%v", g.Board.Squares[location])
		}
		fmt.Println("")
	}
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

// UpdateMeta adds or updates a tag on a game.
func (g *Game) UpdateMeta(key, value string) error {
	newTag := &Tag{
		Key:   key,
		Value: value,
	}

	for i, t := range g.Meta {
		if t != nil && t.Key == key {
			g.Meta[i] = newTag
			return nil
		}
	}

	g.Meta = append(g.Meta, newTag)

	return nil
}

// GetTurn returns or creates a turn, given a turn number.
func (g *Game) GetTurn(number int64) *Turn {
	for _, t := range g.Turns {
		if t != nil && t.Number == number {
			return t
		}
	}

	return &Turn{Number: number}
}

// UpdateTurn adds or updates a turn.
func (g *Game) UpdateTurn(turn *Turn) {
	for i, t := range g.Turns {
		if t != nil && t.Number == turn.Number {
			g.Turns[i] = turn
			return
		}
	}

	g.Turns = append(g.Turns, turn)

	return
}

// DoTurn takes raw input, validates
func (g *Game) DoTurn(mvOneStr, mvTwoStr string) error {
	mvOne, err := NewMove(mvOneStr)
	if err != nil {
		return err
	}

	mvTwo, err := NewMove(mvTwoStr)
	if err != nil {
		return err
	}

	// First turn you place the other person's
	if len(g.Turns) == 0 {
		g.Board.DoMove(mvOne, PlayerBlack)
		g.Board.DoMove(mvTwo, PlayerWhite)
	} else {
		g.Board.DoMove(mvOne, PlayerWhite)
		g.Board.DoMove(mvTwo, PlayerBlack)
	}

	g.Turns = append(g.Turns, &Turn{
		Number: int64(len(g.Turns)),
		First:  mvOne,
		Second: mvTwo,
	})

	return nil
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
