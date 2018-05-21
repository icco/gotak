package gotak

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
