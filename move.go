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
var placeRegex = regexp.MustCompile(`^([CSF])?([a-z]\d+)$`)

// (count)(square)(direction)(drop counts)(stone)
var moveRegex = regexp.MustCompile(`^([1-9]*)([a-z]\d+)([<>+\-])(\d*)([CSF])?$`)

// IsValidMoveCharacter checks if a character is valid for PTN move input
func IsValidMoveCharacter(char string) bool {
	if len(char) != 1 {
		return false
	}
	
	// Check for alphanumeric characters
	if char >= "a" && char <= "z" {
		return true
	}
	if char >= "A" && char <= "Z" {
		return true
	}
	if char >= "0" && char <= "9" {
		return true
	}
	
	// Check for PTN-specific characters: direction markers, stone types
	return strings.Contains("<>+-SsCcFf", char)
}

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
	// Strip quote marks, question marks, and exclamation marks from moves (PTN annotations)
	mv = strings.Trim(mv, "\"'?!")
	m := &Move{Text: mv}
	err := m.Parse()
	return m, err
}

// Parse takes the Text of a move and fills the rest of the attributes of the
// Move object. It will overright past parses or data stored in the move.
func (m *Move) Parse() error {
	if m.Text == "" {
		return fmt.Errorf("move cannot be empty")
	}

	if m.isPlace() {
		return m.parsePlace()
	}

	if m.isMove() {
		return m.parseMove()
	}

	return fmt.Errorf("invalid move format: %s", m.Text)
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
	if countStr == "" {
		countStr = "1"
	}
	totalPieces, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return err
	}
	m.MoveCount = totalPieces

	m.Square = parts[2]
	m.MoveDirection = parts[3]
	m.MoveDropCounts = []int64{}

	drpCountStr := parts[4]
	if drpCountStr == "" {
		drpCountStr = countStr
	}
	var totalDropped int64
	for _, str := range strings.Split(drpCountStr, "") {
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
		return fmt.Errorf("did not drop same pieces picked up: %d != %d", totalDropped, m.MoveCount)
	}

	m.Stone = parts[5]
	if m.Stone == "" {
		m.Stone = StoneFlat
	}

	return nil
}
