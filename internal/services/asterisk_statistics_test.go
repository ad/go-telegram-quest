package services

import (
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

func TestGetAsteriskStepsStats_OnlyCountsCompletedProgress(t *testing.T) {
	queue, cleanup := setupTestDB(t)
	defer cleanup()

	stepRepo := db.NewStepRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	userRepo := db.NewUserRepository(queue)
	statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)

	asteriskStep := &models.Step{
		StepOrder:  1,
		Text:       "Asterisk Step",
		AnswerType: models.AnswerTypeText,
		IsActive:   true,
		IsAsterisk: true,
	}
	stepID, err := stepRepo.Create(asteriskStep)
	if err != nil {
		t.Fatalf("Failed to create asterisk step: %v", err)
	}

	user1 := int64(1001)
	user2 := int64(1002)
	user3 := int64(1003)
	user4 := int64(1004)

	if err := progressRepo.Create(&models.UserProgress{
		UserID: user1,
		StepID: stepID,
		Status: models.StatusApproved,
	}); err != nil {
		t.Fatalf("Failed to create approved progress: %v", err)
	}

	if err := progressRepo.CreateSkipped(user2, stepID); err != nil {
		t.Fatalf("Failed to create skipped progress: %v", err)
	}

	if err := progressRepo.Create(&models.UserProgress{
		UserID: user3,
		StepID: stepID,
		Status: models.StatusPending,
	}); err != nil {
		t.Fatalf("Failed to create pending progress: %v", err)
	}

	if err := progressRepo.Create(&models.UserProgress{
		UserID: user4,
		StepID: stepID,
		Status: models.StatusWaitingReview,
	}); err != nil {
		t.Fatalf("Failed to create waiting_review progress: %v", err)
	}

	stats, err := statsService.GetAsteriskStepsStats()
	if err != nil {
		t.Fatalf("Failed to get asterisk stats: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("Expected 1 asterisk step stat, got %d", len(stats))
	}

	stat := stats[0]

	if stat.AnsweredCount != 1 {
		t.Errorf("Expected AnsweredCount to be 1 (only approved), got %d", stat.AnsweredCount)
	}

	if stat.SkippedCount != 1 {
		t.Errorf("Expected SkippedCount to be 1, got %d", stat.SkippedCount)
	}
}

func TestGetAsteriskStepsStats_NoProgressRecords(t *testing.T) {
	queue, cleanup := setupTestDB(t)
	defer cleanup()

	stepRepo := db.NewStepRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	userRepo := db.NewUserRepository(queue)
	statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)

	asteriskStep := &models.Step{
		StepOrder:  1,
		Text:       "Asterisk Step",
		AnswerType: models.AnswerTypeText,
		IsActive:   true,
		IsAsterisk: true,
	}
	_, err := stepRepo.Create(asteriskStep)
	if err != nil {
		t.Fatalf("Failed to create asterisk step: %v", err)
	}

	stats, err := statsService.GetAsteriskStepsStats()
	if err != nil {
		t.Fatalf("Failed to get asterisk stats: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("Expected 1 asterisk step stat, got %d", len(stats))
	}

	stat := stats[0]

	if stat.AnsweredCount != 0 {
		t.Errorf("Expected AnsweredCount to be 0, got %d", stat.AnsweredCount)
	}

	if stat.SkippedCount != 0 {
		t.Errorf("Expected SkippedCount to be 0, got %d", stat.SkippedCount)
	}
}

func TestGetAsteriskStepsStats_MultipleUsers(t *testing.T) {
	queue, cleanup := setupTestDB(t)
	defer cleanup()

	stepRepo := db.NewStepRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	userRepo := db.NewUserRepository(queue)
	statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)

	asteriskStep := &models.Step{
		StepOrder:  1,
		Text:       "Asterisk Step",
		AnswerType: models.AnswerTypeText,
		IsActive:   true,
		IsAsterisk: true,
	}
	stepID, err := stepRepo.Create(asteriskStep)
	if err != nil {
		t.Fatalf("Failed to create asterisk step: %v", err)
	}

	if err := progressRepo.Create(&models.UserProgress{
		UserID: 1,
		StepID: stepID,
		Status: models.StatusApproved,
	}); err != nil {
		t.Fatalf("Failed to create approved progress for user 1: %v", err)
	}

	if err := progressRepo.Create(&models.UserProgress{
		UserID: 2,
		StepID: stepID,
		Status: models.StatusPending,
	}); err != nil {
		t.Fatalf("Failed to create pending progress for user 2: %v", err)
	}

	if err := progressRepo.Create(&models.UserProgress{
		UserID: 3,
		StepID: stepID,
		Status: models.StatusPending,
	}); err != nil {
		t.Fatalf("Failed to create pending progress for user 3: %v", err)
	}

	stats, err := statsService.GetAsteriskStepsStats()
	if err != nil {
		t.Fatalf("Failed to get asterisk stats: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("Expected 1 asterisk step stat, got %d", len(stats))
	}

	stat := stats[0]

	if stat.AnsweredCount != 1 {
		t.Errorf("Expected AnsweredCount to be 1 (only user 1 approved), got %d. Pending users should not be counted.", stat.AnsweredCount)
	}

	if stat.SkippedCount != 0 {
		t.Errorf("Expected SkippedCount to be 0, got %d", stat.SkippedCount)
	}
}
