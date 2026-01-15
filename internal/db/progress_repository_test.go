package db

import (
	"testing"

	"github.com/ad/go-telegram-quest/internal/models"
)

func TestCreateSkipped_WithExistingProgress(t *testing.T) {
	db, stepRepo := setupTestDB(t)
	defer db.Close()

	queue := NewDBQueue(db)
	progressRepo := NewProgressRepository(queue)

	step := &models.Step{
		StepOrder:    1,
		Text:         "Test step",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
		IsAsterisk:   true,
	}
	_, err := stepRepo.Create(step)
	if err != nil {
		t.Fatalf("Failed to create step: %v", err)
	}

	userID := int64(12345)

	err = progressRepo.Create(&models.UserProgress{
		UserID: userID,
		StepID: step.ID,
		Status: models.StatusPending,
	})
	if err != nil {
		t.Fatalf("Failed to create initial progress: %v", err)
	}

	err = progressRepo.CreateSkipped(userID, step.ID)
	if err != nil {
		t.Fatalf("CreateSkipped failed with existing progress: %v", err)
	}

	progress, err := progressRepo.GetByUserAndStep(userID, step.ID)
	if err != nil {
		t.Fatalf("Failed to get progress: %v", err)
	}

	if progress.Status != models.StatusSkipped {
		t.Errorf("Expected status %s, got %s", models.StatusSkipped, progress.Status)
	}

	if progress.CompletedAt == nil {
		t.Error("Expected completed_at to be set")
	}
}

func TestCreateSkipped_WithoutExistingProgress(t *testing.T) {
	db, stepRepo := setupTestDB(t)
	defer db.Close()

	queue := NewDBQueue(db)
	progressRepo := NewProgressRepository(queue)

	step := &models.Step{
		StepOrder:    1,
		Text:         "Test step",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
		IsAsterisk:   true,
	}
	_, err := stepRepo.Create(step)
	if err != nil {
		t.Fatalf("Failed to create step: %v", err)
	}

	userID := int64(12346)

	err = progressRepo.CreateSkipped(userID, step.ID)
	if err != nil {
		t.Fatalf("CreateSkipped failed: %v", err)
	}

	progress, err := progressRepo.GetByUserAndStep(userID, step.ID)
	if err != nil {
		t.Fatalf("Failed to get progress: %v", err)
	}

	if progress.Status != models.StatusSkipped {
		t.Errorf("Expected status %s, got %s", models.StatusSkipped, progress.Status)
	}

	if progress.CompletedAt == nil {
		t.Error("Expected completed_at to be set")
	}
}
