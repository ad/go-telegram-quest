package handlers

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/ad/go-telegram-quest/internal/services"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func setupTestDB(t *testing.T) (*db.DBQueue, func()) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			first_name TEXT,
			last_name TEXT,
			username TEXT,
			is_blocked BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			step_order INTEGER UNIQUE NOT NULL,
			text TEXT NOT NULL,
			answer_type TEXT NOT NULL DEFAULT 'text',
			has_auto_check BOOLEAN DEFAULT FALSE,
			is_active BOOLEAN DEFAULT TRUE,
			is_deleted BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS step_images (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			step_id INTEGER NOT NULL REFERENCES steps(id),
			file_id TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS step_answers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			step_id INTEGER NOT NULL REFERENCES steps(id),
			answer TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS user_progress (
			user_id INTEGER NOT NULL REFERENCES users(id),
			step_id INTEGER NOT NULL REFERENCES steps(id),
			status TEXT NOT NULL DEFAULT 'pending',
			completed_at DATETIME,
			PRIMARY KEY (user_id, step_id)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS user_answers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			step_id INTEGER NOT NULL REFERENCES steps(id),
			text_answer TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS answer_images (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			answer_id INTEGER NOT NULL REFERENCES user_answers(id),
			file_id TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		INSERT INTO settings (key, value) VALUES 
			('welcome_message', 'Welcome!'),
			('final_message', 'Congratulations!'),
			('correct_answer_message', 'Correct!'),
			('wrong_answer_message', 'Wrong!')
	`)
	if err != nil {
		t.Fatal(err)
	}

	queue := db.NewDBQueue(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func TestProperty16_SettingsBasedResponseMessages(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		settingsRepo := db.NewSettingsRepository(queue)

		correctMsg := rapid.StringMatching(`[a-zA-Z ]{5,30}`).Draw(rt, "correctMsg")
		wrongMsg := rapid.StringMatching(`[a-zA-Z ]{5,30}`).Draw(rt, "wrongMsg")

		if err := settingsRepo.SetCorrectAnswerMessage(correctMsg); err != nil {
			rt.Fatal(err)
		}
		if err := settingsRepo.SetWrongAnswerMessage(wrongMsg); err != nil {
			rt.Fatal(err)
		}

		settings, err := settingsRepo.GetAll()
		if err != nil {
			rt.Fatal(err)
		}

		if settings.CorrectAnswerMessage != correctMsg {
			rt.Errorf("Expected correct message '%s', got '%s'", correctMsg, settings.CorrectAnswerMessage)
		}
		if settings.WrongAnswerMessage != wrongMsg {
			rt.Errorf("Expected wrong message '%s', got '%s'", wrongMsg, settings.WrongAnswerMessage)
		}
	})
}

func TestProperty15_AnswerTypeValidation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)

		answerType := rapid.SampledFrom([]models.AnswerType{
			models.AnswerTypeText,
			models.AnswerTypeImage,
		}).Draw(rt, "answerType")

		step := &models.Step{
			StepOrder:    1,
			Text:         "Test step",
			AnswerType:   answerType,
			HasAutoCheck: answerType == models.AnswerTypeText,
			IsActive:     true,
			IsDeleted:    false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		if answerType == models.AnswerTypeText {
			correctAnswer := rapid.StringMatching(`[a-zA-Z]{3,10}`).Draw(rt, "correctAnswer")
			if err := answerRepo.AddStepAnswer(stepID, correctAnswer); err != nil {
				rt.Fatal(err)
			}
		}

		createdStep, err := stepRepo.GetByID(stepID)
		if err != nil {
			rt.Fatal(err)
		}

		if createdStep.AnswerType != answerType {
			rt.Errorf("Expected answer type %s, got %s", answerType, createdStep.AnswerType)
		}

		if answerType == models.AnswerTypeText {
			if !createdStep.HasAutoCheck {
				rt.Error("Text answer type should have auto-check enabled")
			}
			if len(createdStep.Answers) == 0 {
				rt.Error("Text answer type should have answer variants")
			}
		}

		if answerType == models.AnswerTypeImage {
			if createdStep.HasAutoCheck {
				rt.Error("Image answer type should not have auto-check enabled")
			}
		}
	})
}

func TestProperty8_ImageAnswerStatusTransition(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")

		user := &models.User{
			ID:        userID,
			FirstName: "Test",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:    1,
			Text:         "Send a photo",
			AnswerType:   models.AnswerTypeImage,
			HasAutoCheck: false,
			IsActive:     true,
			IsDeleted:    false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		progress := &models.UserProgress{
			UserID: userID,
			StepID: stepID,
			Status: models.StatusPending,
		}
		if err := progressRepo.Create(progress); err != nil {
			rt.Fatal(err)
		}

		progress.Status = models.StatusWaitingReview
		if err := progressRepo.Update(progress); err != nil {
			rt.Fatal(err)
		}

		updatedProgress, err := progressRepo.GetByUserAndStep(userID, stepID)
		if err != nil {
			rt.Fatal(err)
		}

		if updatedProgress.Status != models.StatusWaitingReview {
			rt.Errorf("Expected status 'waiting_review', got '%s'", updatedProgress.Status)
		}
	})
}

func TestProperty9_AdminDecisionStatusUpdates(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")

		user := &models.User{
			ID:        userID,
			FirstName: "Test",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:    1,
			Text:         "Test step",
			AnswerType:   models.AnswerTypeImage,
			HasAutoCheck: false,
			IsActive:     true,
			IsDeleted:    false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		progress := &models.UserProgress{
			UserID: userID,
			StepID: stepID,
			Status: models.StatusWaitingReview,
		}
		if err := progressRepo.Create(progress); err != nil {
			rt.Fatal(err)
		}

		decision := rapid.SampledFrom([]string{"approve", "reject"}).Draw(rt, "decision")

		if decision == "approve" {
			progress.Status = models.StatusApproved
		} else {
			progress.Status = models.StatusRejected
		}
		if err := progressRepo.Update(progress); err != nil {
			rt.Fatal(err)
		}

		updatedProgress, err := progressRepo.GetByUserAndStep(userID, stepID)
		if err != nil {
			rt.Fatal(err)
		}

		if decision == "approve" {
			if updatedProgress.Status != models.StatusApproved {
				rt.Errorf("Expected status 'approved' after approval, got '%s'", updatedProgress.Status)
			}
		} else {
			if updatedProgress.Status != models.StatusRejected {
				rt.Errorf("Expected status 'rejected' after rejection, got '%s'", updatedProgress.Status)
			}
		}

		if decision == "reject" {
			progress.Status = models.StatusPending
			if err := progressRepo.Update(progress); err != nil {
				rt.Fatal(err)
			}

			resubmitProgress, err := progressRepo.GetByUserAndStep(userID, stepID)
			if err != nil {
				rt.Fatal(err)
			}
			if resubmitProgress.Status != models.StatusPending {
				rt.Errorf("User should be able to resubmit after rejection")
			}
		}
	})
}

func TestProperty18_BlockedUserShadowBan(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")

		user := &models.User{
			ID:        userID,
			FirstName: "Test",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:    1,
			Text:         "Test step",
			AnswerType:   models.AnswerTypeText,
			HasAutoCheck: false,
			IsActive:     true,
			IsDeleted:    false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		progress := &models.UserProgress{
			UserID: userID,
			StepID: stepID,
			Status: models.StatusPending,
		}
		if err := progressRepo.Create(progress); err != nil {
			rt.Fatal(err)
		}

		if err := userRepo.BlockUser(userID); err != nil {
			rt.Fatal(err)
		}

		isBlocked, err := userRepo.IsBlocked(userID)
		if err != nil {
			rt.Fatal(err)
		}
		if !isBlocked {
			rt.Error("User should be blocked after BlockUser call")
		}

		progressAfterBlock, err := progressRepo.GetByUserAndStep(userID, stepID)
		if err != nil {
			rt.Fatal(err)
		}
		if progressAfterBlock.Status != models.StatusPending {
			rt.Errorf("Blocked user's progress should not change, got status '%s'", progressAfterBlock.Status)
		}
	})
}

func TestProperty17_UserBlockStatusToggle(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")

		user := &models.User{
			ID:        userID,
			FirstName: "Test",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		isBlocked, err := userRepo.IsBlocked(userID)
		if err != nil {
			rt.Fatal(err)
		}
		if isBlocked {
			rt.Error("New user should not be blocked")
		}

		if err := userRepo.BlockUser(userID); err != nil {
			rt.Fatal(err)
		}

		isBlocked, err = userRepo.IsBlocked(userID)
		if err != nil {
			rt.Fatal(err)
		}
		if !isBlocked {
			rt.Error("User should be blocked after BlockUser")
		}

		if err := userRepo.BlockUser(userID); err != nil {
			rt.Fatal(err)
		}
		isBlocked, err = userRepo.IsBlocked(userID)
		if err != nil {
			rt.Fatal(err)
		}
		if !isBlocked {
			rt.Error("BlockUser should be idempotent")
		}

		if err := userRepo.UnblockUser(userID); err != nil {
			rt.Fatal(err)
		}

		isBlocked, err = userRepo.IsBlocked(userID)
		if err != nil {
			rt.Fatal(err)
		}
		if isBlocked {
			rt.Error("User should not be blocked after UnblockUser")
		}

		if err := userRepo.UnblockUser(userID); err != nil {
			rt.Fatal(err)
		}
		isBlocked, err = userRepo.IsBlocked(userID)
		if err != nil {
			rt.Fatal(err)
		}
		if isBlocked {
			rt.Error("UnblockUser should be idempotent")
		}
	})
}

func TestProperty21_BlockButtonConditionalDisplay(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		isBlocked := rapid.Bool().Draw(rt, "isBlocked")

		user := &models.User{
			ID:        userID,
			IsBlocked: isBlocked,
		}

		keyboard := BuildUserDetailsKeyboard(user)

		if keyboard == nil {
			rt.Fatal("Keyboard should not be nil")
		}

		if len(keyboard.InlineKeyboard) < 3 {
			rt.Fatal("Keyboard should have at least 3 rows")
		}

		blockRow := keyboard.InlineKeyboard[0]
		if len(blockRow) != 1 {
			rt.Fatalf("Block row should have exactly 1 button, got %d", len(blockRow))
		}

		blockBtn := blockRow[0]

		if isBlocked {
			if blockBtn.Text != "✅ Разблокировать" {
				rt.Errorf("Expected '✅ Разблокировать' for blocked user, got '%s'", blockBtn.Text)
			}
			if !containsUserID(blockBtn.CallbackData, "unblock:", userID) {
				rt.Errorf("Expected callback 'unblock:%d', got '%s'", userID, blockBtn.CallbackData)
			}
		} else {
			if blockBtn.Text != "🚫 Заблокировать" {
				rt.Errorf("Expected '🚫 Заблокировать' for non-blocked user, got '%s'", blockBtn.Text)
			}
			if !containsUserID(blockBtn.CallbackData, "block:", userID) {
				rt.Errorf("Expected callback 'block:%d', got '%s'", userID, blockBtn.CallbackData)
			}
		}

		resetRow := keyboard.InlineKeyboard[1]
		if len(resetRow) != 1 {
			rt.Fatalf("Reset row should have exactly 1 button, got %d", len(resetRow))
		}
		if resetRow[0].Text != "🔄 Сбросить прогресс" {
			rt.Errorf("Expected reset button text '🔄 Сбросить прогресс', got '%s'", resetRow[0].Text)
		}
		if !containsUserID(resetRow[0].CallbackData, "reset:", userID) {
			rt.Errorf("Expected reset callback 'reset:%d', got '%s'", userID, resetRow[0].CallbackData)
		}

		backRow := keyboard.InlineKeyboard[2]
		if len(backRow) != 1 {
			rt.Fatalf("Back row should have exactly 1 button, got %d", len(backRow))
		}
		if backRow[0].Text != "⬅️ Назад" {
			rt.Errorf("Expected back button text '⬅️ Назад', got '%s'", backRow[0].Text)
		}
		if backRow[0].CallbackData != "admin:userlist" {
			rt.Errorf("Expected back callback 'admin:userlist', got '%s'", backRow[0].CallbackData)
		}
	})
}

func containsUserID(callbackData, prefix string, userID int64) bool {
	expected := prefix + fmt.Sprintf("%d", userID)
	return callbackData == expected
}

func TestProperty2_AdminAccessInvariant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		settingsRepo := db.NewSettingsRepository(queue)

		_, err := queue.DB().Exec(`
			INSERT OR IGNORE INTO settings (key, value) VALUES 
				('quest_state', 'not_started'),
				('quest_not_started_message', 'Quest not started'),
				('quest_paused_message', 'Quest paused'),
				('quest_completed_message', 'Quest completed')
		`)
		if err != nil {
			rt.Fatal(err)
		}

		questStateManager := services.NewQuestStateManager(settingsRepo)
		adminID := rapid.Int64Range(1, 1000).Draw(rt, "adminID")

		middleware := services.NewQuestStateMiddleware(questStateManager, adminID)

		questState := rapid.SampledFrom([]services.QuestState{
			services.QuestStateNotStarted,
			services.QuestStateRunning,
			services.QuestStatePaused,
			services.QuestStateCompleted,
		}).Draw(rt, "questState")

		if err := questStateManager.SetState(questState); err != nil {
			rt.Fatal(err)
		}

		shouldProcess, notification := middleware.ShouldProcessMessage(adminID)

		if !shouldProcess {
			rt.Errorf("Admin should always be able to process messages regardless of quest state '%s'", questState)
		}

		if notification != "" {
			rt.Errorf("Admin should not receive state restriction messages, got: '%s'", notification)
		}

		regularUserID := rapid.Int64Range(1001, 2000).Draw(rt, "regularUserID")
		shouldProcessRegular, notificationRegular := middleware.ShouldProcessMessage(regularUserID)

		if questState == services.QuestStateRunning {
			if !shouldProcessRegular {
				rt.Error("Regular users should be able to process messages when quest is running")
			}
			if notificationRegular != "" {
				rt.Error("Regular users should not receive notifications when quest is running")
			}
		} else {
			if shouldProcessRegular {
				rt.Errorf("Regular users should not be able to process messages when quest state is '%s'", questState)
			}
			if notificationRegular == "" {
				rt.Errorf("Regular users should receive notifications when quest state is '%s'", questState)
			}
		}
	})
}
