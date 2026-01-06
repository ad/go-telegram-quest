package services

import (
	"database/sql"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type AchievementEngine struct {
	achievementRepo *db.AchievementRepository
	userRepo        *db.UserRepository
	progressRepo    *db.ProgressRepository
	stepRepo        *db.StepRepository
	queue           *db.DBQueue
	uniqueMutex     sync.Mutex
}

func NewAchievementEngine(
	achievementRepo *db.AchievementRepository,
	userRepo *db.UserRepository,
	progressRepo *db.ProgressRepository,
	stepRepo *db.StepRepository,
	queue *db.DBQueue,
) *AchievementEngine {
	return &AchievementEngine{
		achievementRepo: achievementRepo,
		userRepo:        userRepo,
		progressRepo:    progressRepo,
		stepRepo:        stepRepo,
		queue:           queue,
	}
}

func (e *AchievementEngine) EvaluateUserAchievements(userID int64) ([]string, error) {
	achievements, err := e.achievementRepo.GetActive()
	if err != nil {
		return nil, err
	}

	var awarded []string
	for _, achievement := range achievements {
		if achievement.IsUnique {
			continue
		}

		hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievement.Key)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error checking user achievement: %v", err)
			continue
		}
		if hasAchievement {
			continue
		}

		qualifies, err := e.evaluateConditions(userID, achievement)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating conditions for %s: %v", achievement.Key, err)
			continue
		}

		if qualifies {
			err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
			if err != nil {
				log.Printf("[ACHIEVEMENT_ENGINE] Error assigning achievement %s to user %d: %v", achievement.Key, userID, err)
				continue
			}
			awarded = append(awarded, achievement.Key)
			log.Printf("[ACHIEVEMENT_ENGINE] Awarded achievement %s to user %d", achievement.Key, userID)
		}
	}

	return awarded, nil
}

func (e *AchievementEngine) EvaluateSpecificAchievement(userID int64, achievementKey string) (bool, error) {
	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	if achievement.IsUnique {
		return e.tryAssignUniqueAchievement(userID, achievement)
	}

	qualifies, err := e.evaluateConditions(userID, achievement)
	if err != nil {
		return false, err
	}

	if qualifies {
		err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func (e *AchievementEngine) EvaluateRetroactiveAchievements(achievementKey string) ([]int64, error) {
	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return nil, err
	}

	users, err := e.userRepo.GetAll()
	if err != nil {
		return nil, err
	}

	var awardedUsers []int64
	for _, user := range users {
		hasAchievement, err := e.achievementRepo.HasUserAchievement(user.ID, achievementKey)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error checking user %d achievement: %v", user.ID, err)
			continue
		}
		if hasAchievement {
			continue
		}

		qualifies, earnedAt, err := e.evaluateConditionsWithTimestamp(user.ID, achievement)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating retroactive conditions for user %d: %v", user.ID, err)
			continue
		}

		if qualifies {
			err = e.achievementRepo.AssignToUser(user.ID, achievement.ID, earnedAt, true)
			if err != nil {
				log.Printf("[ACHIEVEMENT_ENGINE] Error assigning retroactive achievement to user %d: %v", user.ID, err)
				continue
			}
			awardedUsers = append(awardedUsers, user.ID)
			log.Printf("[ACHIEVEMENT_ENGINE] Retroactively awarded achievement %s to user %d", achievementKey, user.ID)
		}
	}

	return awardedUsers, nil
}

func (e *AchievementEngine) EvaluateAllRetroactiveAchievements() (map[string][]int64, error) {
	achievements, err := e.achievementRepo.GetActive()
	if err != nil {
		return nil, err
	}

	results := make(map[string][]int64)
	for _, achievement := range achievements {
		if achievement.IsUnique {
			continue
		}

		awardedUsers, err := e.EvaluateRetroactiveAchievements(achievement.Key)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating retroactive achievement %s: %v", achievement.Key, err)
			continue
		}
		if len(awardedUsers) > 0 {
			results[achievement.Key] = awardedUsers
		}
	}

	return results, nil
}

func (e *AchievementEngine) EvaluateUniqueAchievements() error {
	achievements, err := e.achievementRepo.GetActive()
	if err != nil {
		return err
	}

	for _, achievement := range achievements {
		if !achievement.IsUnique {
			continue
		}

		holders, err := e.achievementRepo.GetAchievementHolders(achievement.Key)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error getting holders for %s: %v", achievement.Key, err)
			continue
		}
		if len(holders) > 0 {
			continue
		}

		if achievement.Conditions.Position != nil {
			err = e.assignPositionBasedAchievement(achievement)
			if err != nil {
				log.Printf("[ACHIEVEMENT_ENGINE] Error assigning position-based achievement %s: %v", achievement.Key, err)
			}
		}
	}

	return nil
}

func (e *AchievementEngine) tryAssignUniqueAchievement(userID int64, achievement *models.Achievement) (bool, error) {
	e.uniqueMutex.Lock()
	defer e.uniqueMutex.Unlock()

	holders, err := e.achievementRepo.GetAchievementHolders(achievement.Key)
	if err != nil {
		return false, err
	}
	if len(holders) > 0 {
		return false, nil
	}

	qualifies, err := e.evaluateConditions(userID, achievement)
	if err != nil {
		return false, err
	}

	if qualifies {
		err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func (e *AchievementEngine) assignPositionBasedAchievement(achievement *models.Achievement) error {
	e.uniqueMutex.Lock()
	defer e.uniqueMutex.Unlock()

	holders, err := e.achievementRepo.GetAchievementHolders(achievement.Key)
	if err != nil {
		return err
	}
	if len(holders) > 0 {
		return nil
	}

	position := *achievement.Conditions.Position
	usersWithFirstAnswer, err := e.getUsersOrderedByFirstCorrectAnswer()
	if err != nil {
		return err
	}

	if position > len(usersWithFirstAnswer) {
		return nil
	}

	userID := usersWithFirstAnswer[position-1].UserID
	earnedAt := usersWithFirstAnswer[position-1].FirstCorrectAnswerTime

	return e.achievementRepo.AssignToUser(userID, achievement.ID, earnedAt, true)
}

type UserFirstAnswer struct {
	UserID                 int64
	FirstCorrectAnswerTime time.Time
}

func (e *AchievementEngine) getUsersOrderedByFirstCorrectAnswer() ([]UserFirstAnswer, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		rows, err := db.Query(`
			SELECT p.user_id, MIN(p.completed_at) as first_correct
			FROM user_progress p
			WHERE p.status = 'approved' AND p.completed_at IS NOT NULL
			GROUP BY p.user_id
			ORDER BY first_correct ASC
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var users []UserFirstAnswer
		for rows.Next() {
			var u UserFirstAnswer
			var completedAtStr string
			if err := rows.Scan(&u.UserID, &completedAtStr); err != nil {
				return nil, err
			}
			parsedTime, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", completedAtStr)
			if err != nil {
				parsedTime, err = time.Parse("2006-01-02T15:04:05Z", completedAtStr)
				if err != nil {
					parsedTime, err = time.Parse("2006-01-02 15:04:05", completedAtStr)
					if err != nil {
						parsedTime, _ = time.Parse(time.RFC3339, completedAtStr)
					}
				}
			}
			u.FirstCorrectAnswerTime = parsedTime
			users = append(users, u)
		}
		return users, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]UserFirstAnswer), nil
}

func (e *AchievementEngine) evaluateConditions(userID int64, achievement *models.Achievement) (bool, error) {
	conditions := achievement.Conditions

	if conditions.CorrectAnswers != nil {
		count, err := e.getCorrectAnswersCount(userID)
		if err != nil {
			return false, err
		}
		if count < *conditions.CorrectAnswers {
			return false, nil
		}
	}

	if conditions.Position != nil {
		usersWithFirstAnswer, err := e.getUsersOrderedByFirstCorrectAnswer()
		if err != nil {
			return false, err
		}

		position := *conditions.Position
		if position > len(usersWithFirstAnswer) {
			return false, nil
		}

		if usersWithFirstAnswer[position-1].UserID != userID {
			return false, nil
		}
	}

	if len(conditions.RequiredAchievements) > 0 {
		for _, reqKey := range conditions.RequiredAchievements {
			has, err := e.achievementRepo.HasUserAchievement(userID, reqKey)
			if err != nil {
				return false, err
			}
			if !has {
				return false, nil
			}
		}
	}

	return true, nil
}

func (e *AchievementEngine) evaluateConditionsWithTimestamp(userID int64, achievement *models.Achievement) (bool, time.Time, error) {
	conditions := achievement.Conditions
	earnedAt := time.Now()

	if conditions.CorrectAnswers != nil {
		count, timestamp, err := e.getCorrectAnswersCountWithTimestamp(userID, *conditions.CorrectAnswers)
		if err != nil {
			return false, time.Time{}, err
		}
		if count < *conditions.CorrectAnswers {
			return false, time.Time{}, nil
		}
		if !timestamp.IsZero() {
			earnedAt = timestamp
		}
	}

	if conditions.Position != nil {
		usersWithFirstAnswer, err := e.getUsersOrderedByFirstCorrectAnswer()
		if err != nil {
			return false, time.Time{}, err
		}

		position := *conditions.Position
		if position > len(usersWithFirstAnswer) {
			return false, time.Time{}, nil
		}

		if usersWithFirstAnswer[position-1].UserID != userID {
			return false, time.Time{}, nil
		}
		earnedAt = usersWithFirstAnswer[position-1].FirstCorrectAnswerTime
	}

	if len(conditions.RequiredAchievements) > 0 {
		var latestAchievementTime time.Time
		for _, reqKey := range conditions.RequiredAchievements {
			has, err := e.achievementRepo.HasUserAchievement(userID, reqKey)
			if err != nil {
				return false, time.Time{}, err
			}
			if !has {
				return false, time.Time{}, nil
			}

			userAchievements, err := e.achievementRepo.GetUserAchievements(userID)
			if err != nil {
				return false, time.Time{}, err
			}
			for _, ua := range userAchievements {
				reqAchievement, err := e.achievementRepo.GetByKey(reqKey)
				if err != nil {
					continue
				}
				if ua.AchievementID == reqAchievement.ID && ua.EarnedAt.After(latestAchievementTime) {
					latestAchievementTime = ua.EarnedAt
				}
			}
		}
		if !latestAchievementTime.IsZero() {
			earnedAt = latestAchievementTime
		}
	}

	return true, earnedAt, nil
}

func (e *AchievementEngine) getCorrectAnswersCount(userID int64) (int, error) {
	progress, err := e.progressRepo.GetUserProgress(userID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, p := range progress {
		if p.Status == models.StatusApproved {
			count++
		}
	}
	return count, nil
}

func (e *AchievementEngine) getCorrectAnswersCountWithTimestamp(userID int64, threshold int) (int, time.Time, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		rows, err := db.Query(`
			SELECT completed_at
			FROM user_progress
			WHERE user_id = ? AND status = 'approved' AND completed_at IS NOT NULL
			ORDER BY completed_at ASC
		`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var times []time.Time
		for rows.Next() {
			var t time.Time
			if err := rows.Scan(&t); err != nil {
				return nil, err
			}
			times = append(times, t)
		}
		return times, rows.Err()
	})
	if err != nil {
		return 0, time.Time{}, err
	}

	times := result.([]time.Time)
	count := len(times)

	var thresholdTime time.Time
	if count >= threshold && threshold > 0 {
		thresholdTime = times[threshold-1]
	}

	return count, thresholdTime, nil
}

func (e *AchievementEngine) GetUserPosition(userID int64) (int, error) {
	usersWithFirstAnswer, err := e.getUsersOrderedByFirstCorrectAnswer()
	if err != nil {
		return 0, err
	}

	for i, u := range usersWithFirstAnswer {
		if u.UserID == userID {
			return i + 1, nil
		}
	}

	return 0, nil
}

func (e *AchievementEngine) IsUniqueAchievementAvailable(achievementKey string) (bool, error) {
	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	if !achievement.IsUnique {
		return false, nil
	}

	holders, err := e.achievementRepo.GetAchievementHolders(achievementKey)
	if err != nil {
		return false, err
	}

	return len(holders) == 0, nil
}

func (e *AchievementEngine) GetUniqueAchievementHolder(achievementKey string) (int64, error) {
	holders, err := e.achievementRepo.GetAchievementHolders(achievementKey)
	if err != nil {
		return 0, err
	}

	if len(holders) == 0 {
		return 0, nil
	}

	return holders[0], nil
}

func (e *AchievementEngine) EvaluatePositionBasedAchievements(userID int64) ([]string, error) {
	achievements, err := e.achievementRepo.GetActive()
	if err != nil {
		return nil, err
	}

	var awarded []string
	for _, achievement := range achievements {
		if !achievement.IsUnique || achievement.Conditions.Position == nil {
			continue
		}

		hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievement.Key)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error checking user achievement: %v", err)
			continue
		}
		if hasAchievement {
			continue
		}

		assigned, err := e.tryAssignUniqueAchievement(userID, achievement)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error trying to assign unique achievement %s: %v", achievement.Key, err)
			continue
		}

		if assigned {
			awarded = append(awarded, achievement.Key)
			log.Printf("[ACHIEVEMENT_ENGINE] Awarded unique achievement %s to user %d", achievement.Key, userID)
		}
	}

	return awarded, nil
}

var ProgressThresholds = []int{5, 10, 15, 20, 25}

var ProgressAchievementKeys = map[int]string{
	5:  "beginner_5",
	10: "experienced_10",
	15: "advanced_15",
	20: "expert_20",
	25: "master_25",
}

func (e *AchievementEngine) EvaluateProgressAchievements(userID int64) ([]string, error) {
	correctCount, err := e.getCorrectAnswersCount(userID)
	if err != nil {
		return nil, err
	}

	var awarded []string
	for _, threshold := range ProgressThresholds {
		if correctCount < threshold {
			break
		}

		achievementKey, exists := ProgressAchievementKeys[threshold]
		if !exists {
			continue
		}

		hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error checking progress achievement %s: %v", achievementKey, err)
			continue
		}
		if hasAchievement {
			continue
		}

		achievement, err := e.achievementRepo.GetByKey(achievementKey)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Progress achievement %s not found: %v", achievementKey, err)
			continue
		}

		err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error assigning progress achievement %s to user %d: %v", achievementKey, userID, err)
			continue
		}

		awarded = append(awarded, achievementKey)
		log.Printf("[ACHIEVEMENT_ENGINE] Awarded progress achievement %s to user %d (correct answers: %d)", achievementKey, userID, correctCount)
	}

	return awarded, nil
}

func (e *AchievementEngine) OnCorrectAnswer(userID int64) ([]string, error) {
	return e.EvaluateProgressAchievements(userID)
}

func (e *AchievementEngine) GetProgressAchievementStatus(userID int64) (map[string]bool, int, error) {
	correctCount, err := e.getCorrectAnswersCount(userID)
	if err != nil {
		return nil, 0, err
	}

	status := make(map[string]bool)
	for _, threshold := range ProgressThresholds {
		achievementKey := ProgressAchievementKeys[threshold]
		hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
		if err != nil {
			return nil, 0, err
		}
		status[achievementKey] = hasAchievement
	}

	return status, correctCount, nil
}

func (e *AchievementEngine) GetNextProgressThreshold(userID int64) (int, string, error) {
	correctCount, err := e.getCorrectAnswersCount(userID)
	if err != nil {
		return 0, "", err
	}

	for _, threshold := range ProgressThresholds {
		if correctCount < threshold {
			return threshold, ProgressAchievementKeys[threshold], nil
		}
	}

	return 0, "", nil
}

var CompletionAchievementKeys = map[string]string{
	"winner":          "winner",
	"perfect_path":    "perfect_path",
	"self_sufficient": "self_sufficient",
	"lightning":       "lightning",
	"rocket":          "rocket",
	"cheater":         "cheater",
}

type CompletionStats struct {
	IsCompleted           bool
	TotalSteps            int
	CompletedSteps        int
	TotalAnswers          int
	CorrectAnswers        int
	HintsUsed             int
	CompletionTimeMinutes int
	FirstAnswerTime       *time.Time
	LastAnswerTime        *time.Time
}

func (e *AchievementEngine) GetCompletionStats(userID int64) (*CompletionStats, error) {
	stats := &CompletionStats{}

	activeSteps, err := e.stepRepo.GetActive()
	if err != nil {
		return nil, err
	}
	stats.TotalSteps = len(activeSteps)

	progress, err := e.progressRepo.GetUserProgress(userID)
	if err != nil {
		return nil, err
	}

	for _, p := range progress {
		if p.Status == models.StatusApproved {
			stats.CompletedSteps++
			stats.CorrectAnswers++
		}
	}

	stats.IsCompleted = stats.CompletedSteps >= stats.TotalSteps && stats.TotalSteps > 0

	totalAnswers, hintsUsed, err := e.getUserAnswerStats(userID)
	if err != nil {
		return nil, err
	}
	stats.TotalAnswers = totalAnswers
	stats.HintsUsed = hintsUsed

	firstTime, lastTime, err := e.getUserAnswerTimeRange(userID)
	if err != nil {
		return nil, err
	}
	stats.FirstAnswerTime = firstTime
	stats.LastAnswerTime = lastTime

	if firstTime != nil && lastTime != nil {
		duration := lastTime.Sub(*firstTime)
		stats.CompletionTimeMinutes = int(duration.Minutes())
	}

	return stats, nil
}

func (e *AchievementEngine) getUserAnswerStats(userID int64) (totalAnswers int, hintsUsed int, err error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		var total, hints int
		err := db.QueryRow(`
			SELECT COUNT(*), COALESCE(SUM(CASE WHEN hint_used = 1 THEN 1 ELSE 0 END), 0)
			FROM user_answers
			WHERE user_id = ?
		`, userID).Scan(&total, &hints)
		if err != nil {
			return nil, err
		}
		return []int{total, hints}, nil
	})
	if err != nil {
		return 0, 0, err
	}
	counts := result.([]int)
	return counts[0], counts[1], nil
}

func (e *AchievementEngine) getUserAnswerTimeRange(userID int64) (*time.Time, *time.Time, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		var firstTimeStr, lastTimeStr sql.NullString
		err := db.QueryRow(`
			SELECT MIN(created_at), MAX(created_at)
			FROM user_answers
			WHERE user_id = ?
		`, userID).Scan(&firstTimeStr, &lastTimeStr)
		if err != nil {
			return nil, err
		}
		return []sql.NullString{firstTimeStr, lastTimeStr}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	times := result.([]sql.NullString)
	var first, last *time.Time
	if times[0].Valid && times[0].String != "" {
		parsedTime, err := parseTimeString(times[0].String)
		if err == nil {
			first = &parsedTime
		}
	}
	if times[1].Valid && times[1].String != "" {
		parsedTime, err := parseTimeString(times[1].String)
		if err == nil {
			last = &parsedTime
		}
	}
	return first, last, nil
}

func parseTimeString(s string) (time.Time, error) {
	if idx := strings.Index(s, " m="); idx != -1 {
		s = s[:idx]
	}

	formats := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05.999999999Z07:00",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}

func (e *AchievementEngine) EvaluateCompletionAchievements(userID int64) ([]string, error) {
	stats, err := e.GetCompletionStats(userID)
	if err != nil {
		return nil, err
	}

	if !stats.IsCompleted {
		return nil, nil
	}

	var awarded []string

	winnerAwarded, err := e.tryAwardCompletionAchievement(userID, CompletionAchievementKeys["winner"], func() bool {
		return true
	})
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error awarding winner achievement: %v", err)
	} else if winnerAwarded {
		awarded = append(awarded, CompletionAchievementKeys["winner"])
	}

	perfectPathAwarded, err := e.tryAwardCompletionAchievement(userID, CompletionAchievementKeys["perfect_path"], func() bool {
		return stats.TotalAnswers == stats.CorrectAnswers
	})
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error awarding perfect_path achievement: %v", err)
	} else if perfectPathAwarded {
		awarded = append(awarded, CompletionAchievementKeys["perfect_path"])
	}

	selfSufficientAwarded, err := e.tryAwardCompletionAchievement(userID, CompletionAchievementKeys["self_sufficient"], func() bool {
		return stats.HintsUsed == 0
	})
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error awarding self_sufficient achievement: %v", err)
	} else if selfSufficientAwarded {
		awarded = append(awarded, CompletionAchievementKeys["self_sufficient"])
	}

	hasCheater, _ := e.achievementRepo.HasUserAchievement(userID, CompletionAchievementKeys["cheater"])
	hasLightning, _ := e.achievementRepo.HasUserAchievement(userID, CompletionAchievementKeys["lightning"])
	hasRocket, _ := e.achievementRepo.HasUserAchievement(userID, CompletionAchievementKeys["rocket"])
	speedAchievementExists := hasCheater || hasLightning || hasRocket

	if !speedAchievementExists && stats.CompletionTimeMinutes < 5 {
		cheaterAwarded, err := e.tryAwardCompletionAchievement(userID, CompletionAchievementKeys["cheater"], func() bool {
			return true
		})
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding cheater achievement: %v", err)
		} else if cheaterAwarded {
			awarded = append(awarded, CompletionAchievementKeys["cheater"])
			speedAchievementExists = true
		}
	}

	if !speedAchievementExists && stats.CompletionTimeMinutes < 10 {
		lightningAwarded, err := e.tryAwardCompletionAchievement(userID, CompletionAchievementKeys["lightning"], func() bool {
			return true
		})
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding lightning achievement: %v", err)
		} else if lightningAwarded {
			awarded = append(awarded, CompletionAchievementKeys["lightning"])
			speedAchievementExists = true
		}
	}

	if !speedAchievementExists && stats.CompletionTimeMinutes < 60 {
		rocketAwarded, err := e.tryAwardCompletionAchievement(userID, CompletionAchievementKeys["rocket"], func() bool {
			return true
		})
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding rocket achievement: %v", err)
		} else if rocketAwarded {
			awarded = append(awarded, CompletionAchievementKeys["rocket"])
		}
	}

	return awarded, nil
}

func (e *AchievementEngine) tryAwardCompletionAchievement(userID int64, achievementKey string, condition func() bool) (bool, error) {
	hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	if !condition() {
		return false, nil
	}

	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
	if err != nil {
		return false, err
	}

	log.Printf("[ACHIEVEMENT_ENGINE] Awarded completion achievement %s to user %d", achievementKey, userID)
	return true, nil
}

func (e *AchievementEngine) OnQuestCompleted(userID int64) ([]string, error) {
	return e.EvaluateCompletionAchievements(userID)
}

func (e *AchievementEngine) EvaluateCompletionConditions(userID int64, achievement *models.Achievement) (bool, error) {
	conditions := achievement.Conditions

	stats, err := e.GetCompletionStats(userID)
	if err != nil {
		return false, err
	}

	if !stats.IsCompleted {
		return false, nil
	}

	if conditions.NoErrors != nil && *conditions.NoErrors {
		if stats.TotalAnswers != stats.CorrectAnswers {
			return false, nil
		}
	}

	if conditions.NoHints != nil && *conditions.NoHints {
		if stats.HintsUsed > 0 {
			return false, nil
		}
	}

	if conditions.CompletionTimeMinutes != nil {
		if stats.CompletionTimeMinutes >= *conditions.CompletionTimeMinutes {
			return false, nil
		}
	}

	return true, nil
}

func (e *AchievementEngine) EvaluateCompletionConditionsWithTimestamp(userID int64, achievement *models.Achievement) (bool, time.Time, error) {
	qualifies, err := e.EvaluateCompletionConditions(userID, achievement)
	if err != nil {
		return false, time.Time{}, err
	}
	if !qualifies {
		return false, time.Time{}, nil
	}

	stats, err := e.GetCompletionStats(userID)
	if err != nil {
		return false, time.Time{}, err
	}

	earnedAt := time.Now()
	if stats.LastAnswerTime != nil {
		earnedAt = *stats.LastAnswerTime
	}

	return true, earnedAt, nil
}

func (e *AchievementEngine) EvaluateRetroactiveCompletionAchievements(achievementKey string) ([]int64, error) {
	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return nil, err
	}

	if achievement.Category != models.CategoryCompletion {
		return nil, nil
	}

	users, err := e.userRepo.GetAll()
	if err != nil {
		return nil, err
	}

	var awardedUsers []int64
	for _, user := range users {
		hasAchievement, err := e.achievementRepo.HasUserAchievement(user.ID, achievementKey)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error checking user %d achievement: %v", user.ID, err)
			continue
		}
		if hasAchievement {
			continue
		}

		qualifies, earnedAt, err := e.EvaluateCompletionConditionsWithTimestamp(user.ID, achievement)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating completion conditions for user %d: %v", user.ID, err)
			continue
		}

		if qualifies {
			err = e.achievementRepo.AssignToUser(user.ID, achievement.ID, earnedAt, true)
			if err != nil {
				log.Printf("[ACHIEVEMENT_ENGINE] Error assigning retroactive completion achievement to user %d: %v", user.ID, err)
				continue
			}
			awardedUsers = append(awardedUsers, user.ID)
			log.Printf("[ACHIEVEMENT_ENGINE] Retroactively awarded completion achievement %s to user %d", achievementKey, user.ID)
		}
	}

	return awardedUsers, nil
}

var HintAchievementKeys = map[int]string{
	5:  "hint_5",
	10: "hint_10",
	15: "hint_15",
	25: "hint_25",
}

var HintThresholds = []int{5, 10, 15, 25}

type HintStats struct {
	TotalHintsUsed      int
	CorrectAnswers      int
	HintOnFirstTask     bool
	AllHintsUsed        bool
	TotalAvailableHints int
}

func (e *AchievementEngine) GetHintStats(userID int64) (*HintStats, error) {
	stats := &HintStats{}

	_, hintsUsed, err := e.getUserAnswerStats(userID)
	if err != nil {
		return nil, err
	}
	stats.TotalHintsUsed = hintsUsed

	correctCount, err := e.getCorrectAnswersCount(userID)
	if err != nil {
		return nil, err
	}
	stats.CorrectAnswers = correctCount

	hintOnFirst, err := e.checkHintOnFirstTask(userID)
	if err != nil {
		return nil, err
	}
	stats.HintOnFirstTask = hintOnFirst

	activeSteps, err := e.stepRepo.GetActive()
	if err != nil {
		return nil, err
	}
	stats.TotalAvailableHints = len(activeSteps)
	stats.AllHintsUsed = hintsUsed >= stats.TotalAvailableHints && stats.TotalAvailableHints > 0

	return stats, nil
}

func (e *AchievementEngine) checkHintOnFirstTask(userID int64) (bool, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_answers ua
			JOIN steps s ON ua.step_id = s.id
			WHERE ua.user_id = ? AND ua.hint_used = 1 AND s.step_order = 1
		`, userID).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (e *AchievementEngine) EvaluateHintAchievements(userID int64) ([]string, error) {
	stats, err := e.GetHintStats(userID)
	if err != nil {
		return nil, err
	}

	var awarded []string

	for _, threshold := range HintThresholds {
		if stats.TotalHintsUsed >= threshold {
			achievementKey := HintAchievementKeys[threshold]
			wasAwarded, err := e.tryAwardHintAchievement(userID, achievementKey)
			if err != nil {
				log.Printf("[ACHIEVEMENT_ENGINE] Error awarding hint achievement %s: %v", achievementKey, err)
				continue
			}
			if wasAwarded {
				awarded = append(awarded, achievementKey)
			}
		}
	}

	if stats.AllHintsUsed {
		wasAwarded, err := e.tryAwardHintAchievement(userID, "hint_master")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding hint_master achievement: %v", err)
		} else if wasAwarded {
			awarded = append(awarded, "hint_master")
		}
	}

	if stats.HintOnFirstTask {
		wasAwarded, err := e.tryAwardHintAchievement(userID, "skeptic")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding skeptic achievement: %v", err)
		} else if wasAwarded {
			awarded = append(awarded, "skeptic")
		}
	}

	return awarded, nil
}

func (e *AchievementEngine) tryAwardHintAchievement(userID int64, achievementKey string) (bool, error) {
	hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
	if err != nil {
		return false, err
	}

	log.Printf("[ACHIEVEMENT_ENGINE] Awarded hint achievement %s to user %d", achievementKey, userID)
	return true, nil
}

func (e *AchievementEngine) OnHintUsed(userID int64) ([]string, error) {
	return e.EvaluateHintAchievements(userID)
}

type SpecialStats struct {
	PhotoSubmitted     bool
	PhotoOnTextTask    bool
	ConsecutiveCorrect int
	SpecificAnswerUsed bool
	InactiveHours      int
	IsPostCompletion   bool
	FirstAnswerTime    *time.Time
	LastActivityTime   *time.Time
}

func (e *AchievementEngine) GetSpecialStats(userID int64) (*SpecialStats, error) {
	stats := &SpecialStats{}

	photoSubmitted, err := e.checkPhotoSubmitted(userID)
	if err != nil {
		return nil, err
	}
	stats.PhotoSubmitted = photoSubmitted

	photoOnTextTask, err := e.checkPhotoOnTextTask(userID)
	if err != nil {
		return nil, err
	}
	stats.PhotoOnTextTask = photoOnTextTask

	consecutiveCorrect, err := e.getConsecutiveCorrectCount(userID)
	if err != nil {
		return nil, err
	}
	stats.ConsecutiveCorrect = consecutiveCorrect

	specificAnswerUsed, err := e.checkSpecificAnswer(userID, "сезам откройся")
	if err != nil {
		return nil, err
	}
	stats.SpecificAnswerUsed = specificAnswerUsed

	firstTime, lastTime, err := e.getUserAnswerTimeRange(userID)
	if err != nil {
		return nil, err
	}
	stats.FirstAnswerTime = firstTime
	stats.LastActivityTime = lastTime

	if firstTime != nil {
		inactiveHours, err := e.calculateInactiveHours(userID)
		if err != nil {
			return nil, err
		}
		stats.InactiveHours = inactiveHours
	}

	completionStats, err := e.GetCompletionStats(userID)
	if err != nil {
		return nil, err
	}
	stats.IsPostCompletion = completionStats.IsCompleted

	return stats, nil
}

func (e *AchievementEngine) checkPhotoSubmitted(userID int64) (bool, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM answer_images ai
			JOIN user_answers ua ON ai.answer_id = ua.id
			JOIN steps s ON ua.step_id = s.id
			WHERE ua.user_id = ? AND s.answer_type = 'image'
		`, userID).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (e *AchievementEngine) checkPhotoOnTextTask(userID int64) (bool, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM answer_images ai
			JOIN user_answers ua ON ai.answer_id = ua.id
			JOIN steps s ON ua.step_id = s.id
			WHERE ua.user_id = ? AND s.answer_type = 'text'
		`, userID).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (e *AchievementEngine) getConsecutiveCorrectCount(userID int64) (int, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		rows, err := db.Query(`
			SELECT ua.id, up.status
			FROM user_answers ua
			LEFT JOIN user_progress up ON ua.user_id = up.user_id AND ua.step_id = up.step_id
			WHERE ua.user_id = ?
			ORDER BY ua.created_at ASC
		`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		maxConsecutive := 0
		currentConsecutive := 0
		var lastStepCorrect *bool

		for rows.Next() {
			var answerID int64
			var status sql.NullString
			if err := rows.Scan(&answerID, &status); err != nil {
				return nil, err
			}

			isCorrect := status.Valid && status.String == "approved"

			if isCorrect {
				if lastStepCorrect == nil || *lastStepCorrect {
					currentConsecutive++
				} else {
					currentConsecutive = 1
				}
				if currentConsecutive > maxConsecutive {
					maxConsecutive = currentConsecutive
				}
			} else {
				currentConsecutive = 0
			}

			lastStepCorrect = &isCorrect
		}

		return maxConsecutive, rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (e *AchievementEngine) checkSpecificAnswer(userID int64, answer string) (bool, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_answers
			WHERE user_id = ? AND LOWER(text_answer) = LOWER(?)
		`, userID, answer).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (e *AchievementEngine) calculateInactiveHours(userID int64) (int, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		var firstAnswerStr, lastAnswerStr sql.NullString
		err := db.QueryRow(`
			SELECT MIN(created_at), MAX(created_at)
			FROM user_answers
			WHERE user_id = ?
		`, userID).Scan(&firstAnswerStr, &lastAnswerStr)
		if err != nil {
			return 0, err
		}

		if !firstAnswerStr.Valid || !lastAnswerStr.Valid {
			return 0, nil
		}

		firstTime, err := parseTimeString(firstAnswerStr.String)
		if err != nil {
			return 0, nil
		}

		var answerCount int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM user_answers WHERE user_id = ?
		`, userID).Scan(&answerCount)
		if err != nil {
			return 0, err
		}

		if answerCount <= 1 {
			hoursSinceFirst := int(time.Since(firstTime).Hours())
			return hoursSinceFirst, nil
		}

		return 0, nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (e *AchievementEngine) EvaluateSpecialAchievements(userID int64) ([]string, error) {
	stats, err := e.GetSpecialStats(userID)
	if err != nil {
		return nil, err
	}

	var awarded []string

	if stats.PhotoSubmitted {
		wasAwarded, err := e.tryAwardSpecialAchievement(userID, "photographer")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding photographer achievement: %v", err)
		} else if wasAwarded {
			awarded = append(awarded, "photographer")
		}
	}

	if stats.PhotoOnTextTask {
		wasAwarded, err := e.tryAwardSpecialAchievement(userID, "paparazzi")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding paparazzi achievement: %v", err)
		} else if wasAwarded {
			awarded = append(awarded, "paparazzi")
		}
	}

	if stats.ConsecutiveCorrect >= 10 {
		wasAwarded, err := e.tryAwardSpecialAchievement(userID, "bullseye")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding bullseye achievement: %v", err)
		} else if wasAwarded {
			awarded = append(awarded, "bullseye")
		}
	}

	if stats.SpecificAnswerUsed {
		wasAwarded, err := e.tryAwardSpecialAchievement(userID, "secret_agent")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding secret_agent achievement: %v", err)
		} else if wasAwarded {
			awarded = append(awarded, "secret_agent")
		}
	}

	if stats.InactiveHours >= 24 {
		wasAwarded, err := e.tryAwardSpecialAchievement(userID, "curious")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding curious achievement: %v", err)
		} else if wasAwarded {
			awarded = append(awarded, "curious")
		}
	}

	return awarded, nil
}

func (e *AchievementEngine) tryAwardSpecialAchievement(userID int64, achievementKey string) (bool, error) {
	hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
	if err != nil {
		return false, err
	}

	log.Printf("[ACHIEVEMENT_ENGINE] Awarded special achievement %s to user %d", achievementKey, userID)
	return true, nil
}

func (e *AchievementEngine) OnPhotoSubmitted(userID int64, isTextTask bool) ([]string, error) {
	var awarded []string

	if isTextTask {
		paparazziAwarded, err := e.tryAwardSpecialAchievement(userID, "paparazzi")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding paparazzi: %v", err)
		} else if paparazziAwarded {
			awarded = append(awarded, "paparazzi")
		}
	} else {
		photographerAwarded, err := e.tryAwardSpecialAchievement(userID, "photographer")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding photographer: %v", err)
		} else if photographerAwarded {
			awarded = append(awarded, "photographer")
		}
	}

	return awarded, nil
}

func (e *AchievementEngine) OnAnswerSubmitted(userID int64, answer string) ([]string, error) {
	var awarded []string

	if strings.ToLower(strings.TrimSpace(answer)) == "сезам откройся" {
		secretAgentAwarded, err := e.tryAwardSpecialAchievement(userID, "secret_agent")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding secret_agent: %v", err)
		} else if secretAgentAwarded {
			awarded = append(awarded, "secret_agent")
		}
	}

	consecutiveCorrect, err := e.getConsecutiveCorrectCount(userID)
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error getting consecutive correct count: %v", err)
	} else if consecutiveCorrect >= 10 {
		bullseyeAwarded, err := e.tryAwardSpecialAchievement(userID, "bullseye")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding bullseye: %v", err)
		} else if bullseyeAwarded {
			awarded = append(awarded, "bullseye")
		}
	}

	return awarded, nil
}

func (e *AchievementEngine) OnPostCompletionActivity(userID int64) ([]string, error) {
	completionStats, err := e.GetCompletionStats(userID)
	if err != nil {
		return nil, err
	}

	if !completionStats.IsCompleted {
		return nil, nil
	}

	var awarded []string
	fanAwarded, err := e.tryAwardSpecialAchievement(userID, "fan")
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error awarding fan: %v", err)
	} else if fanAwarded {
		awarded = append(awarded, "fan")
	}

	return awarded, nil
}

func (e *AchievementEngine) CheckInactivityAchievement(userID int64) ([]string, error) {
	inactiveHours, err := e.calculateInactiveHours(userID)
	if err != nil {
		return nil, err
	}

	if inactiveHours >= 24 {
		var awarded []string
		curiousAwarded, err := e.tryAwardSpecialAchievement(userID, "curious")
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error awarding curious: %v", err)
		} else if curiousAwarded {
			awarded = append(awarded, "curious")
		}
		return awarded, nil
	}

	return nil, nil
}

func (e *AchievementEngine) GetCurrentConsecutiveCorrect(userID int64) (int, error) {
	result, err := e.queue.Execute(func(db *sql.DB) (any, error) {
		rows, err := db.Query(`
			SELECT ua.step_id, up.status
			FROM user_answers ua
			LEFT JOIN user_progress up ON ua.user_id = up.user_id AND ua.step_id = up.step_id
			WHERE ua.user_id = ?
			ORDER BY ua.created_at DESC
		`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		currentConsecutive := 0
		for rows.Next() {
			var stepID int64
			var status sql.NullString
			if err := rows.Scan(&stepID, &status); err != nil {
				return nil, err
			}

			isCorrect := status.Valid && status.String == "approved"
			if isCorrect {
				currentConsecutive++
			} else {
				break
			}
		}

		return currentConsecutive, rows.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

var CompositeAchievementKeys = map[string]string{
	"super_collector": "super_collector",
	"super_brain":     "super_brain",
	"legend":          "legend",
}

var SuperCollectorRequiredAchievements = []string{
	"beginner_5",
	"experienced_10",
	"advanced_15",
	"expert_20",
	"master_25",
}

var SuperBrainRequiredConditions = struct {
	NoErrors              bool
	NoHints               bool
	CompletionTimeMinutes int
}{
	NoErrors:              true,
	NoHints:               true,
	CompletionTimeMinutes: 30,
}

var LegendRequiredAchievements = []string{
	"pioneer",
	"second_place",
	"third_place",
	"fourth_place",
	"fifth_place",
	"sixth_place",
	"seventh_place",
	"eighth_place",
	"ninth_place",
	"tenth_place",
	"beginner_5",
	"experienced_10",
	"advanced_15",
	"expert_20",
	"master_25",
	"winner",
	"perfect_path",
	"self_sufficient",
	"lightning",
	"rocket",
	"cheater",
}

type CompositeStats struct {
	HasAllProgressAchievements   bool
	HasAllCompletionAchievements bool
	HasAllPositionAchievements   bool
	IsCompleted                  bool
	NoErrors                     bool
	NoHints                      bool
	CompletionTimeMinutes        int
	UserAchievementKeys          []string
}

func (e *AchievementEngine) GetCompositeStats(userID int64) (*CompositeStats, error) {
	stats := &CompositeStats{}

	userAchievements, err := e.achievementRepo.GetUserAchievements(userID)
	if err != nil {
		return nil, err
	}

	achievementKeys := make(map[string]bool)
	for _, ua := range userAchievements {
		achievement, err := e.achievementRepo.GetByID(ua.AchievementID)
		if err != nil {
			continue
		}
		achievementKeys[achievement.Key] = true
		stats.UserAchievementKeys = append(stats.UserAchievementKeys, achievement.Key)
	}

	stats.HasAllProgressAchievements = true
	for _, key := range SuperCollectorRequiredAchievements {
		if !achievementKeys[key] {
			stats.HasAllProgressAchievements = false
			break
		}
	}

	completionAchievements := []string{"winner", "perfect_path", "self_sufficient", "lightning", "rocket", "cheater"}
	stats.HasAllCompletionAchievements = true
	for _, key := range completionAchievements {
		if !achievementKeys[key] {
			stats.HasAllCompletionAchievements = false
			break
		}
	}

	positionAchievements := []string{
		"pioneer", "second_place", "third_place", "fourth_place", "fifth_place",
		"sixth_place", "seventh_place", "eighth_place", "ninth_place", "tenth_place",
	}
	stats.HasAllPositionAchievements = true
	for _, key := range positionAchievements {
		if !achievementKeys[key] {
			stats.HasAllPositionAchievements = false
			break
		}
	}

	completionStats, err := e.GetCompletionStats(userID)
	if err != nil {
		return nil, err
	}
	stats.IsCompleted = completionStats.IsCompleted
	stats.NoErrors = completionStats.TotalAnswers == completionStats.CorrectAnswers
	stats.NoHints = completionStats.HintsUsed == 0
	stats.CompletionTimeMinutes = completionStats.CompletionTimeMinutes

	return stats, nil
}

func (e *AchievementEngine) EvaluateCompositeAchievements(userID int64) ([]string, error) {
	var awarded []string

	superCollectorAwarded, err := e.evaluateSuperCollector(userID)
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating super_collector: %v", err)
	} else if superCollectorAwarded {
		awarded = append(awarded, CompositeAchievementKeys["super_collector"])
	}

	superBrainAwarded, err := e.evaluateSuperBrain(userID)
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating super_brain: %v", err)
	} else if superBrainAwarded {
		awarded = append(awarded, CompositeAchievementKeys["super_brain"])
	}

	legendAwarded, err := e.evaluateLegend(userID)
	if err != nil {
		log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating legend: %v", err)
	} else if legendAwarded {
		awarded = append(awarded, CompositeAchievementKeys["legend"])
	}

	return awarded, nil
}

func (e *AchievementEngine) evaluateSuperCollector(userID int64) (bool, error) {
	achievementKey := CompositeAchievementKeys["super_collector"]

	hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	for _, reqKey := range SuperCollectorRequiredAchievements {
		has, err := e.achievementRepo.HasUserAchievement(userID, reqKey)
		if err != nil {
			return false, err
		}
		if !has {
			return false, nil
		}
	}

	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
	if err != nil {
		return false, err
	}

	log.Printf("[ACHIEVEMENT_ENGINE] Awarded composite achievement %s to user %d", achievementKey, userID)
	return true, nil
}

func (e *AchievementEngine) evaluateSuperBrain(userID int64) (bool, error) {
	achievementKey := CompositeAchievementKeys["super_brain"]

	hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	stats, err := e.GetCompletionStats(userID)
	if err != nil {
		return false, err
	}

	if !stats.IsCompleted {
		return false, nil
	}

	if stats.TotalAnswers != stats.CorrectAnswers {
		return false, nil
	}

	if stats.HintsUsed > 0 {
		return false, nil
	}

	if stats.CompletionTimeMinutes >= SuperBrainRequiredConditions.CompletionTimeMinutes {
		return false, nil
	}

	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
	if err != nil {
		return false, err
	}

	log.Printf("[ACHIEVEMENT_ENGINE] Awarded composite achievement %s to user %d", achievementKey, userID)
	return true, nil
}

func (e *AchievementEngine) evaluateLegend(userID int64) (bool, error) {
	achievementKey := CompositeAchievementKeys["legend"]

	hasAchievement, err := e.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	for _, reqKey := range LegendRequiredAchievements {
		has, err := e.achievementRepo.HasUserAchievement(userID, reqKey)
		if err != nil {
			return false, err
		}
		if !has {
			return false, nil
		}
	}

	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return false, err
	}

	err = e.achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
	if err != nil {
		return false, err
	}

	log.Printf("[ACHIEVEMENT_ENGINE] Awarded composite achievement %s to user %d", achievementKey, userID)
	return true, nil
}

func (e *AchievementEngine) OnAchievementAwarded(userID int64) ([]string, error) {
	return e.EvaluateCompositeAchievements(userID)
}

func (e *AchievementEngine) EvaluateCompositeConditions(userID int64, achievement *models.Achievement) (bool, error) {
	conditions := achievement.Conditions

	if len(conditions.RequiredAchievements) > 0 {
		for _, reqKey := range conditions.RequiredAchievements {
			has, err := e.achievementRepo.HasUserAchievement(userID, reqKey)
			if err != nil {
				return false, err
			}
			if !has {
				return false, nil
			}
		}
	}

	if conditions.NoErrors != nil && *conditions.NoErrors {
		stats, err := e.GetCompletionStats(userID)
		if err != nil {
			return false, err
		}
		if !stats.IsCompleted || stats.TotalAnswers != stats.CorrectAnswers {
			return false, nil
		}
	}

	if conditions.NoHints != nil && *conditions.NoHints {
		stats, err := e.GetCompletionStats(userID)
		if err != nil {
			return false, err
		}
		if !stats.IsCompleted || stats.HintsUsed > 0 {
			return false, nil
		}
	}

	if conditions.CompletionTimeMinutes != nil {
		stats, err := e.GetCompletionStats(userID)
		if err != nil {
			return false, err
		}
		if !stats.IsCompleted || stats.CompletionTimeMinutes >= *conditions.CompletionTimeMinutes {
			return false, nil
		}
	}

	return true, nil
}

func (e *AchievementEngine) EvaluateCompositeConditionsWithTimestamp(userID int64, achievement *models.Achievement) (bool, time.Time, error) {
	qualifies, err := e.EvaluateCompositeConditions(userID, achievement)
	if err != nil {
		return false, time.Time{}, err
	}
	if !qualifies {
		return false, time.Time{}, nil
	}

	conditions := achievement.Conditions
	earnedAt := time.Now()

	if len(conditions.RequiredAchievements) > 0 {
		var latestTime time.Time
		userAchievements, err := e.achievementRepo.GetUserAchievements(userID)
		if err != nil {
			return false, time.Time{}, err
		}

		for _, reqKey := range conditions.RequiredAchievements {
			reqAchievement, err := e.achievementRepo.GetByKey(reqKey)
			if err != nil {
				continue
			}
			for _, ua := range userAchievements {
				if ua.AchievementID == reqAchievement.ID && ua.EarnedAt.After(latestTime) {
					latestTime = ua.EarnedAt
				}
			}
		}
		if !latestTime.IsZero() {
			earnedAt = latestTime
		}
	}

	return true, earnedAt, nil
}

func (e *AchievementEngine) EvaluateRetroactiveCompositeAchievements(achievementKey string) ([]int64, error) {
	achievement, err := e.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return nil, err
	}

	if achievement.Category != models.CategoryComposite {
		return nil, nil
	}

	users, err := e.userRepo.GetAll()
	if err != nil {
		return nil, err
	}

	var awardedUsers []int64
	for _, user := range users {
		hasAchievement, err := e.achievementRepo.HasUserAchievement(user.ID, achievementKey)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error checking user %d achievement: %v", user.ID, err)
			continue
		}
		if hasAchievement {
			continue
		}

		qualifies, earnedAt, err := e.EvaluateCompositeConditionsWithTimestamp(user.ID, achievement)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error evaluating composite conditions for user %d: %v", user.ID, err)
			continue
		}

		if qualifies {
			err = e.achievementRepo.AssignToUser(user.ID, achievement.ID, earnedAt, true)
			if err != nil {
				log.Printf("[ACHIEVEMENT_ENGINE] Error assigning retroactive composite achievement to user %d: %v", user.ID, err)
				continue
			}
			awardedUsers = append(awardedUsers, user.ID)
			log.Printf("[ACHIEVEMENT_ENGINE] Retroactively awarded composite achievement %s to user %d", achievementKey, user.ID)
		}
	}

	return awardedUsers, nil
}

func (e *AchievementEngine) RecalculatePositionAchievements() (map[string]int64, error) {
	e.uniqueMutex.Lock()
	defer e.uniqueMutex.Unlock()

	positionAchievements := []string{
		"pioneer", "second_place", "third_place", "fourth_place", "fifth_place",
		"sixth_place", "seventh_place", "eighth_place", "ninth_place", "tenth_place",
	}

	usersWithFirstAnswer, err := e.getUsersOrderedByFirstCorrectAnswer()
	if err != nil {
		return nil, err
	}

	awarded := make(map[string]int64)

	for i, key := range positionAchievements {
		achievement, err := e.achievementRepo.GetByKey(key)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Position achievement %s not found: %v", key, err)
			continue
		}

		holders, err := e.achievementRepo.GetAchievementHolders(key)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error getting holders for %s: %v", key, err)
			continue
		}

		position := i + 1
		if position > len(usersWithFirstAnswer) {
			continue
		}

		correctUserID := usersWithFirstAnswer[position-1].UserID
		earnedAt := usersWithFirstAnswer[position-1].FirstCorrectAnswerTime

		if len(holders) > 0 && holders[0] == correctUserID {
			continue
		}

		if len(holders) > 0 {
			for _, holderID := range holders {
				e.achievementRepo.RemoveUserAchievement(holderID, achievement.ID)
				log.Printf("[ACHIEVEMENT_ENGINE] Removed position achievement %s from user %d", key, holderID)
			}
		}

		err = e.achievementRepo.AssignToUser(correctUserID, achievement.ID, earnedAt, true)
		if err != nil {
			log.Printf("[ACHIEVEMENT_ENGINE] Error assigning position achievement %s to user %d: %v", key, correctUserID, err)
			continue
		}

		awarded[key] = correctUserID
		log.Printf("[ACHIEVEMENT_ENGINE] Reassigned position achievement %s to user %d", key, correctUserID)
	}

	return awarded, nil
}
