package services

import (
	"database/sql"
	"time"

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
	User             *models.User
	CurrentStep      *models.Step
	Status           models.ProgressStatus
	IsCompleted      bool
	Statistics       *UserStatistics
	AchievementCount int
	Achievements     []*UserAchievementInfo
}

type UserAchievementInfo struct {
	Name     string
	Category models.AchievementCategory
}

type UserStatistics struct {
	FirstAnswerTime       *time.Time
	LastAnswerTime        *time.Time
	CompletionTime        *time.Duration
	TotalAnswers          int
	ApprovedSteps         int
	Accuracy              int
	AverageResponseTime   *time.Duration
	TimeOnCurrentStep     *time.Duration
	RegistrationDate      time.Time
	TimeSinceRegistration time.Duration
	StepAttempts          []StepAttempt
	LeaderboardPosition   int
	TotalUsers            int
}

type StepAttempt struct {
	StepOrder int
	Attempts  int
}

type QuestStatistics struct {
	TotalUsers       int
	CompletedUsers   int
	InProgressUsers  int
	NotStartedUsers  int
	StepDistribution map[int]int    // step_order -> count of users on that step
	StepTitles       map[int]string // step_order -> step text
}

type UserManager struct {
	userRepo       *db.UserRepository
	stepRepo       *db.StepRepository
	progressRepo   *db.ProgressRepository
	answerRepo     *db.AnswerRepository
	chatStateRepo  *db.ChatStateRepository
	statisticsCalc *UserStatisticsCalculator
}

func NewUserManager(userRepo *db.UserRepository, stepRepo *db.StepRepository, progressRepo *db.ProgressRepository, answerRepo *db.AnswerRepository, chatStateRepo *db.ChatStateRepository, statisticsService *StatisticsService) *UserManager {
	return &UserManager{
		userRepo:       userRepo,
		stepRepo:       stepRepo,
		progressRepo:   progressRepo,
		answerRepo:     answerRepo,
		chatStateRepo:  chatStateRepo,
		statisticsCalc: NewUserStatisticsCalculator(answerRepo, progressRepo, statisticsService),
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
		// Calculate statistics even when no active steps
		statistics, err := m.statisticsCalc.Calculate(user, nil)
		if err != nil {
			return nil, err
		}

		return &UserDetails{
			User:        user,
			IsCompleted: true,
			Statistics:  statistics,
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

	var currentStep *models.Step
	var status models.ProgressStatus = models.StatusPending
	isCompleted := true

	for _, step := range activeSteps {
		if approvedSteps[step.ID] {
			continue
		}

		currentStep = step
		isCompleted = false
		if progress, exists := progressByStep[step.ID]; exists {
			status = progress.Status
		}
		break
	}

	// Calculate statistics with current step information
	statistics, err := m.statisticsCalc.Calculate(user, currentStep)
	if err != nil {
		return nil, err
	}

	return &UserDetails{
		User:        user,
		CurrentStep: currentStep,
		Status:      status,
		IsCompleted: isCompleted,
		Statistics:  statistics,
	}, nil
}

func (m *UserManager) ResetUserProgress(userID int64) error {
	if err := m.progressRepo.DeleteUserProgress(userID); err != nil {
		return err
	}

	if err := m.answerRepo.DeleteUserAnswers(userID); err != nil {
		return err
	}

	if err := m.chatStateRepo.Clear(userID); err != nil {
		return err
	}

	return nil
}

func (m *UserManager) GetQuestStatistics() (*QuestStatistics, error) {
	allUsers, err := m.userRepo.GetAll()
	if err != nil {
		return nil, err
	}

	activeSteps, err := m.stepRepo.GetActive()
	if err != nil {
		return nil, err
	}

	stats := &QuestStatistics{
		TotalUsers:       len(allUsers),
		StepDistribution: make(map[int]int),
		StepTitles:       make(map[int]string),
	}

	for _, step := range activeSteps {
		stats.StepTitles[step.StepOrder] = step.Text
	}

	if len(activeSteps) == 0 {
		// No active steps means everyone is "completed"
		stats.CompletedUsers = len(allUsers)
		return stats, nil
	}

	for _, user := range allUsers {
		userProgress, err := m.progressRepo.GetUserProgress(user.ID)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}

		approvedSteps := make(map[int64]bool)
		for _, p := range userProgress {
			if p.Status == models.StatusApproved {
				approvedSteps[p.StepID] = true
			}
		}

		// Find current step
		currentStepOrder := 0
		isCompleted := true
		for _, step := range activeSteps {
			if !approvedSteps[step.ID] {
				currentStepOrder = step.StepOrder
				isCompleted = false
				break
			}
		}

		if isCompleted {
			stats.CompletedUsers++
		} else if currentStepOrder > 0 {
			stats.InProgressUsers++
			stats.StepDistribution[currentStepOrder]++
		} else {
			stats.NotStartedUsers++
		}
	}

	return stats, nil
}
