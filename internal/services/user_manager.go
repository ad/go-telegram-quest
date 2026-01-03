package services

import (
	"database/sql"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

const UsersPerPage = 10

type UserListPage struct {
	Users       []*models.User
	CurrentPage int
	TotalPages  int
	HasPrev     bool
	HasNext     bool
}

type UserDetails struct {
	User        *models.User
	CurrentStep *models.Step
	Status      models.ProgressStatus
	IsCompleted bool
}

type UserManager struct {
	userRepo     *db.UserRepository
	stepRepo     *db.StepRepository
	progressRepo *db.ProgressRepository
}

func NewUserManager(userRepo *db.UserRepository, stepRepo *db.StepRepository, progressRepo *db.ProgressRepository) *UserManager {
	return &UserManager{
		userRepo:     userRepo,
		stepRepo:     stepRepo,
		progressRepo: progressRepo,
	}
}

func (m *UserManager) GetUserListPage(page int) (*UserListPage, error) {
	if page < 1 {
		page = 1
	}

	allUsers, err := m.userRepo.GetAll()
	if err != nil {
		return nil, err
	}

	totalUsers := len(allUsers)
	totalPages := (totalUsers + UsersPerPage - 1) / UsersPerPage
	if totalPages == 0 {
		totalPages = 1
	}

	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * UsersPerPage
	end := start + UsersPerPage
	if end > totalUsers {
		end = totalUsers
	}

	var pageUsers []*models.User
	if start < totalUsers {
		pageUsers = allUsers[start:end]
	}

	return &UserListPage{
		Users:       pageUsers,
		CurrentPage: page,
		TotalPages:  totalPages,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
	}, nil
}

func (m *UserManager) GetUserDetails(userID int64) (*UserDetails, error) {
	user, err := m.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}

	activeSteps, err := m.stepRepo.GetActive()
	if err != nil {
		return nil, err
	}

	if len(activeSteps) == 0 {
		return &UserDetails{
			User:        user,
			IsCompleted: true,
		}, nil
	}

	userProgress, err := m.progressRepo.GetUserProgress(userID)
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

		return &UserDetails{
			User:        user,
			CurrentStep: step,
			Status:      status,
			IsCompleted: false,
		}, nil
	}

	return &UserDetails{
		User:        user,
		IsCompleted: true,
	}, nil
}
