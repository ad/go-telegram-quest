package services

import (
	"database/sql"
	"sort"

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

type StatisticsService struct {
	queue        *db.DBQueue
	stepRepo     *db.StepRepository
	progressRepo *db.ProgressRepository
	userRepo     *db.UserRepository
}

func NewStatisticsService(queue *db.DBQueue, stepRepo *db.StepRepository, progressRepo *db.ProgressRepository, userRepo *db.UserRepository) *StatisticsService {
	return &StatisticsService{
		queue:        queue,
		stepRepo:     stepRepo,
		progressRepo: progressRepo,
		userRepo:     userRepo,
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

type userWithMaxStep struct {
	user    *models.User
	maxStep int
}

func sortLeadersByMaxStep(users []userWithMaxStep) []*models.User {
	sort.Slice(users, func(i, j int) bool {
		if users[i].maxStep != users[j].maxStep {
			return users[i].maxStep > users[j].maxStep
		}
		return users[i].user.CreatedAt.Before(users[j].user.CreatedAt)
	})

	result := make([]*models.User, len(users))
	for i, u := range users {
		result[i] = u.user
	}
	return result
}
