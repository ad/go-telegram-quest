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

// AudienceSegments — сегменты аудитории по прогрессу
type AudienceSegments struct {
	TotalUsers  int
	MaxStep     int
	Finishers   int // 100%
	Almost      int // 75–99%
	Middle      int // 50–74%
	Beginners   int // 25–49%
	JustStarted int // <25%
	AvgPct      float64
}

// DropoffPoint — шаг где участники уходили
type DropoffPoint struct {
	StepOrder int
	StepText  string
	Starters  int
	Dropped   int
	DropPct   float64
}

// HourlyActivity — активность по часу суток
type HourlyActivity struct {
	Hour        int
	Users       int
	AnswerCount int
}

// GetAudienceSegments возвращает сегменты участников по % прохождения
func (s *StatisticsService) GetAudienceSegments() (*AudienceSegments, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var maxStep int
		err := db.QueryRow(`
			SELECT COALESCE(MAX(step_order), 0)
			FROM steps WHERE is_active = TRUE AND is_deleted = FALSE
		`).Scan(&maxStep)
		if err != nil || maxStep == 0 {
			return nil, fmt.Errorf("no active steps")
		}

		rows, err := db.Query(`
			SELECT ua.user_id,
			       ROUND(100.0 * MAX(s.step_order) / ?) as pct
			FROM user_answers ua
			JOIN steps s ON s.id = ua.step_id
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE
			GROUP BY ua.user_id
		`, maxStep)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		seg := &AudienceSegments{MaxStep: maxStep}
		var totalPct float64
		for rows.Next() {
			var userID int64
			var pct float64
			if err := rows.Scan(&userID, &pct); err != nil {
				return nil, err
			}
			seg.TotalUsers++
			totalPct += pct
			switch {
			case pct >= 100:
				seg.Finishers++
			case pct >= 75:
				seg.Almost++
			case pct >= 50:
				seg.Middle++
			case pct >= 25:
				seg.Beginners++
			default:
				seg.JustStarted++
			}
		}
		if seg.TotalUsers > 0 {
			seg.AvgPct = totalPct / float64(seg.TotalUsers)
		}
		return seg, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(*AudienceSegments), nil
}

// GetDropoffPoints возвращает шаги с наибольшим числом ушедших участников
func (s *StatisticsService) GetDropoffPoints(limit int) ([]DropoffPoint, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			WITH last_steps AS (
				SELECT ua.user_id, MAX(s.step_order) as last_step
				FROM user_answers ua
				JOIN steps s ON s.id = ua.step_id
				WHERE s.is_active = TRUE AND s.is_deleted = FALSE
				GROUP BY ua.user_id
			),
			step_starters AS (
				SELECT s.step_order, s.text, COUNT(DISTINCT ua.user_id) as starters
				FROM steps s
				JOIN user_answers ua ON s.id = ua.step_id
				WHERE s.is_active = TRUE AND s.is_deleted = FALSE
				GROUP BY s.step_order, s.text
			),
			step_droppers AS (
				SELECT last_step as step_order, COUNT(*) as dropped
				FROM last_steps GROUP BY last_step
			)
			SELECT ss.step_order, ss.text, ss.starters,
			       COALESCE(sd.dropped, 0) as dropped,
			       ROUND(100.0 * COALESCE(sd.dropped, 0) / ss.starters, 1) as drop_pct
			FROM step_starters ss
			LEFT JOIN step_droppers sd ON sd.step_order = ss.step_order
			WHERE ss.step_order != (
				SELECT MAX(step_order) FROM steps WHERE is_active = TRUE AND is_deleted = FALSE
			)
			AND COALESCE(sd.dropped, 0) > 0
			ORDER BY drop_pct DESC, dropped DESC
			LIMIT ?
		`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []DropoffPoint
		for rows.Next() {
			var d DropoffPoint
			if err := rows.Scan(&d.StepOrder, &d.StepText, &d.Starters, &d.Dropped, &d.DropPct); err != nil {
				return nil, err
			}
			out = append(out, d)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]DropoffPoint), nil
}

// GetHourlyActivity возвращает активность по часам суток
func (s *StatisticsService) GetHourlyActivity() ([]HourlyActivity, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT CAST(strftime('%H', created_at) AS INTEGER) as hour,
			       COUNT(DISTINCT user_id) as users,
			       COUNT(*) as answers
			FROM user_answers
			GROUP BY hour
			ORDER BY hour
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []HourlyActivity
		for rows.Next() {
			var h HourlyActivity
			if err := rows.Scan(&h.Hour, &h.Users, &h.AnswerCount); err != nil {
				return nil, err
			}
			out = append(out, h)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]HourlyActivity), nil
}

// AnswerDiversityStep — вопрос с индексом неоднозначности ответов
type AnswerDiversityStep struct {
	StepOrder     int
	StepText      string
	Participants  int
	UniqueAnswers int
	Diversity     float64 // UniqueAnswers / Participants
}

// HomeworkStep — вопрос где участники долго думали (приходили снова спустя часы/дни)
type HomeworkStep struct {
	StepOrder        int
	StepText         string
	StrugglingUsers  int
	AvgStruggleHours float64
	MaxStruggleHours float64
}

// GetAnswerDiversity возвращает шаги с авто-проверкой отсортированные по разнообразию ответов.
// Unicode-aware lowercase делается в Go, а не в SQLite.
func (s *StatisticsService) GetAnswerDiversity(limit int) ([]AnswerDiversityStep, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		// Загружаем сырые ответы для шагов с авто-проверкой
		rows, err := db.Query(`
			SELECT s.step_order, s.text, ua.user_id, ua.text_answer
			FROM steps s
			JOIN user_answers ua ON s.id = ua.step_id
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE
			  AND s.has_auto_check = TRUE
			  AND ua.text_answer IS NOT NULL AND ua.text_answer != ''
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		type stepKey struct {
			order int
			text  string
		}
		// step → set of unique lowercase answers
		uniqueAnswers := make(map[stepKey]map[string]struct{})
		// step → set of unique user ids
		uniqueUsers := make(map[stepKey]map[int64]struct{})

		for rows.Next() {
			var order int
			var text, answer string
			var userID int64
			if err := rows.Scan(&order, &text, &userID, &answer); err != nil {
				return nil, err
			}
			k := stepKey{order, text}
			if uniqueAnswers[k] == nil {
				uniqueAnswers[k] = make(map[string]struct{})
				uniqueUsers[k] = make(map[int64]struct{})
			}
			uniqueAnswers[k][strings.ToLower(strings.TrimSpace(answer))] = struct{}{}
			uniqueUsers[k][userID] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		var out []AnswerDiversityStep
		for k, answers := range uniqueAnswers {
			users := len(uniqueUsers[k])
			if users < 3 {
				continue
			}
			out = append(out, AnswerDiversityStep{
				StepOrder:     k.order,
				StepText:      k.text,
				Participants:  users,
				UniqueAnswers: len(answers),
				Diversity:     float64(len(answers)) / float64(users),
			})
		}
		// Сортировка по убыванию diversity
		for i := 1; i < len(out); i++ {
			for j := i; j > 0 && out[j].Diversity > out[j-1].Diversity; j-- {
				out[j], out[j-1] = out[j-1], out[j]
			}
		}
		if len(out) > limit {
			out = out[:limit]
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]AnswerDiversityStep), nil
}

// GetHomeworkSteps возвращает шаги, на которых участники дольше всего думали между попытками.
func (s *StatisticsService) GetHomeworkSteps(limit int) ([]HomeworkStep, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT s.step_order, s.text,
			       COUNT(DISTINCT sub.user_id) as struggling_users,
			       AVG((julianday(sub.max_t) - julianday(sub.min_t)) * 24) as avg_hours,
			       MAX((julianday(sub.max_t) - julianday(sub.min_t)) * 24) as max_hours
			FROM steps s
			JOIN (
				SELECT step_id, user_id,
				       MIN(created_at) as min_t,
				       MAX(created_at) as max_t,
				       COUNT(*) as cnt
				FROM user_answers
				GROUP BY step_id, user_id
				HAVING cnt > 1
			) sub ON sub.step_id = s.id
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE
			GROUP BY s.step_order, s.text
			HAVING struggling_users >= 2
			ORDER BY avg_hours DESC
			LIMIT ?
		`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var out []HomeworkStep
		for rows.Next() {
			var h HomeworkStep
			if err := rows.Scan(&h.StepOrder, &h.StepText, &h.StrugglingUsers, &h.AvgStruggleHours, &h.MaxStruggleHours); err != nil {
				return nil, err
			}
			out = append(out, h)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]HomeworkStep), nil
}

// SpeedrunData — данные спидранера
type SpeedrunData struct {
	FirstName   string
	Username    string
	DurationMin float64
	MaxStep     int
	IsFinisher  bool // дошёл до последнего шага
}

// StubbornData — данные о рекорде упрямства (много попыток на одном шаге)
type StubbornData struct {
	FirstName string
	Username  string
	StepOrder int
	Attempts  int
}

// GetSpeedruns возвращает топ самых быстрых участников (от первого до последнего ответа)
func (s *StatisticsService) GetSpeedruns(limit int) ([]SpeedrunData, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var lastStep int
		_ = db.QueryRow(`
			SELECT COALESCE(MAX(step_order), 0)
			FROM steps WHERE is_active = TRUE AND is_deleted = FALSE
		`).Scan(&lastStep)

		rows, err := db.Query(`
			SELECT u.first_name, u.username,
			       ROUND((julianday(MAX(ua.created_at)) - julianday(MIN(ua.created_at))) * 24 * 60, 1) as duration_min,
			       MAX(s.step_order) as max_step
			FROM user_answers ua
			JOIN users u ON u.id = ua.user_id
			JOIN steps s ON s.id = ua.step_id
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE
			GROUP BY ua.user_id
			HAVING duration_min > 0
			ORDER BY max_step DESC, duration_min ASC
			LIMIT ?
		`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []SpeedrunData
		for rows.Next() {
			var d SpeedrunData
			var firstName, username sql.NullString
			if err := rows.Scan(&firstName, &username, &d.DurationMin, &d.MaxStep); err != nil {
				return nil, err
			}
			d.IsFinisher = lastStep > 0 && d.MaxStep >= lastStep
			d.FirstName = firstName.String
			d.Username = username.String
			out = append(out, d)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SpeedrunData), nil
}

// GetStubbornRecords возвращает топ рекордов упрямства: больше всего попыток на одном шаге
func (s *StatisticsService) GetStubbornRecords(limit int) ([]StubbornData, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT u.first_name, u.username, s.step_order, COUNT(ua.id) as attempts
			FROM user_answers ua
			JOIN users u ON u.id = ua.user_id
			JOIN steps s ON s.id = ua.step_id
			GROUP BY ua.user_id, ua.step_id
			ORDER BY attempts DESC
			LIMIT ?
		`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []StubbornData
		for rows.Next() {
			var d StubbornData
			var firstName, username sql.NullString
			if err := rows.Scan(&firstName, &username, &d.StepOrder, &d.Attempts); err != nil {
				return nil, err
			}
			d.FirstName = firstName.String
			d.Username = username.String
			out = append(out, d)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]StubbornData), nil
}

// AnswerFunnelData содержит данные воронки для одного шага
type AnswerFunnelData struct {
	StepOrder   int
	StepText    string
	UniqueUsers int
}

// HardestStep — данные о самом сложном шаге (авто-проверка)
type HardestStep struct {
	StepOrder     int
	StepText      string
	TotalAttempts int
	UniqueUsers   int
	AvgAttempts   float64
}

// HintStepData — статистика подсказок по шагу
type HintStepData struct {
	StepOrder int
	StepText  string
	HintCount int
}

// TopAnswer — популярный ответ на шаг
type TopAnswer struct {
	Answer string
	Count  int
}

// StepAnswerStats — топ ответов на конкретный шаг
type StepAnswerStats struct {
	StepOrder    int
	StepText     string
	TotalAnswers int
	TopAnswers   []TopAnswer
}

// GetFunnelStats возвращает воронку прохождения: уники по шагам
func (s *StatisticsService) GetFunnelStats() ([]AnswerFunnelData, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT s.step_order, s.text, COUNT(DISTINCT ua.user_id) as unique_users
			FROM steps s
			LEFT JOIN user_answers ua ON s.id = ua.step_id
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE AND s.is_asterisk = FALSE
			GROUP BY s.id, s.step_order, s.text
			ORDER BY s.step_order
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []AnswerFunnelData
		for rows.Next() {
			var d AnswerFunnelData
			if err := rows.Scan(&d.StepOrder, &d.StepText, &d.UniqueUsers); err != nil {
				return nil, err
			}
			out = append(out, d)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]AnswerFunnelData), nil
}

// GetHardestSteps возвращает топ шагов с авто-проверкой по среднему числу попыток
func (s *StatisticsService) GetHardestSteps(limit int) ([]HardestStep, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT
				s.step_order,
				s.text,
				COUNT(ua.id) as total_attempts,
				COUNT(DISTINCT ua.user_id) as unique_users,
				CAST(COUNT(ua.id) AS FLOAT) / NULLIF(COUNT(DISTINCT ua.user_id), 0) as avg_attempts
			FROM steps s
			JOIN user_answers ua ON s.id = ua.step_id
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE AND s.has_auto_check = TRUE
			GROUP BY s.id, s.step_order, s.text
			HAVING unique_users > 0
			ORDER BY avg_attempts DESC
			LIMIT ?
		`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []HardestStep
		for rows.Next() {
			var h HardestStep
			if err := rows.Scan(&h.StepOrder, &h.StepText, &h.TotalAttempts, &h.UniqueUsers, &h.AvgAttempts); err != nil {
				return nil, err
			}
			out = append(out, h)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]HardestStep), nil
}

// GetHintStats возвращает топ шагов по числу использованных подсказок
func (s *StatisticsService) GetHintStats(limit int) ([]HintStepData, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT s.step_order, s.text, COUNT(ua.id) as hint_count
			FROM steps s
			JOIN user_answers ua ON s.id = ua.step_id AND ua.hint_used = TRUE
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE
			GROUP BY s.id, s.step_order, s.text
			ORDER BY hint_count DESC
			LIMIT ?
		`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []HintStepData
		for rows.Next() {
			var h HintStepData
			if err := rows.Scan(&h.StepOrder, &h.StepText, &h.HintCount); err != nil {
				return nil, err
			}
			out = append(out, h)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]HintStepData), nil
}

// GetAutoCheckStepOrders возвращает список step_order шагов с авто-проверкой, на которые есть ответы
func (s *StatisticsService) GetAutoCheckStepOrders() ([]int, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT s.step_order
			FROM steps s
			JOIN user_answers ua ON s.id = ua.step_id
			WHERE s.is_active = TRUE AND s.is_deleted = FALSE AND s.has_auto_check = TRUE
			GROUP BY s.step_order
			ORDER BY s.step_order
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []int
		for rows.Next() {
			var o int
			if err := rows.Scan(&o); err != nil {
				return nil, err
			}
			out = append(out, o)
		}
		return out, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]int), nil
}

// GetTopAnswersForStep возвращает топ ответов для шага с указанным step_order
func (s *StatisticsService) GetTopAnswersForStep(stepOrder int, limit int) (*StepAnswerStats, error) {
	result, err := s.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var stepID int64
		var stepText string
		err := db.QueryRow(`
			SELECT id, text FROM steps
			WHERE step_order = ? AND is_active = TRUE AND is_deleted = FALSE AND has_auto_check = TRUE
		`, stepOrder).Scan(&stepID, &stepText)
		if err != nil {
			return nil, err
		}

		var totalAnswers int
		_ = db.QueryRow(`SELECT COUNT(*) FROM user_answers WHERE step_id = ?`, stepID).Scan(&totalAnswers)

		// Выбираем сырые ответы — LOWER() в SQLite не работает для кириллицы
		rows, err := db.Query(`
			SELECT text_answer, COUNT(*) as cnt
			FROM user_answers
			WHERE step_id = ? AND text_answer IS NOT NULL AND text_answer != ''
			GROUP BY text_answer
			ORDER BY cnt DESC
		`, stepID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		// Объединяем по strings.ToLower (поддерживает Unicode)
		counts := make(map[string]int)
		var order []string
		for rows.Next() {
			var raw string
			var cnt int
			if err := rows.Scan(&raw, &cnt); err != nil {
				return nil, err
			}
			key := strings.ToLower(strings.TrimSpace(raw))
			if _, seen := counts[key]; !seen {
				order = append(order, key)
			}
			counts[key] += cnt
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		// Сортируем по убыванию count
		for i := 1; i < len(order); i++ {
			for j := i; j > 0 && counts[order[j]] > counts[order[j-1]]; j-- {
				order[j], order[j-1] = order[j-1], order[j]
			}
		}

		stats := &StepAnswerStats{
			StepOrder:    stepOrder,
			StepText:     stepText,
			TotalAnswers: totalAnswers,
		}
		for i, key := range order {
			if i >= limit {
				break
			}
			stats.TopAnswers = append(stats.TopAnswers, TopAnswer{Answer: key, Count: counts[key]})
		}
		return stats, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*StepAnswerStats), nil
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
