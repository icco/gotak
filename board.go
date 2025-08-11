package gotak

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Board is a current state of a game of Tak.
type Board struct {
	Size    int64
	Squares map[string][]*Stone
}

// SquareFunc is a function that takes a string and a stone, does something,
// and returns an errorif there is an issue.
type SquareFunc func(string, []*Stone) error

// IterateOverSquares takes a SquareFunc, and applies it to every square in a
// board. If the SquareFunc returns an error, the iteration stops and the
// function returns.
func (b *Board) IterateOverSquares(f SquareFunc) error {
	letters := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	for x := int64(0); x < b.Size; x++ {
		for y := int64(1); y <= b.Size; y++ {
			location := letters[x] + strconv.FormatInt(y, 10)
			err := f(location, b.Squares[location])
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Init creates a board once a board size is set.
func (b *Board) Init() error {
	if b.Size < 4 || b.Size >= 10 {
		return fmt.Errorf("%v is not a valid board size", b.Size)
	}

	b.Squares = map[string][]*Stone{}
	b.IterateOverSquares(func(l string, s []*Stone) error {
		b.Squares[l] = []*Stone{}
		return nil
	})

	return nil
}

func (b *Board) String() string {
	return fmt.Sprintf("%+v", b.Squares)
}

// TopStone returns the top stone for a square.
func (b *Board) TopStone(square string) *Stone {
	if len(b.Squares[square]) > 0 {
		return b.Squares[square][len(b.Squares[square])-1]
	}

	return nil
}

// Color returns the player integer responding to the top most stone of the
// square's stack.
func (b *Board) Color(square string) int {
	stn := b.TopStone(square)

	if stn != nil {
		return stn.Player
	}

	return PlayerNone
}

// Translate takes a square and a direction, and returns the square identifier
// as if you had moved in that direction.
func Translate(square, direction string) string {
	parts := strings.Split(square, "")
	vertical := parts[1]
	horizantal := parts[0]

	switch direction {
	case "n", MoveUp:
		vertical = string([]byte(vertical)[0] + 1)
	case "e", MoveRight:
		horizantal = string([]byte(horizantal)[0] + 1)
	case "s", MoveDown:
		vertical = string([]byte(vertical)[0] - 1)
	case "w", MoveLeft:
		horizantal = string([]byte(horizantal)[0] - 1)
	}

	return strings.Join([]string{horizantal, vertical}, "")
}

// IsEdge determines if the passed in space is a board edge.
func (b *Board) IsEdge(l string) bool {
	parts := strings.Split(l, "")

	horizantal := parts[0]
	if horizantal == "a" {
		return true
	}

	if horizantal == string([]byte("a")[0]+byte(b.Size)-1) {
		return true
	}

	vertical, _ := strconv.ParseInt(parts[1], 10, 64)
	if vertical == 1 {
		return true
	}

	if vertical == b.Size {
		return true
	}

	return false
}

// FindRoad starts at square l and uses a flood fill algorithm to find a road.
// Returns true if a road exists from the starting square to any of the valid end edges.
func (b *Board) FindRoad(startSquare string, validEndEdges []string) bool {
	if b.Color(startSquare) == PlayerNone {
		return false
	}

	playerColor := b.Color(startSquare)
	visited := make(map[string]bool)
	queue := []string{startSquare}
	visited[startSquare] = true

	// Convert validEndEdges to map for faster lookup
	endEdgeMap := make(map[string]bool)
	for _, edge := range validEndEdges {
		endEdgeMap[edge] = true
	}

	// BFS to find connected road squares
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Check if we reached a valid end edge
		if endEdgeMap[current] {
			return true
		}

		// Check all four directions
		directions := []string{MoveUp, MoveDown, MoveLeft, MoveRight}
		for _, dir := range directions {
			next := Translate(current, dir)
			
			// Skip if out of bounds or already visited
			if !b.isValidSquare(next) || visited[next] {
				continue
			}

			// Skip if different color or standing stone (can't be part of road)
			if b.Color(next) != playerColor {
				continue
			}

			topStone := b.TopStone(next)
			if topStone == nil || topStone.Type == StoneStanding {
				continue
			}

			visited[next] = true
			queue = append(queue, next)
		}
	}

	return false
}

// isValidSquare checks if a square identifier is valid for this board
func (b *Board) isValidSquare(square string) bool {
	if len(square) < 2 {
		return false
	}
	
	col := square[0]
	if col < 'a' || col >= byte('a')+byte(b.Size) {
		return false
	}
	
	rowStr := square[1:]
	row, err := strconv.ParseInt(rowStr, 10, 64)
	if err != nil || row < 1 || row > b.Size {
		return false
	}
	
	return true
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
	if mv.isPlace() {
		stone := &Stone{
			Player: player,
			Type:   mv.Stone,
		}
		b.Squares[mv.Square] = append(b.Squares[mv.Square], stone)

		return nil
	}

	if mv.isMove() {
		begin := int64(len(b.Squares[mv.Square])) - mv.MoveCount
		stones := b.Squares[mv.Square][begin:]
		b.Squares[mv.Square] = b.Squares[mv.Square][:begin]

		squares := []string{}

		currentSpace := mv.Square
		nextSpace := ""
		for i := 0; i < len(mv.MoveDropCounts); i++ {
			switch mv.MoveDirection {
			case MoveLeft:
				nextSpace = string(currentSpace[0]-1) + string(currentSpace[1])
			case MoveRight:
				nextSpace = string(currentSpace[0]+1) + string(currentSpace[1])
			case MoveUp:
				nextSpace = string(currentSpace[0]) + string(currentSpace[1]+1)
			case MoveDown:
				nextSpace = string(currentSpace[0]) + string(currentSpace[1]-1)
			}

			currentSpace = nextSpace
			squares = append(squares, nextSpace)
		}

		// pop and shift
		for i, s := range squares {
			for j := int64(0); j < mv.MoveDropCounts[i]; j++ {
				st := stones[0]
				b.Squares[s] = append(b.Squares[s], st)
				stones = stones[1:]
			}
		}
	}

	return nil
}
