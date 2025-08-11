package gotak

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
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

// NewGame is a factory for Game structs. It initializes all fields.
func NewGame(size, id int64, slug string) (*Game, error) {
	g := &Game{
		ID:    id,
		Slug:  slug,
		Board: &Board{Size: size},
		Turns: []*Turn{},
		Meta:  []*Tag{},
	}

	err := g.Board.Init()
	if err != nil {
		return nil, err
	}

	err = g.UpdateMeta("Size", strconv.FormatInt(size, 10))
	if err != nil {
		return nil, err
	}

	return g, nil
}

// GetMaxStonesForBoardSize returns the maximum number of stones per player based on board size
func (g *Game) GetMaxStonesForBoardSize() int64 {
	switch g.Board.Size {
	case 3:
		return 10
	case 4:
		return 15
	case 5:
		return 21
	case 6:
		return 30
	case 7:
		return 40
	case 8:
		return 50
	case 9:
		return 50 // Assuming 9x9 uses same as 8x8
	default:
		return 21 // Default to 5x5 count
	}
}

// GetCapstoneCount returns the number of capstones per player based on board size
func (g *Game) GetCapstoneCount() int64 {
	switch g.Board.Size {
	case 3, 4:
		return 0
	case 5, 6:
		return 1
	case 7:
		return 1 // Rules say "?" for 7x7, assuming 1
	case 8, 9:
		return 2
	default:
		return 1
	}
}

// PrintCurrentState is an attempt to render a tak game as text.
func (g *Game) PrintCurrentState() {
	g.Board.IterateOverSquares(func(l string, s []*Stone) error {
		fmt.Printf("%v", s)
		return nil
	})
}

// GameOver determines if a game is over and who won. A game is over if a
// player has a continuous path from one side of the board to the other.
func (g *Game) GameOver() (int, bool) {
	letters := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}

	// Check for road wins for both players
	for player := PlayerWhite; player <= PlayerBlack; player++ {
		// Check horizontal roads (left to right)
		leftEdge := []string{}
		rightEdge := []string{}
		for y := int64(1); y <= g.Board.Size; y++ {
			leftEdge = append(leftEdge, letters[0]+strconv.FormatInt(y, 10))
			rightEdge = append(rightEdge, letters[g.Board.Size-1]+strconv.FormatInt(y, 10))
		}

		// Check if any left edge square can reach any right edge square
		for _, startSquare := range leftEdge {
			if g.Board.Color(startSquare) == player {
				topStone := g.Board.TopStone(startSquare)
				if topStone != nil && (topStone.Type == StoneFlat || topStone.Type == StoneCap) {
					if g.Board.FindRoad(startSquare, rightEdge) {
						return player, true
					}
				}
			}
		}

		// Check vertical roads (bottom to top)
		bottomEdge := []string{}
		topEdge := []string{}
		for x := int64(0); x < g.Board.Size; x++ {
			bottomEdge = append(bottomEdge, letters[x]+"1")
			topEdge = append(topEdge, letters[x]+strconv.FormatInt(g.Board.Size, 10))
		}

		// Check if any bottom edge square can reach any top edge square
		for _, startSquare := range bottomEdge {
			if g.Board.Color(startSquare) == player {
				topStone := g.Board.TopStone(startSquare)
				if topStone != nil && (topStone.Type == StoneFlat || topStone.Type == StoneCap) {
					if g.Board.FindRoad(startSquare, topEdge) {
						return player, true
					}
				}
			}
		}
	}

	// Check for flat win conditions
	totalSquares := g.Board.Size * g.Board.Size
	occupiedSquares := int64(0)
	whiteFlatCount := int64(0)
	blackFlatCount := int64(0)
	whiteStoneCount := int64(0)
	blackStoneCount := int64(0)

	err := g.Board.IterateOverSquares(func(location string, stones []*Stone) error {
		if len(stones) > 0 {
			occupiedSquares++
			topStone := stones[len(stones)-1]
			
			if topStone.Player == PlayerWhite {
				whiteStoneCount++
				if topStone.Type == StoneFlat {
					whiteFlatCount++
				}
			} else if topStone.Player == PlayerBlack {
				blackStoneCount++
				if topStone.Type == StoneFlat {
					blackFlatCount++
				}
			}
		}
		return nil
	})
	
	if err != nil {
		return 0, false
	}

	// Game ends if board is full
	if occupiedSquares == totalSquares {
		if whiteFlatCount > blackFlatCount {
			return PlayerWhite, true
		} else if blackFlatCount > whiteFlatCount {
			return PlayerBlack, true
		}
		// Tie game
		return 0, true
	}

	// Check if either player has run out of stones
	maxStones := g.GetMaxStonesForBoardSize()
	if whiteStoneCount >= maxStones || blackStoneCount >= maxStones {
		if whiteFlatCount > blackFlatCount {
			return PlayerWhite, true
		} else if blackFlatCount > whiteFlatCount {
			return PlayerBlack, true
		}
		// Tie game
		return 0, true
	}

	return 0, false
}

// GetMeta does a linear search for the key specified and returns the value. It
// returns an error if the key does not exist.
func (g *Game) GetMeta(key string) (string, error) {
	for _, t := range g.Meta {
		if t != nil && t.Key == key {
			return t.Value, nil
		}
	}

	return "", fmt.Errorf("no such meta key %q", key)
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
func (g *Game) GetTurn(number int64) (*Turn, error) {
	max := float64(0)
	for _, t := range g.Turns {
		if t != nil {
			max = math.Max(max, float64(t.Number))
			if t.Number == number {
				return t, nil
			}
		}
	}

	if float64(number) > max+1 {
		return nil, fmt.Errorf("%v cannot be greater than one more than the current max turn number %v", number, max)
	}

	return &Turn{Number: number}, nil
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
}

// DoTurn takes raw input, validates and executes a full turn with both players
func (g *Game) DoTurn(mvOneStr, mvTwoStr string) error {
	mvOne, err := NewMove(mvOneStr)
	if err != nil {
		return err
	}

	mvTwo, err := NewMove(mvTwoStr)
	if err != nil {
		return err
	}

	// First turn: each player places opponent's stone
	if len(g.Turns) == 0 {
		// First move must be a flat stone placement only
		if !mvOne.isPlace() || mvOne.Stone != StoneFlat {
			return fmt.Errorf("first move must be flat stone placement")
		}
		if !mvTwo.isPlace() || mvTwo.Stone != StoneFlat {
			return fmt.Errorf("first move must be flat stone placement")
		}
		
		// Player 1 (white) places black stone, Player 2 (black) places white stone
		err = g.Board.DoMove(mvOne, PlayerBlack)
		if err != nil {
			return fmt.Errorf("first move error: %v", err)
		}
		err = g.Board.DoMove(mvTwo, PlayerWhite)
		if err != nil {
			return fmt.Errorf("first move error: %v", err)
		}
	} else {
		// Normal turns: each player places their own stones
		err = g.Board.DoMove(mvOne, PlayerWhite)
		if err != nil {
			return fmt.Errorf("white move error: %v", err)
		}
		err = g.Board.DoMove(mvTwo, PlayerBlack)
		if err != nil {
			return fmt.Errorf("black move error: %v", err)
		}
	}

	g.Turns = append(g.Turns, &Turn{
		Number: int64(len(g.Turns)) + 1,
		First:  mvOne,
		Second: mvTwo,
	})

	return nil
}

// DoSingleMove executes a single move by a specific player
func (g *Game) DoSingleMove(moveStr string, player int) error {
	mv, err := NewMove(moveStr)
	if err != nil {
		return err
	}

	// Validate it's the correct player's turn
	turnNumber := int64(len(g.Turns))
	
	// First turn special handling
	if turnNumber == 0 {
		// First turn: each player places opponent's stone
		if !mv.isPlace() || mv.Stone != StoneFlat {
			return fmt.Errorf("first move must be flat stone placement")
		}
		
		// Place opponent's color
		opponentPlayer := PlayerWhite
		if player == PlayerWhite {
			opponentPlayer = PlayerBlack
		}
		
		err = g.Board.DoMove(mv, opponentPlayer)
		if err != nil {
			return fmt.Errorf("first move error: %v", err)
		}
	} else {
		// Normal move: place own color
		err = g.Board.DoMove(mv, player)
		if err != nil {
			return fmt.Errorf("move error: %v", err)
		}
	}

	// Update or create turn
	currentTurn, err := g.GetTurn(turnNumber)
	if err != nil {
		return err
	}

	// Determine if this is first or second move of the turn
	expectedPlayer := PlayerWhite
	if turnNumber%2 != 0 {
		expectedPlayer = PlayerBlack
	}

	if player == expectedPlayer {
		if currentTurn.First == nil {
			currentTurn.First = mv
		} else {
			return fmt.Errorf("player %d already moved this turn", player)
		}
	} else {
		if currentTurn.Second == nil {
			currentTurn.Second = mv
		} else {
			return fmt.Errorf("player %d already moved this turn", player)
		}
	}

	g.UpdateTurn(currentTurn)

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
			return turn, fmt.Errorf("line does not have correct number of parts: %+v", fields)
		}

		// TODO: Support branches. Right now we discard things that are not ints.
		numberVal := fields[0]
		numberVal = strings.TrimRight(numberVal, ".")
		if regexp.MustCompile("[^0-9]+").MatchString(numberVal) {
			log.Warnw("not a number, ignoring line", "number", numberVal)
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
