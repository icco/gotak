package gotak

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Move is a single move in Tak.
type Move struct {
	// Both Drop and Move
	Stone  string
	Square string

	// Move only
	MoveCount      int64
	MoveDirection  string
	MoveDropCounts []int64

	Text string
}

// (stone)(square)
var placeRegex = regexp.MustCompile(`^(C|S|F)?([a-z][0-9]+)$`)

// (count)(square)(direction)(drop counts)(stone)
var moveRegex = regexp.MustCompile(`^([1-9]*)([a-z][0-9]+)([<>+\-])([0-9]+)(C|S|F)?$`)

// Directions
const (
	MoveUp    = "+"
	MoveDown  = "-"
	MoveLeft  = "<"
	MoveRight = ">"
)

// NewMove takes in a move string and returns a move object that has been
// parsed.
func NewMove(mv string) (*Move, error) {
	m := &Move{Text: mv}
	err := m.Parse()
	return m, err
}

// Parse takes the Text of a move and fills the rest of the attributes of the
// Move object. It will overright past parses or data stored in the move.
func (m *Move) Parse() error {
	if m.isPlace() {
		return m.parsePlace()
	}

	if m.isMove() {
		return m.parseMove()
	}

	return nil
}

func (m *Move) isPlace() bool {
	return placeRegex.MatchString(m.Text)
}

func (m *Move) isMove() bool {
	return moveRegex.MatchString(m.Text)
}

func (m *Move) parsePlace() error {
	parts := placeRegex.FindStringSubmatch(m.Text)
	location := ""
	if len(parts) == 2 {
		location = parts[1]
	}

	if len(parts) == 3 {
		location = parts[2]
		m.Stone = parts[1]
	}

	if m.Stone == "" {
		m.Stone = StoneFlat
	}

	m.Square = location

	return nil
}

func (m *Move) parseMove() error {
	parts := moveRegex.FindStringSubmatch(m.Text)

	countStr := parts[1]
	totalPieces, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return err
	}
	m.MoveCount = totalPieces

	m.Square = parts[2]
	m.MoveDirection = parts[3]
	m.MoveDropCounts = []int64{}

	var totalDropped int64
	for _, str := range strings.Split(parts[4], "") {
		drpCount, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return err
		}
		totalDropped += drpCount
		if totalDropped > m.MoveCount {
			return fmt.Errorf("tried to drop more pieces than available: %d > %d", totalDropped, totalPieces)
		}
		m.MoveDropCounts = append(m.MoveDropCounts, drpCount)
	}

	if totalDropped != m.MoveCount {
		return fmt.Errorf("Did not drop same pieces picked up: %d != %d", totalDropped, totalPieces)
	}

	m.Stone = parts[5]
	if m.Stone == "" {
		m.Stone = StoneFlat
	}

	return nil
}

func (m *Move) String() string {
	return m.Text
}
