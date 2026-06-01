package gotak

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTPS parses a Tak Position System string and returns the resulting
// board state, the player to move next, and the (1-indexed) move number.
//
// The TPS format is `<position> <player> <move>`, where position is a
// slash-separated list of rows from top (highest rank) to bottom (rank 1).
// Each row is a comma-separated list of squares. An empty square is `x`,
// optionally followed by a count (`x3` = three empty squares in a row).
// A stack is encoded as a sequence of `1` (white) / `2` (black) digits,
// bottom stone first, optionally suffixed with a stone type for the top
// stone (`S` for standing, `C` for capstone; default is flat).
//
// Example: `x5/x5/x5/x5/1,1,1,1,1 1 5` — a 5x5 board with white flats on
// the bottom row, white to move, on move 5.
func ParseTPS(tps string) (*Board, int, int64, error) {
	fields := strings.Fields(strings.TrimSpace(tps))
	if len(fields) != 3 {
		return nil, 0, 0, fmt.Errorf("tps must have 3 space-separated fields, got %d", len(fields))
	}

	rows := strings.Split(fields[0], "/")
	size := int64(len(rows))
	if size < 4 || size > 9 {
		return nil, 0, 0, fmt.Errorf("tps row count %d is out of supported range [4,9]", size)
	}

	player, err := strconv.Atoi(fields[1])
	if err != nil || (player != PlayerWhite && player != PlayerBlack) {
		return nil, 0, 0, fmt.Errorf("tps player field must be 1 or 2, got %q", fields[1])
	}

	move, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || move < 1 {
		return nil, 0, 0, fmt.Errorf("tps move number must be a positive integer, got %q", fields[2])
	}

	b := &Board{Size: size}
	if err := b.Init(); err != nil {
		return nil, 0, 0, err
	}

	// Rows are written top-to-bottom in TPS, but the board is indexed
	// bottom-to-top, so the first row in the string is rank `size`.
	for i, row := range rows {
		rank := size - int64(i)
		col := int64(0)

		cells := strings.Split(row, ",")
		for _, cell := range cells {
			cell = strings.TrimSpace(cell)
			if cell == "" {
				return nil, 0, 0, fmt.Errorf("empty cell in tps row %q", row)
			}

			if cell[0] == 'x' {
				count := int64(1)
				if len(cell) > 1 {
					n, err := strconv.ParseInt(cell[1:], 10, 64)
					if err != nil || n < 1 {
						return nil, 0, 0, fmt.Errorf("invalid empty-square count %q", cell)
					}
					count = n
				}
				col += count
				continue
			}

			stack, err := parseTPSStack(cell)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("row %q: %w", row, err)
			}

			if col >= size {
				return nil, 0, 0, fmt.Errorf("row %q has more squares than board size %d", row, size)
			}
			square := fmt.Sprintf("%c%d", 'a'+col, rank)
			b.Squares[square] = stack
			col++
		}

		if col != size {
			return nil, 0, 0, fmt.Errorf("row %q describes %d squares, expected %d", row, col, size)
		}
	}

	return b, player, move, nil
}

func parseTPSStack(cell string) ([]*Stone, error) {
	topType := StoneFlat
	body := cell
	switch last := body[len(body)-1]; last {
	case 'S':
		topType = StoneStanding
		body = body[:len(body)-1]
	case 'C':
		topType = StoneCap
		body = body[:len(body)-1]
	}

	if body == "" {
		return nil, fmt.Errorf("stack %q has a type marker but no stones", cell)
	}

	stack := make([]*Stone, 0, len(body))
	for i := range len(body) {
		var p int
		switch body[i] {
		case '1':
			p = PlayerWhite
		case '2':
			p = PlayerBlack
		default:
			return nil, fmt.Errorf("stack %q contains non-stone character %q", cell, string(body[i]))
		}
		stoneType := StoneFlat
		if i == len(body)-1 {
			stoneType = topType
		}
		stack = append(stack, &Stone{Type: stoneType, Player: p})
	}

	return stack, nil
}
