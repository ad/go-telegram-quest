package db

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNewSettingsInitialization(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if err := InitSchema(sqlDB); err != nil {
		t.Fatal(err)
	}

	var value string
	err = sqlDB.QueryRow("SELECT value FROM settings WHERE key = 'required_group_chat_id'").Scan(&value)
	if err != nil {
		t.Fatalf("Failed to get required_group_chat_id: %v", err)
	}
	if value != "0" {
		t.Errorf("Expected required_group_chat_id to be '0', got '%s'", value)
	}

	err = sqlDB.QueryRow("SELECT value FROM settings WHERE key = 'group_chat_invite_link'").Scan(&value)
	if err != nil {
		t.Fatalf("Failed to get group_chat_invite_link: %v", err)
	}
	if value != "" {
		t.Errorf("Expected group_chat_invite_link to be empty, got '%s'", value)
	}
}
