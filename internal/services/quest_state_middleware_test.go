package services

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func setupTestDBForMiddleware(t *testing.T) (*db.DBQueue, func()) {
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

	queue := db.NewDBQueue(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func TestProperty3_RegularUserStateBasedProcessing(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForMiddleware(t)
		defer cleanup()

		settingsRepo := db.NewSettingsRepository(queue)
		stateManager := NewQuestStateManager(settingsRepo)

		adminID := rapid.Int64Range(1, 1000).Draw(rt, "adminID")
		middleware := NewQuestStateMiddleware(stateManager, adminID)

		allStates := []QuestState{
			QuestStateNotStarted,
			QuestStateRunning,
			QuestStatePaused,
			QuestStateCompleted,
		}

		testState := allStates[rapid.IntRange(0, len(allStates)-1).Draw(rt, "testState")]
		err := stateManager.SetState(testState)
		if err != nil {
			rt.Fatalf("Failed to set test state %s: %v", testState, err)
		}

		regularUserID := rapid.Int64Range(1001, 999999).Draw(rt, "regularUserID")
		if regularUserID == adminID {
			regularUserID = adminID + 1
		}

		shouldProcess, notification := middleware.ShouldProcessMessage(regularUserID)

		if testState == QuestStateRunning {
			if !shouldProcess {
				rt.Errorf("Regular user should be able to process messages when quest is running, but got shouldProcess=false")
			}
			if notification != "" {
				rt.Errorf("Regular user should not receive notification when quest is running, but got: %s", notification)
			}
		} else {
			if shouldProcess {
				rt.Errorf("Regular user should not be able to process messages when quest state is %s, but got shouldProcess=true", testState)
			}
			if notification == "" {
				rt.Errorf("Regular user should receive notification when quest state is %s, but got empty notification", testState)
			}

			expectedMessage := stateManager.GetStateMessage(testState)
			if notification != expectedMessage {
				rt.Errorf("Notification mismatch for state %s: expected %q, got %q", testState, expectedMessage, notification)
			}
		}

		adminShouldProcess, adminNotification := middleware.ShouldProcessMessage(adminID)
		if !adminShouldProcess {
			rt.Errorf("Admin should always be able to process messages regardless of quest state %s, but got shouldProcess=false", testState)
		}
		if adminNotification != "" {
			rt.Errorf("Admin should not receive state restriction notifications, but got: %s", adminNotification)
		}

		stateNotification := middleware.GetStateNotification()
		expectedStateMessage := stateManager.GetStateMessage(testState)
		if testState == QuestStateRunning {
			if stateNotification != "" {
				rt.Errorf("GetStateNotification should return empty string for running state, but got: %s", stateNotification)
			}
		} else {
			if stateNotification != expectedStateMessage {
				rt.Errorf("GetStateNotification mismatch for state %s: expected %q, got %q", testState, expectedStateMessage, stateNotification)
			}
		}
	})
}
