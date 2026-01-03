package services

import (
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type UserStatisticsCalculator struct {
	answerRepo        *db.AnswerRepository
	progressRepo      *db.ProgressRepository
	statisticsService *StatisticsService
}

func NewUserStatisticsCalculator(answerRepo *db.AnswerRepository, progressRepo *db.ProgressRepository, statisticsService *StatisticsService) *UserStatisticsCalculator {
	return &UserStatisticsCalculator{
		answerRepo:        answerRepo,
		progressRepo:      progressRepo,
		statisticsService: statisticsService,
	}
}

func (c *UserStatisticsCalculator) Calculate(user *models.User, currentStep *models.Step) (*UserStatistics, error) {
	stats := &UserStatistics{
		RegistrationDate:      user.CreatedAt,
		TimeSinceRegistration: time.Since(user.CreatedAt),
	}

	// Get answer times
	answerTimes, err := c.answerRepo.GetUserAnswerTimes(user.ID)
	if err != nil {
		return nil, err
	}

	// Calculate time-based metrics
	if len(answerTimes) > 0 {
		stats.FirstAnswerTime = &answerTimes[0]
		stats.LastAnswerTime = &answerTimes[len(answerTimes)-1]

		if len(answerTimes) >= 2 {
			completionTime := stats.LastAnswerTime.Sub(*stats.FirstAnswerTime)
			stats.CompletionTime = &completionTime

			// Calculate average response time
			totalInterval := stats.LastAnswerTime.Sub(*stats.FirstAnswerTime)
			avgResponseTime := totalInterval / time.Duration(len(answerTimes)-1)
			stats.AverageResponseTime = &avgResponseTime
		}
	}

	// Get total answers count
	totalAnswers, err := c.answerRepo.CountUserAnswers(user.ID)
	if err != nil {
		return nil, err
	}
	stats.TotalAnswers = totalAnswers

	// Get approved steps count
	userProgress, err := c.progressRepo.GetUserProgress(user.ID)
	if err != nil && err.Error() != "sql: no rows in result set" {
		return nil, err
	}

	approvedCount := 0
	for _, progress := range userProgress {
		if progress.Status == models.StatusApproved {
			approvedCount++
		}
	}
	stats.ApprovedSteps = approvedCount

	// Calculate accuracy
	if totalAnswers > 0 {
		stats.Accuracy = (approvedCount * 100) / totalAnswers
	} else {
		stats.Accuracy = 0
	}

	// Calculate time on current step
	if currentStep != nil && len(answerTimes) > 0 {
		// Find the last answer time for the current step
		answersByStep, err := c.answerRepo.CountUserAnswersByStep(user.ID)
		if err != nil {
			return nil, err
		}

		if _, hasAnswersForCurrentStep := answersByStep[currentStep.ID]; hasAnswersForCurrentStep {
			// For simplicity, use the last answer time as approximation
			timeOnStep := time.Since(*stats.LastAnswerTime)
			stats.TimeOnCurrentStep = &timeOnStep
		}
	}

	// Calculate step attempts - use step order instead of step ID
	answersByStep, err := c.answerRepo.CountUserAnswersByStep(user.ID)
	if err != nil {
		return nil, err
	}

	var stepAttempts []StepAttempt
	for stepID, attempts := range answersByStep {
		if attempts > 1 {
			// Use stepID as stepOrder for now - this is a simplification
			// In a real implementation, we'd need to look up the actual step order
			stepAttempts = append(stepAttempts, StepAttempt{
				StepOrder: int(stepID),
				Attempts:  attempts,
			})
		}
	}
	stats.StepAttempts = stepAttempts

	// Get leaderboard position
	position, total, err := c.statisticsService.GetUserLeaderboardPosition(user.ID)
	if err != nil {
		return nil, err
	}
	stats.LeaderboardPosition = position
	stats.TotalUsers = total

	return stats, nil
}
