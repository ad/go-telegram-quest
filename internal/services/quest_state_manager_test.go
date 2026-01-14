package services

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func setupTestDBForQuestState(t *testing.T) (*db.DBQueue, func()) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
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
		INSERT OR IGNORE INTO settings (key, value) VALUES 
			('quest_state', 'not_started'),
			('quest_not_started_message', 'Квест ещё не начался. Ожидайте объявления о старте!'),
			('quest_paused_message', 'Квест временно приостановлен. Скоро мы продолжим!'),
			('quest_completed_message', 'Квест завершён! Спасибо за участие!')
	`)
	if err != nil {
		t.Fatal(err)
	}

	queue := db.NewDBQueueForTest(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func setupTestDBForDataPreservation(t *testing.T) (*db.DBQueue, func()) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	// Create all required tables
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			first_name TEXT,
			last_name TEXT,
			username TEXT,
			is_blocked INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS user_progress (
			user_id INTEGER,
			step_id INTEGER,
			status TEXT NOT NULL,
			completed_at DATETIME,
			PRIMARY KEY (user_id, step_id)
		);

		CREATE TABLE IF NOT EXISTS user_answers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			step_id INTEGER NOT NULL,
			text_answer TEXT,
    		hint_used BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS answer_images (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			answer_id INTEGER NOT NULL,
			file_id TEXT NOT NULL,
			position INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS user_chat_state (
			user_id INTEGER PRIMARY KEY,
			last_task_message_id INTEGER,
			last_user_answer_message_id INTEGER,
			last_reaction_message_id INTEGER,
			hint_message_id INTEGER DEFAULT 0,
			current_step_hint_used BOOLEAN DEFAULT FALSE,
			awaiting_next_step BOOLEAN DEFAULT FALSE
		);

		CREATE TABLE IF NOT EXISTS step_answers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			step_id INTEGER NOT NULL,
			answer TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = sqlDB.Exec(`
		INSERT OR IGNORE INTO settings (key, value) VALUES 
			('quest_state', 'not_started'),
			('quest_not_started_message', 'Квест ещё не начался. Ожидайте объявления о старте!'),
			('quest_paused_message', 'Квест временно приостановлен. Скоро мы продолжим!'),
			('quest_completed_message', 'Квест завершён! Спасибо за участие!')
	`)
	if err != nil {
		t.Fatal(err)
	}

	queue := db.NewDBQueueForTest(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func TestProperty1_StatePersistenceAndRestoration(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForQuestState(t)
		defer cleanup()

		settingsRepo := db.NewSettingsRepository(queue)
		manager := NewQuestStateManager(settingsRepo)

		validStates := []QuestState{
			QuestStateNotStarted,
			QuestStateRunning,
			QuestStatePaused,
			QuestStateCompleted,
		}

		randomState := validStates[rapid.IntRange(0, len(validStates)-1).Draw(rt, "randomState")]

		err := manager.SetState(randomState)
		if err != nil {
			rt.Fatalf("Failed to set state %s: %v", randomState, err)
		}

		retrievedState, err := manager.GetCurrentState()
		if err != nil {
			rt.Fatalf("Failed to get current state: %v", err)
		}
		if retrievedState != randomState {
			rt.Errorf("State not persisted correctly: expected %s, got %s", randomState, retrievedState)
		}

		newManager := NewQuestStateManager(settingsRepo)
		restoredState, err := newManager.GetCurrentState()
		if err != nil {
			rt.Fatalf("Failed to restore state: %v", err)
		}
		if restoredState != randomState {
			rt.Errorf("State not restored correctly after manager recreation: expected %s, got %s", randomState, restoredState)
		}

		anotherState := validStates[rapid.IntRange(0, len(validStates)-1).Draw(rt, "anotherState")]
		err = newManager.SetState(anotherState)
		if err != nil {
			rt.Fatalf("Failed to set new state %s: %v", anotherState, err)
		}

		finalState, err := newManager.GetCurrentState()
		if err != nil {
			rt.Fatalf("Failed to get final state: %v", err)
		}
		if finalState != anotherState {
			rt.Errorf("Final state not persisted correctly: expected %s, got %s", anotherState, finalState)
		}

		thirdManager := NewQuestStateManager(settingsRepo)
		finalRestoredState, err := thirdManager.GetCurrentState()
		if err != nil {
			rt.Fatalf("Failed to restore final state: %v", err)
		}
		if finalRestoredState != anotherState {
			rt.Errorf("Final state not restored correctly: expected %s, got %s", anotherState, finalRestoredState)
		}
	})
}

func TestProperty4_StateValidationAndTransitions(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForQuestState(t)
		defer cleanup()

		settingsRepo := db.NewSettingsRepository(queue)
		manager := NewQuestStateManager(settingsRepo)

		validStates := []QuestState{
			QuestStateNotStarted,
			QuestStateRunning,
			QuestStatePaused,
			QuestStateCompleted,
		}

		initialState := validStates[rapid.IntRange(0, len(validStates)-1).Draw(rt, "initialState")]
		err := manager.SetState(initialState)
		if err != nil {
			rt.Fatalf("Failed to set initial valid state %s: %v", initialState, err)
		}

		currentState, err := manager.GetCurrentState()
		if err != nil {
			rt.Fatalf("Failed to get current state: %v", err)
		}
		if currentState != initialState {
			rt.Errorf("Expected state %s, got %s", initialState, currentState)
		}

		newState := validStates[rapid.IntRange(0, len(validStates)-1).Draw(rt, "newState")]
		err = manager.SetState(newState)
		if err != nil {
			rt.Fatalf("Failed to set new valid state %s: %v", newState, err)
		}

		finalState, err := manager.GetCurrentState()
		if err != nil {
			rt.Fatalf("Failed to get final state: %v", err)
		}
		if finalState != newState {
			rt.Errorf("Expected final state %s, got %s", newState, finalState)
		}

		invalidState := QuestState("invalid_state")
		err = manager.SetState(invalidState)
		if err == nil {
			rt.Error("Expected error when setting invalid state, but got none")
		}

		stateAfterInvalid, err := manager.GetCurrentState()
		if err != nil {
			rt.Fatalf("Failed to get state after invalid attempt: %v", err)
		}
		if stateAfterInvalid != newState {
			rt.Errorf("State changed after invalid attempt: expected %s, got %s", newState, stateAfterInvalid)
		}

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		isAdmin := rapid.Bool().Draw(rt, "isAdmin")

		allowed := manager.IsUserAllowed(userID, isAdmin)

		if isAdmin {
			if !allowed {
				rt.Error("Admin should always be allowed regardless of quest state")
			}
		} else {
			expectedAllowed := (finalState == QuestStateRunning)
			if allowed != expectedAllowed {
				rt.Errorf("Regular user access incorrect: state=%s, expected=%v, got=%v",
					finalState, expectedAllowed, allowed)
			}
		}

		if finalState != QuestStateRunning {
			message := manager.GetStateMessage(finalState)
			if message == "" {
				rt.Errorf("Expected non-empty message for state %s", finalState)
			}
		}
	})
}

func TestProperty5_DataPreservationDuringStateChanges(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForDataPreservation(t)
		defer cleanup()

		settingsRepo := db.NewSettingsRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)
		chatStateRepo := db.NewChatStateRepository(queue)
		manager := NewQuestStateManager(settingsRepo)

		// Generate random user data
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		stepID := rapid.Int64Range(1, 100).Draw(rt, "stepID")

		// Create user
		user := &models.User{
			ID:        userID,
			FirstName: rapid.StringOf(rapid.Rune()).Draw(rt, "firstName"),
			LastName:  rapid.StringOf(rapid.Rune()).Draw(rt, "lastName"),
			Username:  rapid.StringOf(rapid.Rune()).Draw(rt, "username"),
			IsBlocked: rapid.Bool().Draw(rt, "isBlocked"),
		}
		err := userRepo.CreateOrUpdate(user)
		if err != nil {
			rt.Fatalf("Failed to create user: %v", err)
		}

		// Create user progress
		progress := &models.UserProgress{
			UserID: userID,
			StepID: stepID,
			Status: models.StatusPending,
		}
		err = progressRepo.Create(progress)
		if err != nil {
			rt.Fatalf("Failed to create progress: %v", err)
		}

		// Create user answer
		textAnswer := rapid.StringOf(rapid.Rune()).Draw(rt, "textAnswer")
		answerID, err := answerRepo.CreateTextAnswer(userID, stepID, textAnswer, false)
		if err != nil {
			rt.Fatalf("Failed to create answer: %v", err)
		}

		// Create chat state
		chatState := &models.ChatState{
			UserID:                  userID,
			LastTaskMessageID:       rapid.IntRange(1, 10000).Draw(rt, "taskMsgID"),
			LastUserAnswerMessageID: rapid.IntRange(1, 10000).Draw(rt, "answerMsgID"),
			LastReactionMessageID:   rapid.IntRange(1, 10000).Draw(rt, "reactionMsgID"),
		}
		err = chatStateRepo.Save(chatState)
		if err != nil {
			rt.Fatalf("Failed to create chat state: %v", err)
		}

		// Capture initial data state
		initialUser, err := userRepo.GetByID(userID)
		if err != nil {
			rt.Fatalf("Failed to get initial user: %v", err)
		}

		initialProgress, err := progressRepo.GetByUserAndStep(userID, stepID)
		if err != nil {
			rt.Fatalf("Failed to get initial progress: %v", err)
		}

		initialAnswerCount, err := answerRepo.CountUserAnswers(userID)
		if err != nil {
			rt.Fatalf("Failed to get initial answer count: %v", err)
		}

		initialChatState, err := chatStateRepo.Get(userID)
		if err != nil {
			rt.Fatalf("Failed to get initial chat state: %v", err)
		}

		// Perform multiple state transitions
		validStates := []QuestState{
			QuestStateNotStarted,
			QuestStateRunning,
			QuestStatePaused,
			QuestStateCompleted,
		}

		numTransitions := rapid.IntRange(1, 5).Draw(rt, "numTransitions")
		for i := 0; i < numTransitions; i++ {
			newState := validStates[rapid.IntRange(0, len(validStates)-1).Draw(rt, "newState")]
			err = manager.SetState(newState)
			if err != nil {
				rt.Fatalf("Failed to set state %s: %v", newState, err)
			}
		}

		// Verify all user data is preserved after state transitions
		finalUser, err := userRepo.GetByID(userID)
		if err != nil {
			rt.Fatalf("Failed to get final user: %v", err)
		}

		if finalUser.ID != initialUser.ID ||
			finalUser.FirstName != initialUser.FirstName ||
			finalUser.LastName != initialUser.LastName ||
			finalUser.Username != initialUser.Username ||
			finalUser.IsBlocked != initialUser.IsBlocked {
			rt.Errorf("User data changed during state transitions: initial=%+v, final=%+v", initialUser, finalUser)
		}

		finalProgress, err := progressRepo.GetByUserAndStep(userID, stepID)
		if err != nil {
			rt.Fatalf("Failed to get final progress: %v", err)
		}

		if finalProgress.UserID != initialProgress.UserID ||
			finalProgress.StepID != initialProgress.StepID ||
			finalProgress.Status != initialProgress.Status {
			rt.Errorf("Progress data changed during state transitions: initial=%+v, final=%+v", initialProgress, finalProgress)
		}

		finalAnswerCount, err := answerRepo.CountUserAnswers(userID)
		if err != nil {
			rt.Fatalf("Failed to get final answer count: %v", err)
		}

		if finalAnswerCount != initialAnswerCount {
			rt.Errorf("Answer count changed during state transitions: initial=%d, final=%d", initialAnswerCount, finalAnswerCount)
		}

		finalChatState, err := chatStateRepo.Get(userID)
		if err != nil {
			rt.Fatalf("Failed to get final chat state: %v", err)
		}

		if finalChatState.UserID != initialChatState.UserID ||
			finalChatState.LastTaskMessageID != initialChatState.LastTaskMessageID ||
			finalChatState.LastUserAnswerMessageID != initialChatState.LastUserAnswerMessageID ||
			finalChatState.LastReactionMessageID != initialChatState.LastReactionMessageID {
			rt.Errorf("Chat state changed during state transitions: initial=%+v, final=%+v", initialChatState, finalChatState)
		}

		// Verify that the answer ID is still valid and accessible
		if answerID <= 0 {
			rt.Error("Answer ID should be positive after state transitions")
		}
	})
}

func TestProperty7_ErrorHandlingStability(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Test with a database that will return errors
		sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
		if err != nil {
			rt.Fatal(err)
		}
		defer sqlDB.Close()

		// Create initial schema
		_, err = sqlDB.Exec(`
			CREATE TABLE IF NOT EXISTS settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)
		`)
		if err != nil {
			rt.Fatal(err)
		}

		queue := db.NewDBQueueForTest(sqlDB)
		defer queue.Close()
		settingsRepo := db.NewSettingsRepository(queue)
		manager := NewQuestStateManager(settingsRepo)

		// Test GetCurrentState with missing key (sql.ErrNoRows) - should fallback gracefully
		state, err := manager.GetCurrentState()
		if err != nil {
			rt.Fatalf("GetCurrentState should handle missing key gracefully, but got error: %v", err)
		}
		// Should fallback to not_started when key is missing
		if state != QuestStateNotStarted {
			rt.Errorf("Expected fallback to not_started state when key missing, got: %s", state)
		}

		// Test with invalid state in database
		_, err = sqlDB.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('quest_state', 'invalid_state')")
		if err != nil {
			rt.Fatal(err)
		}

		state, err = manager.GetCurrentState()
		if err != nil {
			rt.Fatalf("GetCurrentState should handle invalid state gracefully, but got error: %v", err)
		}
		// Should fallback to running state when invalid state is found
		if state != QuestStateRunning {
			rt.Errorf("Expected fallback to running state for invalid state, got: %s", state)
		}

		// Test IsUserAllowed with database errors - should allow access
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		isAdmin := rapid.Bool().Draw(rt, "isAdmin")

		allowed := manager.IsUserAllowed(userID, isAdmin)
		if !allowed && !isAdmin {
			// For regular users, should allow access when database has invalid state (fallback to running)
			rt.Error("IsUserAllowed should allow access when database has invalid state")
		}
		if !allowed && isAdmin {
			rt.Error("Admin should always be allowed regardless of database state")
		}

		// Test SetState with valid states - should work normally
		validStates := []QuestState{
			QuestStateNotStarted,
			QuestStateRunning,
			QuestStatePaused,
			QuestStateCompleted,
		}
		randomState := validStates[rapid.IntRange(0, len(validStates)-1).Draw(rt, "randomState")]

		err = manager.SetState(randomState)
		if err != nil {
			rt.Fatalf("SetState should work with valid state, but got error: %v", err)
		}

		// Test GetStateMessage with missing message key - should return default message
		// Drop the message table to simulate missing messages
		_, err = sqlDB.Exec("DELETE FROM settings WHERE key LIKE 'quest_%_message'")
		if err != nil {
			rt.Fatal(err)
		}

		message := manager.GetStateMessage(QuestStatePaused)
		expectedDefault := "Квест временно приостановлен. Скоро мы продолжим!"
		if message != expectedDefault {
			rt.Errorf("Expected default message when message key is missing, got: %s", message)
		}

		// Test invalid state handling
		invalidState := QuestState("invalid_" + rapid.StringOf(rapid.Rune()).Draw(rt, "invalidSuffix"))
		err = manager.SetState(invalidState)
		if err == nil {
			rt.Error("SetState should return error for invalid state")
		}

		// Test that system remains stable after error conditions
		// Verify we can still call methods without panicking
		currentState, err := manager.GetCurrentState()
		if err != nil {
			rt.Fatalf("GetCurrentState should work after error conditions: %v", err)
		}

		// Should be the valid state we set earlier
		if currentState != randomState {
			rt.Errorf("Expected state %s after error conditions, got %s", randomState, currentState)
		}

		_ = manager.IsUserAllowed(userID, isAdmin)
		_ = manager.GetStateMessage(QuestStateCompleted)
	})
}
