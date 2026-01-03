package main

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/handlers"
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

	adminID := int64(123456)

	errorManager := services.NewErrorManager(nil, adminID)
	stateResolver := services.NewStateResolver(stepRepo, progressRepo, userRepo)
	answerChecker := services.NewAnswerChecker(answerRepo, progressRepo, userRepo)
	msgManager := services.NewMessageManager(nil, chatStateRepo, errorManager)
	statsService := services.NewStatisticsService(dbQueue, stepRepo, progressRepo, userRepo)
	userManager := services.NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, statsService)
	questStateManager := services.NewQuestStateManager(settingsRepo)

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

	adminID := int64(123456)

	statsService := services.NewStatisticsService(dbQueue, stepRepo, progressRepo, userRepo)
	userManager := services.NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, statsService)
	questStateManager := services.NewQuestStateManager(settingsRepo)

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
