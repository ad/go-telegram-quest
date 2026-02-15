package services

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type StepStats struct {
	StepID    int64
	StepOrder int
	Text      string
	Count     int
}

type AsteriskStepStats struct {
	StepID        int64
	StepOrder     int
	Text          string
	AnsweredCount int
	SkippedCount  int
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
			Text:      step.Text,
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
			       COALESCE(MAX(st.step_order), 0) as max_step,
			       COALESCE((
			           SELECT p2.completed_at
			           FROM user_progress p2
			           JOIN steps st2 ON p2.step_id = st2.id AND st2.is_active = TRUE AND st2.is_deleted = FALSE
			           WHERE p2.user_id = u.id AND p2.status = 'approved'
			           ORDER BY st2.step_order DESC
			           LIMIT 1
			       ), u.created_at) as max_step_completed_at
			FROM users u
			LEFT JOIN user_progress p ON u.id = p.user_id AND p.status = 'approved'
			LEFT JOIN steps st ON p.step_id = st.id AND st.is_active = TRUE AND st.is_deleted = FALSE
			GROUP BY u.id
			ORDER BY max_step DESC, max_step_completed_at ASC
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
			var maxStepCompletedAt string
			if err := rows.Scan(&user.ID, &firstName, &lastName, &username, &user.CreatedAt, &maxStep, &maxStepCompletedAt); err != nil {
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
		var userMaxStepCompletedAt string
		err := db.QueryRow(`
			SELECT COALESCE(MAX(st.step_order), 0),
			       COALESCE((
			           SELECT p2.completed_at
			           FROM user_progress p2
			           JOIN steps st2 ON p2.step_id = st2.id AND st2.is_active = TRUE AND st2.is_deleted = FALSE
			           WHERE p2.user_id = u.id AND p2.status = 'approved'
			           ORDER BY st2.step_order DESC
			           LIMIT 1
			       ), u.created_at)
			FROM users u
			LEFT JOIN user_progress p ON u.id = p.user_id AND p.status = 'approved'
			LEFT JOIN steps st ON p.step_id = st.id AND st.is_active = TRUE AND st.is_deleted = FALSE
			WHERE u.id = ?
			GROUP BY u.id, u.created_at
		`, userID).Scan(&userMaxStep, &userMaxStepCompletedAt)
		if err != nil {
			return nil, err
		}

		var position int
		err = db.QueryRow(`
			SELECT COUNT(*) + 1 as position
			FROM (
				SELECT u.id,
				       COALESCE(MAX(st.step_order), 0) as max_step,
				       COALESCE((
				           SELECT p2.completed_at
				           FROM user_progress p2
				           JOIN steps st2 ON p2.step_id = st2.id AND st2.is_active = TRUE AND st2.is_deleted = FALSE
				           WHERE p2.user_id = u.id AND p2.status = 'approved'
				           ORDER BY st2.step_order DESC
				           LIMIT 1
				       ), u.created_at) as max_step_completed_at
				FROM users u
				LEFT JOIN user_progress p ON u.id = p.user_id AND p.status = 'approved'
				LEFT JOIN steps st ON p.step_id = st.id AND st.is_active = TRUE AND st.is_deleted = FALSE
				WHERE u.id != ?
				GROUP BY u.id, u.created_at
			) ranked
			WHERE max_step > ?
			   OR (max_step = ? AND max_step_completed_at < ?)
		`, userID, userMaxStep, userMaxStep, userMaxStepCompletedAt).Scan(&position)
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

func (s *StatisticsService) GetUserAsteriskStats(userID int64) (answered int, total int, err error) {
	total, err = s.stepRepo.GetAsteriskStepsCount()
	if err != nil {
		return 0, 0, err
	}

	answered, err = s.stepRepo.GetAnsweredAsteriskStepsCount(userID)
	if err != nil {
		return 0, 0, err
	}

	return answered, total, nil
}

func (s *StatisticsService) GetAsteriskStepsStats() ([]AsteriskStepStats, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT 
				s.id,
				s.step_order,
				s.text,
				COALESCE(SUM(CASE WHEN p.status = 'approved' THEN 1 ELSE 0 END), 0) as answered_count,
				COALESCE(SUM(CASE WHEN p.status = 'skipped' THEN 1 ELSE 0 END), 0) as skipped_count
			FROM steps s
			LEFT JOIN user_progress p ON s.id = p.step_id AND (p.status = 'approved' OR p.status = 'skipped')
			WHERE s.is_asterisk = TRUE AND s.is_active = TRUE AND s.is_deleted = FALSE
			GROUP BY s.id, s.step_order
			ORDER BY s.step_order
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var stats []AsteriskStepStats
		for rows.Next() {
			var stat AsteriskStepStats
			if err := rows.Scan(&stat.StepID, &stat.StepOrder, &stat.Text, &stat.AnsweredCount, &stat.SkippedCount); err != nil {
				return nil, err
			}
			// log.Printf("[ASTERISK_STATS] Step %d: answered=%d, skipped=%d", stat.StepOrder, stat.AnsweredCount, stat.SkippedCount)
			stats = append(stats, stat)
		}
		return stats, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]AsteriskStepStats), nil
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

func (s *StatisticsService) FormatCompletionStats(userID int64) string {
	position, totalUsers, err := s.GetUserLeaderboardPosition(userID)
	if err != nil {
		log.Printf("[STATS] GetUserLeaderboardPosition error for user %d: %v", userID, err)
		return ""
	}

	answered, _, _, err := s.GetUserProgress(userID)
	if err != nil {
		log.Printf("[STATS] GetUserProgress error for user %d: %v", userID, err)
		return ""
	}

	totalAnswers, hintsUsed, firstTime, lastTime, err := s.getUserDetailedAnswerStats(userID)
	if err != nil {
		log.Printf("[STATS] getUserDetailedAnswerStats error for user %d: %v", userID, err)
		return ""
	}

	log.Printf("[STATS] User %d: position=%d, totalUsers=%d, answered=%d, totalAnswers=%d, hintsUsed=%d",
		userID, position, totalUsers, answered, totalAnswers, hintsUsed)

	var lines []string

	// Место в рейтинге
	if totalUsers > 1 {
		switch position {
		case 1:
			lines = append(lines, "🥇 Невероятно! Вы раньше других прошли этот квест!")
		case 2:
			lines = append(lines, fmt.Sprintf("🥈 Отлично! Серебро ваше — вам удалось занять второе место из %d!", totalUsers))
		case 3:
			lines = append(lines, fmt.Sprintf("🥉 Бронза! Вы в тройке лидеров из %d участников!", totalUsers))
		default:
			if position <= totalUsers/10 && totalUsers >= 10 {
				lines = append(lines, fmt.Sprintf("🏅 Вы в топ-10%% — место %d из %d!", position, totalUsers))
			} else {
				lines = append(lines, fmt.Sprintf("🏅 Ваше место: %d из %d участников", position, totalUsers))
			}
		}
	} else {
		lines = append(lines, "🏆 Вы покорили этот квест!")
	}

	// Время прохождения
	if firstTime != nil && lastTime != nil {
		duration := lastTime.Sub(*firstTime)
		if duration > 0 {
			durationStr := formatDurationFriendly(duration)
			if duration < time.Hour {
				lines = append(lines, fmt.Sprintf("⚡ Скоростное прохождение за %s!", durationStr))
			} else if duration < 24*time.Hour {
				lines = append(lines, fmt.Sprintf("⏱ Квест пройден за %s", durationStr))
			} else {
				lines = append(lines, fmt.Sprintf("⏱ Путь к победе занял %s", durationStr))
			}
		}
	}

	// Точность ответов
	if totalAnswers > 0 {
		accuracy := 100
		if totalAnswers > answered {
			accuracy = (answered * 100) / totalAnswers
		}

		if accuracy == 100 {
			lines = append(lines, "🎯 Идеально! Все ответы с первой попытки!")
		} else if accuracy >= 80 {
			lines = append(lines, fmt.Sprintf("🎯 Впечатляет! Точность %d%% — почти без ошибок!", accuracy))
		} else if accuracy >= 50 {
			lines = append(lines, fmt.Sprintf("🎯 Неплохо! Точность ответов: %d%%", accuracy))
		}
	}

	// Подсказки
	if hintsUsed == 0 {
		lines = append(lines, "💡 Вау! Прошли без единой подсказки — настоящий эксперт!")
	} else if hintsUsed == 1 {
		lines = append(lines, "💡 Почти самостоятельно! Всего одна подсказка")
	} else if hintsUsed <= 3 {
		lines = append(lines, fmt.Sprintf("💡 Немного помощи не помешало: %d подсказки", hintsUsed))
	} else {
		lines = append(lines, fmt.Sprintf("💡 Подсказки — наши друзья: использовано %d", hintsUsed))
	}

	// Вопросы со звёздочкой
	answeredAsterisk, totalAsterisk, err := s.GetUserAsteriskStats(userID)
	if err == nil && totalAsterisk > 0 {
		if answeredAsterisk == totalAsterisk {
			lines = append(lines, "⭐ Все вопросы со звёздочкой решены!")
		} else if answeredAsterisk > 0 {
			lines = append(lines, fmt.Sprintf("⭐ Вопросы со звёздочкой: %d из %d", answeredAsterisk, totalAsterisk))
		} else {
			lines = append(lines, fmt.Sprintf("⭐ Вопросы со звёздочкой: 0 из %d", totalAsterisk))
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return "\n📊 <b>Ваши результаты:</b>\n\n" + strings.Join(lines, "\n") + "\n"
}

func (s *StatisticsService) getUserDetailedAnswerStats(userID int64) (int, int, *time.Time, *time.Time, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var totalAnswers, hintsUsed int
		var firstTimeStr, lastTimeStr sql.NullString
		err := db.QueryRow(`
			SELECT 
				COUNT(*), 
				COALESCE(SUM(CASE WHEN ua.hint_used = 1 THEN 1 ELSE 0 END), 0),
				MIN(ua.created_at),
				MAX(ua.created_at)
			FROM user_answers ua
			WHERE ua.user_id = ?
			AND NOT EXISTS (
				SELECT 1 FROM user_progress up 
				WHERE up.user_id = ua.user_id 
				AND up.step_id = ua.step_id 
				AND up.status = 'skipped'
			)
		`, userID).Scan(&totalAnswers, &hintsUsed, &firstTimeStr, &lastTimeStr)
		return []interface{}{totalAnswers, hintsUsed, firstTimeStr, lastTimeStr}, err
	})
	if err != nil {
		return 0, 0, nil, nil, err
	}
	res := result.([]interface{})
	totalAnswers := res[0].(int)
	hintsUsed := res[1].(int)
	firstTimeStr := res[2].(sql.NullString)
	lastTimeStr := res[3].(sql.NullString)

	var first, last *time.Time
	if firstTimeStr.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", firstTimeStr.String); err == nil {
			first = &t
		} else if t, err := time.Parse(time.RFC3339, firstTimeStr.String); err == nil {
			first = &t
		}
	}
	if lastTimeStr.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", lastTimeStr.String); err == nil {
			last = &t
		} else if t, err := time.Parse(time.RFC3339, lastTimeStr.String); err == nil {
			last = &t
		}
	}
	return totalAnswers, hintsUsed, first, last, nil
}

func formatDurationFriendly(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dд %dч", days, hours)
		}
		return fmt.Sprintf("%dд", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dч %dм", hours, minutes)
		}
		return fmt.Sprintf("%dч", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dм", minutes)
	}
	return "меньше минуты"
}
