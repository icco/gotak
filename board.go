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
	Size    int64
	Squares map[string][]*Stone
}

// Init creates a board once a board size is set.
func (b *Board) Init() error {
	if b.Size < 4 || b.Size >= 10 {
		return fmt.Errorf("%d is not a valid board size", b.Size)
	}

	b.Squares = map[string][]*Stone{}
	letters := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	for x := int64(0); x < b.Size; x++ {
		for y := int64(1); y <= b.Size; y++ {
			location := letters[x] + strconv.FormatInt(y, 10)
			b.Squares[location] = []*Stone{}
		}
	}

	return nil
}

func (b *Board) String() string {
	return fmt.Sprintf("%+v", b.Squares)
}

// DoMove modifies the boards state based off of a move.
//
// Move notation is from https://www.reddit.com/r/Tak/wiki/portable_tak_notation
//
// The notation format for placing stones is: (stone)(square).
//
// The notation format for moving one or more stones is:
// (count)(square)(direction)(drop counts)(stone)
//
// 1. The count of stones to be lifted from a square is given. This may be
// omitted only if the count is 1.
//
// 2. The square which stones are being moved from is given. This is always
// required.
//
// 3. The direction to move the stones is given. This is always required.
//
// 4. The number of stones to drop on each square in the given direction are
// listed, without spaces. This may be omitted if all of the stones given in
// the count are dropped on a square immediately adjacent to the source square.
// If the stack is moving more than one square, all drop counts must be listed
// and must add up to equal the lift count from parameter 1 above.
//
// 5. The stone type of the top stone of the moved stack is given. If the top
// stone is a flat stone the F identifier is never needed, flat stones are
// always assumed. If the top stone is a standing stone or capstone, the S or C
// can be used, though it is not required and infrequently used.
func (b *Board) DoMove(mv *Move, player int) error {
	mvStr := mv.String()

	placeRegex := regexp.MustCompile(`^(C|S)?([a-z][0-9]+)$`)
	if placeRegex.MatchString(mvStr) {
		parts := placeRegex.FindStringSubmatch(mvStr)
		// log.Printf("Place piece: %+v", parts)
		stone := &Stone{
			Player: player,
		}
		location := ""
		if len(parts) == 2 {
			location = parts[1]
		}

		if len(parts) == 3 {
			location = parts[2]
			stone.Type = parts[1]
		}

		if stone.Type == "" {
			stone.Type = "f"
		}
		b.Squares[location] = append(b.Squares[location], stone)
		return nil
	}

	// (count)(square)(direction)(drop counts)(stone)
	moveRegex := regexp.MustCompile(`^([1-9]*)([a-z][0-9]+)([<>+\-])([0-9]+)(C|S)?$`)
	if moveRegex.MatchString(mvStr) {
		parts := moveRegex.FindStringSubmatch(mvStr)
		//log.Printf("move piece: %+v", parts)

		countStr := parts[1]
		totalPieces, err := strconv.ParseInt(countStr, 10, 64)
		if err != nil {
			return err
		}

		location := parts[2]

		direction := parts[3]

		var totalDropped int64
		drpCounts := []int64{}
		for _, str := range strings.Split(parts[4], "") {
			drpCount, err := strconv.ParseInt(str, 10, 64)
			if err != nil {
				return err
			}
			totalDropped += drpCount
			if totalDropped > totalPieces {
				return fmt.Errorf("tried to drop more pieces than available: %d > %d", totalDropped, totalPieces)
			}
			drpCounts = append(drpCounts, drpCount)
		}

		if totalDropped != totalPieces {
			return fmt.Errorf("Did not drop same pieces picked up: %d != %d", totalDropped, totalPieces)
		}

		stoneType := parts[5]
		if stoneType == "" {
			stoneType = "f"
		}

		// Get current pieces
		begin := max(0, len(b.Squares[location])-1-int(totalPieces))
		end := len(b.Squares[location])
		stones := b.Squares[location][begin:end]

		squares := []string{}

		currentSpace := location
		nextSpace := ""
		for i := 0; i < len(stones); i++ {
			switch direction {
			case "<":
				// < Left
				nextSpace = string(currentSpace[0]-1) + string(currentSpace[1])
			case ">":
				// > Right
				nextSpace = string(currentSpace[0]+1) + string(currentSpace[1])
			case "+":
				// + Up
				nextSpace = string(currentSpace[0]) + string(currentSpace[1]+1)
			case "-":
				// - Down
				nextSpace = string(currentSpace[0]) + string(currentSpace[1]-1)
			}

			//log.Printf("%s %s", direction, nextSpace)
			currentSpace = nextSpace
			squares = append(squares, nextSpace)
		}
		//log.Printf("%s %+v %+v", mv, squares, stones)

		// pop and shift
		//x, a = a[0], a[1:]

	}
	return nil
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// Stone is a single Tak stone.
type Stone struct {
	Type   string
	Player int
}

func (s *Stone) String() string {
	return fmt.Sprintf("%d(%s)", s.Player, s.Type)
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

	for _, t := range g.Turns {
		g.Board.DoMove(t.First, 1)
		g.Board.DoMove(t.Second, 2)
	}
	log.Printf("Game: %+v", g)
}
