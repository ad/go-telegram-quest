package services

import (
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

func TestCheckAsteriskAchievement_NonAsteriskStep(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	userRepo := db.NewUserRepository(queue)
	achievementRepo := db.NewAchievementRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	user := createTestUserForEngine(t, userRepo, 1)
	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

	step := &models.Step{
		StepOrder:    1,
		Text:         "Regular question",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
		IsAsterisk:   false,
		Answers:      []string{"answer"},
	}
	stepID, err := stepRepo.Create(step)
	if err != nil {
		t.Fatal(err)
	}
	step.ID = stepID

	awarded, err := engine.CheckAsteriskAchievement(user.ID, step.ID)
	if err != nil {
		t.Fatalf("CheckAsteriskAchievement failed: %v", err)
	}

	if len(awarded) != 0 {
		t.Errorf("Expected no achievements for non-asterisk step, got %v", awarded)
	}
}

func TestCheckAsteriskAchievement_AsteriskStep(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	userRepo := db.NewUserRepository(queue)
	achievementRepo := db.NewAchievementRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	user := createTestUserForEngine(t, userRepo, 1)
	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

	step := &models.Step{
		StepOrder:    1,
		Text:         "Asterisk question",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
		IsAsterisk:   true,
		Answers:      []string{"answer"},
	}
	stepID, err := stepRepo.Create(step)
	if err != nil {
		t.Fatal(err)
	}
	step.ID = stepID

	awarded, err := engine.CheckAsteriskAchievement(user.ID, step.ID)
	if err != nil {
		t.Fatalf("CheckAsteriskAchievement failed: %v", err)
	}

	if len(awarded) != 1 {
		t.Fatalf("Expected 1 achievement, got %d", len(awarded))
	}

	if awarded[0] != "asterisk" {
		t.Errorf("Expected 'asterisk' achievement, got '%s'", awarded[0])
	}

	hasAchievement, err := achievementRepo.HasUserAchievement(user.ID, "asterisk")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if !hasAchievement {
		t.Error("User should have asterisk achievement")
	}
}

func TestCheckAsteriskAchievement_OnlyOnce(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	userRepo := db.NewUserRepository(queue)
	achievementRepo := db.NewAchievementRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	user := createTestUserForEngine(t, userRepo, 1)
	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

	step1 := &models.Step{
		StepOrder:    1,
		Text:         "First asterisk question",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
		IsAsterisk:   true,
		Answers:      []string{"answer1"},
	}
	stepID1, err := stepRepo.Create(step1)
	if err != nil {
		t.Fatal(err)
	}
	step1.ID = stepID1

	step2 := &models.Step{
		StepOrder:    2,
		Text:         "Second asterisk question",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
		IsAsterisk:   true,
		Answers:      []string{"answer2"},
	}
	stepID2, err := stepRepo.Create(step2)
	if err != nil {
		t.Fatal(err)
	}
	step2.ID = stepID2

	awarded1, err := engine.CheckAsteriskAchievement(user.ID, step1.ID)
	if err != nil {
		t.Fatalf("First CheckAsteriskAchievement failed: %v", err)
	}

	if len(awarded1) != 1 || awarded1[0] != "asterisk" {
		t.Errorf("Expected 'asterisk' achievement on first call, got %v", awarded1)
	}

	awarded2, err := engine.CheckAsteriskAchievement(user.ID, step2.ID)
	if err != nil {
		t.Fatalf("Second CheckAsteriskAchievement failed: %v", err)
	}

	if len(awarded2) != 0 {
		t.Errorf("Expected no achievement on second call (already awarded), got %v", awarded2)
	}

	userAchievements, err := achievementRepo.GetUserAchievements(user.ID)
	if err != nil {
		t.Fatalf("GetUserAchievements failed: %v", err)
	}

	asteriskCount := 0
	for _, ua := range userAchievements {
		achievement, err := achievementRepo.GetByID(ua.AchievementID)
		if err != nil {
			continue
		}
		if achievement.Key == "asterisk" {
			asteriskCount++
		}
	}

	if asteriskCount != 1 {
		t.Errorf("Expected exactly 1 asterisk achievement, got %d", asteriskCount)
	}
}

func TestCheckAsteriskAchievement_MultipleUsers(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	userRepo := db.NewUserRepository(queue)
	achievementRepo := db.NewAchievementRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	user1 := createTestUserForEngine(t, userRepo, 1)
	user2 := createTestUserForEngine(t, userRepo, 2)
	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

	step := &models.Step{
		StepOrder:    1,
		Text:         "Asterisk question",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
		IsAsterisk:   true,
		Answers:      []string{"answer"},
	}
	stepID, err := stepRepo.Create(step)
	if err != nil {
		t.Fatal(err)
	}
	step.ID = stepID

	awarded1, err := engine.CheckAsteriskAchievement(user1.ID, step.ID)
	if err != nil {
		t.Fatalf("CheckAsteriskAchievement for user1 failed: %v", err)
	}
	if len(awarded1) != 1 || awarded1[0] != "asterisk" {
		t.Errorf("Expected 'asterisk' achievement for user1, got %v", awarded1)
	}

	awarded2, err := engine.CheckAsteriskAchievement(user2.ID, step.ID)
	if err != nil {
		t.Fatalf("CheckAsteriskAchievement for user2 failed: %v", err)
	}
	if len(awarded2) != 1 || awarded2[0] != "asterisk" {
		t.Errorf("Expected 'asterisk' achievement for user2, got %v", awarded2)
	}

	hasAchievement1, _ := achievementRepo.HasUserAchievement(user1.ID, "asterisk")
	hasAchievement2, _ := achievementRepo.HasUserAchievement(user2.ID, "asterisk")

	if !hasAchievement1 {
		t.Error("User1 should have asterisk achievement")
	}
	if !hasAchievement2 {
		t.Error("User2 should have asterisk achievement")
	}
}
