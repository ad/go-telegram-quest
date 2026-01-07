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

func TestProperty19_UserListPagination(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)
		achievementRepo := db.NewAchievementRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		chatStateRepo := db.NewChatStateRepository(queue)
		achievementEngine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
		manager := NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, achievementRepo, statsService, achievementEngine)

		numUsers := rapid.IntRange(0, 35).Draw(rt, "numUsers")
		for i := 1; i <= numUsers; i++ {
			user := &models.User{
				ID:        int64(i * 1000),
				FirstName: "User",
			}
			if err := userRepo.CreateOrUpdate(user); err != nil {
				rt.Fatal(err)
			}
		}

		page := rapid.IntRange(1, 5).Draw(rt, "page")
		result, err := manager.GetUserListPage(page)
		if err != nil {
			rt.Fatal(err)
		}

		if len(result.Users) > UsersPerPage {
			rt.Errorf("Page has %d users, expected at most %d", len(result.Users), UsersPerPage)
		}

		expectedTotalPages := (numUsers + UsersPerPage - 1) / UsersPerPage
		if expectedTotalPages == 0 {
			expectedTotalPages = 1
		}
		if result.TotalPages != expectedTotalPages {
			rt.Errorf("Expected %d total pages, got %d", expectedTotalPages, result.TotalPages)
		}

		if result.HasPrev != (result.CurrentPage > 1) {
			rt.Errorf("HasPrev=%v but CurrentPage=%d", result.HasPrev, result.CurrentPage)
		}

		if result.HasNext != (result.CurrentPage < result.TotalPages) {
			rt.Errorf("HasNext=%v but CurrentPage=%d, TotalPages=%d", result.HasNext, result.CurrentPage, result.TotalPages)
		}

		if numUsers > 0 {
			effectivePage := page
			if effectivePage > expectedTotalPages {
				effectivePage = expectedTotalPages
			}
			start := (effectivePage - 1) * UsersPerPage
			expectedOnPage := numUsers - start
			if expectedOnPage > UsersPerPage {
				expectedOnPage = UsersPerPage
			}
			if len(result.Users) != expectedOnPage {
				rt.Errorf("Expected %d users on page %d, got %d", expectedOnPage, effectivePage, len(result.Users))
			}
		}
	})
}

func TestProperty20_UserDetailsCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)
		achievementRepo := db.NewAchievementRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		chatStateRepo := db.NewChatStateRepository(queue)
		achievementEngine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
		manager := NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, achievementRepo, statsService, achievementEngine)

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		firstName := rapid.StringMatching(`[A-Za-z]{0,10}`).Draw(rt, "firstName")
		lastName := rapid.StringMatching(`[A-Za-z]{0,10}`).Draw(rt, "lastName")
		username := rapid.StringMatching(`[a-z]{0,10}`).Draw(rt, "username")
		isBlocked := rapid.Bool().Draw(rt, "isBlocked")

		user := &models.User{
			ID:        userID,
			FirstName: firstName,
			LastName:  lastName,
			Username:  username,
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		if isBlocked {
			if err := userRepo.BlockUser(userID); err != nil {
				rt.Fatal(err)
			}
		}

		numSteps := rapid.IntRange(0, 5).Draw(rt, "numSteps")
		for i := 1; i <= numSteps; i++ {
			step := &models.Step{
				StepOrder:  i,
				Text:       "Step text",
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			if _, err := stepRepo.Create(step); err != nil {
				rt.Fatal(err)
			}
		}

		details, err := manager.GetUserDetails(userID)
		if err != nil {
			rt.Fatal(err)
		}

		if details.User == nil {
			rt.Fatal("User should not be nil")
		}
		if details.User.ID != userID {
			rt.Errorf("Expected user ID %d, got %d", userID, details.User.ID)
		}
		if details.User.FirstName != firstName {
			rt.Errorf("Expected firstName %q, got %q", firstName, details.User.FirstName)
		}
		if details.User.LastName != lastName {
			rt.Errorf("Expected lastName %q, got %q", lastName, details.User.LastName)
		}
		if details.User.Username != username {
			rt.Errorf("Expected username %q, got %q", username, details.User.Username)
		}
		if details.User.IsBlocked != isBlocked {
			rt.Errorf("Expected isBlocked %v, got %v", isBlocked, details.User.IsBlocked)
		}

		if numSteps == 0 {
			if !details.IsCompleted {
				rt.Error("Expected IsCompleted=true when no steps exist")
			}
		} else {
			if details.IsCompleted {
				rt.Error("Expected IsCompleted=false when steps exist and not completed")
			}
			if details.CurrentStep == nil {
				rt.Error("Expected CurrentStep to be set when steps exist")
			} else if details.CurrentStep.StepOrder != 1 {
				rt.Errorf("Expected CurrentStep.StepOrder=1, got %d", details.CurrentStep.StepOrder)
			}
			if details.Status != models.StatusPending {
				rt.Errorf("Expected Status=pending, got %s", details.Status)
			}
		}
	})
}
func TestProperty3_RestartAchievementPreservation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)
		achievementRepo := db.NewAchievementRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		chatStateRepo := db.NewChatStateRepository(queue)
		achievementEngine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
		manager := NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, achievementRepo, statsService, achievementEngine)

		// Initialize achievements
		_, err := queue.Execute(func(sqlDB *sql.DB) (any, error) {
			return nil, db.InitializeDefaultAchievements(sqlDB)
		})
		if err != nil {
			rt.Fatal(err)
		}

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		firstName := rapid.StringMatching(`[A-Za-z]{1,10}`).Draw(rt, "firstName")

		user := &models.User{
			ID:        userID,
			FirstName: firstName,
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		// Create some progress and achievements for the user
		numSteps := rapid.IntRange(1, 3).Draw(rt, "numSteps")
		for i := 1; i <= numSteps; i++ {
			step := &models.Step{
				StepOrder:  i,
				Text:       "Step text",
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			stepID, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}

			// Add some progress
			progress := &models.UserProgress{
				UserID:      userID,
				StepID:      stepID,
				Status:      models.StatusApproved,
				CompletedAt: &time.Time{},
			}
			if err := progressRepo.Create(progress); err != nil {
				rt.Fatal(err)
			}
		}

		// Award some other achievements
		otherAchievements := []string{"beginner_5", "photographer"}
		for _, key := range otherAchievements {
			achievement, err := achievementRepo.GetByKey(key)
			if err == nil {
				achievementRepo.AssignToUser(userID, achievement.ID, time.Now(), false)
			}
		}

		// Check achievements before reset
		achievementsBefore, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatal(err)
		}

		// Reset user progress
		err = manager.ResetUserProgress(userID)
		if err != nil {
			rt.Fatal(err)
		}

		// Check that restart achievement is present after reset
		hasRestart, err := achievementRepo.HasUserAchievement(userID, "restart")
		if err != nil {
			rt.Fatal(err)
		}
		if !hasRestart {
			rt.Error("User should have restart achievement after progress reset")
		}

		// Check achievements after reset
		achievementsAfter, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatal(err)
		}

		// Verify that preserved achievements are still there
		preservedKeys := map[string]bool{
			"winner_1": true,
			"winner_2": true,
			"winner_3": true,
			"restart":  true,
			"cheater":  true,
		}

		for _, ua := range achievementsBefore {
			achievement, err := achievementRepo.GetByID(ua.AchievementID)
			if err != nil {
				continue
			}
			if preservedKeys[achievement.Key] {
				// This achievement should still be present
				found := false
				for _, afterUA := range achievementsAfter {
					if afterUA.AchievementID == ua.AchievementID {
						found = true
						break
					}
				}
				if !found {
					rt.Errorf("Preserved achievement %s should still be present after reset", achievement.Key)
				}
			}
		}

		// Verify progress was actually cleared
		progressAfter, err := progressRepo.GetUserProgress(userID)
		if err != nil {
			rt.Fatal(err)
		}
		if len(progressAfter) > 0 {
			rt.Error("User progress should be cleared after reset")
		}
	})
}
func TestProperty4_RestartAchievementIdempotence(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupTestDBForUserStats(t)
		defer cleanup()

		userRepo := db.NewUserRepository(queue)
		stepRepo := db.NewStepRepository(queue)
		progressRepo := db.NewProgressRepository(queue)
		answerRepo := db.NewAnswerRepository(queue)
		achievementRepo := db.NewAchievementRepository(queue)
		statsService := NewStatisticsService(queue, stepRepo, progressRepo, userRepo)
		chatStateRepo := db.NewChatStateRepository(queue)
		achievementEngine := NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, queue)
		manager := NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, achievementRepo, statsService, achievementEngine)

		// Initialize achievements
		_, err := queue.Execute(func(sqlDB *sql.DB) (any, error) {
			return nil, db.InitializeDefaultAchievements(sqlDB)
		})
		if err != nil {
			rt.Fatal(err)
		}

		userID := rapid.Int64Range(1, 1000).Draw(rt, "userID")
		firstName := rapid.StringMatching(`[A-Za-z]{1,10}`).Draw(rt, "firstName")

		user := &models.User{
			ID:        userID,
			FirstName: firstName,
		}
		if err := userRepo.CreateOrUpdate(user); err != nil {
			rt.Fatal(err)
		}

		// Create some progress for the user
		numSteps := rapid.IntRange(1, 3).Draw(rt, "numSteps")
		for i := 1; i <= numSteps; i++ {
			step := &models.Step{
				StepOrder:  i,
				Text:       "Step text",
				AnswerType: models.AnswerTypeText,
				IsActive:   true,
				IsDeleted:  false,
			}
			stepID, err := stepRepo.Create(step)
			if err != nil {
				rt.Fatal(err)
			}

			// Add some progress
			progress := &models.UserProgress{
				UserID:      userID,
				StepID:      stepID,
				Status:      models.StatusApproved,
				CompletedAt: &time.Time{},
			}
			if err := progressRepo.Create(progress); err != nil {
				rt.Fatal(err)
			}
		}

		// Perform multiple resets
		numResets := rapid.IntRange(2, 5).Draw(rt, "numResets")
		for i := 0; i < numResets; i++ {
			// Add some progress before each reset (except the first)
			if i > 0 {
				for j := 1; j <= numSteps; j++ {
					steps, _ := stepRepo.GetActive()
					if len(steps) > 0 {
						progress := &models.UserProgress{
							UserID:      userID,
							StepID:      steps[0].ID,
							Status:      models.StatusApproved,
							CompletedAt: &time.Time{},
						}
						progressRepo.Create(progress)
					}
				}
			}

			// Reset user progress
			err = manager.ResetUserProgress(userID)
			if err != nil {
				rt.Fatal(err)
			}
		}

		// Check that user has exactly one restart achievement
		userAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatal(err)
		}

		restartCount := 0
		for _, ua := range userAchievements {
			achievement, err := achievementRepo.GetByID(ua.AchievementID)
			if err != nil {
				continue
			}
			if achievement.Key == "restart" {
				restartCount++
			}
		}

		if restartCount != 1 {
			rt.Errorf("Expected exactly 1 restart achievement after %d resets, got %d", numResets, restartCount)
		}

		// Verify that restart achievement is present
		hasRestart, err := achievementRepo.HasUserAchievement(userID, "restart")
		if err != nil {
			rt.Fatal(err)
		}
		if !hasRestart {
			rt.Error("User should have restart achievement after multiple resets")
		}
	})
}
