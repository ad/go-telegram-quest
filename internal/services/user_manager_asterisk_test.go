package services

import (
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

func TestUserManager_AsteriskStepLogic(t *testing.T) {
	queue, cleanup := setupTestDBForUserStats(t)
	defer cleanup()

	stepRepo := db.NewStepRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	userRepo := db.NewUserRepository(queue)
	answerRepo := db.NewAnswerRepository(queue)
	statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)

	// Создаем пользователя
	user := &models.User{
		ID:        1000,
		FirstName: "Test",
		LastName:  "User",
	}
	err := userRepo.CreateOrUpdate(user)
	if err != nil {
		t.Fatal("Failed to create user:", err)
	}

	// Создаем шаги: обычные шаги 1-2, шаг 3 со звездочкой, обычные шаги 4-5
	steps := []*models.Step{
		{StepOrder: 1, Text: "Шаг 1", IsActive: true, IsAsterisk: false, AnswerType: models.AnswerTypeText},
		{StepOrder: 2, Text: "Шаг 2", IsActive: true, IsAsterisk: false, AnswerType: models.AnswerTypeText},
		{StepOrder: 3, Text: "Шаг 3 со звездочкой", IsActive: true, IsAsterisk: true, AnswerType: models.AnswerTypeText},
		{StepOrder: 4, Text: "Шаг 4", IsActive: true, IsAsterisk: false, AnswerType: models.AnswerTypeText},
		{StepOrder: 5, Text: "Шаг 5", IsActive: true, IsAsterisk: false, AnswerType: models.AnswerTypeText},
	}

	for _, step := range steps {
		stepID, err := stepRepo.Create(step)
		if err != nil {
			t.Fatal("Failed to create step:", err)
		}
		step.ID = stepID
	}

	userManager := NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, nil, nil, statsService, nil)

	// Тест 1: Пользователь прошел шаги 1-2, пропустил шаг 3 (со звездочкой), прошел шаги 4-5
	// Должен считаться завершившим квест
	progressEntries := []struct {
		stepID int64
		status models.ProgressStatus
	}{
		{steps[0].ID, models.StatusApproved}, // Шаг 1
		{steps[1].ID, models.StatusApproved}, // Шаг 2
		{steps[2].ID, models.StatusSkipped},  // Шаг 3 (пропущен)
		{steps[3].ID, models.StatusApproved}, // Шаг 4
		{steps[4].ID, models.StatusApproved}, // Шаг 5
	}

	for _, entry := range progressEntries {
		progress := &models.UserProgress{
			UserID: user.ID,
			StepID: entry.stepID,
			Status: entry.status,
		}
		err := progressRepo.Create(progress)
		if err != nil {
			t.Fatal("Failed to save progress:", err)
		}
	}

	// Проверяем, что пользователь завершил квест
	details, err := userManager.GetUserDetails(user.ID)
	if err != nil {
		t.Fatal("Failed to get user details:", err)
	}

	if !details.IsCompleted {
		t.Errorf("Expected user to have completed quest, but IsCompleted = %v", details.IsCompleted)
	}

	if details.CurrentStep != nil {
		t.Errorf("Expected CurrentStep to be nil for completed quest, but got step %d", details.CurrentStep.StepOrder)
	}

	// Проверяем статистику квеста
	questStats, err := userManager.GetQuestStatistics()
	if err != nil {
		t.Fatal("Failed to get quest statistics:", err)
	}

	if questStats.CompletedUsers != 1 {
		t.Errorf("Expected 1 completed user, got %d", questStats.CompletedUsers)
	}

	if questStats.InProgressUsers != 0 {
		t.Errorf("Expected 0 in-progress users, got %d", questStats.InProgressUsers)
	}

	// Тест 2: Создаем второго пользователя, который прошел только шаги 1-2 и застрял на обычном шаге 4
	user2 := &models.User{
		ID:        2000,
		FirstName: "Test2",
		LastName:  "User2",
	}
	err = userRepo.CreateOrUpdate(user2)
	if err != nil {
		t.Fatal("Failed to create user2:", err)
	}

	// Пользователь 2 прошел шаги 1-2, пропустил шаг 3, но не прошел шаг 4
	progressEntries2 := []struct {
		stepID int64
		status models.ProgressStatus
	}{
		{steps[0].ID, models.StatusApproved}, // Шаг 1
		{steps[1].ID, models.StatusApproved}, // Шаг 2
		{steps[2].ID, models.StatusSkipped},  // Шаг 3 (пропущен)
		// Шаг 4 не пройден
	}

	for _, entry := range progressEntries2 {
		progress := &models.UserProgress{
			UserID: user2.ID,
			StepID: entry.stepID,
			Status: entry.status,
		}
		err := progressRepo.Create(progress)
		if err != nil {
			t.Fatal("Failed to save progress:", err)
		}
	}

	// Проверяем, что пользователь 2 НЕ завершил квест и находится на шаге 4
	details2, err := userManager.GetUserDetails(user2.ID)
	if err != nil {
		t.Fatal("Failed to get user2 details:", err)
	}

	if details2.IsCompleted {
		t.Errorf("Expected user2 to NOT have completed quest, but IsCompleted = %v", details2.IsCompleted)
	}

	if details2.CurrentStep == nil || details2.CurrentStep.StepOrder != 4 {
		if details2.CurrentStep == nil {
			t.Errorf("Expected CurrentStep to be step 4, but got nil")
		} else {
			t.Errorf("Expected CurrentStep to be step 4, but got step %d", details2.CurrentStep.StepOrder)
		}
	}

	// Проверяем обновленную статистику квеста
	questStats2, err := userManager.GetQuestStatistics()
	if err != nil {
		t.Fatal("Failed to get quest statistics:", err)
	}

	if questStats2.CompletedUsers != 1 {
		t.Errorf("Expected 1 completed user, got %d", questStats2.CompletedUsers)
	}

	if questStats2.InProgressUsers != 1 {
		t.Errorf("Expected 1 in-progress user, got %d", questStats2.InProgressUsers)
	}

	// Проверяем, что пользователь 2 находится на шаге 4
	if questStats2.StepDistribution[4] != 1 {
		t.Errorf("Expected 1 user on step 4, got %d", questStats2.StepDistribution[4])
	}
}
