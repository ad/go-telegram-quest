package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func setupAchievementTestDB(t *testing.T) (*sql.DB, *AchievementRepository, *UserRepository) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}

	queue := NewDBQueue(db)
	achievementRepo := NewAchievementRepository(queue)
	userRepo := NewUserRepository(queue)

	return db, achievementRepo, userRepo
}

func createTestAchievement(t *testing.T, repo *AchievementRepository, key string) *models.Achievement {
	achievement := &models.Achievement{
		Key:         key,
		Name:        "Test Achievement " + key,
		Description: "Test description for " + key,
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions:  models.AchievementConditions{},
		IsActive:    true,
	}

	err := repo.Create(achievement)
	if err != nil {
		t.Fatal(err)
	}

	return achievement
}

func createTestUser(t *testing.T, repo *UserRepository, id int64) *models.User {
	user := &models.User{
		ID:        id,
		FirstName: "Test",
		LastName:  "User",
		Username:  "testuser",
	}

	err := repo.CreateOrUpdate(user)
	if err != nil {
		t.Fatal(err)
	}

	return user
}

func TestProperty4_AchievementAssignmentUniqueness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db, achievementRepo, userRepo := setupAchievementTestDB(t)
		defer db.Close()

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		createTestUser(t, userRepo, userID)

		achievementKey := rapid.StringMatching(`[a-z][a-z0-9_]{2,19}`).Draw(rt, "achievementKey")
		achievement := createTestAchievement(t, achievementRepo, achievementKey)

		assignmentCount := rapid.IntRange(2, 10).Draw(rt, "assignmentCount")

		baseTime := time.Now()
		for i := 0; i < assignmentCount; i++ {
			earnedAt := baseTime.Add(time.Duration(i) * time.Hour)
			isRetroactive := rapid.Bool().Draw(rt, "isRetroactive")
			err := achievementRepo.AssignToUser(userID, achievement.ID, earnedAt, isRetroactive)
			if err != nil {
				rt.Fatalf("AssignToUser failed: %v", err)
			}
		}

		userAchievements, err := achievementRepo.GetUserAchievements(userID)
		if err != nil {
			rt.Fatalf("GetUserAchievements failed: %v", err)
		}

		achievementCount := 0
		for _, ua := range userAchievements {
			if ua.AchievementID == achievement.ID {
				achievementCount++
			}
		}

		if achievementCount != 1 {
			rt.Fatalf("Expected exactly 1 assignment for achievement %s to user %d, got %d",
				achievementKey, userID, achievementCount)
		}

		count, err := achievementRepo.CountUserAchievements(userID)
		if err != nil {
			rt.Fatalf("CountUserAchievements failed: %v", err)
		}

		if count != 1 {
			rt.Fatalf("Expected count of 1 for user %d, got %d", userID, count)
		}
	})
}

func TestAchievementRepository_DuplicateAssignment(t *testing.T) {
	db, achievementRepo, userRepo := setupAchievementTestDB(t)
	defer db.Close()

	createTestUser(t, userRepo, 1)
	achievement := createTestAchievement(t, achievementRepo, "test_achievement")

	earnedAt := time.Now()
	err := achievementRepo.AssignToUser(1, achievement.ID, earnedAt, false)
	if err != nil {
		t.Fatalf("First assignment failed: %v", err)
	}

	err = achievementRepo.AssignToUser(1, achievement.ID, earnedAt.Add(time.Hour), true)
	if err != nil {
		t.Fatalf("Second assignment should not return error: %v", err)
	}

	userAchievements, err := achievementRepo.GetUserAchievements(1)
	if err != nil {
		t.Fatalf("GetUserAchievements failed: %v", err)
	}

	if len(userAchievements) != 1 {
		t.Fatalf("Expected 1 achievement, got %d", len(userAchievements))
	}

	if userAchievements[0].IsRetroactive {
		t.Error("First assignment should be preserved (not retroactive)")
	}
}

func TestAchievementRepository_GetByInvalidID(t *testing.T) {
	db, achievementRepo, _ := setupAchievementTestDB(t)
	defer db.Close()

	_, err := achievementRepo.GetByID(999999)
	if err == nil {
		t.Error("Expected error for non-existent achievement ID")
	}
}

func TestAchievementRepository_GetByInvalidKey(t *testing.T) {
	db, achievementRepo, _ := setupAchievementTestDB(t)
	defer db.Close()

	_, err := achievementRepo.GetByKey("non_existent_key")
	if err == nil {
		t.Error("Expected error for non-existent achievement key")
	}
}

func TestAchievementRepository_UpdatePreservesUserRecords(t *testing.T) {
	db, achievementRepo, userRepo := setupAchievementTestDB(t)
	defer db.Close()

	createTestUser(t, userRepo, 1)
	achievement := createTestAchievement(t, achievementRepo, "update_test")

	earnedAt := time.Now()
	err := achievementRepo.AssignToUser(1, achievement.ID, earnedAt, false)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	achievement.Name = "Updated Name"
	achievement.Description = "Updated Description"
	err = achievementRepo.Update(achievement)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	userAchievements, err := achievementRepo.GetUserAchievements(1)
	if err != nil {
		t.Fatalf("GetUserAchievements failed: %v", err)
	}

	if len(userAchievements) != 1 {
		t.Fatalf("Expected 1 achievement after update, got %d", len(userAchievements))
	}

	if userAchievements[0].AchievementID != achievement.ID {
		t.Error("User achievement should still reference the same achievement ID")
	}
}

func TestAchievementRepository_HasUserAchievement(t *testing.T) {
	db, achievementRepo, userRepo := setupAchievementTestDB(t)
	defer db.Close()

	createTestUser(t, userRepo, 1)
	achievement := createTestAchievement(t, achievementRepo, "has_test")

	has, err := achievementRepo.HasUserAchievement(1, "has_test")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if has {
		t.Error("User should not have achievement before assignment")
	}

	err = achievementRepo.AssignToUser(1, achievement.ID, time.Now(), false)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	has, err = achievementRepo.HasUserAchievement(1, "has_test")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if !has {
		t.Error("User should have achievement after assignment")
	}
}

func TestAchievementRepository_GetAchievementHolders(t *testing.T) {
	db, achievementRepo, userRepo := setupAchievementTestDB(t)
	defer db.Close()

	createTestUser(t, userRepo, 1)
	createTestUser(t, userRepo, 2)
	createTestUser(t, userRepo, 3)
	achievement := createTestAchievement(t, achievementRepo, "holders_test")

	baseTime := time.Now()
	achievementRepo.AssignToUser(2, achievement.ID, baseTime, false)
	achievementRepo.AssignToUser(1, achievement.ID, baseTime.Add(time.Hour), false)
	achievementRepo.AssignToUser(3, achievement.ID, baseTime.Add(2*time.Hour), false)

	holders, err := achievementRepo.GetAchievementHolders("holders_test")
	if err != nil {
		t.Fatalf("GetAchievementHolders failed: %v", err)
	}

	if len(holders) != 3 {
		t.Fatalf("Expected 3 holders, got %d", len(holders))
	}

	if holders[0] != 2 || holders[1] != 1 || holders[2] != 3 {
		t.Errorf("Holders should be ordered by earned_at: expected [2,1,3], got %v", holders)
	}
}

func TestAchievementRepository_GetAchievementStats(t *testing.T) {
	db, achievementRepo, userRepo := setupAchievementTestDB(t)
	defer db.Close()

	createTestUser(t, userRepo, 1)
	createTestUser(t, userRepo, 2)
	achievement1 := createTestAchievement(t, achievementRepo, "stats_test_1")
	achievement2 := createTestAchievement(t, achievementRepo, "stats_test_2")

	achievementRepo.AssignToUser(1, achievement1.ID, time.Now(), false)
	achievementRepo.AssignToUser(2, achievement1.ID, time.Now(), false)
	achievementRepo.AssignToUser(1, achievement2.ID, time.Now(), false)

	stats, err := achievementRepo.GetAchievementStats()
	if err != nil {
		t.Fatalf("GetAchievementStats failed: %v", err)
	}

	if stats["stats_test_1"] != 2 {
		t.Errorf("Expected 2 users for stats_test_1, got %d", stats["stats_test_1"])
	}
	if stats["stats_test_2"] != 1 {
		t.Errorf("Expected 1 user for stats_test_2, got %d", stats["stats_test_2"])
	}
}
