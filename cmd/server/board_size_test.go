package main

import (
	"errors"
	"testing"
)

func TestValidateBoardSize(t *testing.T) {
	cases := []struct {
		size int
		mode string
		ok   bool
	}{
		{3, "human", false},
		{4, "human", true},
		{9, "human", true},
		{10, "human", false},
		{4, "ai", true},
		{8, "ai", true},
		{9, "ai", false},
		{0, "ai", false},
	}
	for _, c := range cases {
		err := validateBoardSize(c.size, c.mode)
		if (err == nil) != c.ok {
			t.Errorf("validateBoardSize(%d, %q) = %v, want ok=%v", c.size, c.mode, err, c.ok)
		}
		if err != nil && !errors.Is(err, errInvalidBoardSize) {
			t.Errorf("validateBoardSize(%d, %q) error %v does not wrap errInvalidBoardSize", c.size, c.mode, err)
		}
	}
}

func TestCreateGame_rejectsUnsupportedSizes(t *testing.T) {
	db := setupTestDB(t)
	user := createTestUser(t, db)
	for _, c := range []struct {
		size int
		mode string
	}{{3, "human"}, {10, "human"}, {9, "ai"}} {
		if _, err := createGame(db, c.size, user.ID, c.mode); !errors.Is(err, errInvalidBoardSize) {
			t.Errorf("createGame(%d, %q) err = %v, want errInvalidBoardSize", c.size, c.mode, err)
		}
	}
	var count int64
	if err := db.Model(&Game{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("rejected sizes left %d games behind", count)
	}
}
