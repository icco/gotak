package main

import (
	"os"
	"testing"
)

func TestGetDB_reusesPool(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("needs DATABASE_URL")
	}
	first, err := getDB()
	if err != nil {
		t.Fatalf("getDB: %v", err)
	}
	second, err := getDB()
	if err != nil {
		t.Fatalf("getDB: %v", err)
	}
	if first != second {
		t.Error("getDB opened a second pool instead of reusing the first")
	}
}

func TestOpenDB_emptyURL(t *testing.T) {
	if _, err := openDB(""); err == nil {
		t.Error("openDB with no URL should fail")
	}
}
