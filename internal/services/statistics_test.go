package services

import (
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func TestProperty10_StatisticsCalculationCorrectness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)

		numSteps := rapid.IntRange(1, 5).Draw(rt, "numSteps")
		numUsers := rapid.IntRange(1, 5).Draw(rt, "numUsers")

		var stepIDs []int64
		for i := 1; i <= numSteps; i++ {
			step := &models.Step{
				StepOrder:  i,
				Text:       "Step",
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			id, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}
			stepIDs = append(stepIDs, id)
		}

		var userIDs []int64
		for i := 1; i <= numUsers; i++ {
			userID := int64(i * 1000)
			user := &models.User{ID: userID}
			if err := userRepo.CreateOrUpdate(user); err != nil {
				rt.Fatal(err)
			}
			userIDs = append(userIDs, userID)
		}

		expectedCounts := make(map[int64]int)
		userMaxStep := make(map[int64]int)

		for _, userID := range userIDs {
			numApproved := rapid.IntRange(0, numSteps).Draw(rt, "numApproved")
			for i := 0; i < numApproved; i++ {
				progress := &models.UserProgress{
					UserID: userID,
					StepID: stepIDs[i],
					Status: models.StatusApproved,
				}
				if err := progressRepo.Create(progress); err != nil {
					rt.Fatal(err)
				}
				expectedCounts[stepIDs[i]]++
				if i+1 > userMaxStep[userID] {
					userMaxStep[userID] = i + 1
				}
			}
		}

		stats, err := statsService.CalculateStats()
		if err != nil {
			rt.Fatal(err)
		}

		for _, stepStat := range stats.StepStats {
			expected := expectedCounts[stepStat.StepID]
			if stepStat.Count != expected {
				rt.Errorf("Step %d: expected count %d, got %d", stepStat.StepID, expected, stepStat.Count)
			}
		}

		if len(stats.Leaders) != numUsers {
			rt.Errorf("Expected %d leaders, got %d", numUsers, len(stats.Leaders))
		}

		for i := 1; i < len(stats.Leaders); i++ {
			prevMax := userMaxStep[stats.Leaders[i-1].ID]
			currMax := userMaxStep[stats.Leaders[i].ID]
			if prevMax < currMax {
				rt.Errorf("Leaders not sorted correctly: user %d (max=%d) before user %d (max=%d)",
					stats.Leaders[i-1].ID, prevMax, stats.Leaders[i].ID, currMax)
			}
		}
	})
}

func TestProperty9_LeaderboardPositionCalculation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDB(t)
		defer cleanup()

		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		userRepo := db.NewUserRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)

		numSteps := rapid.IntRange(1, 5).Draw(rt, "numSteps")
		numUsers := rapid.IntRange(2, 10).Draw(rt, "numUsers")

		var stepIDs []int64
		for i := 1; i <= numSteps; i++ {
			step := &models.Step{
				StepOrder:  i,
				Text:       "Step",
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			id, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}
			stepIDs = append(stepIDs, id)
		}

		var userIDs []int64
		userMaxSteps := make(map[int64]int)

		for i := 1; i <= numUsers; i++ {
			userID := int64(i * 1000)
			user := &models.User{ID: userID}
			if err := userRepo.CreateOrUpdate(user); err != nil {
				rt.Fatal(err)
			}
			userIDs = append(userIDs, userID)

			numApproved := rapid.IntRange(0, numSteps).Draw(rt, "numApproved")
			for j := 0; j < numApproved; j++ {
				progress := &models.UserProgress{
					UserID: userID,
					StepID: stepIDs[j],
					Status: models.StatusApproved,
				}
				if err := progressRepo.Create(progress); err != nil {
					rt.Fatal(err)
				}
			}
			userMaxSteps[userID] = numApproved
		}

		testUserID := userIDs[rapid.IntRange(0, len(userIDs)-1).Draw(rt, "testUserIndex")]

		position, total, err := statsService.GetUserLeaderboardPosition(testUserID)
		if err != nil {
			rt.Fatal(err)
		}

		if total != numUsers {
			rt.Errorf("Expected total users %d, got %d", numUsers, total)
		}

		if position < 1 || position > numUsers {
			rt.Errorf("Position %d out of valid range [1, %d]", position, numUsers)
		}

		testUserMaxStep := userMaxSteps[testUserID]

		// Count users with strictly better performance (higher max step)
		strictlyBetterUsers := 0
		for _, userID := range userIDs {
			if userID == testUserID {
				continue
			}
			otherMaxStep := userMaxSteps[userID]
			if otherMaxStep > testUserMaxStep {
				strictlyBetterUsers++
			}
		}

		// Position should be at least strictlyBetterUsers + 1
		if position < strictlyBetterUsers+1 {
			rt.Errorf("Position %d is too high, should be at least %d (users with better performance + 1)",
				position, strictlyBetterUsers+1)
		}

		// Debug output
		rt.Logf("Test user %d has max step %d, position %d", testUserID, testUserMaxStep, position)
		rt.Logf("Strictly better users: %d", strictlyBetterUsers)
		for _, userID := range userIDs {
			rt.Logf("User %d has max step %d", userID, userMaxSteps[userID])
		}
	})
}
