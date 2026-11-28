package main

import (
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGetDB_reusesPool(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("needs DATABASE_URL")
	}

	admin, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	schema := "test_" + strings.ToLower(rand.Text())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Setenv("DATABASE_URL", u.String())

	dbMu.Lock()
	saved := dbConn
	dbConn = nil
	dbMu.Unlock()
	t.Cleanup(func() {
		dbMu.Lock()
		opened := dbConn
		dbConn = saved
		dbMu.Unlock()
		if opened != nil {
			if err := closeDB(opened); err != nil {
				t.Errorf("close pool: %v", err)
			}
		}
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		if err := closeDB(admin); err != nil {
			t.Errorf("close admin: %v", err)
		}
	})

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
