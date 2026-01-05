package services

import (
	"database/sql"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type StepStats struct {
	StepID    int64
	StepOrder int
	Count     int
}

type Statistics struct {
	StepStats []StepStats
	Leaders   []*models.User
}

type ExtendedStatistics struct {
	StepStats           []StepStats
	Leaders             []*models.User
	TotalAchievements   int
	AchievementsByUser  map[int64]int
	TopAchievementUsers []UserAchievementStats
}

type UserAchievementStats struct {
	User             *models.User
	AchievementCount int
}

type StatisticsService struct {
	queue           *db.DBQueue
	stepRepo        *db.StepRepository
	progressRepo    *db.ProgressRepository
	userRepo        *db.UserRepository
	achievementRepo *db.AchievementRepository
}

func NewStatisticsService(queue *db.DBQueue, stepRepo *db.StepRepository, progressRepo *db.ProgressRepository, userRepo *db.UserRepository) *StatisticsService {
	return &StatisticsService{
		queue:           queue,
		stepRepo:        stepRepo,
		progressRepo:    progressRepo,
		userRepo:        userRepo,
		achievementRepo: nil,
	}
}

func NewStatisticsServiceWithAchievements(queue *db.DBQueue, stepRepo *db.StepRepository, progressRepo *db.ProgressRepository, userRepo *db.UserRepository, achievementRepo *db.AchievementRepository) *StatisticsService {
	return &StatisticsService{
		queue:           queue,
		stepRepo:        stepRepo,
		progressRepo:    progressRepo,
		userRepo:        userRepo,
		achievementRepo: achievementRepo,
	}
}

func (s *StatisticsService) CalculateStats() (*Statistics, error) {
	steps, err := s.stepRepo.GetActive()
	if err != nil {
		return nil, err
	}

	var stepStats []StepStats
	for _, step := range steps {
		count, err := s.progressRepo.CountByStep(step.ID, models.StatusApproved)
		if err != nil {
			return nil, err
		}
		stepStats = append(stepStats, StepStats{
			StepID:    step.ID,
			StepOrder: step.StepOrder,
			Count:     count,
		})
	}

	leaders, err := s.GetLeaders()
	if err != nil {
		return nil, err
	}

	return &Statistics{
		StepStats: stepStats,
		Leaders:   leaders,
	}, nil
}

func (s *StatisticsService) GetLeaders() ([]*models.User, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT u.id, u.first_name, u.last_name, u.username, u.created_at, 
			       COALESCE(MAX(st.step_order), 0) as max_step
			FROM users u
			LEFT JOIN user_progress p ON u.id = p.user_id AND p.status = 'approved'
			LEFT JOIN steps st ON p.step_id = st.id AND st.is_active = TRUE AND st.is_deleted = FALSE
			GROUP BY u.id
			ORDER BY max_step DESC, u.created_at ASC
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var users []*models.User
		for rows.Next() {
			var user models.User
			var firstName, lastName, username sql.NullString
			var maxStep int
			if err := rows.Scan(&user.ID, &firstName, &lastName, &username, &user.CreatedAt, &maxStep); err != nil {
				return nil, err
			}
			user.FirstName = firstName.String
			user.LastName = lastName.String
			user.Username = username.String
			users = append(users, &user)
		}
		return users, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.User), nil
}

func (s *StatisticsService) GetUserMaxStep(userID int64) (int, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var maxStep int
		err := db.QueryRow(`
			SELECT COALESCE(MAX(st.step_order), 0)
			FROM user_progress p
			JOIN steps st ON p.step_id = st.id AND st.is_active = TRUE AND st.is_deleted = FALSE
			WHERE p.user_id = ? AND p.status = 'approved'
		`, userID).Scan(&maxStep)
		return maxStep, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (s *StatisticsService) GetUserLeaderboardPosition(userID int64) (int, int, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var userMaxStep int
		var userCreatedAt string
		err := db.QueryRow(`
			SELECT COALESCE(MAX(st.step_order), 0), u.created_at
			FROM users u
			LEFT JOIN user_progress p ON u.id = p.user_id AND p.status = 'approved'
			LEFT JOIN steps st ON p.step_id = st.id AND st.is_active = TRUE AND st.is_deleted = FALSE
			WHERE u.id = ?
			GROUP BY u.id, u.created_at
		`, userID).Scan(&userMaxStep, &userCreatedAt)
		if err != nil {
			return nil, err
		}

		var position int
		err = db.QueryRow(`
			SELECT COUNT(*) + 1 as position
			FROM (
				SELECT u.id, COALESCE(MAX(st.step_order), 0) as max_step, u.created_at
				FROM users u
				LEFT JOIN user_progress p ON u.id = p.user_id AND p.status = 'approved'
				LEFT JOIN steps st ON p.step_id = st.id AND st.is_active = TRUE AND st.is_deleted = FALSE
				WHERE u.id != ?
				GROUP BY u.id, u.created_at
			) ranked
			WHERE max_step > ? 
			   OR (max_step = ? AND created_at < ?)
		`, userID, userMaxStep, userMaxStep, userCreatedAt).Scan(&position)
		if err != nil {
			return nil, err
		}

		var total int
		err = db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total)
		if err != nil {
			return nil, err
		}

		return []int{position, total}, nil
	})
	if err != nil {
		return 0, 0, err
	}

	results := result.([]int)
	return results[0], results[1], nil
}

func (s *StatisticsService) GetUserProgress(userID int64) (int, int, float64, error) {
	activeCount, err := s.stepRepo.GetActiveStepsCount()
	if err != nil {
		return 0, 0, 0, err
	}

	answeredCount, err := s.stepRepo.GetAnsweredStepsCount(userID)
	if err != nil {
		return 0, 0, 0, err
	}

	var percentage float64
	if activeCount > 0 {
		percentage = float64(answeredCount) / float64(activeCount) * 100
	}

	return answeredCount, activeCount, percentage, nil
}

func (s *StatisticsService) GetUserAchievementCount(userID int64) (int, error) {
	if s.achievementRepo == nil {
		return 0, nil
	}
	return s.achievementRepo.CountUserAchievements(userID)
}

func (s *StatisticsService) CalculateExtendedStats() (*ExtendedStatistics, error) {
	basicStats, err := s.CalculateStats()
	if err != nil {
		return nil, err
	}

	extStats := &ExtendedStatistics{
		StepStats:          basicStats.StepStats,
		Leaders:            basicStats.Leaders,
		AchievementsByUser: make(map[int64]int),
	}

	if s.achievementRepo == nil {
		return extStats, nil
	}

	achievementStats, err := s.achievementRepo.GetAchievementStats()
	if err != nil {
		return extStats, nil
	}

	totalAchievements := 0
	for _, count := range achievementStats {
		totalAchievements += count
	}
	extStats.TotalAchievements = totalAchievements

	userCounts, err := s.achievementRepo.GetUsersWithAchievementCount()
	if err != nil {
		return extStats, nil
	}
	extStats.AchievementsByUser = userCounts

	type userCount struct {
		userID int64
		count  int
	}
	var sortedUsers []userCount
	for userID, count := range userCounts {
		sortedUsers = append(sortedUsers, userCount{userID, count})
	}

	for i := 0; i < len(sortedUsers)-1; i++ {
		for j := i + 1; j < len(sortedUsers); j++ {
			if sortedUsers[j].count > sortedUsers[i].count {
				sortedUsers[i], sortedUsers[j] = sortedUsers[j], sortedUsers[i]
			}
		}
	}

	limit := 10
	if len(sortedUsers) < limit {
		limit = len(sortedUsers)
	}

	for i := 0; i < limit; i++ {
		user, err := s.userRepo.GetByID(sortedUsers[i].userID)
		if err != nil {
			continue
		}
		extStats.TopAchievementUsers = append(extStats.TopAchievementUsers, UserAchievementStats{
			User:             user,
			AchievementCount: sortedUsers[i].count,
		})
	}

	return extStats, nil
}

func (s *StatisticsService) GetUserStatisticsWithAchievements(userID int64) (*UserStatisticsWithAchievements, error) {
	stats := &UserStatisticsWithAchievements{}

	answered, total, percentage, err := s.GetUserProgress(userID)
	if err != nil {
		return nil, err
	}
	stats.AnsweredSteps = answered
	stats.TotalSteps = total
	stats.ProgressPercentage = percentage

	position, totalUsers, err := s.GetUserLeaderboardPosition(userID)
	if err != nil {
		return nil, err
	}
	stats.LeaderboardPosition = position
	stats.TotalUsers = totalUsers

	if s.achievementRepo != nil {
		achievementCount, err := s.achievementRepo.CountUserAchievements(userID)
		if err == nil {
			stats.AchievementCount = achievementCount
		}
	}

	return stats, nil
}

type UserStatisticsWithAchievements struct {
	AnsweredSteps       int
	TotalSteps          int
	ProgressPercentage  float64
	LeaderboardPosition int
	TotalUsers          int
	AchievementCount    int
}
