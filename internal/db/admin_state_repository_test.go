package db

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
)

func setupAdminStateTestDB(t *testing.T) (*sql.DB, *AdminStateRepository) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}

	queue := NewDBQueue(db)
	adminStateRepo := NewAdminStateRepository(queue)

	return db, adminStateRepo
}

func TestAdminStateRepository_SaveAndGetTargetUserID(t *testing.T) {
	db, repo := setupAdminStateTestDB(t)
	defer db.Close()

	adminUserID := int64(123)
	targetUserID := int64(456)

	state := &models.AdminState{
		UserID:       adminUserID,
		CurrentState: "admin_send_message",
		TargetUserID: targetUserID,
	}

	err := repo.Save(state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	retrievedState, err := repo.Get(adminUserID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrievedState.TargetUserID != targetUserID {
		t.Errorf("Expected TargetUserID %d, got %d", targetUserID, retrievedState.TargetUserID)
	}

	if retrievedState.CurrentState != "admin_send_message" {
		t.Errorf("Expected CurrentState 'admin_send_message', got '%s'", retrievedState.CurrentState)
	}
}

func TestAdminStateRepository_UpdateTargetUserID(t *testing.T) {
	db, repo := setupAdminStateTestDB(t)
	defer db.Close()

	adminUserID := int64(123)
	initialTargetUserID := int64(456)
	updatedTargetUserID := int64(789)

	initialState := &models.AdminState{
		UserID:       adminUserID,
		CurrentState: "admin_send_message",
		TargetUserID: initialTargetUserID,
	}

	err := repo.Save(initialState)
	if err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	updatedState := &models.AdminState{
		UserID:       adminUserID,
		CurrentState: "admin_send_message",
		TargetUserID: updatedTargetUserID,
	}

	err = repo.Save(updatedState)
	if err != nil {
		t.Fatalf("Update save failed: %v", err)
	}

	retrievedState, err := repo.Get(adminUserID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrievedState.TargetUserID != updatedTargetUserID {
		t.Errorf("Expected updated TargetUserID %d, got %d", updatedTargetUserID, retrievedState.TargetUserID)
	}
}

func TestAdminStateRepository_DefaultTargetUserID(t *testing.T) {
	db, repo := setupAdminStateTestDB(t)
	defer db.Close()

	adminUserID := int64(123)

	state := &models.AdminState{
		UserID:       adminUserID,
		CurrentState: "some_other_state",
	}

	err := repo.Save(state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	retrievedState, err := repo.Get(adminUserID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrievedState.TargetUserID != 0 {
		t.Errorf("Expected default TargetUserID 0, got %d", retrievedState.TargetUserID)
	}
}

func TestAdminStateRepository_ClearPreservesTargetUserID(t *testing.T) {
	db, repo := setupAdminStateTestDB(t)
	defer db.Close()

	adminUserID := int64(123)
	targetUserID := int64(456)

	state := &models.AdminState{
		UserID:       adminUserID,
		CurrentState: "admin_send_message",
		TargetUserID: targetUserID,
	}

	err := repo.Save(state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	err = repo.Clear(adminUserID)
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	_, err = repo.Get(adminUserID)
	if err == nil {
		t.Error("Expected error when getting cleared state")
	}
}

func TestAdminStateRepository_GetNonExistentState(t *testing.T) {
	db, repo := setupAdminStateTestDB(t)
	defer db.Close()

	_, err := repo.Get(999999)
	if err == nil {
		t.Error("Expected error for non-existent admin state")
	}
}
