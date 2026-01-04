package services

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func setupTestDBForUserStats(t *testing.T) (*db.DBQueue, func()) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatal(err)
	}

	queue := db.NewDBQueue(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func TestProperty1_AnswerTimeBounds(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		answerRepo := db.NewAnswerRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		calculator := NewUserStatisticsCalculator(answerRepo, progressRepo, statsService)

		userID := rapid.Int64Range(1, 1000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
			CreatedAt: time.Now().Add(-time.Hour * 24),
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:  1,
			Text:       "Test Step",
			AnswerType: models.AnswerTypeText,
			IsActive:   true,
			IsDeleted:  false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		numAnswers := rapid.IntRange(0, 10).Draw(rt, "numAnswers")
		var expectedTimes []time.Time

		baseTime := time.Now().Add(-time.Hour * 2)
		for i := range numAnswers {
			answerTime := baseTime.Add(time.Duration(i) * time.Minute * 10)
			expectedTimes = append(expectedTimes, answerTime)

			answerID, err := answerRepo.CreateTextAnswer(userID, stepID, "test answer", false)
			if err != nil {
				rt.Fatal(err)
			}

			_, err = queue.Execute(func(db *sql.DB) (interface{}, error) {
				_, err := db.Exec(`UPDATE user_answers SET created_at = ? WHERE id = ?`,
					answerTime, answerID)
				return nil, err
			})
			if err != nil {
				rt.Fatal(err)
			}
		}

		stats, err := calculator.Calculate(user, step)
		if err != nil {
			rt.Fatal(err)
		}

		if numAnswers == 0 {
			if stats.FirstAnswerTime != nil {
				rt.Errorf("Expected FirstAnswerTime to be nil for user with no answers")
			}
			if stats.LastAnswerTime != nil {
				rt.Errorf("Expected LastAnswerTime to be nil for user with no answers")
			}
		} else {
			if stats.FirstAnswerTime == nil {
				rt.Errorf("Expected FirstAnswerTime to be non-nil for user with answers")
			} else if !stats.FirstAnswerTime.Equal(expectedTimes[0]) {
				rt.Errorf("FirstAnswerTime mismatch: expected %v, got %v", expectedTimes[0], *stats.FirstAnswerTime)
			}

			if stats.LastAnswerTime == nil {
				rt.Errorf("Expected LastAnswerTime to be non-nil for user with answers")
			} else if !stats.LastAnswerTime.Equal(expectedTimes[len(expectedTimes)-1]) {
				rt.Errorf("LastAnswerTime mismatch: expected %v, got %v", expectedTimes[len(expectedTimes)-1], *stats.LastAnswerTime)
			}
		}
	})
}

func TestProperty2_CompletionTimeCalculation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		answerRepo := db.NewAnswerRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		calculator := NewUserStatisticsCalculator(answerRepo, progressRepo, statsService)

		userID := rapid.Int64Range(1, 1000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
			CreatedAt: time.Now().Add(-time.Hour * 24),
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:  1,
			Text:       "Test Step",
			AnswerType: models.AnswerTypeText,
			IsActive:   true,
			IsDeleted:  false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		numAnswers := rapid.IntRange(0, 5).Draw(rt, "numAnswers")

		baseTime := time.Now().Add(-time.Hour * 2)
		var firstTime, lastTime time.Time

		for i := range numAnswers {
			answerTime := baseTime.Add(time.Duration(i) * time.Minute * 30)
			if i == 0 {
				firstTime = answerTime
			}
			if i == numAnswers-1 {
				lastTime = answerTime
			}

			answerID, err := answerRepo.CreateTextAnswer(userID, stepID, "test answer", false)
			if err != nil {
				rt.Fatal(err)
			}

			_, err = queue.Execute(func(db *sql.DB) (interface{}, error) {
				_, err := db.Exec(`UPDATE user_answers SET created_at = ? WHERE id = ?`,
					answerTime, answerID)
				return nil, err
			})
			if err != nil {
				rt.Fatal(err)
			}
		}

		stats, err := calculator.Calculate(user, step)
		if err != nil {
			rt.Fatal(err)
		}

		if numAnswers < 2 {
			if stats.CompletionTime != nil {
				rt.Errorf("Expected CompletionTime to be nil for user with fewer than 2 answers")
			}
		} else {
			if stats.CompletionTime == nil {
				rt.Errorf("Expected CompletionTime to be non-nil for user with 2+ answers")
			} else {
				expectedDuration := lastTime.Sub(firstTime)
				if *stats.CompletionTime != expectedDuration {
					rt.Errorf("CompletionTime mismatch: expected %v, got %v", expectedDuration, *stats.CompletionTime)
				}
			}
		}
	})
}

func TestProperty3_CountAccuracy(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		answerRepo := db.NewAnswerRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		calculator := NewUserStatisticsCalculator(answerRepo, progressRepo, statsService)

		userID := rapid.Int64Range(1, 1000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
			CreatedAt: time.Now().Add(-time.Hour * 24),
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		numSteps := rapid.IntRange(1, 5).Draw(rt, "numSteps")
		var stepIDs []int64

		for i := range numSteps {
			step := &models.Step{
				StepOrder:  i + 1,
				Text:       "Test Step",
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			stepID, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}
			stepIDs = append(stepIDs, stepID)
		}

		totalAnswers := rapid.IntRange(0, 20).Draw(rt, "totalAnswers")
		approvedSteps := rapid.IntRange(0, numSteps).Draw(rt, "approvedSteps")

		for i := range totalAnswers {
			stepID := stepIDs[i%len(stepIDs)]
			_, err := answerRepo.CreateTextAnswer(userID, stepID, "test answer", false)
			if err != nil {
				rt.Fatal(err)
			}
		}

		for i := range approvedSteps {
			progress := &models.UserProgress{
				UserID: userID,
				StepID: stepIDs[i],
				Status: models.StatusApproved,
			}
			if err := progressRepo.Create(progress); err != nil {
				rt.Fatal(err)
			}
		}

		stats, err := calculator.Calculate(user, &models.Step{ID: stepIDs[0]})
		if err != nil {
			rt.Fatal(err)
		}

		if stats.TotalAnswers != totalAnswers {
			rt.Errorf("TotalAnswers mismatch: expected %d, got %d", totalAnswers, stats.TotalAnswers)
		}

		if stats.ApprovedSteps != approvedSteps {
			rt.Errorf("ApprovedSteps mismatch: expected %d, got %d", approvedSteps, stats.ApprovedSteps)
		}
	})
}

func TestProperty4_AccuracyPercentageCalculation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		answerRepo := db.NewAnswerRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		calculator := NewUserStatisticsCalculator(answerRepo, progressRepo, statsService)

		userID := rapid.Int64Range(1, 1000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
			CreatedAt: time.Now().Add(-time.Hour * 24),
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:  1,
			Text:       "Test Step",
			AnswerType: models.AnswerTypeText,
			IsActive:   true,
			IsDeleted:  false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		totalAnswers := rapid.IntRange(0, 20).Draw(rt, "totalAnswers")
		approvedSteps := rapid.IntRange(0, min(totalAnswers, 5)).Draw(rt, "approvedSteps")

		for range totalAnswers {
			_, err := answerRepo.CreateTextAnswer(userID, stepID, "test answer", false)
			if err != nil {
				rt.Fatal(err)
			}
		}

		// Only create one progress record per step to avoid constraint violation
		if approvedSteps > 0 {
			progress := &models.UserProgress{
				UserID: userID,
				StepID: stepID,
				Status: models.StatusApproved,
			}
			if err := progressRepo.Create(progress); err != nil {
				rt.Fatal(err)
			}
		}

		stats, err := calculator.Calculate(user, step)
		if err != nil {
			rt.Fatal(err)
		}

		var expectedAccuracy int
		if totalAnswers > 0 && approvedSteps > 0 {
			expectedAccuracy = (1 * 100) / totalAnswers // Only one approved step
		} else {
			expectedAccuracy = 0
		}

		if stats.Accuracy != expectedAccuracy {
			rt.Errorf("Accuracy mismatch: expected %d%%, got %d%%", expectedAccuracy, stats.Accuracy)
		}
	})
}

func TestProperty5_AverageResponseTimeCalculation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		answerRepo := db.NewAnswerRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		calculator := NewUserStatisticsCalculator(answerRepo, progressRepo, statsService)

		userID := rapid.Int64Range(1, 1000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
			CreatedAt: time.Now().Add(-time.Hour * 24),
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		step := &models.Step{
			StepOrder:  1,
			Text:       "Test Step",
			AnswerType: models.AnswerTypeText,
			IsActive:   true,
			IsDeleted:  false,
		}
		stepID, err := stepRepo.Create(step)
		if err != nil {
			rt.Fatal(err)
		}

		numAnswers := rapid.IntRange(0, 10).Draw(rt, "numAnswers")
		intervalMinutes := rapid.IntRange(1, 60).Draw(rt, "intervalMinutes")

		baseTime := time.Now().Add(-time.Hour * 2)
		var firstTime, lastTime time.Time

		for i := range numAnswers {
			answerTime := baseTime.Add(time.Duration(i*intervalMinutes) * time.Minute)
			if i == 0 {
				firstTime = answerTime
			}
			if i == numAnswers-1 {
				lastTime = answerTime
			}

			answerID, err := answerRepo.CreateTextAnswer(userID, stepID, "test answer", false)
			if err != nil {
				rt.Fatal(err)
			}

			_, err = queue.Execute(func(db *sql.DB) (interface{}, error) {
				_, err := db.Exec(`UPDATE user_answers SET created_at = ? WHERE id = ?`,
					answerTime, answerID)
				return nil, err
			})
			if err != nil {
				rt.Fatal(err)
			}
		}

		stats, err := calculator.Calculate(user, step)
		if err != nil {
			rt.Fatal(err)
		}

		if numAnswers < 2 {
			if stats.AverageResponseTime != nil {
				rt.Errorf("Expected AverageResponseTime to be nil for user with fewer than 2 answers")
			}
		} else {
			if stats.AverageResponseTime == nil {
				rt.Errorf("Expected AverageResponseTime to be non-nil for user with 2+ answers")
			} else {
				totalInterval := lastTime.Sub(firstTime)
				expectedAvg := totalInterval / time.Duration(numAnswers-1)
				if *stats.AverageResponseTime != expectedAvg {
					rt.Errorf("AverageResponseTime mismatch: expected %v, got %v", expectedAvg, *stats.AverageResponseTime)
				}
			}
		}
	})
}

func TestProperty8_StepAttemptsAccuracy(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		answerRepo := db.NewAnswerRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		calculator := NewUserStatisticsCalculator(answerRepo, progressRepo, statsService)

		userID := rapid.Int64Range(1, 1000).Draw(rt, "userID")
		user := &models.User{
			ID:        userID,
			FirstName: "Test",
			CreatedAt: time.Now().Add(-time.Hour * 24),
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		numSteps := rapid.IntRange(1, 5).Draw(rt, "numSteps")
		var stepIDs []int64
		expectedAttempts := make(map[int64]int)

		for i := range numSteps {
			step := &models.Step{
				StepOrder:  i + 1,
				Text:       "Test Step",
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			stepID, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}
			stepIDs = append(stepIDs, stepID)

			attempts := rapid.IntRange(1, 5).Draw(rt, "attempts")
			expectedAttempts[stepID] = attempts

			for range attempts {
				_, err := answerRepo.CreateTextAnswer(userID, stepID, "test answer", false)
				if err != nil {
					rt.Fatal(err)
				}
			}
		}

		stats, err := calculator.Calculate(user, &models.Step{ID: stepIDs[0]})
		if err != nil {
			rt.Fatal(err)
		}

		stepAttemptsMap := make(map[int]int)
		for _, attempt := range stats.StepAttempts {
			stepAttemptsMap[attempt.StepOrder] = attempt.Attempts
		}

		for stepID, expectedCount := range expectedAttempts {
			if expectedCount > 1 {
				found := false
				for _, attempt := range stats.StepAttempts {
					if int64(attempt.StepOrder) == stepID && attempt.Attempts == expectedCount {
						found = true
						break
					}
				}
				if !found {
					rt.Errorf("Expected step %d with %d attempts to be in StepAttempts", stepID, expectedCount)
				}
			}
		}

		for _, attempt := range stats.StepAttempts {
			if attempt.Attempts <= 1 {
				rt.Errorf("StepAttempts should only contain steps with more than 1 attempt, found step %d with %d attempts",
					attempt.StepOrder, attempt.Attempts)
			}
		}
	})
}
