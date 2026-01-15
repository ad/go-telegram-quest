package services

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
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
			is_asterisk BOOLEAN DEFAULT FALSE,
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

	queue := db.NewDBQueue(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func TestProperty2_ProgressRestorationConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		resolver := NewStateResolver(stepRepo, progressRepo, userRepo)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		numSteps := rapid.IntRange(1, 10).Draw(rt, "numSteps")

		var stepIDs []int64
		for i := 1; i <= numSteps; i++ {
			step := &models.Step{
				StepOrder:  i,
				Text:       "Step " + string(rune('0'+i)),
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			id, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}
			stepIDs = append(stepIDs, id)
		}

		numApproved := rapid.IntRange(0, numSteps).Draw(rt, "numApproved")
		for i := 0; i < numApproved; i++ {
			progress := &models.UserProgress{
				UserID: userID,
				StepID: stepIDs[i],
				Status: models.StatusApproved,
			}
			if err := progressRepo.Create(progress); err != nil {
				rt.Fatal(err)
			}
		}

		state, err := resolver.ResolveState(userID)
		if err != nil {
			rt.Fatal(err)
		}

		if numApproved == numSteps {
			if !state.IsCompleted {
				rt.Errorf("Expected quest to be completed when all %d steps are approved", numSteps)
			}
		} else {
			if state.IsCompleted {
				rt.Errorf("Expected quest to not be completed when only %d of %d steps are approved", numApproved, numSteps)
			}
			if state.CurrentStep == nil {
				rt.Fatal("Expected current step to be set")
			}
			expectedStepID := stepIDs[numApproved]
			if state.CurrentStep.ID != expectedStepID {
				rt.Errorf("Expected current step ID %d, got %d", expectedStepID, state.CurrentStep.ID)
			}
		}
	})
}

func TestProperty4_StepCompletionTransitions(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		resolver := NewStateResolver(stepRepo, progressRepo, userRepo)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		numSteps := rapid.IntRange(2, 10).Draw(rt, "numSteps")

		var stepIDs []int64
		for i := 1; i <= numSteps; i++ {
			step := &models.Step{
				StepOrder:  i,
				Text:       "Step " + string(rune('0'+i)),
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			id, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}
			stepIDs = append(stepIDs, id)
		}

		stateBefore, err := resolver.ResolveState(userID)
		if err != nil {
			rt.Fatal(err)
		}

		if stateBefore.CurrentStep == nil {
			rt.Fatal("Expected current step before completion")
		}
		currentStepID := stateBefore.CurrentStep.ID

		progress := &models.UserProgress{
			UserID: userID,
			StepID: currentStepID,
			Status: models.StatusApproved,
		}
		if err := progressRepo.Create(progress); err != nil {
			rt.Fatal(err)
		}

		stateAfter, err := resolver.ResolveState(userID)
		if err != nil {
			rt.Fatal(err)
		}

		if stateAfter.IsCompleted {
			if numSteps > 1 {
				rt.Errorf("Quest should not be completed after approving only first step")
			}
		} else {
			if stateAfter.CurrentStep == nil {
				rt.Fatal("Expected next step after completion")
			}
			if stateAfter.CurrentStep.StepOrder <= stateBefore.CurrentStep.StepOrder {
				rt.Errorf("Next step order (%d) should be greater than current (%d)",
					stateAfter.CurrentStep.StepOrder, stateBefore.CurrentStep.StepOrder)
			}
		}
	})
}

func TestStateResolver_SkippedStepsNotReturnedAsCurrent(t *testing.T) {
	queue, cleanup := setupTestDB(t)
	defer cleanup()

	stepRepo := db.NewStepRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	userRepo := db.NewUserRepository(queue)
	resolver := NewStateResolver(stepRepo, progressRepo, userRepo)

	userID := int64(12345)

	step1 := &models.Step{
		StepOrder:  1,
		Text:       "Step 1",
		AnswerType: models.AnswerTypeText,
		IsActive:   true,
		IsAsterisk: true,
	}
	step1ID, err := stepRepo.Create(step1)
	if err != nil {
		t.Fatal(err)
	}

	step2 := &models.Step{
		StepOrder:  2,
		Text:       "Step 2",
		AnswerType: models.AnswerTypeText,
		IsActive:   true,
	}
	step2ID, err := stepRepo.Create(step2)
	if err != nil {
		t.Fatal(err)
	}

	if err := progressRepo.CreateSkipped(userID, step1ID); err != nil {
		t.Fatalf("Failed to skip step 1: %v", err)
	}

	state, err := resolver.ResolveState(userID)
	if err != nil {
		t.Fatalf("Failed to resolve state: %v", err)
	}

	if state.IsCompleted {
		t.Error("Quest should not be completed after skipping first step")
	}

	if state.CurrentStep == nil {
		t.Fatal("Expected current step to be set")
	}

	if state.CurrentStep.ID != step2ID {
		t.Errorf("Expected current step to be step 2 (ID %d), got step ID %d", step2ID, state.CurrentStep.ID)
	}

	if state.CurrentStep.StepOrder != 2 {
		t.Errorf("Expected current step order to be 2, got %d", state.CurrentStep.StepOrder)
	}
}
