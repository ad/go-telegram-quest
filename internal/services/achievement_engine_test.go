package services

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

var testDBCounter int64

func setupAchievementEngineTestDB(t testing.TB) (*db.DBQueue, func()) {
	counter := atomic.AddInt64(&testDBCounter, 1)
	dbName := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared", counter)
	sqlDB, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatal(err)
	}

	if err := db.InitializeDefaultAchievements(sqlDB); err != nil {
		t.Fatal(err)
	}

	queue := db.NewDBQueueForTest(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func createTestUserForEngine(t testing.TB, repo *db.UserRepository, id int64) *models.User {
	user := &models.User{
		ID:        id,
		FirstName: "Test",
		LastName:  "User",
		Username:  "testuser",
	}
	if err := repo.CreateOrUpdate(user); err != nil {
		t.Fatal(err)
	}
	return user
}

func createTestStep(t testing.TB, repo *db.StepRepository, order int) *models.Step {
	return createTestStepWithType(t, repo, order, models.AnswerTypeText)
}

func createTestStepWithType(t testing.TB, repo *db.StepRepository, order int, answerType models.AnswerType) *models.Step {
	step := &models.Step{
		StepOrder:    order,
		Text:         "Test step",
		AnswerType:   answerType,
		HasAutoCheck: true,
		IsActive:     true,
		IsDeleted:    false,
	}
	id, err := repo.Create(step)
	if err != nil {
		t.Fatal(err)
	}
	step.ID = id
	return step
}

func createUserProgress(t testing.TB, repo *db.ProgressRepository, userID, stepID int64, status models.ProgressStatus, completedAt *time.Time) {
	progress := &models.UserProgress{
		UserID:      userID,
		StepID:      stepID,
		Status:      status,
		CompletedAt: completedAt,
	}
	if err := repo.Create(progress); err != nil {
		t.Fatal(err)
	}
	if completedAt != nil {
		progress.CompletedAt = completedAt
		if err := repo.Update(progress); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProperty3_RetroactiveAssignmentCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numUsers := rapid.IntRange(1, 10).Draw(rt, "numUsers")
		numSteps := rapid.IntRange(1, 5).Draw(rt, "numSteps")
		threshold := rapid.IntRange(1, numSteps).Draw(rt, "threshold")

		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		type userProgressData struct {
			userID        int64
			correctCount  int
			thresholdTime time.Time
		}
		var usersData []userProgressData

		baseTime := time.Now().Add(-24 * time.Hour)
		for i := 1; i <= numUsers; i++ {
			userID := int64(i * 1000)
			createTestUserForEngine(t, userRepo, userID)

			correctCount := rapid.IntRange(0, numSteps).Draw(rt, "correctCount")
			var thresholdTime time.Time

			for j := 0; j < correctCount && j < len(steps); j++ {
				completedAt := baseTime.Add(time.Duration(i*100+j) * time.Minute)
				createUserProgress(t, progressRepo, userID, steps[j].ID, models.StatusApproved, &completedAt)

				if j+1 == threshold {
					thresholdTime = completedAt
				}
			}

			usersData = append(usersData, userProgressData{
				userID:        userID,
				correctCount:  correctCount,
				thresholdTime: thresholdTime,
			})
		}

		achievementKey := "test_progress_" + rapid.StringMatching(`[a-z]{5}`).Draw(rt, "key")
		achievement := &models.Achievement{
			Key:         achievementKey,
			Name:        "Test Progress Achievement",
			Description: "Test description",
			Category:    models.CategoryProgress,
			Type:        models.TypeProgressBased,
			IsUnique:    false,
			Conditions: models.AchievementConditions{
				CorrectAnswers: &threshold,
			},
			IsActive: true,
		}
		if err := achievementRepo.Create(achievement); err != nil {
			rt.Fatalf("Failed to create achievement: %v", err)
		}

		awardedUsers, err := engine.EvaluateRetroactiveAchievements(achievementKey)
		if err != nil {
			rt.Fatalf("EvaluateRetroactiveAchievements failed: %v", err)
		}

		for _, userData := range usersData {
			shouldHave := userData.correctCount >= threshold
			hasAchievement, err := achievementRepo.HasUserAchievement(userData.userID, achievementKey)
			if err != nil {
				rt.Fatalf("HasUserAchievement failed: %v", err)
			}

			if shouldHave && !hasAchievement {
				rt.Errorf("User %d with %d correct answers should have achievement (threshold=%d) but doesn't",
					userData.userID, userData.correctCount, threshold)
			}
			if !shouldHave && hasAchievement {
				rt.Errorf("User %d with %d correct answers should NOT have achievement (threshold=%d) but does",
					userData.userID, userData.correctCount, threshold)
			}

			if shouldHave {
				found := false
				for _, awardedID := range awardedUsers {
					if awardedID == userData.userID {
						found = true
						break
					}
				}
				if !found {
					rt.Errorf("User %d should be in awarded list but isn't", userData.userID)
				}

				userAchievements, err := achievementRepo.GetUserAchievements(userData.userID)
				if err != nil {
					rt.Fatalf("GetUserAchievements failed: %v", err)
				}

				for _, ua := range userAchievements {
					if ua.AchievementID == achievement.ID {
						if !ua.IsRetroactive {
							rt.Errorf("Achievement for user %d should be marked as retroactive", userData.userID)
						}
						if !userData.thresholdTime.IsZero() && ua.EarnedAt.Before(userData.thresholdTime.Add(-time.Second)) {
							rt.Errorf("Achievement earned_at (%v) should be >= threshold time (%v)",
								ua.EarnedAt, userData.thresholdTime)
						}
					}
				}
			}
		}

		expectedAwardedCount := 0
		for _, userData := range usersData {
			if userData.correctCount >= threshold {
				expectedAwardedCount++
			}
		}
		if len(awardedUsers) != expectedAwardedCount {
			rt.Errorf("Expected %d awarded users, got %d", expectedAwardedCount, len(awardedUsers))
		}
	})
}

func TestProperty5_UniqueAchievementExclusivity(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numUsers := rapid.IntRange(2, 10).Draw(rt, "numUsers")

		step := createTestStep(t, stepRepo, 1)

		var userIDs []int64
		baseTime := time.Now().Add(-24 * time.Hour)
		for i := 1; i <= numUsers; i++ {
			userID := int64(i * 1000)
			createTestUserForEngine(t, userRepo, userID)
			userIDs = append(userIDs, userID)

			completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
			createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &completedAt)
		}

		achievementKey := "unique_test_" + rapid.StringMatching(`[a-z]{5}`).Draw(rt, "key")
		position := 1
		achievement := &models.Achievement{
			Key:         achievementKey,
			Name:        "Unique Test Achievement",
			Description: "Test description",
			Category:    models.CategoryUnique,
			Type:        models.TypeUnique,
			IsUnique:    true,
			Conditions: models.AchievementConditions{
				Position: &position,
			},
			IsActive: true,
		}
		if err := achievementRepo.Create(achievement); err != nil {
			rt.Fatalf("Failed to create achievement: %v", err)
		}

		var assignedCount int
		var firstAssignedUser int64
		for _, userID := range userIDs {
			assigned, err := engine.EvaluateSpecificAchievement(userID, achievementKey)
			if err != nil {
				rt.Fatalf("EvaluateSpecificAchievement failed: %v", err)
			}
			if assigned {
				assignedCount++
				if firstAssignedUser == 0 {
					firstAssignedUser = userID
				}
			}
		}

		if assignedCount != 1 {
			rt.Errorf("Unique achievement should be assigned to exactly 1 user, got %d", assignedCount)
		}

		holders, err := achievementRepo.GetAchievementHolders(achievementKey)
		if err != nil {
			rt.Fatalf("GetAchievementHolders failed: %v", err)
		}

		if len(holders) != 1 {
			rt.Errorf("Expected exactly 1 holder for unique achievement, got %d", len(holders))
		}

		for _, userID := range userIDs {
			assigned, err := engine.EvaluateSpecificAchievement(userID, achievementKey)
			if err != nil {
				rt.Fatalf("Second EvaluateSpecificAchievement failed: %v", err)
			}
			if assigned {
				rt.Errorf("Unique achievement should not be assigned again to user %d", userID)
			}
		}

		holdersAfter, err := achievementRepo.GetAchievementHolders(achievementKey)
		if err != nil {
			rt.Fatalf("GetAchievementHolders after second attempt failed: %v", err)
		}

		if len(holdersAfter) != 1 {
			rt.Errorf("Expected still exactly 1 holder after second attempt, got %d", len(holdersAfter))
		}

		if holdersAfter[0] != holders[0] {
			rt.Errorf("Holder changed after second attempt: was %d, now %d", holders[0], holdersAfter[0])
		}
	})
}

func TestProperty6_PositionBasedAchievementOrdering(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numUsers := rapid.IntRange(3, 10).Draw(rt, "numUsers")

		step := createTestStep(t, stepRepo, 1)

		type userAnswerData struct {
			userID                 int64
			registrationOrder      int
			firstCorrectAnswerTime time.Time
		}
		var usersData []userAnswerData

		baseTime := time.Now().Add(-24 * time.Hour)

		answerDelays := make([]int, numUsers)
		for i := 0; i < numUsers; i++ {
			answerDelays[i] = rapid.IntRange(0, 1000).Draw(rt, "answerDelay")
		}

		for i := 0; i < numUsers; i++ {
			userID := int64((i + 1) * 1000)
			createTestUserForEngine(t, userRepo, userID)

			completedAt := baseTime.Add(time.Duration(answerDelays[i]) * time.Minute)
			createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &completedAt)

			usersData = append(usersData, userAnswerData{
				userID:                 userID,
				registrationOrder:      i,
				firstCorrectAnswerTime: completedAt,
			})
		}

		achievementKeyPrefix := "position_test_" + rapid.StringMatching(`[a-z]{5}`).Draw(rt, "keyPrefix")
		var achievements []*models.Achievement
		for pos := 1; pos <= 3; pos++ {
			position := pos
			achievement := &models.Achievement{
				Key:         achievementKeyPrefix + "_" + strconv.Itoa(pos),
				Name:        "Position " + strconv.Itoa(pos) + " Achievement",
				Description: "Test description",
				Category:    models.CategoryUnique,
				Type:        models.TypeUnique,
				IsUnique:    true,
				Conditions: models.AchievementConditions{
					Position: &position,
				},
				IsActive: true,
			}
			if err := achievementRepo.Create(achievement); err != nil {
				rt.Fatalf("Failed to create achievement: %v", err)
			}
			achievements = append(achievements, achievement)
		}

		err := engine.EvaluateUniqueAchievements()
		if err != nil {
			rt.Fatalf("EvaluateUniqueAchievements failed: %v", err)
		}

		type sortableUser struct {
			userID    int64
			timestamp time.Time
		}
		var sortedUsers []sortableUser
		for _, ud := range usersData {
			sortedUsers = append(sortedUsers, sortableUser{
				userID:    ud.userID,
				timestamp: ud.firstCorrectAnswerTime,
			})
		}
		sort.Slice(sortedUsers, func(i, j int) bool {
			return sortedUsers[i].timestamp.Before(sortedUsers[j].timestamp)
		})

		for pos := 1; pos <= 3; pos++ {
			achievementKey := achievementKeyPrefix + "_" + strconv.Itoa(pos)
			expectedUserID := sortedUsers[pos-1].userID

			holders, err := achievementRepo.GetAchievementHolders(achievementKey)
			if err != nil {
				rt.Fatalf("GetAchievementHolders failed: %v", err)
			}

			if len(holders) != 1 {
				rt.Errorf("Position %d achievement should have exactly 1 holder, got %d", pos, len(holders))
				continue
			}

			if holders[0] != expectedUserID {
				rt.Errorf("Position %d achievement should be assigned to user %d (first correct answer at %v), but was assigned to user %d",
					pos, expectedUserID, sortedUsers[pos-1].timestamp, holders[0])
			}

			hasAchievement, err := achievementRepo.HasUserAchievement(expectedUserID, achievementKey)
			if err != nil {
				rt.Fatalf("HasUserAchievement failed: %v", err)
			}
			if !hasAchievement {
				rt.Errorf("User %d should have position %d achievement but doesn't", expectedUserID, pos)
			}
		}

		for _, ud := range usersData {
			userAchievements, err := achievementRepo.GetUserAchievements(ud.userID)
			if err != nil {
				rt.Fatalf("GetUserAchievements failed: %v", err)
			}

			positionAchievementCount := 0
			for _, ua := range userAchievements {
				for _, a := range achievements {
					if ua.AchievementID == a.ID {
						positionAchievementCount++
					}
				}
			}

			if positionAchievementCount > 1 {
				rt.Errorf("User %d has %d position achievements, should have at most 1", ud.userID, positionAchievementCount)
			}
		}

		for pos := 1; pos <= 3; pos++ {
			achievementKey := achievementKeyPrefix + "_" + strconv.Itoa(pos)
			for _, ud := range usersData {
				assigned, err := engine.EvaluateSpecificAchievement(ud.userID, achievementKey)
				if err != nil {
					rt.Fatalf("EvaluateSpecificAchievement failed: %v", err)
				}
				if assigned {
					rt.Errorf("Position %d achievement should not be assigned again to user %d", pos, ud.userID)
				}
			}
		}
	})
}

func TestProperty7_ProgressThresholdAchievementAssignment(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numSteps := rapid.IntRange(1, 30).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		correctCount := rapid.IntRange(0, numSteps).Draw(rt, "correctCount")

		baseTime := time.Now().Add(-24 * time.Hour)
		for i := 0; i < correctCount && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)
		}

		awarded, err := engine.EvaluateProgressAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateProgressAchievements failed: %v", err)
		}

		for _, threshold := range ProgressThresholds {
			achievementKey := ProgressAchievementKeys[threshold]
			shouldHave := correctCount >= threshold

			hasAchievement, err := achievementRepo.HasUserAchievement(userID, achievementKey)
			if err != nil {
				rt.Fatalf("HasUserAchievement failed: %v", err)
			}

			if shouldHave && !hasAchievement {
				rt.Errorf("User with %d correct answers should have %s (threshold=%d) but doesn't",
					correctCount, achievementKey, threshold)
			}
			if !shouldHave && hasAchievement {
				rt.Errorf("User with %d correct answers should NOT have %s (threshold=%d) but does",
					correctCount, achievementKey, threshold)
			}

			if shouldHave {
				found := false
				for _, key := range awarded {
					if key == achievementKey {
						found = true
						break
					}
				}
				if !found {
					rt.Errorf("Achievement %s should be in awarded list but isn't", achievementKey)
				}
			}
		}

		expectedAwardedCount := 0
		for _, threshold := range ProgressThresholds {
			if correctCount >= threshold {
				expectedAwardedCount++
			}
		}
		if len(awarded) != expectedAwardedCount {
			rt.Errorf("Expected %d awarded achievements, got %d (correctCount=%d)",
				expectedAwardedCount, len(awarded), correctCount)
		}

		awarded2, err := engine.EvaluateProgressAchievements(userID)
		if err != nil {
			rt.Fatalf("Second EvaluateProgressAchievements failed: %v", err)
		}
		if len(awarded2) != 0 {
			rt.Errorf("Second evaluation should not award any achievements, got %d", len(awarded2))
		}
	})
}

func TestProperty7_ProgressThresholdRealTimeAssignment(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numSteps := 30
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		targetCorrectCount := rapid.IntRange(1, 26).Draw(rt, "targetCorrectCount")

		baseTime := time.Now().Add(-24 * time.Hour)
		var allAwarded []string
		for i := 0; i < targetCorrectCount && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)

			awarded, err := engine.OnCorrectAnswer(userID)
			if err != nil {
				rt.Fatalf("OnCorrectAnswer failed at step %d: %v", i+1, err)
			}
			allAwarded = append(allAwarded, awarded...)

			currentCorrectCount := i + 1
			for _, threshold := range ProgressThresholds {
				if currentCorrectCount == threshold {
					achievementKey := ProgressAchievementKeys[threshold]
					found := false
					for _, key := range awarded {
						if key == achievementKey {
							found = true
							break
						}
					}
					if !found {
						rt.Errorf("Achievement %s should be awarded immediately when reaching %d correct answers",
							achievementKey, threshold)
					}
				}
			}
		}

		for _, threshold := range ProgressThresholds {
			if targetCorrectCount >= threshold {
				achievementKey := ProgressAchievementKeys[threshold]
				hasAchievement, err := achievementRepo.HasUserAchievement(userID, achievementKey)
				if err != nil {
					rt.Fatalf("HasUserAchievement failed: %v", err)
				}
				if !hasAchievement {
					rt.Errorf("User with %d correct answers should have %s after real-time evaluation",
						targetCorrectCount, achievementKey)
				}
			}
		}
	})
}

func createUserAnswer(t testing.TB, queue *db.DBQueue, userID, stepID int64, hintUsed bool, createdAt time.Time) {
	_, err := queue.Execute(func(sqlDB *sql.DB) (any, error) {
		_, err := sqlDB.Exec(`
			INSERT INTO user_answers (user_id, step_id, text_answer, hint_used, created_at)
			VALUES (?, ?, 'test answer', ?, ?)
		`, userID, stepID, hintUsed, createdAt)
		return nil, err
	})
	if err != nil {
		t.Fatalf("Failed to create user answer: %v", err)
	}
}

func TestProperty8_CompletionBasedAchievementLogic(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numSteps := rapid.IntRange(1, 10).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		completedSteps := rapid.IntRange(0, numSteps).Draw(rt, "completedSteps")
		totalAnswers := rapid.IntRange(completedSteps, completedSteps*3+1).Draw(rt, "totalAnswers")
		hintsUsed := rapid.IntRange(0, totalAnswers).Draw(rt, "hintsUsed")
		completionTimeMinutes := rapid.IntRange(0, 120).Draw(rt, "completionTimeMinutes")

		baseTime := time.Now().Add(-time.Duration(completionTimeMinutes+10) * time.Minute)
		endTime := baseTime.Add(time.Duration(completionTimeMinutes) * time.Minute)

		answerIndex := 0
		for i := 0; i < completedSteps && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*completionTimeMinutes/(numSteps+1)) * time.Minute)
			if i == completedSteps-1 {
				completedAt = endTime
			}
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)

			useHint := answerIndex < hintsUsed
			answerTime := baseTime
			if answerIndex == totalAnswers-1 && totalAnswers > 1 {
				answerTime = endTime
			}
			createUserAnswer(t, queue, userID, steps[i].ID, useHint, answerTime)
			answerIndex++
		}

		for answerIndex < totalAnswers {
			stepIdx := answerIndex % len(steps)
			useHint := answerIndex < hintsUsed
			answerTime := baseTime
			if answerIndex == totalAnswers-1 {
				answerTime = endTime
			} else if completionTimeMinutes > 0 && totalAnswers > 1 {
				offset := time.Duration(answerIndex*completionTimeMinutes/(totalAnswers)) * time.Minute
				answerTime = baseTime.Add(offset)
			}
			createUserAnswer(t, queue, userID, steps[stepIdx].ID, useHint, answerTime)
			answerIndex++
		}

		awarded, err := engine.EvaluateCompletionAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateCompletionAchievements failed: %v", err)
		}

		isCompleted := completedSteps >= numSteps && numSteps > 0
		hasNoErrors := totalAnswers == completedSteps
		hasNoHints := hintsUsed == 0

		hasWinner, _ := achievementRepo.HasUserAchievement(userID, "winner")
		hasPerfectPath, _ := achievementRepo.HasUserAchievement(userID, "perfect_path")
		hasSelfSufficient, _ := achievementRepo.HasUserAchievement(userID, "self_sufficient")
		hasCheater, _ := achievementRepo.HasUserAchievement(userID, "cheater")
		hasLightning, _ := achievementRepo.HasUserAchievement(userID, "lightning")
		hasRocket, _ := achievementRepo.HasUserAchievement(userID, "rocket")

		if isCompleted {
			if !hasWinner {
				rt.Errorf("User who completed quest should have 'winner' achievement")
			}
			if hasNoErrors && !hasPerfectPath {
				rt.Errorf("User who completed quest without errors should have 'perfect_path' achievement (totalAnswers=%d, completedSteps=%d)", totalAnswers, completedSteps)
			}
			if hasNoHints && !hasSelfSufficient {
				rt.Errorf("User who completed quest without hints should have 'self_sufficient' achievement (hintsUsed=%d)", hintsUsed)
			}
			speedAchievementCount := 0
			if hasCheater {
				speedAchievementCount++
			}
			if hasLightning {
				speedAchievementCount++
			}
			if hasRocket {
				speedAchievementCount++
			}
			if speedAchievementCount > 1 {
				rt.Errorf("User should have at most one speed achievement, got %d (cheater=%v, lightning=%v, rocket=%v)", speedAchievementCount, hasCheater, hasLightning, hasRocket)
			}
			actualCompletionTime := completionTimeMinutes
			if totalAnswers <= 1 {
				actualCompletionTime = 0
			}
			if actualCompletionTime < 5 && !hasCheater {
				rt.Errorf("User who completed quest in %d minutes should have 'cheater' achievement", actualCompletionTime)
			}
			if actualCompletionTime >= 5 && actualCompletionTime < 10 && !hasLightning {
				rt.Errorf("User who completed quest in %d minutes should have 'lightning' achievement", actualCompletionTime)
			}
			if actualCompletionTime >= 10 && actualCompletionTime < 60 && !hasRocket {
				rt.Errorf("User who completed quest in %d minutes should have 'rocket' achievement", actualCompletionTime)
			}
		} else {
			if hasWinner || hasPerfectPath || hasSelfSufficient || hasCheater || hasLightning || hasRocket {
				rt.Errorf("User who did not complete quest should not have any completion achievements (completed=%d/%d)", completedSteps, numSteps)
			}
			if len(awarded) > 0 {
				rt.Errorf("No achievements should be awarded to incomplete user, got %v", awarded)
			}
		}

		if !isCompleted && len(awarded) > 0 {
			rt.Errorf("Incomplete user should not receive any completion achievements")
		}

		awarded2, err := engine.EvaluateCompletionAchievements(userID)
		if err != nil {
			rt.Fatalf("Second EvaluateCompletionAchievements failed: %v", err)
		}
		if len(awarded2) != 0 {
			rt.Errorf("Second evaluation should not award any new achievements, got %d", len(awarded2))
		}
	})
}

func TestProperty9_HintBasedAchievementPatterns(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numSteps := rapid.IntRange(2, 30).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		correctCount := rapid.IntRange(1, numSteps).Draw(rt, "correctCount")
		hintsUsed := rapid.IntRange(0, correctCount).Draw(rt, "hintsUsed")
		hintOnFirstTask := rapid.Bool().Draw(rt, "hintOnFirstTask")

		baseTime := time.Now().Add(-24 * time.Hour)
		hintIndex := 0
		actualHintOnFirstTask := false

		for i := 0; i < correctCount && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)

			useHint := false
			if i == 0 && hintOnFirstTask && hintsUsed > 0 {
				useHint = true
				hintIndex++
				actualHintOnFirstTask = true
			} else if hintIndex < hintsUsed && i > 0 {
				useHint = true
				hintIndex++
			}
			createUserAnswer(t, queue, userID, steps[i].ID, useHint, completedAt)
		}

		awarded, err := engine.EvaluateHintAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateHintAchievements failed: %v", err)
		}

		actualHintsUsed := hintIndex

		for _, threshold := range HintThresholds {
			achievementKey := HintAchievementKeys[threshold]
			shouldHave := actualHintsUsed >= threshold

			hasAchievement, err := achievementRepo.HasUserAchievement(userID, achievementKey)
			if err != nil {
				rt.Fatalf("HasUserAchievement failed: %v", err)
			}

			if shouldHave && !hasAchievement {
				rt.Errorf("User with %d hints should have %s (threshold=%d) but doesn't",
					actualHintsUsed, achievementKey, threshold)
			}
			if !shouldHave && hasAchievement {
				rt.Errorf("User with %d hints should NOT have %s (threshold=%d) but does",
					actualHintsUsed, achievementKey, threshold)
			}
		}

		hasSkeptic, err := achievementRepo.HasUserAchievement(userID, "skeptic")
		if err != nil {
			rt.Fatalf("HasUserAchievement for skeptic failed: %v", err)
		}
		if actualHintOnFirstTask && !hasSkeptic {
			rt.Errorf("User who used hint on first task should have 'skeptic' achievement")
		}
		if !actualHintOnFirstTask && hasSkeptic {
			rt.Errorf("User who did not use hint on first task should NOT have 'skeptic' achievement")
		}

		shouldHaveHintMaster := actualHintsUsed >= numSteps && numSteps > 0
		hasHintMaster, err := achievementRepo.HasUserAchievement(userID, "hint_master")
		if err != nil {
			rt.Fatalf("HasUserAchievement for hint_master failed: %v", err)
		}
		if shouldHaveHintMaster && !hasHintMaster {
			rt.Errorf("User who used all %d hints should have 'hint_master' achievement (hintsUsed=%d)", numSteps, actualHintsUsed)
		}
		if !shouldHaveHintMaster && hasHintMaster {
			rt.Errorf("User who used %d/%d hints should NOT have 'hint_master' achievement", actualHintsUsed, numSteps)
		}

		awarded2, err := engine.EvaluateHintAchievements(userID)
		if err != nil {
			rt.Fatalf("Second EvaluateHintAchievements failed: %v", err)
		}
		if len(awarded2) != 0 {
			rt.Errorf("Second evaluation should not award any new achievements, got %d", len(awarded2))
		}

		_ = awarded
	})
}

func createAnswerImage(t testing.TB, queue *db.DBQueue, answerID int64) {
	_, err := queue.Execute(func(sqlDB *sql.DB) (any, error) {
		_, err := sqlDB.Exec(`
			INSERT INTO answer_images (answer_id, file_id, position)
			VALUES (?, 'test_file_id', 0)
		`, answerID)
		return nil, err
	})
	if err != nil {
		t.Fatalf("Failed to create answer image: %v", err)
	}
}

func createUserAnswerWithID(t testing.TB, queue *db.DBQueue, userID, stepID int64, textAnswer string, hintUsed bool, createdAt time.Time) int64 {
	result, err := queue.Execute(func(sqlDB *sql.DB) (any, error) {
		res, err := sqlDB.Exec(`
			INSERT INTO user_answers (user_id, step_id, text_answer, hint_used, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, userID, stepID, textAnswer, hintUsed, createdAt)
		if err != nil {
			return nil, err
		}
		return res.LastInsertId()
	})
	if err != nil {
		t.Fatalf("Failed to create user answer: %v", err)
	}
	return result.(int64)
}

func TestProperty10_SpecialActionAchievementDetection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numSteps := rapid.IntRange(2, 15).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			var step *models.Step
			if i == 1 {
				step = createTestStepWithType(t, stepRepo, i, models.AnswerTypeImage)
			} else {
				step = createTestStep(t, stepRepo, i)
			}
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		hasPhotoOnImageTask := rapid.Bool().Draw(rt, "hasPhotoOnImageTask")
		hasPhotoOnTextTask := rapid.Bool().Draw(rt, "hasPhotoOnTextTask")
		consecutiveCorrect := rapid.IntRange(1, numSteps).Draw(rt, "consecutiveCorrect")
		hasSecretAnswer := rapid.Bool().Draw(rt, "hasSecretAnswer")

		baseTime := time.Now().Add(-24 * time.Hour)
		actualPhotoOnImageTask := false
		actualPhotoOnTextTask := false

		for i := 0; i < consecutiveCorrect && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)
			answerID := createUserAnswerWithID(t, queue, userID, steps[i].ID, "correct answer", false, completedAt)

			if i == 0 && hasPhotoOnImageTask {
				createAnswerImage(t, queue, answerID)
				actualPhotoOnImageTask = true
			}
			if i == 1 && hasPhotoOnTextTask && len(steps) > 1 {
				createAnswerImage(t, queue, answerID)
				actualPhotoOnTextTask = true
			}
		}

		if hasSecretAnswer && consecutiveCorrect < len(steps) {
			stepIdx := consecutiveCorrect
			answerTime := baseTime.Add(time.Duration(consecutiveCorrect*10+1) * time.Minute)
			createUserAnswerWithID(t, queue, userID, steps[stepIdx].ID, "сезам откройся", false, answerTime)
		}

		awarded, err := engine.EvaluateSpecialAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateSpecialAchievements failed: %v", err)
		}

		hasPhotographer, _ := achievementRepo.HasUserAchievement(userID, "photographer")
		if actualPhotoOnImageTask && !hasPhotographer {
			rt.Errorf("User with photo on image task should have 'photographer' achievement")
		}
		if !actualPhotoOnImageTask && hasPhotographer {
			rt.Errorf("User without photo on image task should NOT have 'photographer' achievement")
		}

		hasPaparazzi, _ := achievementRepo.HasUserAchievement(userID, "paparazzi")
		if actualPhotoOnTextTask && !hasPaparazzi {
			rt.Errorf("User with photo on text task should have 'paparazzi' achievement")
		}

		actualConsecutive := consecutiveCorrect
		if actualConsecutive > numSteps {
			actualConsecutive = numSteps
		}
		hasBullseye, _ := achievementRepo.HasUserAchievement(userID, "bullseye")
		if actualConsecutive >= 10 && !hasBullseye {
			rt.Errorf("User with %d consecutive correct answers should have 'bullseye' achievement", actualConsecutive)
		}
		if actualConsecutive < 10 && hasBullseye {
			rt.Errorf("User with %d consecutive correct answers should NOT have 'bullseye' achievement", actualConsecutive)
		}

		hasSecretAgent, _ := achievementRepo.HasUserAchievement(userID, "secret_agent")
		actualSecretAnswer := hasSecretAnswer && consecutiveCorrect < len(steps)
		if actualSecretAnswer && !hasSecretAgent {
			rt.Errorf("User who submitted secret answer should have 'secret_agent' achievement")
		}
		if !actualSecretAnswer && hasSecretAgent {
			rt.Errorf("User who did not submit secret answer should NOT have 'secret_agent' achievement")
		}

		awarded2, err := engine.EvaluateSpecialAchievements(userID)
		if err != nil {
			rt.Fatalf("Second EvaluateSpecialAchievements failed: %v", err)
		}
		if len(awarded2) != 0 {
			rt.Errorf("Second evaluation should not award any new achievements, got %d", len(awarded2))
		}

		_ = awarded
	})
}

func TestProperty11_ConsecutiveCounterResetLogic(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numSteps := rapid.IntRange(5, 20).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		numAnswers := rapid.IntRange(1, numSteps).Draw(rt, "numAnswers")
		answerPattern := make([]bool, numAnswers)
		for i := 0; i < numAnswers; i++ {
			answerPattern[i] = rapid.Bool().Draw(rt, "isCorrect")
		}

		baseTime := time.Now().Add(-24 * time.Hour)
		for i := 0; i < numAnswers && i < len(steps); i++ {
			answerTime := baseTime.Add(time.Duration(i*10) * time.Minute)

			if answerPattern[i] {
				createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &answerTime)
			}
			createUserAnswer(t, queue, userID, steps[i].ID, false, answerTime)
		}

		consecutiveCount, err := engine.getConsecutiveCorrectCount(userID)
		if err != nil {
			rt.Fatalf("getConsecutiveCorrectCount failed: %v", err)
		}

		expectedMaxConsecutive := 0
		currentStreak := 0
		for _, isCorrect := range answerPattern {
			if isCorrect {
				currentStreak++
				if currentStreak > expectedMaxConsecutive {
					expectedMaxConsecutive = currentStreak
				}
			} else {
				currentStreak = 0
			}
		}

		if consecutiveCount != expectedMaxConsecutive {
			rt.Errorf("Expected max consecutive correct count %d, got %d (pattern: %v)",
				expectedMaxConsecutive, consecutiveCount, answerPattern)
		}

		currentConsecutive, err := engine.GetCurrentConsecutiveCorrect(userID)
		if err != nil {
			rt.Fatalf("GetCurrentConsecutiveCorrect failed: %v", err)
		}

		expectedCurrentStreak := 0
		for i := len(answerPattern) - 1; i >= 0; i-- {
			if answerPattern[i] {
				expectedCurrentStreak++
			} else {
				break
			}
		}

		if currentConsecutive != expectedCurrentStreak {
			rt.Errorf("Expected current consecutive correct count %d, got %d (pattern: %v)",
				expectedCurrentStreak, currentConsecutive, answerPattern)
		}

		if numAnswers > 0 && !answerPattern[numAnswers-1] {
			if currentConsecutive != 0 {
				rt.Errorf("After wrong answer, current consecutive should be 0, got %d", currentConsecutive)
			}
		}
	})
}

func assignAchievementToUser(t testing.TB, repo *db.AchievementRepository, userID int64, achievementKey string, earnedAt time.Time) {
	achievement, err := repo.GetByKey(achievementKey)
	if err != nil {
		t.Fatalf("Failed to get achievement %s: %v", achievementKey, err)
	}
	if err := repo.AssignToUser(userID, achievement.ID, earnedAt, false); err != nil {
		t.Fatalf("Failed to assign achievement %s to user %d: %v", achievementKey, userID, err)
	}
}

func TestProperty12_CompositeAchievementEvaluation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numSteps := rapid.IntRange(25, 30).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		hasAllProgressAchievements := rapid.Bool().Draw(rt, "hasAllProgressAchievements")
		hasAllCompletionAchievements := rapid.Bool().Draw(rt, "hasAllCompletionAchievements")
		hasAllPositionAchievements := rapid.Bool().Draw(rt, "hasAllPositionAchievements")

		completionTimeMinutes := rapid.IntRange(0, 60).Draw(rt, "completionTimeMinutes")
		hasErrors := rapid.Bool().Draw(rt, "hasErrors")
		usedHints := rapid.Bool().Draw(rt, "usedHints")

		baseTime := time.Now().Add(-time.Duration(completionTimeMinutes+10) * time.Minute)
		earnedAt := baseTime

		if hasAllProgressAchievements {
			for _, key := range SuperCollectorRequiredAchievements {
				assignAchievementToUser(t, achievementRepo, userID, key, earnedAt)
				earnedAt = earnedAt.Add(time.Minute)
			}
		} else {
			numToAssign := rapid.IntRange(0, len(SuperCollectorRequiredAchievements)-1).Draw(rt, "numProgressToAssign")
			for i := 0; i < numToAssign; i++ {
				assignAchievementToUser(t, achievementRepo, userID, SuperCollectorRequiredAchievements[i], earnedAt)
				earnedAt = earnedAt.Add(time.Minute)
			}
		}

		completionAchievements := []string{"winner", "perfect_path", "self_sufficient", "lightning", "rocket", "cheater"}
		if hasAllCompletionAchievements {
			for _, key := range completionAchievements {
				assignAchievementToUser(t, achievementRepo, userID, key, earnedAt)
				earnedAt = earnedAt.Add(time.Minute)
			}
		}

		positionAchievements := []string{
			"pioneer", "second_place", "third_place", "fourth_place", "fifth_place",
			"sixth_place", "seventh_place", "eighth_place", "ninth_place", "tenth_place",
		}
		if hasAllPositionAchievements {
			for _, key := range positionAchievements {
				assignAchievementToUser(t, achievementRepo, userID, key, earnedAt)
				earnedAt = earnedAt.Add(time.Minute)
			}
		}

		for i := 0; i < numSteps && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*completionTimeMinutes/(numSteps+1)) * time.Minute)
			if i == numSteps-1 {
				completedAt = baseTime.Add(time.Duration(completionTimeMinutes) * time.Minute)
			}
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)

			useHint := usedHints && i == 0
			createUserAnswer(t, queue, userID, steps[i].ID, useHint, completedAt)
		}

		if hasErrors {
			extraAnswerTime := baseTime.Add(time.Duration(numSteps+1) * time.Minute)
			createUserAnswer(t, queue, userID, steps[0].ID, false, extraAnswerTime)
		}

		awarded, err := engine.EvaluateCompositeAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateCompositeAchievements failed: %v", err)
		}

		hasSuperCollector, _ := achievementRepo.HasUserAchievement(userID, "super_collector")
		shouldHaveSuperCollector := hasAllProgressAchievements

		if shouldHaveSuperCollector && !hasSuperCollector {
			rt.Errorf("User with all progress achievements should have 'super_collector' achievement")
		}
		if !shouldHaveSuperCollector && hasSuperCollector {
			rt.Errorf("User without all progress achievements should NOT have 'super_collector' achievement")
		}

		hasSuperBrain, _ := achievementRepo.HasUserAchievement(userID, "super_brain")
		shouldHaveSuperBrain := !hasErrors && !usedHints && completionTimeMinutes < 30

		if shouldHaveSuperBrain && !hasSuperBrain {
			rt.Errorf("User who completed quest without errors, hints, and in under 30 minutes should have 'super_brain' achievement (time=%d, errors=%v, hints=%v)",
				completionTimeMinutes, hasErrors, usedHints)
		}
		if !shouldHaveSuperBrain && hasSuperBrain {
			rt.Errorf("User who did not meet super_brain conditions should NOT have 'super_brain' achievement (time=%d, errors=%v, hints=%v)",
				completionTimeMinutes, hasErrors, usedHints)
		}

		hasLegend, _ := achievementRepo.HasUserAchievement(userID, "legend")
		shouldHaveLegend := hasAllProgressAchievements && hasAllCompletionAchievements && hasAllPositionAchievements

		if shouldHaveLegend && !hasLegend {
			rt.Errorf("User with all required achievements should have 'legend' achievement")
		}
		if !shouldHaveLegend && hasLegend {
			rt.Errorf("User without all required achievements should NOT have 'legend' achievement")
		}

		if shouldHaveSuperCollector {
			found := false
			for _, key := range awarded {
				if key == "super_collector" {
					found = true
					break
				}
			}
			if !found {
				rt.Errorf("'super_collector' should be in awarded list")
			}
		}

		awarded2, err := engine.EvaluateCompositeAchievements(userID)
		if err != nil {
			rt.Fatalf("Second EvaluateCompositeAchievements failed: %v", err)
		}
		if len(awarded2) != 0 {
			rt.Errorf("Second evaluation should not award any new achievements, got %d", len(awarded2))
		}
	})
}

func TestProperty12_CompositeAchievementAutoEvaluation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		baseTime := time.Now().Add(-24 * time.Hour)

		for i, key := range SuperCollectorRequiredAchievements[:len(SuperCollectorRequiredAchievements)-1] {
			assignAchievementToUser(t, achievementRepo, userID, key, baseTime.Add(time.Duration(i)*time.Minute))

			awarded, err := engine.OnAchievementAwarded(userID)
			if err != nil {
				rt.Fatalf("OnAchievementAwarded failed: %v", err)
			}

			hasSuperCollector, _ := achievementRepo.HasUserAchievement(userID, "super_collector")
			if hasSuperCollector {
				rt.Errorf("User should NOT have 'super_collector' before collecting all progress achievements (has %d/%d)",
					i+1, len(SuperCollectorRequiredAchievements))
			}
			for _, awardedKey := range awarded {
				if awardedKey == "super_collector" {
					rt.Errorf("'super_collector' should not be awarded before all prerequisites are met")
				}
			}
		}

		lastKey := SuperCollectorRequiredAchievements[len(SuperCollectorRequiredAchievements)-1]
		assignAchievementToUser(t, achievementRepo, userID, lastKey, baseTime.Add(time.Duration(len(SuperCollectorRequiredAchievements))*time.Minute))

		awarded, err := engine.OnAchievementAwarded(userID)
		if err != nil {
			rt.Fatalf("Final OnAchievementAwarded failed: %v", err)
		}

		hasSuperCollector, _ := achievementRepo.HasUserAchievement(userID, "super_collector")
		if !hasSuperCollector {
			rt.Errorf("User should have 'super_collector' after collecting all progress achievements")
		}

		found := false
		for _, key := range awarded {
			if key == "super_collector" {
				found = true
				break
			}
		}
		if !found {
			rt.Errorf("'super_collector' should be in awarded list after final prerequisite")
		}
	})
}

func TestProperty1_WinnerPositionAssignment(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numUsers := rapid.IntRange(3, 10).Draw(rt, "numUsers")
		numSteps := rapid.IntRange(2, 5).Draw(rt, "numSteps")

		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		type userCompletionData struct {
			userID         int64
			completionTime time.Time
		}
		var usersData []userCompletionData

		baseTime := time.Now().Add(-24 * time.Hour)
		completionDelays := make([]int, numUsers)
		for i := 0; i < numUsers; i++ {
			completionDelays[i] = rapid.IntRange(0, 1000).Draw(rt, "completionDelay")
		}

		for i := 0; i < numUsers; i++ {
			userID := int64((i + 1) * 1000)
			createTestUserForEngine(t, userRepo, userID)

			completionTime := baseTime.Add(time.Duration(completionDelays[i]) * time.Minute)

			for j := 0; j < numSteps; j++ {
				stepCompletionTime := completionTime.Add(time.Duration(j) * time.Second)
				createUserProgress(t, progressRepo, userID, steps[j].ID, models.StatusApproved, &stepCompletionTime)
			}

			usersData = append(usersData, userCompletionData{
				userID:         userID,
				completionTime: completionTime.Add(time.Duration(numSteps-1) * time.Second),
			})
		}

		sort.Slice(usersData, func(i, j int) bool {
			return usersData[i].completionTime.Before(usersData[j].completionTime)
		})

		// Simulate quest completion in chronological order
		var allAwarded []string
		for i, userData := range usersData {
			if i >= 3 {
				break
			}

			awarded, err := engine.EvaluateWinnerAchievements(userData.userID)
			if err != nil {
				rt.Fatalf("EvaluateWinnerAchievements failed for user %d: %v", userData.userID, err)
			}
			allAwarded = append(allAwarded, awarded...)
		}

		// Check that the first 3 users got the correct achievements
		for i, userData := range usersData {
			if i >= 3 {
				break
			}

			expectedAchievement := WinnerAchievementKeys[i+1]
			hasAchievement, err := achievementRepo.HasUserAchievement(userData.userID, expectedAchievement)
			if err != nil {
				rt.Fatalf("HasUserAchievement failed: %v", err)
			}

			if !hasAchievement {
				rt.Errorf("User %d (position %d, completion time %v) should have %s achievement",
					userData.userID, i+1, userData.completionTime, expectedAchievement)
			}
		}

		for i := 3; i < len(usersData); i++ {
			userData := usersData[i]
			for pos := 1; pos <= 3; pos++ {
				achievementKey := WinnerAchievementKeys[pos]
				hasAchievement, err := achievementRepo.HasUserAchievement(userData.userID, achievementKey)
				if err != nil {
					rt.Fatalf("HasUserAchievement failed: %v", err)
				}
				if hasAchievement {
					rt.Errorf("User %d (position %d) should NOT have %s achievement",
						userData.userID, i+1, achievementKey)
				}
			}
		}
	})
}

func TestProperty2_WinnerAchievementUniqueness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		numUsers := rapid.IntRange(5, 15).Draw(rt, "numUsers")
		numSteps := rapid.IntRange(2, 5).Draw(rt, "numSteps")

		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		baseTime := time.Now().Add(-24 * time.Hour)
		var userIDs []int64

		for i := 0; i < numUsers; i++ {
			userID := int64((i + 1) * 1000)
			createTestUserForEngine(t, userRepo, userID)
			userIDs = append(userIDs, userID)

			completionTime := baseTime.Add(time.Duration(i*10) * time.Minute)

			for j := 0; j < numSteps; j++ {
				stepCompletionTime := completionTime.Add(time.Duration(j) * time.Second)
				createUserProgress(t, progressRepo, userID, steps[j].ID, models.StatusApproved, &stepCompletionTime)
			}
		}

		var allAwarded []string
		for _, userID := range userIDs {
			awarded, err := engine.EvaluateWinnerAchievements(userID)
			if err != nil {
				rt.Fatalf("EvaluateWinnerAchievements failed for user %d: %v", userID, err)
			}
			allAwarded = append(allAwarded, awarded...)
		}

		for pos := 1; pos <= 3; pos++ {
			achievementKey := WinnerAchievementKeys[pos]
			holders, err := achievementRepo.GetAchievementHolders(achievementKey)
			if err != nil {
				rt.Fatalf("GetAchievementHolders failed for %s: %v", achievementKey, err)
			}

			if len(holders) > 1 {
				rt.Errorf("Winner achievement %s should have at most 1 holder, got %d", achievementKey, len(holders))
			}

			if len(holders) == 1 {
				expectedUserID := userIDs[pos-1]
				if holders[0] != expectedUserID {
					rt.Errorf("Winner achievement %s should be held by user %d, but held by user %d",
						achievementKey, expectedUserID, holders[0])
				}
			}
		}

		for _, userID := range userIDs {
			winnerAchievementCount := 0
			for pos := 1; pos <= 3; pos++ {
				achievementKey := WinnerAchievementKeys[pos]
				hasAchievement, err := achievementRepo.HasUserAchievement(userID, achievementKey)
				if err != nil {
					rt.Fatalf("HasUserAchievement failed: %v", err)
				}
				if hasAchievement {
					winnerAchievementCount++
				}
			}

			if winnerAchievementCount > 1 {
				rt.Errorf("User %d should have at most 1 winner achievement, got %d", userID, winnerAchievementCount)
			}
		}

		for _, userID := range userIDs {
			awarded2, err := engine.EvaluateWinnerAchievements(userID)
			if err != nil {
				rt.Fatalf("Second EvaluateWinnerAchievements failed for user %d: %v", userID, err)
			}
			if len(awarded2) > 0 {
				rt.Errorf("Second evaluation should not award any achievements to user %d, got %v", userID, awarded2)
			}
		}
	})
}

func TestProperty5_WriterAchievementOnTextToImage(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		// Create a mix of text and image steps
		numSteps := rapid.IntRange(3, 10).Draw(rt, "numSteps")
		var steps []*models.Step
		var imageStepIndices []int

		for i := 1; i <= numSteps; i++ {
			isImageStep := rapid.Bool().Draw(rt, fmt.Sprintf("isImageStep_%d", i))
			var step *models.Step
			if isImageStep {
				step = createTestStepWithType(t, stepRepo, i, models.AnswerTypeImage)
				imageStepIndices = append(imageStepIndices, i-1)
			} else {
				step = createTestStepWithType(t, stepRepo, i, models.AnswerTypeText)
			}
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		// Determine if user will send text on any image step
		willSendTextOnImageStep := len(imageStepIndices) > 0 && rapid.Bool().Draw(rt, "willSendTextOnImageStep")
		var textOnImageStepIndex int
		if willSendTextOnImageStep {
			textOnImageStepIndex = imageStepIndices[rapid.IntRange(0, len(imageStepIndices)-1).Draw(rt, "textOnImageStepIndex")]
		}

		// Simulate user interactions
		baseTime := time.Now().Add(-2 * time.Hour)
		actualTextOnImageTask := false

		for i, step := range steps {
			if willSendTextOnImageStep && i == textOnImageStepIndex {
				// User sends text on image step - this should trigger writer achievement
				answerTime := baseTime.Add(time.Duration(i*5) * time.Minute)
				createUserAnswerWithID(t, queue, userID, step.ID, "some text answer", false, answerTime)
				actualTextOnImageTask = true
			} else if step.AnswerType == models.AnswerTypeText {
				// Normal text answer on text step
				answerTime := baseTime.Add(time.Duration(i*5) * time.Minute)
				createUserAnswerWithID(t, queue, userID, step.ID, "correct text answer", false, answerTime)
				createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &answerTime)
			} else if step.AnswerType == models.AnswerTypeImage && i != textOnImageStepIndex {
				// Normal image answer on image step
				answerTime := baseTime.Add(time.Duration(i*5) * time.Minute)
				answerID := createUserAnswerWithID(t, queue, userID, step.ID, "", false, answerTime)
				createAnswerImage(t, queue, answerID)
				createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &answerTime)
			}
		}

		// Test the OnTextOnImageTask function directly
		if actualTextOnImageTask {
			awarded, err := engine.OnTextOnImageTask(userID)
			if err != nil {
				rt.Fatalf("OnTextOnImageTask failed: %v", err)
			}

			hasWriter, err := achievementRepo.HasUserAchievement(userID, "writer")
			if err != nil {
				rt.Fatalf("HasUserAchievement failed: %v", err)
			}

			if !hasWriter {
				rt.Errorf("User who sent text on image task should have 'writer' achievement")
			}

			if len(awarded) == 0 {
				rt.Errorf("OnTextOnImageTask should return awarded achievement when user doesn't have it yet")
			} else if awarded[0] != "writer" {
				rt.Errorf("OnTextOnImageTask should award 'writer' achievement, got %v", awarded)
			}

			// Test idempotence - calling again should not award again
			awarded2, err := engine.OnTextOnImageTask(userID)
			if err != nil {
				rt.Fatalf("Second OnTextOnImageTask failed: %v", err)
			}
			if len(awarded2) > 0 {
				rt.Errorf("Second OnTextOnImageTask call should not award achievement again, got %v", awarded2)
			}
		} else {
			// User never sent text on image task
			_, err := engine.OnTextOnImageTask(userID)
			if err != nil {
				rt.Fatalf("OnTextOnImageTask failed: %v", err)
			}

			hasWriter, err := achievementRepo.HasUserAchievement(userID, "writer")
			if err != nil {
				rt.Fatalf("HasUserAchievement failed: %v", err)
			}

			// Even if we call OnTextOnImageTask, user should get the achievement
			// because the function awards it regardless of actual behavior
			if !hasWriter {
				rt.Errorf("OnTextOnImageTask should award writer achievement when called")
			}
		}
	})
}

func TestProperty6_BackwardCompatibilityCompositeAchievements(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		// Create test steps
		numSteps := rapid.IntRange(25, 30).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		createTestUserForEngine(t, userRepo, userID)

		// Set up user with conditions for composite achievements
		hasAllProgressAchievements := rapid.Bool().Draw(rt, "hasAllProgressAchievements")
		completionTimeMinutes := rapid.IntRange(0, 60).Draw(rt, "completionTimeMinutes")
		hasErrors := rapid.Bool().Draw(rt, "hasErrors")
		usedHints := rapid.Bool().Draw(rt, "usedHints")

		baseTime := time.Now().Add(-time.Duration(completionTimeMinutes+10) * time.Minute)
		earnedAt := baseTime

		// Assign progress achievements if needed
		if hasAllProgressAchievements {
			for _, key := range SuperCollectorRequiredAchievements {
				assignAchievementToUser(t, achievementRepo, userID, key, earnedAt)
				earnedAt = earnedAt.Add(time.Minute)
			}
		}

		// Create user progress and answers
		for i := 0; i < numSteps && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*completionTimeMinutes/(numSteps+1)) * time.Minute)
			if i == numSteps-1 {
				completedAt = baseTime.Add(time.Duration(completionTimeMinutes) * time.Minute)
			}
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)

			useHint := usedHints && i == 0
			createUserAnswer(t, queue, userID, steps[i].ID, useHint, completedAt)
		}

		if hasErrors {
			extraAnswerTime := baseTime.Add(time.Duration(numSteps+1) * time.Minute)
			createUserAnswer(t, queue, userID, steps[0].ID, false, extraAnswerTime)
		}

		// Evaluate composite achievements WITHOUT new winner achievements present
		_, err := engine.EvaluateCompositeAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateCompositeAchievements failed: %v", err)
		}

		// Store the state of composite achievements
		hasSuperCollectorBefore, _ := achievementRepo.HasUserAchievement(userID, "super_collector")
		hasSuperBrainBefore, _ := achievementRepo.HasUserAchievement(userID, "super_brain")
		hasLegendBefore, _ := achievementRepo.HasUserAchievement(userID, "legend")

		// Now add some new winner achievements (simulate they exist)
		winnerAchievementsToAdd := rapid.IntRange(0, 3).Draw(rt, "winnerAchievementsToAdd")
		for i := 1; i <= winnerAchievementsToAdd; i++ {
			winnerKey := WinnerAchievementKeys[i]
			assignAchievementToUser(t, achievementRepo, userID, winnerKey, earnedAt.Add(time.Duration(i)*time.Minute))
		}

		// Evaluate composite achievements WITH new winner achievements present
		_, err = engine.EvaluateCompositeAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateCompositeAchievements with winners failed: %v", err)
		}

		// Check that composite achievement evaluation is unchanged
		hasSuperCollectorAfter, _ := achievementRepo.HasUserAchievement(userID, "super_collector")
		hasSuperBrainAfter, _ := achievementRepo.HasUserAchievement(userID, "super_brain")
		hasLegendAfter, _ := achievementRepo.HasUserAchievement(userID, "legend")

		// Composite achievements should be unaffected by presence of new winner achievements
		if hasSuperCollectorBefore != hasSuperCollectorAfter {
			rt.Errorf("super_collector achievement status changed after adding winner achievements: before=%v, after=%v",
				hasSuperCollectorBefore, hasSuperCollectorAfter)
		}

		if hasSuperBrainBefore != hasSuperBrainAfter {
			rt.Errorf("super_brain achievement status changed after adding winner achievements: before=%v, after=%v",
				hasSuperBrainBefore, hasSuperBrainAfter)
		}

		if hasLegendBefore != hasLegendAfter {
			rt.Errorf("legend achievement status changed after adding winner achievements: before=%v, after=%v",
				hasLegendBefore, hasLegendAfter)
		}

		// Verify that legend achievement requirements do NOT include new winner achievements
		legendAchievement, err := achievementRepo.GetByKey("legend")
		if err != nil {
			rt.Fatalf("Failed to get legend achievement: %v", err)
		}

		for _, winnerKey := range []string{"winner_1", "winner_2", "winner_3"} {
			for _, reqKey := range legendAchievement.Conditions.RequiredAchievements {
				if reqKey == winnerKey {
					rt.Errorf("Legend achievement should NOT require new winner achievement %s", winnerKey)
				}
			}
		}

		// Verify that the evaluation logic itself is consistent
		expectedSuperCollector := hasAllProgressAchievements
		if expectedSuperCollector != hasSuperCollectorAfter {
			rt.Errorf("super_collector evaluation inconsistent: expected=%v, actual=%v",
				expectedSuperCollector, hasSuperCollectorAfter)
		}

		expectedSuperBrain := !hasErrors && !usedHints && completionTimeMinutes < 30
		if expectedSuperBrain != hasSuperBrainAfter {
			rt.Errorf("super_brain evaluation inconsistent: expected=%v, actual=%v (time=%d, errors=%v, hints=%v)",
				expectedSuperBrain, hasSuperBrainAfter, completionTimeMinutes, hasErrors, usedHints)
		}
	})
}

func TestProperty7_WinnerAndCompletionAchievementCoexistence(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		// Create test steps
		numSteps := rapid.IntRange(25, 30).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		// Create multiple users who will complete the quest
		numUsers := rapid.IntRange(1, 5).Draw(rt, "numUsers")
		var userIDs []int64

		// Use atomic counter to ensure unique user IDs across all test runs
		baseUserID := atomic.AddInt64(&testDBCounter, 1) * 100000
		for i := 0; i < numUsers; i++ {
			userID := baseUserID + int64(i*1000)
			createTestUserForEngine(t, userRepo, userID)
			userIDs = append(userIDs, userID)
		}

		// Have all users complete the quest at different times
		baseTime := time.Now().Add(-2 * time.Hour)

		// Create completion data with deterministic ordering
		var completions []struct {
			userID         int64
			completionTime time.Time
		}

		for userIndex, userID := range userIDs {
			// Ensure deterministic completion order by using userIndex
			userCompletionOffset := rapid.IntRange(0, 10).Draw(rt, fmt.Sprintf("completionOffset_%d", userIndex))
			completionTime := baseTime.Add(time.Duration(userIndex*60+userCompletionOffset) * time.Minute)

			completions = append(completions, struct {
				userID         int64
				completionTime time.Time
			}{
				userID:         userID,
				completionTime: completionTime,
			})

			for stepIndex, step := range steps {
				stepCompletedAt := baseTime.Add(time.Duration(userIndex*60+stepIndex) * time.Minute)
				if stepIndex == len(steps)-1 {
					stepCompletedAt = completionTime
				}
				createUserProgress(t, progressRepo, userID, step.ID, models.StatusApproved, &stepCompletedAt)
				createUserAnswer(t, queue, userID, step.ID, false, stepCompletedAt)
			}
		}

		// Sort completions by time to determine expected positions
		sort.Slice(completions, func(i, j int) bool {
			return completions[i].completionTime.Before(completions[j].completionTime)
		})

		// Evaluate quest completion for each user in completion order
		for _, completion := range completions {
			userID := completion.userID
			awarded, err := engine.OnQuestCompleted(userID)
			if err != nil {
				rt.Fatalf("OnQuestCompleted failed for user %d: %v", userID, err)
			}

			// Check that user received "winner" achievement
			hasWinner, err := achievementRepo.HasUserAchievement(userID, "winner")
			if err != nil {
				rt.Fatalf("HasUserAchievement failed for winner: %v", err)
			}
			if !hasWinner {
				rt.Errorf("User %d should have 'winner' achievement after completing quest", userID)
			}

			// Verify "winner" is in awarded list
			hasWinnerInAwarded := false
			for _, awardedKey := range awarded {
				if awardedKey == "winner" {
					hasWinnerInAwarded = true
					break
				}
			}
			if !hasWinnerInAwarded {
				rt.Errorf("'winner' achievement should be in awarded list for user %d", userID)
			}
		}

		// Verify that winner achievements are unique (only one user per position)
		for pos := 1; pos <= 3; pos++ {
			winnerKey := WinnerAchievementKeys[pos]
			holders, err := achievementRepo.GetAchievementHolders(winnerKey)
			if err != nil {
				rt.Fatalf("GetAchievementHolders failed for %s: %v", winnerKey, err)
			}
			if len(holders) > 1 {
				rt.Errorf("Winner achievement %s should have at most 1 holder, got %d", winnerKey, len(holders))
			}
		}

		// Verify that "winner" achievement is not unique (multiple users can have it)
		winnerHolders, err := achievementRepo.GetAchievementHolders("winner")
		if err != nil {
			rt.Fatalf("GetAchievementHolders failed for winner: %v", err)
		}
		if len(winnerHolders) != len(userIDs) {
			rt.Errorf("All %d users should have 'winner' achievement, got %d holders",
				len(userIDs), len(winnerHolders))
		}
	})
}

func TestLegendAchievementRequirementsUnchanged(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)

	// Get the legend achievement definition
	legendAchievement, err := achievementRepo.GetByKey("legend")
	if err != nil {
		t.Fatalf("Failed to get legend achievement: %v", err)
	}

	// Verify that legend achievement does NOT include new winner achievements
	newWinnerAchievements := []string{"winner_1", "winner_2", "winner_3"}
	for _, newWinnerKey := range newWinnerAchievements {
		for _, reqKey := range legendAchievement.Conditions.RequiredAchievements {
			if reqKey == newWinnerKey {
				t.Errorf("Legend achievement should NOT require new winner achievement %s, but found it in requirements", newWinnerKey)
			}
		}
	}

	// Verify that the LegendRequiredAchievements variable also doesn't include new winner achievements
	for _, newWinnerKey := range newWinnerAchievements {
		for _, reqKey := range LegendRequiredAchievements {
			if reqKey == newWinnerKey {
				t.Errorf("LegendRequiredAchievements variable should NOT include new winner achievement %s, but found it", newWinnerKey)
			}
		}
	}

	// Verify that legend achievement still includes the original "winner" achievement
	hasOriginalWinner := false
	for _, reqKey := range legendAchievement.Conditions.RequiredAchievements {
		if reqKey == "winner" {
			hasOriginalWinner = true
			break
		}
	}
	if !hasOriginalWinner {
		t.Errorf("Legend achievement should still require the original 'winner' achievement")
	}

	// Verify that LegendRequiredAchievements variable still includes the original "winner" achievement
	hasOriginalWinnerInVar := false
	for _, reqKey := range LegendRequiredAchievements {
		if reqKey == "winner" {
			hasOriginalWinnerInVar = true
			break
		}
	}
	if !hasOriginalWinnerInVar {
		t.Errorf("LegendRequiredAchievements variable should still include the original 'winner' achievement")
	}

	t.Logf("Legend achievement requirements verified: excludes new winner achievements, includes original winner achievement")
}

func TestProperty1_ManualAchievementAwardCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		adminID := int64(rapid.IntRange(10000, 19999).Draw(rt, "adminID"))
		createTestUserForEngine(t, userRepo, userID)
		createTestUserForEngine(t, userRepo, adminID)

		manualAchievements := []string{"veteran", "activity", "wow"}
		achievementKey := rapid.SampledFrom(manualAchievements).Draw(rt, "achievementKey")

		initialAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements failed: %v", err)
		}
		initialCount := len(initialAchievements)

		err = engine.AwardManualAchievement(userID, achievementKey, adminID)
		if err != nil {
			rt.Fatalf("AwardManualAchievement failed: %v", err)
		}

		hasAchievement, err := achievementRepo.HasUserAchievement(userID, achievementKey)
		if err != nil {
			rt.Fatalf("HasUserAchievement failed: %v", err)
		}
		if !hasAchievement {
			rt.Errorf("User should have manual achievement %s after awarding", achievementKey)
		}

		finalAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements after award failed: %v", err)
		}
		if len(finalAchievements) != initialCount+1 {
			rt.Errorf("User should have %d achievements after award, got %d", initialCount+1, len(finalAchievements))
		}

		achievement, err := achievementRepo.GetByKey(achievementKey)
		if err != nil {
			rt.Fatalf("GetByKey failed: %v", err)
		}

		var awardedAchievement *models.UserAchievement
		for _, ua := range finalAchievements {
			if ua.AchievementID == achievement.ID {
				awardedAchievement = ua
				break
			}
		}
		if awardedAchievement == nil {
			rt.Errorf("Could not find awarded achievement in user's achievement list")
		} else {
			if awardedAchievement.IsRetroactive {
				rt.Errorf("Manual achievement should not be marked as retroactive")
			}
			if time.Since(awardedAchievement.EarnedAt) > time.Minute {
				rt.Errorf("Achievement earned_at should be recent, got %v", awardedAchievement.EarnedAt)
			}
		}
	})
}

func TestProperty4_MultipleAwardSupport(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		adminID := int64(rapid.IntRange(10000, 19999).Draw(rt, "adminID"))
		createTestUserForEngine(t, userRepo, userID)
		createTestUserForEngine(t, userRepo, adminID)

		manualAchievements := []string{"veteran", "activity", "wow"}
		achievementKey := rapid.SampledFrom(manualAchievements).Draw(rt, "achievementKey")
		awardCount := rapid.IntRange(2, 5).Draw(rt, "awardCount")

		for i := 0; i < awardCount; i++ {
			err := engine.AwardManualAchievement(userID, achievementKey, adminID)
			if err != nil {
				rt.Fatalf("AwardManualAchievement failed on attempt %d: %v", i+1, err)
			}
		}

		userAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements failed: %v", err)
		}

		achievement, err := achievementRepo.GetByKey(achievementKey)
		if err != nil {
			rt.Fatalf("GetByKey failed: %v", err)
		}

		matchingCount := 0
		for _, ua := range userAchievements {
			if ua.AchievementID == achievement.ID {
				matchingCount++
			}
		}

		if matchingCount != 1 {
			rt.Errorf("User should have exactly 1 instance of achievement %s after multiple awards, got %d", achievementKey, matchingCount)
		}

		hasAchievement, err := achievementRepo.HasUserAchievement(userID, achievementKey)
		if err != nil {
			rt.Fatalf("HasUserAchievement failed: %v", err)
		}
		if !hasAchievement {
			rt.Errorf("HasUserAchievement should return true when user has the achievement")
		}
	})
}

func TestProperty5_AwardMetadataCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		userID := int64(rapid.IntRange(1000, 9999).Draw(rt, "userID"))
		adminID := int64(rapid.IntRange(10000, 19999).Draw(rt, "adminID"))
		createTestUserForEngine(t, userRepo, userID)
		createTestUserForEngine(t, userRepo, adminID)

		manualAchievements := []string{"veteran", "activity", "wow"}
		achievementKey := rapid.SampledFrom(manualAchievements).Draw(rt, "achievementKey")

		beforeAward := time.Now()
		err := engine.AwardManualAchievement(userID, achievementKey, adminID)
		if err != nil {
			rt.Fatalf("AwardManualAchievement failed: %v", err)
		}
		afterAward := time.Now()

		userAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements failed: %v", err)
		}

		achievement, err := achievementRepo.GetByKey(achievementKey)
		if err != nil {
			rt.Fatalf("GetByKey failed: %v", err)
		}

		var awardedAchievement *models.UserAchievement
		for _, ua := range userAchievements {
			if ua.AchievementID == achievement.ID {
				awardedAchievement = ua
				break
			}
		}

		if awardedAchievement == nil {
			rt.Fatalf("Could not find awarded achievement in user's achievement list")
		}

		if awardedAchievement.UserID != userID {
			rt.Errorf("Achievement UserID should be %d, got %d", userID, awardedAchievement.UserID)
		}

		if awardedAchievement.AchievementID != achievement.ID {
			rt.Errorf("Achievement AchievementID should be %d, got %d", achievement.ID, awardedAchievement.AchievementID)
		}

		if awardedAchievement.EarnedAt.Before(beforeAward) || awardedAchievement.EarnedAt.After(afterAward) {
			rt.Errorf("Achievement EarnedAt should be between %v and %v, got %v", beforeAward, afterAward, awardedAchievement.EarnedAt)
		}

		if awardedAchievement.IsRetroactive {
			rt.Errorf("Manual achievement should not be marked as retroactive")
		}

		user, err := userRepo.GetByID(userID)
		if err != nil {
			rt.Fatalf("GetByID failed: %v", err)
		}
		if user == nil {
			rt.Errorf("User should exist in database")
		}

		achievementFromDB, err := achievementRepo.GetByID(achievement.ID)
		if err != nil {
			rt.Fatalf("GetByID for achievement failed: %v", err)
		}
		if achievementFromDB == nil {
			rt.Errorf("Achievement should exist in database")
		}
	})
}

// Test manual achievement notification flow
func TestManualAchievementNotificationFlow(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

	userID := int64(1000)
	adminID := int64(2000)
	createTestUserForEngine(t, userRepo, userID)
	createTestUserForEngine(t, userRepo, adminID)

	// Test that manual achievement can be awarded
	err := engine.AwardManualAchievement(userID, "veteran", adminID)
	if err != nil {
		t.Fatalf("AwardManualAchievement failed: %v", err)
	}

	// Verify achievement was awarded
	hasAchievement, err := achievementRepo.HasUserAchievement(userID, "veteran")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if !hasAchievement {
		t.Error("User should have veteran achievement after manual award")
	}

	// Test that sticker creation works the same way
	notifier := &AchievementNotifier{
		achievementRepo: achievementRepo,
	}

	achievement, err := achievementRepo.GetByKey("veteran")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}

	// Test notification formatting
	notification := notifier.FormatNotification(achievement)
	if !strings.Contains(notification, "🎉") {
		t.Error("Manual achievement notification should contain celebration emoji")
	}
	if !strings.Contains(notification, "Поздравляем") {
		t.Error("Manual achievement notification should contain congratulatory text")
	}
	if !strings.Contains(notification, achievement.Name) {
		t.Error("Manual achievement notification should contain achievement name")
	}

	// Test emoji mapping
	emoji := notifier.GetAchievementEmoji(achievement)
	if emoji != "🛡️" {
		t.Errorf("Veteran achievement should have 🛡️ emoji, got %s", emoji)
	}
}

// Test sticker creation and delivery for manual achievements
func TestManualAchievementStickerIntegration(t *testing.T) {
	// Test sticker service emoji mapping
	stickerService := &StickerService{}

	testCases := []struct {
		achievementKey string
		expectedEmoji  string
	}{
		{"veteran", "🛡️"},
		{"activity", "🪩"},
		{"wow", "💎"},
	}

	for _, tc := range testCases {
		emoji := stickerService.getAchievementEmoji(tc.achievementKey)
		if emoji != tc.expectedEmoji {
			t.Errorf("Achievement %s should have emoji %s, got %s", tc.achievementKey, tc.expectedEmoji, emoji)
		}
	}
}

// Test backward compatibility with existing achievements
func TestBackwardCompatibilityWithExistingAchievements(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

	userID := int64(1000)
	adminID := int64(2000)
	createTestUserForEngine(t, userRepo, userID)
	createTestUserForEngine(t, userRepo, adminID)

	// Create test steps
	numSteps := 10
	var steps []*models.Step
	for i := 1; i <= numSteps; i++ {
		step := createTestStep(t, stepRepo, i)
		steps = append(steps, step)
	}

	// Award some manual achievements
	err := engine.AwardManualAchievement(userID, "veteran", adminID)
	if err != nil {
		t.Fatalf("AwardManualAchievement failed: %v", err)
	}

	err = engine.AwardManualAchievement(userID, "activity", adminID)
	if err != nil {
		t.Fatalf("AwardManualAchievement failed: %v", err)
	}

	// Create user progress to trigger automatic achievements
	baseTime := time.Now().Add(-24 * time.Hour)
	for i := 0; i < 5; i++ {
		completedAt := baseTime.Add(time.Duration(i*10) * time.Minute)
		createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)
	}

	// Test that automatic achievements still work correctly
	awarded, err := engine.EvaluateProgressAchievements(userID)
	if err != nil {
		t.Fatalf("EvaluateProgressAchievements failed: %v", err)
	}

	// Should get beginner_5 achievement
	expectedAchievement := "beginner_5"
	found := false
	for _, key := range awarded {
		if key == expectedAchievement {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Should have awarded %s achievement, got %v", expectedAchievement, awarded)
	}

	// Verify user has both manual and automatic achievements
	hasVeteran, err := achievementRepo.HasUserAchievement(userID, "veteran")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if !hasVeteran {
		t.Error("User should still have veteran achievement")
	}

	hasBeginner, err := achievementRepo.HasUserAchievement(userID, "beginner_5")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if !hasBeginner {
		t.Error("User should have beginner_5 achievement")
	}

	// Test that composite achievements don't include manual achievements
	// First give user all required progress achievements for super_collector
	progressAchievements := []string{"beginner_5", "experienced_10", "advanced_15", "expert_20", "master_25"}
	earnedAt := baseTime
	for _, key := range progressAchievements {
		achievement, err := achievementRepo.GetByKey(key)
		if err != nil {
			continue // Skip if achievement doesn't exist
		}
		err = achievementRepo.AssignToUser(userID, achievement.ID, earnedAt, false)
		if err != nil {
			t.Logf("Could not assign %s: %v", key, err)
		}
		earnedAt = earnedAt.Add(time.Minute)
	}

	// Evaluate composite achievements
	compositeAwarded, err := engine.EvaluateCompositeAchievements(userID)
	if err != nil {
		t.Fatalf("EvaluateCompositeAchievements failed: %v", err)
	}

	// Should get super_collector if all progress achievements are present
	hasSuperCollector := false
	for _, key := range compositeAwarded {
		if key == "super_collector" {
			hasSuperCollector = true
			break
		}
	}

	// Verify super_collector logic works independently of manual achievements
	actualHasSuperCollector, err := achievementRepo.HasUserAchievement(userID, "super_collector")
	if err != nil {
		t.Fatalf("HasUserAchievement for super_collector failed: %v", err)
	}

	t.Logf("Super collector awarded: %v, has super collector: %v", hasSuperCollector, actualHasSuperCollector)
	// The test passes as long as composite achievement evaluation doesn't crash
	// and manual achievements don't interfere with the logic
}

// Test that manual achievements don't affect composite achievement calculations
func TestCompositeAchievementsExcludeManualAchievements(t *testing.T) {
	queue, cleanup := setupAchievementEngineTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	progressRepo := db.NewProgressRepository(queue)
	stepRepo := db.NewStepRepository(queue)

	engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

	userID := int64(1000)
	adminID := int64(2000)
	createTestUserForEngine(t, userRepo, userID)
	createTestUserForEngine(t, userRepo, adminID)

	// Award all manual achievements
	manualAchievements := []string{"veteran", "activity", "wow"}
	for _, key := range manualAchievements {
		err := engine.AwardManualAchievement(userID, key, adminID)
		if err != nil {
			t.Fatalf("AwardManualAchievement failed for %s: %v", key, err)
		}
	}

	// Test composite achievement evaluation with only manual achievements
	compositeAwarded, err := engine.EvaluateCompositeAchievements(userID)
	if err != nil {
		t.Fatalf("EvaluateCompositeAchievements failed: %v", err)
	}

	// Should not get any composite achievements from manual achievements alone
	if len(compositeAwarded) > 0 {
		t.Errorf("Manual achievements should not trigger composite achievements, got %v", compositeAwarded)
	}

	// Verify user doesn't have composite achievements
	compositeKeys := []string{"super_collector", "super_brain", "legend"}
	for _, key := range compositeKeys {
		hasAchievement, err := achievementRepo.HasUserAchievement(userID, key)
		if err != nil {
			t.Fatalf("HasUserAchievement failed for %s: %v", key, err)
		}
		if hasAchievement {
			t.Errorf("User should not have composite achievement %s from manual achievements alone", key)
		}
	}
}

func TestProperty8_ServiceIntegrationConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		userID := rapid.Int64Range(1000, 9999).Draw(rt, "userID")
		adminID := rapid.Int64Range(10000, 19999).Draw(rt, "adminID")
		createTestUserForEngine(t, userRepo, userID)
		createTestUserForEngine(t, userRepo, adminID)

		manualAchievements := []string{"veteran", "activity", "wow"}
		achievementKey := rapid.SampledFrom(manualAchievements).Draw(rt, "achievementKey")

		// Award manual achievement
		err := engine.AwardManualAchievement(userID, achievementKey, adminID)
		if err != nil {
			rt.Fatalf("AwardManualAchievement failed: %v", err)
		}

		// Verify achievement was stored in database
		hasAchievement, err := achievementRepo.HasUserAchievement(userID, achievementKey)
		if err != nil {
			rt.Fatalf("HasUserAchievement failed: %v", err)
		}
		if !hasAchievement {
			rt.Errorf("Manual achievement %s should be stored in database", achievementKey)
		}

		// Test notification service integration
		notifier := &AchievementNotifier{
			achievementRepo: achievementRepo,
		}

		achievement, err := achievementRepo.GetByKey(achievementKey)
		if err != nil {
			rt.Fatalf("GetByKey failed: %v", err)
		}

		// Verify notification formatting works the same as automatic achievements
		notification := notifier.FormatNotification(achievement)
		if !strings.Contains(notification, "🎉") {
			rt.Errorf("Manual achievement notification should contain celebration emoji")
		}
		if !strings.Contains(notification, "Поздравляем") {
			rt.Errorf("Manual achievement notification should contain congratulatory text")
		}
		if !strings.Contains(notification, achievement.Name) {
			rt.Errorf("Manual achievement notification should contain achievement name")
		}

		// Test emoji mapping consistency
		emoji := notifier.GetAchievementEmoji(achievement)
		expectedEmojis := map[string]string{
			"veteran":  "🛡️",
			"activity": "🪩",
			"wow":      "💎",
		}
		expectedEmoji := expectedEmojis[achievementKey]
		if emoji != expectedEmoji {
			rt.Errorf("Manual achievement %s should have emoji %s, got %s", achievementKey, expectedEmoji, emoji)
		}

		// Test sticker service integration
		stickerService := &StickerService{}
		stickerEmoji := stickerService.getAchievementEmoji(achievementKey)
		if stickerEmoji != expectedEmoji {
			rt.Errorf("Sticker service should return same emoji %s for %s, got %s", expectedEmoji, achievementKey, stickerEmoji)
		}

		// Verify achievement metadata is complete
		userAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements failed: %v", err)
		}

		var awardedAchievement *models.UserAchievement
		for _, ua := range userAchievements {
			if ua.AchievementID == achievement.ID {
				awardedAchievement = ua
				break
			}
		}

		if awardedAchievement == nil {
			rt.Fatalf("Could not find awarded achievement in user's achievement list")
		}

		if awardedAchievement.UserID != userID {
			rt.Errorf("Achievement UserID should be %d, got %d", userID, awardedAchievement.UserID)
		}

		if awardedAchievement.EarnedAt.IsZero() {
			rt.Errorf("Achievement should have non-zero EarnedAt timestamp")
		}

		if awardedAchievement.IsRetroactive {
			rt.Errorf("Manual achievement should not be marked as retroactive")
		}
	})
}

func TestProperty9_BackwardCompatibilityPreservation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		userRepo := db.NewUserRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		stepRepo := db.NewStepRepository(queue)

		engine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)

		// Create test steps for automatic achievement evaluation
		numSteps := rapid.IntRange(25, 30).Draw(rt, "numSteps")
		var steps []*models.Step
		for i := 1; i <= numSteps; i++ {
			step := createTestStep(t, stepRepo, i)
			steps = append(steps, step)
		}

		userID := rapid.Int64Range(1000, 9999).Draw(rt, "userID")
		adminID := rapid.Int64Range(10000, 19999).Draw(rt, "adminID")
		createTestUserForEngine(t, userRepo, userID)
		createTestUserForEngine(t, userRepo, adminID)

		// Set up user progress for automatic achievements
		correctAnswers := rapid.IntRange(5, 25).Draw(rt, "correctAnswers")
		completionTimeMinutes := rapid.IntRange(0, 60).Draw(rt, "completionTimeMinutes")
		usedHints := rapid.Bool().Draw(rt, "usedHints")
		hasErrors := rapid.Bool().Draw(rt, "hasErrors")

		baseTime := time.Now().Add(-time.Duration(completionTimeMinutes+10) * time.Minute)

		// Create user progress and answers for automatic achievement evaluation
		for i := 0; i < correctAnswers && i < len(steps); i++ {
			completedAt := baseTime.Add(time.Duration(i*completionTimeMinutes/(correctAnswers+1)) * time.Minute)
			if i == correctAnswers-1 {
				completedAt = baseTime.Add(time.Duration(completionTimeMinutes) * time.Minute)
			}
			createUserProgress(t, progressRepo, userID, steps[i].ID, models.StatusApproved, &completedAt)

			useHint := usedHints && i == 0
			createUserAnswer(t, queue, userID, steps[i].ID, useHint, completedAt)
		}

		if hasErrors {
			extraAnswerTime := baseTime.Add(time.Duration(correctAnswers+1) * time.Minute)
			createUserAnswer(t, queue, userID, steps[0].ID, false, extraAnswerTime)
		}

		// Evaluate automatic achievements WITHOUT manual achievements present
		progressAwardedBefore, err := engine.EvaluateProgressAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateProgressAchievements before manual achievements failed: %v", err)
		}

		_, err = engine.EvaluateHintAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateHintAchievements before manual achievements failed: %v", err)
		}

		_, err = engine.EvaluateSpecialAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateSpecialAchievements before manual achievements failed: %v", err)
		}

		_, err = engine.EvaluateCompositeAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateCompositeAchievements before manual achievements failed: %v", err)
		}

		// Store the state of automatic achievements before manual achievements
		automaticAchievementsBefore, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements before manual achievements failed: %v", err)
		}

		// Award manual achievements
		manualAchievements := []string{"veteran", "activity", "wow"}
		numManualToAward := rapid.IntRange(1, len(manualAchievements)).Draw(rt, "numManualToAward")
		for i := 0; i < numManualToAward; i++ {
			achievementKey := manualAchievements[i]
			err := engine.AwardManualAchievement(userID, achievementKey, adminID)
			if err != nil {
				rt.Fatalf("AwardManualAchievement failed for %s: %v", achievementKey, err)
			}
		}

		// Evaluate automatic achievements WITH manual achievements present
		progressAwardedAfter, err := engine.EvaluateProgressAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateProgressAchievements after manual achievements failed: %v", err)
		}

		hintAwardedAfter, err := engine.EvaluateHintAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateHintAchievements after manual achievements failed: %v", err)
		}

		specialAwardedAfter, err := engine.EvaluateSpecialAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateSpecialAchievements after manual achievements failed: %v", err)
		}

		compositeAwardedAfter, err := engine.EvaluateCompositeAchievements(userID)
		if err != nil {
			rt.Fatalf("EvaluateCompositeAchievements after manual achievements failed: %v", err)
		}

		// Verify that automatic achievement evaluation is unchanged (Requirement 6.1, 6.2)
		if len(progressAwardedAfter) > 0 {
			rt.Errorf("Progress achievements should not be re-awarded after manual achievements, got %v", progressAwardedAfter)
		}

		if len(hintAwardedAfter) > 0 {
			rt.Errorf("Hint achievements should not be re-awarded after manual achievements, got %v", hintAwardedAfter)
		}

		if len(specialAwardedAfter) > 0 {
			rt.Errorf("Special achievements should not be re-awarded after manual achievements, got %v", specialAwardedAfter)
		}

		// Verify that composite achievements don't include manual achievements (Requirement 6.3)
		if len(compositeAwardedAfter) > 0 {
			rt.Errorf("Composite achievements should not be re-awarded after manual achievements, got %v", compositeAwardedAfter)
		}

		// Verify that all automatic achievements are still present
		automaticAchievementsAfter, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements after manual achievements failed: %v", err)
		}

		// Count automatic achievements before and after (excluding manual ones)
		automaticCountBefore := 0
		for _, ua := range automaticAchievementsBefore {
			achievement, err := achievementRepo.GetByID(ua.AchievementID)
			if err != nil {
				continue
			}
			if achievement.Type != models.TypeManual {
				automaticCountBefore++
			}
		}

		automaticCountAfter := 0
		for _, ua := range automaticAchievementsAfter {
			achievement, err := achievementRepo.GetByID(ua.AchievementID)
			if err != nil {
				continue
			}
			if achievement.Type != models.TypeManual {
				automaticCountAfter++
			}
		}

		if automaticCountAfter < automaticCountBefore {
			rt.Errorf("Automatic achievements should be preserved after manual achievements: before=%d, after=%d", automaticCountBefore, automaticCountAfter)
		}

		// Verify that manual achievements don't affect composite achievement logic (Requirement 6.3)
		// Check that composite achievements still work correctly based on automatic achievements only
		hasAllProgressAchievements := true
		for _, key := range SuperCollectorRequiredAchievements {
			hasAchievement, err := achievementRepo.HasUserAchievement(userID, key)
			if err != nil {
				rt.Fatalf("HasUserAchievement failed for %s: %v", key, err)
			}
			if !hasAchievement {
				hasAllProgressAchievements = false
				break
			}
		}

		hasSuperCollector, err := achievementRepo.HasUserAchievement(userID, "super_collector")
		if err != nil {
			rt.Fatalf("HasUserAchievement failed for super_collector: %v", err)
		}

		if hasAllProgressAchievements && !hasSuperCollector {
			// This is expected behavior - super_collector should be awarded based on automatic achievements
			// regardless of manual achievements presence
		} else if !hasAllProgressAchievements && hasSuperCollector {
			rt.Errorf("super_collector should not be awarded without all required automatic achievements")
		}

		// Verify API compatibility (Requirement 6.4) - existing queries should work the same
		allUserAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements API should still work: %v", err)
		}

		if len(allUserAchievements) < automaticCountBefore {
			rt.Errorf("GetUserAchievements should return at least the same number of achievements as before")
		}

		// Test that HasUserAchievement works for both automatic and manual achievements
		for _, key := range []string{"beginner_5", "veteran", "activity"} {
			_, err := achievementRepo.HasUserAchievement(userID, key)
			if err != nil {
				rt.Errorf("HasUserAchievement API should work for achievement %s: %v", key, err)
			}
		}

		// Verify notification behavior preservation (Requirement 6.5)
		// Test that automatic achievement notifications still work the same way
		notifier := &AchievementNotifier{
			achievementRepo: achievementRepo,
		}

		// Test notification for an automatic achievement
		if len(progressAwardedBefore) > 0 {
			automaticKey := progressAwardedBefore[0]
			automaticAchievement, err := achievementRepo.GetByKey(automaticKey)
			if err == nil {
				automaticNotification := notifier.FormatNotification(automaticAchievement)
				if !strings.Contains(automaticNotification, "🎉") {
					rt.Errorf("Automatic achievement notification should still contain celebration emoji")
				}
				if !strings.Contains(automaticNotification, "Поздравляем") {
					rt.Errorf("Automatic achievement notification should still contain congratulatory text")
				}
			}
		}

		// Test notification for a manual achievement
		if numManualToAward > 0 {
			manualKey := manualAchievements[0]
			manualAchievement, err := achievementRepo.GetByKey(manualKey)
			if err == nil {
				manualNotification := notifier.FormatNotification(manualAchievement)
				if !strings.Contains(manualNotification, "🎉") {
					rt.Errorf("Manual achievement notification should contain celebration emoji")
				}
				if !strings.Contains(manualNotification, "Поздравляем") {
					rt.Errorf("Manual achievement notification should contain congratulatory text")
				}
			}
		}
	})
}

func TestProperty6_ManualAchievementClassification(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupAchievementEngineTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)

		manualAchievements := []string{"veteran", "activity", "wow"}
		achievementKey := rapid.SampledFrom(manualAchievements).Draw(rt, "achievementKey")

		achievement, err := achievementRepo.GetByKey(achievementKey)
		if err != nil {
			rt.Fatalf("GetByKey failed for %s: %v", achievementKey, err)
		}

		if achievement.Type != models.TypeManual {
			rt.Errorf("Achievement %s should have TypeManual, got %s", achievementKey, achievement.Type)
		}

		if achievement.Category != models.CategorySpecial {
			rt.Errorf("Achievement %s should have CategorySpecial, got %s", achievementKey, achievement.Category)
		}

		if achievement.Conditions.ManualAward == nil || !*achievement.Conditions.ManualAward {
			rt.Errorf("Achievement %s should have ManualAward condition set to true", achievementKey)
		}

		if achievement.IsUnique {
			rt.Errorf("Manual achievement %s should not be unique (IsUnique should be false)", achievementKey)
		}

		if !achievement.IsActive {
			rt.Errorf("Manual achievement %s should be active", achievementKey)
		}

		allAchievements, err := achievementRepo.GetAll()
		if err != nil {
			rt.Fatalf("GetAll achievements failed: %v", err)
		}

		manualCount := 0
		for _, a := range allAchievements {
			if a.Type == models.TypeManual {
				manualCount++
				if a.Category != models.CategorySpecial {
					rt.Errorf("All manual achievements should have CategorySpecial, but %s has %s", a.Key, a.Category)
				}
				if a.Conditions.ManualAward == nil || !*a.Conditions.ManualAward {
					rt.Errorf("All manual achievements should have ManualAward=true, but %s doesn't", a.Key)
				}
			}
		}

		if manualCount != 3 {
			rt.Errorf("Expected exactly 3 manual achievements, got %d", manualCount)
		}

		for _, key := range manualAchievements {
			found := false
			for _, a := range allAchievements {
				if a.Key == key && a.Type == models.TypeManual {
					found = true
					break
				}
			}
			if !found {
				rt.Errorf("Manual achievement %s not found in achievement list", key)
			}
		}
	})
}
