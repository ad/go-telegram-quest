package services

import (
	"testing"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

func setupRetroactiveProcessorTestDB(t testing.TB) (*db.DBQueue, func()) {
	return setupAchievementEngineTestDB(t)
}

func TestRetroactiveProcessor_ProcessAchievementSync_BatchProcessing(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	step := createTestStep(t, stepRepo, 1)

	numUsers := 15
	baseTime := time.Now().Add(-24 * time.Hour)
	for i := 1; i <= numUsers; i++ {
		userID := int64(i * 1000)
		createTestUserForEngine(t, userRepo, userID)

		if i <= 10 {
			completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
			createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &completedAt)
		}
	}

	threshold := 1
	achievement := &models.Achievement{
		Key:         "test_batch_achievement",
		Name:        "Test Batch Achievement",
		Description: "Test description",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: &threshold,
		},
		IsActive: true,
	}
	if err := achievementRepo.Create(achievement); err != nil {
		t.Fatalf("Failed to create achievement: %v", err)
	}

	progress, err := processor.ProcessAchievementSync("test_batch_achievement", 5)
	if err != nil {
		t.Fatalf("ProcessAchievementSync failed: %v", err)
	}

	if progress == nil {
		t.Fatal("Progress should not be nil")
	}

	if progress.TotalUsers != numUsers {
		t.Errorf("Expected TotalUsers=%d, got %d", numUsers, progress.TotalUsers)
	}

	if progress.ProcessedUsers != numUsers {
		t.Errorf("Expected ProcessedUsers=%d, got %d", numUsers, progress.ProcessedUsers)
	}

	if progress.AwardedCount != 10 {
		t.Errorf("Expected AwardedCount=10, got %d", progress.AwardedCount)
	}

	if progress.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount=0, got %d", progress.ErrorCount)
	}

	if progress.IsRunning {
		t.Error("Processing should be complete")
	}

	if progress.EndTime == nil {
		t.Error("EndTime should be set after completion")
	}

	for i := 1; i <= numUsers; i++ {
		userID := int64(i * 1000)
		hasAchievement, err := achievementRepo.HasUserAchievement(userID, "test_batch_achievement")
		if err != nil {
			t.Fatalf("HasUserAchievement failed: %v", err)
		}

		shouldHave := i <= 10
		if shouldHave && !hasAchievement {
			t.Errorf("User %d should have achievement but doesn't", userID)
		}
		if !shouldHave && hasAchievement {
			t.Errorf("User %d should NOT have achievement but does", userID)
		}
	}
}

func TestRetroactiveProcessor_ProcessAchievementAsync(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	step := createTestStep(t, stepRepo, 1)

	numUsers := 5
	baseTime := time.Now().Add(-24 * time.Hour)
	for i := 1; i <= numUsers; i++ {
		userID := int64(i * 1000)
		createTestUserForEngine(t, userRepo, userID)

		completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
		createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &completedAt)
	}

	threshold := 1
	achievement := &models.Achievement{
		Key:         "test_async_achievement",
		Name:        "Test Async Achievement",
		Description: "Test description",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: &threshold,
		},
		IsActive: true,
	}
	if err := achievementRepo.Create(achievement); err != nil {
		t.Fatalf("Failed to create achievement: %v", err)
	}

	err := processor.ProcessAchievementAsync("test_async_achievement", 2)
	if err != nil {
		t.Fatalf("ProcessAchievementAsync failed: %v", err)
	}

	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for async processing to complete")
		case <-ticker.C:
			if !processor.IsProcessing("test_async_achievement") {
				progress := processor.GetProgress("test_async_achievement")
				if progress != nil && progress.ProcessedUsers == numUsers {
					if progress.AwardedCount != numUsers {
						t.Errorf("Expected AwardedCount=%d, got %d", numUsers, progress.AwardedCount)
					}
					return
				}
			}
		}
	}
}

func TestRetroactiveProcessor_ErrorHandling(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	_, err := processor.ProcessAchievementSync("nonexistent_achievement", 10)
	if err != nil {
		t.Fatalf("ProcessAchievementSync should not return error for nonexistent achievement: %v", err)
	}

	progress := processor.GetProgress("nonexistent_achievement")
	if progress == nil {
		t.Fatal("Progress should exist even for failed processing")
	}

	if progress.ErrorCount == 0 {
		t.Error("Expected errors for nonexistent achievement")
	}
}

func TestRetroactiveProcessor_GetProgress(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	progress := processor.GetProgress("unknown_key")
	if progress != nil {
		t.Error("GetProgress should return nil for unknown achievement key")
	}
}

func TestRetroactiveProcessor_CancelProcessing(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	cancelled := processor.CancelProcessing("nonexistent")
	if cancelled {
		t.Error("CancelProcessing should return false for non-running process")
	}
}

func TestRetroactiveProcessor_GetAllProgress(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	step := createTestStep(t, stepRepo, 1)

	userID := int64(1000)
	createTestUserForEngine(t, userRepo, userID)
	baseTime := time.Now().Add(-24 * time.Hour)
	createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &baseTime)

	threshold := 1
	for i := 1; i <= 2; i++ {
		achievement := &models.Achievement{
			Key:         "test_all_progress_" + string(rune('a'+i-1)),
			Name:        "Test Achievement",
			Description: "Test description",
			Category:    models.CategoryProgress,
			Type:        models.TypeProgressBased,
			IsUnique:    false,
			Conditions: models.AchievementConditions{
				CorrectAnswers: &threshold,
			},
			IsActive: true,
		}
		if err := achievementRepo.Create(achievement); err != nil {
			t.Fatalf("Failed to create achievement: %v", err)
		}
	}

	processor.ProcessAchievementSync("test_all_progress_a", 10)
	processor.ProcessAchievementSync("test_all_progress_b", 10)

	allProgress := processor.GetAllProgress()
	if len(allProgress) != 2 {
		t.Errorf("Expected 2 progress entries, got %d", len(allProgress))
	}
}

func TestRetroactiveProcessor_DuplicateProcessingPrevention(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	step := createTestStep(t, stepRepo, 1)

	userID := int64(1000)
	createTestUserForEngine(t, userRepo, userID)
	baseTime := time.Now().Add(-24 * time.Hour)
	createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &baseTime)

	threshold := 1
	achievement := &models.Achievement{
		Key:         "test_duplicate_prevention",
		Name:        "Test Achievement",
		Description: "Test description",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: &threshold,
		},
		IsActive: true,
	}
	if err := achievementRepo.Create(achievement); err != nil {
		t.Fatalf("Failed to create achievement: %v", err)
	}

	progress1, _ := processor.ProcessAchievementSync("test_duplicate_prevention", 10)
	progress2, _ := processor.ProcessAchievementSync("test_duplicate_prevention", 10)

	if progress1.AwardedCount != 1 {
		t.Errorf("First processing should award 1, got %d", progress1.AwardedCount)
	}

	if progress2.AwardedCount != 0 {
		t.Errorf("Second processing should award 0 (already assigned), got %d", progress2.AwardedCount)
	}
}

func TestRetroactiveProcessor_CompletionAchievements(t *testing.T) {
	queue, cleanup := setupRetroactiveProcessorTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
	processor := NewRetroactiveProcessor(engine, achievementRepo, userRepo)

	numSteps := 3
	var steps []*models.Step
	for i := 1; i <= numSteps; i++ {
		step := createTestStep(t, stepRepo, i)
		steps = append(steps, step)
	}

	userID := int64(1000)
	createTestUserForEngine(t, userRepo, userID)

	baseTime := time.Now().Add(-24 * time.Hour)
	for i, step := range steps {
		completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
		createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &completedAt)
		createUserAnswer(t, queue, userID, step.ID, false, completedAt)
	}

	achievement := &models.Achievement{
		Key:         "test_completion_retroactive",
		Name:        "Test Completion Achievement",
		Description: "Test description",
		Category:    models.CategoryCompletion,
		Type:        models.TypeTimeBased,
		IsUnique:    false,
		Conditions:  models.AchievementConditions{},
		IsActive:    true,
	}
	if err := achievementRepo.Create(achievement); err != nil {
		t.Fatalf("Failed to create achievement: %v", err)
	}

	progress, err := processor.ProcessAchievementSync("test_completion_retroactive", 10)
	if err != nil {
		t.Fatalf("ProcessAchievementSync failed: %v", err)
	}

	if progress.AwardedCount != 1 {
		t.Errorf("Expected AwardedCount=1, got %d", progress.AwardedCount)
	}

	hasAchievement, err := achievementRepo.HasUserAchievement(userID, "test_completion_retroactive")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if !hasAchievement {
		t.Error("User should have completion achievement")
	}
}
