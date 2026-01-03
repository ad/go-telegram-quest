package services

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
)

func setupTestDBForQuestStats(t *testing.T) (*db.DBQueue, func()) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	queue := db.NewDBQueue(sqlDB)

	cleanup := func() {
		queue.Close()
		sqlDB.Close()
	}

	return queue, cleanup
}

func TestGetQuestStatistics(t *testing.T) {
	queue, cleanup := setupTestDBForQuestStats(t)
	defer cleanup()

	userRepo := db.NewUserRepository(queue)
	stepRepo := db.NewStepRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	answerRepo := db.NewAnswerRepository(queue)
	chatStateRepo := db.NewChatStateRepository(queue)
	statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
	userManager := NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, statsService)

	// Create test steps
	step1ID, _ := stepRepo.Create(&models.Step{
		StepOrder:    1,
		Text:         "Step 1",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
	})

	step2ID, _ := stepRepo.Create(&models.Step{
		StepOrder:    2,
		Text:         "Step 2",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
	})

	step3ID, _ := stepRepo.Create(&models.Step{
		StepOrder:    3,
		Text:         "Step 3",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
	})

	// Create test users
	user1 := &models.User{ID: 1, FirstName: "User", LastName: "One", Username: "user1"}
	userRepo.CreateOrUpdate(user1)

	user2 := &models.User{ID: 2, FirstName: "User", LastName: "Two", Username: "user2"}
	userRepo.CreateOrUpdate(user2)

	user3 := &models.User{ID: 3, FirstName: "User", LastName: "Three", Username: "user3"}
	userRepo.CreateOrUpdate(user3)

	user4 := &models.User{ID: 4, FirstName: "User", LastName: "Four", Username: "user4"}
	userRepo.CreateOrUpdate(user4)

	// User 1: Completed all steps
	progressRepo.Create(&models.UserProgress{UserID: user1.ID, StepID: step1ID, Status: models.StatusApproved})
	progressRepo.Create(&models.UserProgress{UserID: user1.ID, StepID: step2ID, Status: models.StatusApproved})
	progressRepo.Create(&models.UserProgress{UserID: user1.ID, StepID: step3ID, Status: models.StatusApproved})

	// User 2: On step 2 (approved step 1, pending step 2)
	progressRepo.Create(&models.UserProgress{UserID: user2.ID, StepID: step1ID, Status: models.StatusApproved})
	progressRepo.Create(&models.UserProgress{UserID: user2.ID, StepID: step2ID, Status: models.StatusPending})

	// User 3: On step 1 (pending step 1)
	progressRepo.Create(&models.UserProgress{UserID: user3.ID, StepID: step1ID, Status: models.StatusPending})

	// User 4: Not started (no progress records)

	// Get quest statistics
	stats, err := userManager.GetQuestStatistics()
	if err != nil {
		t.Fatalf("GetQuestStatistics failed: %v", err)
	}

	// Verify total users
	if stats.TotalUsers != 4 {
		t.Errorf("Expected TotalUsers=4, got %d", stats.TotalUsers)
	}

	// Verify completed users
	if stats.CompletedUsers != 1 {
		t.Errorf("Expected CompletedUsers=1, got %d", stats.CompletedUsers)
	}

	// Verify in-progress users (User 2, User 3, User 4 are all in progress or not started)
	// Actually, with my logic:
	// User 2: currentStepOrder = 2
	// User 3: currentStepOrder = 1
	// User 4: currentStepOrder = 1
	if stats.InProgressUsers != 3 {
		t.Errorf("Expected InProgressUsers=3, got %d", stats.InProgressUsers)
	}

	// Verify step distribution
	if stats.StepDistribution[1] != 2 { // User 3 and User 4
		t.Errorf("Expected 2 users on step 1, got %d", stats.StepDistribution[1])
	}

	if stats.StepDistribution[2] != 1 { // User 2
		t.Errorf("Expected 1 user on step 2, got %d", stats.StepDistribution[2])
	}
}
