package main

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/handlers"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/ad/go-telegram-quest/internal/services"
	_ "modernc.org/sqlite"
)

func TestComponentInitialization(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	settingsRepo := db.NewSettingsRepository(dbQueue)
	questStateManager := services.NewQuestStateManager(settingsRepo)

	if questStateManager == nil {
		t.Fatal("QuestStateManager should not be nil")
	}

	currentState, err := questStateManager.GetCurrentState()
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}

	if currentState != services.QuestStateNotStarted {
		t.Errorf("Expected initial state to be %s, got %s", services.QuestStateNotStarted, currentState)
	}
}

func TestBotHandlerIntegration(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	userRepo := db.NewUserRepository(dbQueue)
	stepRepo := db.NewStepRepository(dbQueue)
	progressRepo := db.NewProgressRepository(dbQueue)
	answerRepo := db.NewAnswerRepository(dbQueue)
	settingsRepo := db.NewSettingsRepository(dbQueue)
	chatStateRepo := db.NewChatStateRepository(dbQueue)
	adminMessagesRepo := db.NewAdminMessagesRepository(dbQueue)
	adminStateRepo := db.NewAdminStateRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)

	adminID := int64(123456)

	errorManager := services.NewErrorManager(nil, adminID)
	stateResolver := services.NewStateResolver(stepRepo, progressRepo, userRepo)
	answerChecker := services.NewAnswerChecker(answerRepo, progressRepo, userRepo)
	msgManager := services.NewMessageManager(nil, chatStateRepo, errorManager)
	statsService := services.NewStatisticsService(dbQueue, stepRepo, progressRepo, userRepo)
	userManager := services.NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, statsService)
	questStateManager := services.NewQuestStateManager(settingsRepo)
	achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, dbQueue)
	achievementNotifier := services.NewAchievementNotifier(nil, achievementRepo, msgManager)
	achievementService := services.NewAchievementService(achievementRepo, userRepo)

	handler := handlers.NewBotHandler(
		nil,
		adminID,
		errorManager,
		stateResolver,
		answerChecker,
		msgManager,
		statsService,
		userRepo,
		stepRepo,
		progressRepo,
		answerRepo,
		settingsRepo,
		chatStateRepo,
		adminMessagesRepo,
		adminStateRepo,
		userManager,
		questStateManager,
		achievementEngine,
		achievementNotifier,
		achievementService,
		"",
	)

	if handler == nil {
		t.Fatal("BotHandler should not be nil")
	}
}

func TestQuestStateMiddlewareIntegration(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	settingsRepo := db.NewSettingsRepository(dbQueue)
	questStateManager := services.NewQuestStateManager(settingsRepo)
	adminID := int64(123456)

	middleware := services.NewQuestStateMiddleware(questStateManager, adminID)
	if middleware == nil {
		t.Fatal("QuestStateMiddleware should not be nil")
	}

	shouldProcess, notification := middleware.ShouldProcessMessage(adminID)
	if !shouldProcess {
		t.Error("Admin should always be allowed to process messages")
	}
	if notification != "" {
		t.Error("Admin should not receive state notifications")
	}

	regularUserID := int64(789012)
	shouldProcess, notification = middleware.ShouldProcessMessage(regularUserID)
	if shouldProcess {
		t.Error("Regular user should not be allowed when quest is not started")
	}
	if notification == "" {
		t.Error("Regular user should receive state notification when quest is not started")
	}
}

func TestAdminHandlerIntegration(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	stepRepo := db.NewStepRepository(dbQueue)
	answerRepo := db.NewAnswerRepository(dbQueue)
	settingsRepo := db.NewSettingsRepository(dbQueue)
	adminStateRepo := db.NewAdminStateRepository(dbQueue)
	userRepo := db.NewUserRepository(dbQueue)
	progressRepo := db.NewProgressRepository(dbQueue)
	chatStateRepo := db.NewChatStateRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)

	adminID := int64(123456)

	statsService := services.NewStatisticsService(dbQueue, stepRepo, progressRepo, userRepo)
	userManager := services.NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, statsService)
	questStateManager := services.NewQuestStateManager(settingsRepo)
	achievementService := services.NewAchievementService(achievementRepo, userRepo)
	achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, dbQueue)

	adminHandler := handlers.NewAdminHandler(
		nil,
		adminID,
		stepRepo,
		answerRepo,
		settingsRepo,
		adminStateRepo,
		userManager,
		userRepo,
		questStateManager,
		achievementService,
		achievementEngine,
		"",
	)

	if adminHandler == nil {
		t.Fatal("AdminHandler should not be nil")
	}
}

func createTempDB(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	return tmpFile.Name()
}

func TestAchievementSystemEndToEnd_CompleteFlow(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatalf("Failed to initialize default achievements: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	userRepo := db.NewUserRepository(dbQueue)
	stepRepo := db.NewStepRepository(dbQueue)
	progressRepo := db.NewProgressRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)

	achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, dbQueue)
	achievementService := services.NewAchievementService(achievementRepo, userRepo)

	userID := int64(12345)
	user := &models.User{
		ID:        userID,
		FirstName: "Test",
		LastName:  "User",
		Username:  "testuser",
	}
	if err := userRepo.CreateOrUpdate(user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	for i := 1; i <= 5; i++ {
		step := &models.Step{
			StepOrder:    i,
			Text:         "Test step",
			AnswerType:   models.AnswerTypeText,
			HasAutoCheck: true,
			IsActive:     true,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}

		progress := &models.UserProgress{
			UserID: userID,
			StepID: stepID,
			Status: models.StatusApproved,
		}
		if err := progressRepo.Create(progress); err != nil {
			t.Fatalf("Failed to create progress: %v", err)
		}

		awarded, err := achievementEngine.EvaluateProgressAchievements(userID)
		if err != nil {
			t.Fatalf("Failed to evaluate progress achievements: %v", err)
		}

		if i == 5 {
			found := false
			for _, key := range awarded {
				if key == "beginner_5" {
					found = true
					break
				}
			}
			if !found {
				t.Error("User should receive beginner_5 achievement after 5 correct answers")
			}
		}
	}

	hasAchievement, err := achievementRepo.HasUserAchievement(userID, "beginner_5")
	if err != nil {
		t.Fatalf("Failed to check achievement: %v", err)
	}
	if !hasAchievement {
		t.Error("User should have beginner_5 achievement")
	}

	userAchievements, err := achievementService.GetUserAchievements(userID)
	if err != nil {
		t.Fatalf("Failed to get user achievements: %v", err)
	}
	if len(userAchievements) == 0 {
		t.Error("User should have at least one achievement")
	}

	count, err := achievementService.GetUserAchievementCount(userID)
	if err != nil {
		t.Fatalf("Failed to get achievement count: %v", err)
	}
	if count == 0 {
		t.Error("Achievement count should be greater than 0")
	}
}

func TestAchievementSystemEndToEnd_RetroactiveProcessing(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	userRepo := db.NewUserRepository(dbQueue)
	stepRepo := db.NewStepRepository(dbQueue)
	progressRepo := db.NewProgressRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)

	achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, dbQueue)
	retroactiveProcessor := services.NewRetroactiveProcessor(achievementEngine, achievementRepo, userRepo)

	baseTime := time.Now().Add(-1 * time.Hour)
	for j := 1; j <= 5; j++ {
		step := &models.Step{
			StepOrder:    j,
			Text:         "Test step",
			AnswerType:   models.AnswerTypeText,
			HasAutoCheck: true,
			IsActive:     true,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}

		for i := 1; i <= 3; i++ {
			userID := int64(i * 1000)
			if j == 1 {
				user := &models.User{
					ID:        userID,
					FirstName: "User",
					LastName:  "Test",
				}
				if err := userRepo.CreateOrUpdate(user); err != nil {
					t.Fatalf("Failed to create user: %v", err)
				}
			}

			completedAt := baseTime.Add(time.Duration((i-1)*5+j) * time.Minute)
			progress := &models.UserProgress{
				UserID:      userID,
				StepID:      stepID,
				Status:      models.StatusApproved,
				CompletedAt: &completedAt,
			}
			if err := progressRepo.Create(progress); err != nil {
				t.Fatalf("Failed to create progress: %v", err)
			}
			if err := progressRepo.Update(progress); err != nil {
				t.Fatalf("Failed to update progress: %v", err)
			}
		}
	}

	threshold := 5
	achievement := &models.Achievement{
		Key:         "retroactive_test",
		Name:        "Retroactive Test",
		Description: "Test retroactive achievement",
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

	progress, err := retroactiveProcessor.ProcessAchievementSync("retroactive_test", 10)
	if err != nil {
		t.Fatalf("Failed to process retroactive achievement: %v", err)
	}

	if progress.AwardedCount != 3 {
		t.Errorf("Expected 3 users to receive achievement, got %d", progress.AwardedCount)
	}

	for i := 1; i <= 3; i++ {
		userID := int64(i * 1000)
		hasAchievement, err := achievementRepo.HasUserAchievement(userID, "retroactive_test")
		if err != nil {
			t.Fatalf("Failed to check achievement: %v", err)
		}
		if !hasAchievement {
			t.Errorf("User %d should have retroactive_test achievement", userID)
		}
	}
}

func TestAchievementSystemEndToEnd_DefaultAchievementsInitialization(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatalf("Failed to initialize default achievements: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	achievementRepo := db.NewAchievementRepository(dbQueue)

	achievements, err := achievementRepo.GetAll()
	if err != nil {
		t.Fatalf("Failed to get achievements: %v", err)
	}

	if len(achievements) == 0 {
		t.Fatal("No achievements were initialized")
	}

	expectedKeys := []string{
		"pioneer", "second_place", "third_place",
		"beginner_5", "experienced_10", "advanced_15", "expert_20", "master_25",
		"winner", "perfect_path", "self_sufficient", "lightning", "rocket", "cheater",
		"hint_5", "hint_10", "hint_15", "hint_25", "hint_master", "skeptic",
		"photographer", "bullseye", "secret_agent", "curious", "paparazzi", "fan",
		"super_collector", "super_brain", "legend",
	}

	achievementMap := make(map[string]bool)
	for _, a := range achievements {
		achievementMap[a.Key] = true
	}

	for _, key := range expectedKeys {
		if !achievementMap[key] {
			t.Errorf("Expected achievement %s to be initialized", key)
		}
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatalf("Second initialization should not fail: %v", err)
	}

	achievementsAfter, err := achievementRepo.GetAll()
	if err != nil {
		t.Fatalf("Failed to get achievements after second init: %v", err)
	}

	if len(achievementsAfter) != len(achievements) {
		t.Errorf("Achievement count changed after second initialization: %d -> %d", len(achievements), len(achievementsAfter))
	}
}

func TestAchievementSystemEndToEnd_UniqueAchievements(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatalf("Failed to initialize default achievements: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	userRepo := db.NewUserRepository(dbQueue)
	stepRepo := db.NewStepRepository(dbQueue)
	progressRepo := db.NewProgressRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)
	answerRepo := db.NewAnswerRepository(dbQueue)

	achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, dbQueue)

	step := &models.Step{
		StepOrder:    1,
		Text:         "Test step",
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: true,
		IsActive:     true,
	}
	stepID, err := stepRepo.Create(step)
	if err != nil {
		t.Fatalf("Failed to create step: %v", err)
	}

	baseTime := time.Now().Add(-1 * time.Hour)
	for i := 1; i <= 3; i++ {
		userID := int64(i * 1000)
		user := &models.User{
			ID:        userID,
			FirstName: "User",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		if _, err := answerRepo.CreateTextAnswer(userID, stepID, "correct", false); err != nil {
			t.Fatalf("Failed to create answer: %v", err)
		}

		completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
		progress := &models.UserProgress{
			UserID:      userID,
			StepID:      stepID,
			Status:      models.StatusApproved,
			CompletedAt: &completedAt,
		}
		if err := progressRepo.Create(progress); err != nil {
			t.Fatalf("Failed to create progress: %v", err)
		}
		if err := progressRepo.Update(progress); err != nil {
			t.Fatalf("Failed to update progress: %v", err)
		}

		awarded, err := achievementEngine.EvaluatePositionBasedAchievements(userID)
		if err != nil {
			t.Fatalf("Failed to evaluate position achievements: %v", err)
		}

		expectedKey := ""
		switch i {
		case 1:
			expectedKey = "pioneer"
		case 2:
			expectedKey = "second_place"
		case 3:
			expectedKey = "third_place"
		}

		found := false
		for _, key := range awarded {
			if key == expectedKey {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("User %d should receive %s achievement", i, expectedKey)
		}
	}

	pioneerHolders, err := achievementRepo.GetAchievementHolders("pioneer")
	if err != nil {
		t.Fatalf("Failed to get achievement holders: %v", err)
	}
	if len(pioneerHolders) != 1 {
		t.Errorf("Pioneer achievement should have exactly 1 holder, got %d", len(pioneerHolders))
	}
}

func TestAchievementSystemEndToEnd_CompositeAchievements(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatalf("Failed to initialize default achievements: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	userRepo := db.NewUserRepository(dbQueue)
	stepRepo := db.NewStepRepository(dbQueue)
	progressRepo := db.NewProgressRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)

	achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, dbQueue)

	userID := int64(12345)
	user := &models.User{
		ID:        userID,
		FirstName: "Test",
	}
	if err := userRepo.CreateOrUpdate(user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	for i := 1; i <= 25; i++ {
		step := &models.Step{
			StepOrder:    i,
			Text:         "Test step",
			AnswerType:   models.AnswerTypeText,
			HasAutoCheck: true,
			IsActive:     true,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}

		progress := &models.UserProgress{
			UserID: userID,
			StepID: stepID,
			Status: models.StatusApproved,
		}
		if err := progressRepo.Create(progress); err != nil {
			t.Fatalf("Failed to create progress: %v", err)
		}

		achievementEngine.EvaluateProgressAchievements(userID)
	}

	progressAchievements := []string{"beginner_5", "experienced_10", "advanced_15", "expert_20", "master_25"}
	for _, key := range progressAchievements {
		hasAchievement, err := achievementRepo.HasUserAchievement(userID, key)
		if err != nil {
			t.Fatalf("Failed to check achievement %s: %v", key, err)
		}
		if !hasAchievement {
			t.Errorf("User should have %s achievement", key)
		}
	}

	awarded, err := achievementEngine.EvaluateCompositeAchievements(userID)
	if err != nil {
		t.Fatalf("Failed to evaluate composite achievements: %v", err)
	}

	found := false
	for _, key := range awarded {
		if key == "super_collector" {
			found = true
			break
		}
	}
	if !found {
		t.Error("User should receive super_collector achievement after collecting all progress achievements")
	}
}

func TestAchievementSystemEndToEnd_NotificationPreparation(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatalf("Failed to initialize default achievements: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	achievementRepo := db.NewAchievementRepository(dbQueue)

	notifier := services.NewAchievementNotifier(nil, achievementRepo, nil)

	notifications, err := notifier.PrepareNotifications([]string{"beginner_5", "experienced_10"})
	if err != nil {
		t.Fatalf("Failed to prepare notifications: %v", err)
	}

	if len(notifications) != 2 {
		t.Errorf("Expected 2 notifications, got %d", len(notifications))
	}

	for _, notification := range notifications {
		if notification == nil || notification.Message == "" {
			t.Error("Notification should not be empty")
		}
	}
}

func TestAchievementSystemEndToEnd_StatisticsCalculation(t *testing.T) {
	tempDB := createTempDB(t)
	defer os.Remove(tempDB)

	sqlDB, err := sql.Open("sqlite", tempDB+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatalf("Failed to initialize default achievements: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	userRepo := db.NewUserRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)

	achievementService := services.NewAchievementService(achievementRepo, userRepo)

	stats, err := achievementService.GetAchievementStatistics()
	if err != nil {
		t.Fatalf("Failed to get achievement statistics: %v", err)
	}

	if stats == nil {
		t.Fatal("Statistics should not be nil")
	}

	if stats.TotalAchievements == 0 {
		t.Error("Total achievements should be greater than 0")
	}

	if len(stats.AchievementsByCategory) == 0 {
		t.Error("Achievements by category should not be empty")
	}
}
