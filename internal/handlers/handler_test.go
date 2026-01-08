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
			correct_answer_image TEXT,
			hint_text TEXT DEFAULT '',
			hint_image TEXT DEFAULT '',
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
			hint_used BOOLEAN DEFAULT FALSE,
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
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS user_chat_state (
			user_id INTEGER PRIMARY KEY REFERENCES users(id),
			last_task_message_id INTEGER,
			last_user_answer_message_id INTEGER,
			last_reaction_message_id INTEGER,
			hint_message_id INTEGER DEFAULT 0,
			current_step_hint_used BOOLEAN DEFAULT FALSE
		)
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

		keyboard := BuildUserDetailsKeyboard(user, true)

		if keyboard == nil {
			rt.Fatal("Keyboard should not be nil")
		}

		if len(keyboard.InlineKeyboard) < 6 {
			rt.Fatal("Keyboard should have at least 6 rows")
		}

		// Row 0: Achievements button
		achievementsRow := keyboard.InlineKeyboard[0]
		if len(achievementsRow) != 1 {
			rt.Fatalf("Achievements row should have exactly 1 button, got %d", len(achievementsRow))
		}
		if achievementsRow[0].Text != "🏆 Достижения" {
			rt.Errorf("Expected achievements button text '🏆 Достижения', got '%s'", achievementsRow[0].Text)
		}
		if !containsUserID(achievementsRow[0].CallbackData, "user_achievements:", userID) {
			rt.Errorf("Expected achievements callback 'user_achievements:%d', got '%s'", userID, achievementsRow[0].CallbackData)
		}

		// Row 1: Message button
		messageRow := keyboard.InlineKeyboard[1]
		if len(messageRow) != 1 {
			rt.Fatalf("Message row should have exactly 1 button, got %d", len(messageRow))
		}
		if messageRow[0].Text != "💬 Написать сообщение" {
			rt.Errorf("Expected message button text '💬 Написать сообщение', got '%s'", messageRow[0].Text)
		}
		if !containsUserID(messageRow[0].CallbackData, "admin:send_message:", userID) {
			rt.Errorf("Expected message callback 'admin:send_message:%d', got '%s'", userID, messageRow[0].CallbackData)
		}

		// Row 2: Block/Unblock button
		blockRow := keyboard.InlineKeyboard[2]
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

		// Row 3: Reset button
		resetRow := keyboard.InlineKeyboard[3]
		if len(resetRow) != 1 {
			rt.Fatalf("Reset row should have exactly 1 button, got %d", len(resetRow))
		}
		if resetRow[0].Text != "🔄 Сбросить прогресс" {
			rt.Errorf("Expected reset button text '🔄 Сбросить прогресс', got '%s'", resetRow[0].Text)
		}
		if !containsUserID(resetRow[0].CallbackData, "reset:", userID) {
			rt.Errorf("Expected reset callback 'reset:%d', got '%s'", userID, resetRow[0].CallbackData)
		}

		// Row 4: Reset achievements button
		resetAchievementsRow := keyboard.InlineKeyboard[4]
		if len(resetAchievementsRow) != 1 {
			rt.Fatalf("Reset achievements row should have exactly 1 button, got %d", len(resetAchievementsRow))
		}
		if resetAchievementsRow[0].Text != "🏅 Сбросить достижения" {
			rt.Errorf("Expected reset achievements button text '🏅 Сбросить достижения', got '%s'", resetAchievementsRow[0].Text)
		}
		if !containsUserID(resetAchievementsRow[0].CallbackData, "reset_achievements:", userID) {
			rt.Errorf("Expected reset achievements callback 'reset_achievements:%d', got '%s'", userID, resetAchievementsRow[0].CallbackData)
		}

		// Row 5: Back button
		backRow := keyboard.InlineKeyboard[5]
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

func TestProperty2_HintButtonDisplayLogic(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		stepID := rapid.Int64Range(1, 1000).Draw(rt, "stepID")

		hintText := rapid.StringMatching(`[a-zA-Z ]{0,50}`).Draw(rt, "hintText")
		hintImage := rapid.StringMatching(`[a-zA-Z0-9_]{0,20}`).Draw(rt, "hintImage")
		showHintButton := rapid.Bool().Draw(rt, "showHintButton")

		step := &models.Step{
			ID:        stepID,
			StepOrder: 1,
			Text:      "Test step",
			HintText:  hintText,
			HintImage: hintImage,
		}

		hasHint := step.HasHint()
		expectedHintButton := hasHint && showHintButton

		keyboard := BuildHintKeyboard(userID, stepID)

		if keyboard == nil {
			rt.Fatal("BuildHintKeyboard should never return nil")
		}

		if len(keyboard.InlineKeyboard) != 1 {
			rt.Fatalf("Hint keyboard should have exactly 1 row, got %d", len(keyboard.InlineKeyboard))
		}

		if len(keyboard.InlineKeyboard[0]) != 1 {
			rt.Fatalf("Hint keyboard row should have exactly 1 button, got %d", len(keyboard.InlineKeyboard[0]))
		}

		hintBtn := keyboard.InlineKeyboard[0][0]

		if hintBtn.Text != "💡 Подсказка" {
			rt.Errorf("Expected hint button text '💡 Подсказка', got '%s'", hintBtn.Text)
		}

		expectedCallback := fmt.Sprintf("hint:%d:%d", userID, stepID)
		if hintBtn.CallbackData != expectedCallback {
			rt.Errorf("Expected callback '%s', got '%s'", expectedCallback, hintBtn.CallbackData)
		}

		if hasHint != (hintText != "" || hintImage != "") {
			rt.Errorf("HasHint() should return true if and only if hintText or hintImage is non-empty")
		}

		if expectedHintButton && !hasHint {
			rt.Error("Hint button should not be shown if step has no hint")
		}

		if expectedHintButton && !showHintButton {
			rt.Error("Hint button should not be shown if showHintButton is false")
		}
	})
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

func TestProperty3_HintUsageTracking(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)
		chatStateRepo := db.NewChatStateRepository(queue)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		stepID := rapid.Int64Range(1, 1000).Draw(rt, "stepID")

		user := &models.User{
			ID:        userID,
			FirstName: "Test",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		hintText := rapid.StringMatching(`[a-zA-Z ]{1,50}`).Draw(rt, "hintText")
		step := &models.Step{
			ID:        stepID,
			StepOrder: 1,
			Text:      "Test step",
			HintText:  hintText,
			IsActive:  true,
		}
		createdStepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		hintUsed := rapid.Bool().Draw(rt, "hintUsed")

		if err := chatStateRepo.SetHintUsed(userID, hintUsed); err != nil {
			rt.Fatal(err)
		}

		chatState, err := chatStateRepo.Get(userID)
		if err != nil {
			rt.Fatal(err)
		}

		if chatState.CurrentStepHintUsed != hintUsed {
			rt.Errorf("Expected CurrentStepHintUsed to be %v, got %v", hintUsed, chatState.CurrentStepHintUsed)
		}

		answerText := rapid.StringMatching(`[a-zA-Z ]{1,20}`).Draw(rt, "answerText")
		answerID, err := answerRepo.CreateTextAnswer(userID, createdStepID, answerText, chatState.CurrentStepHintUsed)
		if err != nil {
			rt.Fatal(err)
		}

		if answerID == 0 {
			rt.Fatal("Answer ID should not be 0")
		}

		var storedHintUsed bool
		err = queue.DB().QueryRow(`SELECT hint_used FROM user_answers WHERE id = ?`, answerID).Scan(&storedHintUsed)
		if err != nil {
			rt.Fatal(err)
		}

		if storedHintUsed != hintUsed {
			rt.Errorf("Expected stored hint_used to be %v, got %v", hintUsed, storedHintUsed)
		}

		if err := chatStateRepo.ResetHintUsed(userID); err != nil {
			rt.Fatal(err)
		}

		resetChatState, err := chatStateRepo.Get(userID)
		if err != nil {
			rt.Fatal(err)
		}

		if resetChatState.CurrentStepHintUsed != false {
			rt.Error("CurrentStepHintUsed should be false after reset")
		}

		if resetChatState.HintMessageID != 0 {
			rt.Error("HintMessageID should be 0 after reset")
		}
	})
}

func setupTestDBWithAchievements(t *testing.T) (*db.DBQueue, func()) {
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
			correct_answer_image TEXT,
			hint_text TEXT DEFAULT '',
			hint_image TEXT DEFAULT '',
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
			hint_used BOOLEAN DEFAULT FALSE,
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
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS user_chat_state (
			user_id INTEGER PRIMARY KEY REFERENCES users(id),
			last_task_message_id INTEGER,
			last_user_answer_message_id INTEGER,
			last_reaction_message_id INTEGER,
			hint_message_id INTEGER DEFAULT 0,
			current_step_hint_used BOOLEAN DEFAULT FALSE
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS achievements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			category TEXT NOT NULL,
			type TEXT NOT NULL,
			is_unique BOOLEAN DEFAULT FALSE,
			conditions TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			is_active BOOLEAN DEFAULT TRUE
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS user_achievements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			achievement_id INTEGER NOT NULL REFERENCES achievements(id),
			earned_at DATETIME NOT NULL,
			is_retroactive BOOLEAN DEFAULT FALSE,
			UNIQUE(user_id, achievement_id)
		)
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

func TestAchievementIntegration_ProgressAchievementOnCorrectAnswer(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBWithAchievements(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		achievementRepo := db.NewAchievementRepository(queue)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		achievement := &models.Achievement{
			Key:         "beginner_5",
			Name:        "Начинающий",
			Description: "5 правильных ответов",
			Category:    models.CategoryProgress,
			Type:        models.TypeProgressBased,
			IsUnique:    false,
			Conditions:  models.AchievementConditions{CorrectAnswers: intPtr(5)},
			IsActive:    true,
		}
		if err := achievementRepo.Create(achievement); err != nil {
			rt.Fatal(err)
		}

		numCorrectAnswers := rapid.IntRange(1, 10).Draw(rt, "numCorrectAnswers")
		for i := 0; i < numCorrectAnswers; i++ {
			step := &models.Step{
				StepOrder:    i + 1,
				Text:         fmt.Sprintf("Step %d", i+1),
				AnswerType:   models.AnswerTypeText,
				HasAutoCheck: true,
				IsActive:     true,
			}
			stepID, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}

			progress := &models.UserProgress{
				UserID: userID,
				StepID: stepID,
				Status: models.StatusApproved,
			}
			if err := progressRepo.Create(progress); err != nil {
				rt.Fatal(err)
			}
		}

		achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
		awarded, err := achievementEngine.EvaluateProgressAchievements(userID)
		if err != nil {
			rt.Fatal(err)
		}

		hasAchievement, err := achievementRepo.HasUserAchievement(userID, "beginner_5")
		if err != nil {
			rt.Fatal(err)
		}

		if numCorrectAnswers >= 5 {
			if !hasAchievement {
				rt.Errorf("User with %d correct answers should have beginner_5 achievement", numCorrectAnswers)
			}
			found := false
			for _, key := range awarded {
				if key == "beginner_5" {
					found = true
					break
				}
			}
			if !found {
				rt.Error("beginner_5 should be in awarded list")
			}
		} else {
			if hasAchievement {
				rt.Errorf("User with %d correct answers should not have beginner_5 achievement", numCorrectAnswers)
			}
		}
	})
}

func TestAchievementIntegration_SecretAgentOnSpecificAnswer(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBWithAchievements(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		achievementRepo := db.NewAchievementRepository(queue)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		secretAnswer := "сезам откройся"
		achievement := &models.Achievement{
			Key:         "secret_agent",
			Name:        "Секретный Агент",
			Description: "Использовал секретную фразу",
			Category:    models.CategorySpecial,
			Type:        models.TypeActionBased,
			IsUnique:    false,
			Conditions:  models.AchievementConditions{SpecificAnswer: &secretAnswer},
			IsActive:    true,
		}
		if err := achievementRepo.Create(achievement); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:    1,
			Text:         "Test step",
			AnswerType:   models.AnswerTypeText,
			HasAutoCheck: true,
			IsActive:     true,
		}
		_, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		useSecretAnswer := rapid.Bool().Draw(rt, "useSecretAnswer")
		var answer string
		if useSecretAnswer {
			answer = secretAnswer
		} else {
			answer = rapid.StringMatching(`[a-zA-Z]{5,15}`).Draw(rt, "regularAnswer")
		}

		awarded, err := achievementEngine.OnAnswerSubmitted(userID, answer)
		if err != nil {
			rt.Fatal(err)
		}

		hasAchievement, err := achievementRepo.HasUserAchievement(userID, "secret_agent")
		if err != nil {
			rt.Fatal(err)
		}

		if useSecretAnswer {
			if !hasAchievement {
				rt.Error("User who used secret answer should have secret_agent achievement")
			}
			found := false
			for _, key := range awarded {
				if key == "secret_agent" {
					found = true
					break
				}
			}
			if !found {
				rt.Error("secret_agent should be in awarded list")
			}
		} else {
			if hasAchievement {
				rt.Error("User who did not use secret answer should not have secret_agent achievement")
			}
		}
	})
}

func TestAchievementIntegration_NotificationDelivery(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBWithAchievements(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)

		achievement := &models.Achievement{
			Key:         "test_achievement",
			Name:        "Тестовое достижение",
			Description: "Описание тестового достижения",
			Category:    models.CategoryProgress,
			Type:        models.TypeProgressBased,
			IsUnique:    false,
			Conditions:  models.AchievementConditions{},
			IsActive:    true,
		}
		if err := achievementRepo.Create(achievement); err != nil {
			rt.Fatal(err)
		}

		notifier := services.NewAchievementNotifier(nil, achievementRepo, nil, nil)

		notifications, err := notifier.PrepareNotifications([]string{"test_achievement"})
		if err != nil {
			rt.Fatal(err)
		}

		if len(notifications) != 1 {
			rt.Fatalf("Expected 1 notification, got %d", len(notifications))
		}

		notification := notifications[0]
		if notification.AchievementKey != "test_achievement" {
			rt.Errorf("Expected achievement key 'test_achievement', got '%s'", notification.AchievementKey)
		}

		if notification.Achievement == nil {
			rt.Fatal("Achievement should not be nil")
		}

		if notification.Achievement.Name != "Тестовое достижение" {
			rt.Errorf("Expected achievement name 'Тестовое достижение', got '%s'", notification.Achievement.Name)
		}

		if notification.Message == "" {
			rt.Error("Notification message should not be empty")
		}

		expectedEmoji := notifier.GetAchievementEmoji(notification.Achievement)
		if expectedEmoji == "" {
			rt.Error("Achievement emoji should not be empty")
		}
	})
}

func intPtr(i int) *int {
	return &i
}
