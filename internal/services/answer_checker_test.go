package services

import (
	"strings"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func TestProperty6_CaseInsensitiveAnswerMatching(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		checker := NewAnswerChecker(answerRepo, progressRepo, userRepo)

		step := &models.Step{
			StepOrder:    1,
			Text:         "Test step",
			AnswerType:   models.AnswerTypeText,
			HasAutoCheck: true,
			IsActive:     true,
			IsDeleted:    false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		baseAnswer := rapid.StringMatching(`[a-zA-Z]{3,10}`).Draw(rt, "baseAnswer")
		if err := answerRepo.AddStepAnswer(stepID, baseAnswer); err != nil {
			rt.Fatal(err)
		}

		caseVariant := rapid.SampledFrom([]string{
			strings.ToLower(baseAnswer),
			strings.ToUpper(baseAnswer),
			mixCase(baseAnswer),
		}).Draw(rt, "caseVariant")

		result, err := checker.CheckTextAnswer(stepID, caseVariant)
		if err != nil {
			rt.Fatal(err)
		}

		if !result.IsCorrect {
			rt.Errorf("Expected answer '%s' to match variant '%s' (case-insensitive)", caseVariant, baseAnswer)
		}
	})
}

func mixCase(s string) string {
	result := make([]byte, len(s))
	for i, c := range []byte(s) {
		if i%2 == 0 {
			result[i] = byte(strings.ToUpper(string(c))[0])
		} else {
			result[i] = byte(strings.ToLower(string(c))[0])
		}
	}
	return string(result)
}
