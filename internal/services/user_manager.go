package services

import (
	"database/sql"
	"log"
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
	userRepo          *db.UserRepository
	stepRepo          *db.StepRepository
	progressRepo      *db.ProgressRepository
	answerRepo        *db.AnswerRepository
	chatStateRepo     *db.ChatStateRepository
	achievementRepo   *db.AchievementRepository
	statisticsCalc    *UserStatisticsCalculator
	achievementEngine *AchievementEngine
}

func NewUserManager(userRepo *db.UserRepository, stepRepo *db.StepRepository, progressRepo *db.ProgressRepository, answerRepo *db.AnswerRepository, chatStateRepo *db.ChatStateRepository, achievementRepo *db.AchievementRepository, statisticsService *StatisticsService, achievementEngine *AchievementEngine) *UserManager {
	return &UserManager{
		userRepo:          userRepo,
		stepRepo:          stepRepo,
		progressRepo:      progressRepo,
		answerRepo:        answerRepo,
		chatStateRepo:     chatStateRepo,
		achievementRepo:   achievementRepo,
		statisticsCalc:    NewUserStatisticsCalculator(answerRepo, progressRepo, statisticsService),
		achievementEngine: achievementEngine,
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

	completedSteps := make(map[int64]bool)
	progressByStep := make(map[int64]*models.UserProgress)
	for _, p := range userProgress {
		progressByStep[p.StepID] = p
		// Шаг считается завершенным если он одобрен или пропущен (для шагов со звездочкой)
		if p.Status == models.StatusApproved || p.Status == models.StatusSkipped {
			completedSteps[p.StepID] = true
		}
	}

	var currentStep *models.Step
	var status models.ProgressStatus = models.StatusPending
	isCompleted := true

	for _, step := range activeSteps {
		if completedSteps[step.ID] {
			continue
		}

		// Для обычных шагов (без звездочки) требуется прохождение
		// Для шагов со звездочкой можно пропустить
		if !step.IsAsterisk {
			currentStep = step
			isCompleted = false
			if progress, exists := progressByStep[step.ID]; exists {
				status = progress.Status
			}
			break
		} else {
			// Шаг со звездочкой не пройден и не пропущен
			if progress, exists := progressByStep[step.ID]; exists {
				if progress.Status != models.StatusApproved && progress.Status != models.StatusSkipped {
					currentStep = step
					isCompleted = false
					status = progress.Status
					break
				}
			} else {
				// Шаг со звездочкой еще не начат
				currentStep = step
				isCompleted = false
				status = models.StatusPending
				break
			}
		}
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

// Achievements that should be preserved during progress reset
var PreservedAchievements = []string{
	"winner_1",
	"winner_2",
	"winner_3",
	"restart",
	"cheater",
}

func (m *UserManager) ResetUserProgress(userID int64) error {
	// Award restart achievement before clearing data
	if m.achievementEngine != nil {
		_, err := m.achievementEngine.OnProgressReset(userID)
		if err != nil {
			log.Printf("[USER_MANAGER] Error awarding restart achievement for user %d: %v", userID, err)
		}
	}

	// Get restart achievement ID for preservation
	restartAchievementID, err := m.getRestartAchievementID(userID)
	if err != nil {
		log.Printf("[USER_MANAGER] Error getting restart achievement ID for user %d: %v", userID, err)
	}

	// First, get achievements that should be preserved
	preservedAchievements, err := m.getPreservedAchievements(userID)
	if err != nil {
		log.Printf("[USER_MANAGER] Error getting preserved achievements for user %d: %v", userID, err)
		// Continue with reset even if we can't preserve achievements
	}

	// Clear user progress, answers, achievements, and chat state
	if err := m.progressRepo.DeleteUserProgress(userID); err != nil {
		return err
	}

	if err := m.answerRepo.DeleteUserAnswers(userID); err != nil {
		return err
	}

	// Delete all user achievements
	if err := m.achievementRepo.DeleteUserAchievements(userID); err != nil {
		return err
	}

	if err := m.chatStateRepo.Clear(userID); err != nil {
		return err
	}

	// Restore preserved achievements
	if len(preservedAchievements) > 0 {
		if err := m.restorePreservedAchievements(userID, preservedAchievements); err != nil {
			log.Printf("[USER_MANAGER] Error restoring preserved achievements for user %d: %v", userID, err)
			// Don't fail the reset if we can't restore achievements
		}
	}

	// Re-assign restart achievement if it was just awarded
	if restartAchievementID > 0 {
		err := m.preserveAchievementOnReset(userID, restartAchievementID)
		if err != nil {
			log.Printf("[USER_MANAGER] Error preserving restart achievement for user %d: %v", userID, err)
		}
	}

	return nil
}

func (m *UserManager) getRestartAchievementID(userID int64) (int64, error) {
	achievement, err := m.achievementRepo.GetByKey("restart")
	if err != nil {
		return 0, err
	}

	hasAchievement, err := m.achievementRepo.HasUserAchievement(userID, "restart")
	if err != nil {
		return 0, err
	}

	if hasAchievement {
		return achievement.ID, nil
	}

	return 0, nil
}

func (m *UserManager) preserveAchievementOnReset(userID int64, achievementID int64) error {
	return m.achievementRepo.AssignToUser(userID, achievementID, time.Now(), false)
}

func (m *UserManager) getPreservedAchievements(userID int64) ([]models.UserAchievement, error) {
	userAchievements, err := m.achievementRepo.GetUserAchievements(userID)
	if err != nil {
		return nil, err
	}

	var preserved []models.UserAchievement
	for _, ua := range userAchievements {
		achievement, err := m.achievementRepo.GetByID(ua.AchievementID)
		if err != nil {
			continue
		}

		// Check if this achievement should be preserved
		for _, preservedKey := range PreservedAchievements {
			if achievement.Key == preservedKey {
				preserved = append(preserved, *ua)
				break
			}
		}
	}

	return preserved, nil
}

func (m *UserManager) restorePreservedAchievements(userID int64, achievements []models.UserAchievement) error {
	for _, ua := range achievements {
		err := m.achievementRepo.AssignToUser(userID, ua.AchievementID, ua.EarnedAt, ua.IsRetroactive)
		if err != nil {
			log.Printf("[USER_MANAGER] Error restoring achievement %d to user %d: %v", ua.AchievementID, userID, err)
			continue
		}
		// log.Printf("[USER_MANAGER] Restored preserved achievement %d to user %d", ua.AchievementID, userID)
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

		completedSteps := make(map[int64]bool)
		progressByStep := make(map[int64]*models.UserProgress)
		for _, p := range userProgress {
			progressByStep[p.StepID] = p
			// Шаг считается завершенным если он одобрен или пропущен (для шагов со звездочкой)
			if p.Status == models.StatusApproved || p.Status == models.StatusSkipped {
				completedSteps[p.StepID] = true
			}
		}

		// Find current step
		currentStepOrder := 0
		isCompleted := true
		for _, step := range activeSteps {
			if completedSteps[step.ID] {
				continue
			}

			// Для обычных шагов (без звездочки) требуется прохождение
			// Для шагов со звездочкой можно пропустить
			if !step.IsAsterisk {
				currentStepOrder = step.StepOrder
				isCompleted = false
				break
			} else {
				// Шаг со звездочкой не пройден и не пропущен
				if progress, exists := progressByStep[step.ID]; exists {
					if progress.Status != models.StatusApproved && progress.Status != models.StatusSkipped {
						currentStepOrder = step.StepOrder
						isCompleted = false
						break
					}
				} else {
					// Шаг со звездочкой еще не начат
					currentStepOrder = step.StepOrder
					isCompleted = false
					break
				}
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
