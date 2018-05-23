package gotak

import (
	"fmt"
	"log"
	"strconv"
)

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
	log.Printf("Do Move %+v: %+v", mv.Square, b.Squares[mv.Square])
	if mv.isPlace() {
		stone := &Stone{
			Player: player,
			Type:   mv.Stone,
		}
		b.Squares[mv.Square] = append(b.Squares[mv.Square], stone)

		return nil
	}

	if mv.isMove() {
		begin := len(b.Squares[mv.Square]) - int(mv.MoveCount)
		log.Printf(" | %d", begin)
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
				log.Printf("pop[%s](%d < %d) : %+v", s, j, mv.MoveDropCounts[i], stones)

				st := stones[0]
				b.Squares[s] = append(b.Squares[s], st)
				stones = stones[1:]
			}
		}
	}

	return nil
}
