package services

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
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
