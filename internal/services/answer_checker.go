package services

import (
	"strings"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type CheckResult struct {
	IsCorrect  bool
	Percentage int
}

type AnswerChecker struct {
	answerRepo   *db.AnswerRepository
	progressRepo *db.ProgressRepository
	userRepo     *db.UserRepository
}

func NewAnswerChecker(answerRepo *db.AnswerRepository, progressRepo *db.ProgressRepository, userRepo *db.UserRepository) *AnswerChecker {
	return &AnswerChecker{
		answerRepo:   answerRepo,
		progressRepo: progressRepo,
		userRepo:     userRepo,
	}
}

func (c *AnswerChecker) CheckTextAnswer(stepID int64, answer string) (*CheckResult, error) {
	variants, err := c.answerRepo.GetStepAnswers(stepID)
	if err != nil {
		return nil, err
	}

	normalizedAnswer := strings.ToLower(strings.TrimSpace(answer))
	// log.Printf("[ANSWER_CHECKER] stepID=%d answer='%s' normalized='%s' variants=%v", stepID, answer, normalizedAnswer, variants)

	isCorrect := false
	for _, variant := range variants {
		if normalizedAnswer == variant {
			isCorrect = true
			break
		}
	}

	result := &CheckResult{
		IsCorrect: isCorrect,
	}

	if isCorrect {
		percentage, err := c.calculatePercentage(stepID)
		if err != nil {
			return nil, err
		}
		result.Percentage = percentage
	}

	// log.Printf("[ANSWER_CHECKER] isCorrect=%t percentage=%d", isCorrect, result.Percentage)
	return result, nil
}

func (c *AnswerChecker) calculatePercentage(stepID int64) (int, error) {
	approvedCount, err := c.progressRepo.CountByStep(stepID, models.StatusApproved)
	if err != nil {
		return 0, err
	}

	users, err := c.userRepo.GetAll()
	if err != nil {
		return 0, err
	}

	totalUsers := len(users)
	if totalUsers == 0 {
		return 100, nil
	}

	return (approvedCount * 100) / totalUsers, nil
}
