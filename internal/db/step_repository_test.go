package db

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (*sql.DB, *StepRepository) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}

	queue := NewDBQueue(db)
	repo := NewStepRepository(queue)

	return db, repo
}

func createTestStep(t *testing.T, repo *StepRepository, text string) int64 {
	maxOrder, _ := repo.GetMaxOrder()
	step := &models.Step{
		StepOrder:    maxOrder + 1,
		Text:         text,
		AnswerType:   models.AnswerTypeText,
		HasAutoCheck: false,
		IsActive:     true,
		IsDeleted:    false,
	}

	id, err := repo.Create(step)
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func TestSwapStepOrder(t *testing.T) {
	_, repo := setupTestDB(t)

	step1ID := createTestStep(t, repo, "Step 1")
	step2ID := createTestStep(t, repo, "Step 2")
	step3ID := createTestStep(t, repo, "Step 3")

	step1Before, _ := repo.GetByID(step1ID)
	step3Before, _ := repo.GetByID(step3ID)

	err := repo.SwapStepOrder(step1ID, step3ID)
	if err != nil {
		t.Fatalf("SwapStepOrder failed: %v", err)
	}

	step1After, _ := repo.GetByID(step1ID)
	step3After, _ := repo.GetByID(step3ID)

	if step1After.StepOrder != step3Before.StepOrder {
		t.Errorf("Expected step1 order to be %d, got %d", step3Before.StepOrder, step1After.StepOrder)
	}

	if step3After.StepOrder != step1Before.StepOrder {
		t.Errorf("Expected step3 order to be %d, got %d", step1Before.StepOrder, step3After.StepOrder)
	}

	step2, _ := repo.GetByID(step2ID)
	if step2.StepOrder != 2 {
		t.Errorf("Expected step2 order to remain 2, got %d", step2.StepOrder)
	}
}

func TestMoveStepUp(t *testing.T) {
	_, repo := setupTestDB(t)

	createTestStep(t, repo, "Step 1")
	step2ID := createTestStep(t, repo, "Step 2")
	createTestStep(t, repo, "Step 3")

	step2Before, _ := repo.GetByID(step2ID)

	err := repo.MoveStepUp(step2ID)
	if err != nil {
		t.Fatalf("MoveStepUp failed: %v", err)
	}

	step2After, _ := repo.GetByID(step2ID)
	if step2After.StepOrder >= step2Before.StepOrder {
		t.Errorf("Step should have moved up, before: %d, after: %d", step2Before.StepOrder, step2After.StepOrder)
	}
}

func TestMoveStepDown(t *testing.T) {
	_, repo := setupTestDB(t)

	step1ID := createTestStep(t, repo, "Step 1")
	createTestStep(t, repo, "Step 2")
	createTestStep(t, repo, "Step 3")

	step1Before, _ := repo.GetByID(step1ID)

	err := repo.MoveStepDown(step1ID)
	if err != nil {
		t.Fatalf("MoveStepDown failed: %v", err)
	}

	step1After, _ := repo.GetByID(step1ID)
	if step1After.StepOrder <= step1Before.StepOrder {
		t.Errorf("Step should have moved down, before: %d, after: %d", step1Before.StepOrder, step1After.StepOrder)
	}
}

func TestCanMoveUp(t *testing.T) {
	_, repo := setupTestDB(t)

	step1ID := createTestStep(t, repo, "Step 1")
	step2ID := createTestStep(t, repo, "Step 2")

	steps, _ := repo.GetAll()
	var step1Order, step2Order int
	for _, step := range steps {
		if step.ID == step1ID {
			step1Order = step.StepOrder
		}
		if step.ID == step2ID {
			step2Order = step.StepOrder
		}
	}

	canMove, err := repo.CanMoveUp(step1ID)
	if err != nil {
		t.Fatal(err)
	}
	if canMove && step1Order == 1 {
		t.Error("First step should not be able to move up")
	}

	canMove, err = repo.CanMoveUp(step2ID)
	if err != nil {
		t.Fatal(err)
	}
	if !canMove && step2Order > 1 {
		t.Error("Second step should be able to move up")
	}
}

func TestCanMoveDown(t *testing.T) {
	_, repo := setupTestDB(t)

	step1ID := createTestStep(t, repo, "Step 1")
	step2ID := createTestStep(t, repo, "Step 2")

	canMove, err := repo.CanMoveDown(step1ID)
	if err != nil {
		t.Fatal(err)
	}
	if !canMove {
		t.Error("First step should be able to move down")
	}

	canMove, err = repo.CanMoveDown(step2ID)
	if err != nil {
		t.Fatal(err)
	}
	if canMove {
		t.Error("Last step should not be able to move down")
	}
}
