package gotak

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
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
	mvStr := mv.String()

	placeRegex := regexp.MustCompile(`^(C|S|F)?([a-z][0-9]+)$`)
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
			stone.Type = StoneFlat
		}
		b.Squares[location] = append(b.Squares[location], stone)
		return nil
	}

	// (count)(square)(direction)(drop counts)(stone)
	moveRegex := regexp.MustCompile(`^([1-9]*)([a-z][0-9]+)([<>+\-])([0-9]+)(C|S|F)?$`)
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
			stoneType = StoneFlat
		}

		// Get current pieces
		begin := max(0, len(b.Squares[location])-1-int(totalPieces))
		end := max(1, len(b.Squares[location])-1)
		log.Printf("%d %d", begin, end)
		stones := b.Squares[location][begin:end]
		//log.Printf("%s %+v", mv, stones)

		squares := []string{}

		currentSpace := location
		nextSpace := ""
		for i := 0; i < len(drpCounts); i++ {
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

		// pop and shift
		for i, s := range squares {
			//log.Printf("%+v %+v", s, drpCounts[i])
			for j := int64(0); j < drpCounts[i]; j++ {
				log.Printf("%+v %+v", stones, drpCounts[i])
				st := stones[0]
				b.Squares[s] = append(b.Squares[s], st)
				if len(stones) > 1 {
					b.Squares[location] = stones[1:]
				} else {
					b.Squares[location] = []*Stone{}
				}
			}
		}
	}

	return nil
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
