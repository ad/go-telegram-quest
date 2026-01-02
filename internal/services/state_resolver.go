package services

import (
	"database/sql"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type UserState struct {
	UserID      int64
	CurrentStep *models.Step
	Status      models.ProgressStatus
	IsCompleted bool
}

type StateResolver struct {
	stepRepo     *db.StepRepository
	progressRepo *db.ProgressRepository
	userRepo     *db.UserRepository
}

func NewStateResolver(stepRepo *db.StepRepository, progressRepo *db.ProgressRepository, userRepo *db.UserRepository) *StateResolver {
	return &StateResolver{
		stepRepo:     stepRepo,
		progressRepo: progressRepo,
		userRepo:     userRepo,
	}
}

func (r *StateResolver) ResolveState(userID int64) (*UserState, error) {
	activeSteps, err := r.stepRepo.GetActive()
	if err != nil {
		return nil, err
	}

	if len(activeSteps) == 0 {
		return &UserState{
			UserID:      userID,
			IsCompleted: true,
		}, nil
	}

	userProgress, err := r.progressRepo.GetUserProgress(userID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	approvedSteps := make(map[int64]bool)
	progressByStep := make(map[int64]*models.UserProgress)
	for _, p := range userProgress {
		progressByStep[p.StepID] = p
		if p.Status == models.StatusApproved {
			approvedSteps[p.StepID] = true
		}
	}

	for _, step := range activeSteps {
		if approvedSteps[step.ID] {
			continue
		}

		status := models.StatusPending
		if progress, exists := progressByStep[step.ID]; exists {
			status = progress.Status
		}

		return &UserState{
			UserID:      userID,
			CurrentStep: step,
			Status:      status,
			IsCompleted: false,
		}, nil
	}

	return &UserState{
		UserID:      userID,
		IsCompleted: true,
	}, nil
}
