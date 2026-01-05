package services

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
)

var serviceTestDBCounter int64

func setupAchievementServiceTestDB(t testing.TB) (*db.DBQueue, func()) {
	counter := atomic.AddInt64(&serviceTestDBCounter, 1)
	dbName := fmt.Sprintf("file:svcmemdb%d?mode=memory&cache=shared", counter)
	sqlDB, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.InitSchema(sqlDB); err != nil {
		t.Fatal(err)
	}

	queue := db.NewDBQueueForTest(sqlDB)
	return queue, func() {
		queue.Close()
		sqlDB.Close()
	}
}

func createTestUserForService(t testing.TB, repo *db.UserRepository, id int64) *models.User {
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

func createTestAchievement(t testing.TB, repo *db.AchievementRepository, key, name string, category models.AchievementCategory) *models.Achievement {
	achievement := &models.Achievement{
		Key:         key,
		Name:        name,
		Description: "Test achievement",
		Category:    category,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions:  models.AchievementConditions{},
		IsActive:    true,
	}
	if err := repo.Create(achievement); err != nil {
		t.Fatal(err)
	}
	return achievement
}

func TestAchievementService_CreateAchievement(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	achievement := &models.Achievement{
		Key:         "test_achievement",
		Name:        "Test Achievement",
		Description: "Test description",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions:  models.AchievementConditions{},
		IsActive:    true,
	}

	err := service.CreateAchievement(achievement)
	if err != nil {
		t.Fatalf("CreateAchievement failed: %v", err)
	}

	if achievement.ID == 0 {
		t.Error("Achievement ID should be set after creation")
	}

	retrieved, err := service.GetAchievementByKey("test_achievement")
	if err != nil {
		t.Fatalf("GetAchievementByKey failed: %v", err)
	}

	if retrieved.Name != "Test Achievement" {
		t.Errorf("Expected name 'Test Achievement', got '%s'", retrieved.Name)
	}
}

func TestAchievementService_UpdateAchievement(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	achievement := createTestAchievement(t, achievementRepo, "update_test", "Original Name", models.CategoryProgress)

	achievement.Name = "Updated Name"
	achievement.Description = "Updated description"

	err := service.UpdateAchievement(achievement)
	if err != nil {
		t.Fatalf("UpdateAchievement failed: %v", err)
	}

	retrieved, err := service.GetAchievementByKey("update_test")
	if err != nil {
		t.Fatalf("GetAchievementByKey failed: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", retrieved.Name)
	}
	if retrieved.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got '%s'", retrieved.Description)
	}
}

func TestAchievementService_GetAllAchievements(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	// Get initial count of default achievements
	initialAchievements, err := service.GetAllAchievements()
	if err != nil {
		t.Fatalf("GetAllAchievements failed: %v", err)
	}
	initialCount := len(initialAchievements)

	createTestAchievement(t, achievementRepo, "ach1", "Achievement 1", models.CategoryProgress)
	createTestAchievement(t, achievementRepo, "ach2", "Achievement 2", models.CategoryCompletion)
	createTestAchievement(t, achievementRepo, "ach3", "Achievement 3", models.CategorySpecial)

	achievements, err := service.GetAllAchievements()
	if err != nil {
		t.Fatalf("GetAllAchievements failed: %v", err)
	}

	if len(achievements) != initialCount+3 {
		t.Errorf("Expected %d achievements, got %d", initialCount+3, len(achievements))
	}
}

func TestAchievementService_GetActiveAchievements(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	// Get initial count of active achievements (default achievements are all active)
	initialActive, err := service.GetActiveAchievements()
	if err != nil {
		t.Fatalf("GetActiveAchievements failed: %v", err)
	}
	initialCount := len(initialActive)

	active := createTestAchievement(t, achievementRepo, "active", "Active Achievement", models.CategoryProgress)
	inactive := createTestAchievement(t, achievementRepo, "inactive", "Inactive Achievement", models.CategoryProgress)

	inactive.IsActive = false
	if err := achievementRepo.Update(inactive); err != nil {
		t.Fatal(err)
	}

	achievements, err := service.GetActiveAchievements()
	if err != nil {
		t.Fatalf("GetActiveAchievements failed: %v", err)
	}

	// Should have initial count + 1 (active) since inactive is not counted
	if len(achievements) != initialCount+1 {
		t.Errorf("Expected %d active achievements, got %d", initialCount+1, len(achievements))
	}

	// Verify the new active achievement is in the list
	found := false
	for _, a := range achievements {
		if a.Key == active.Key {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected active achievement key '%s' to be in the list", active.Key)
	}
}

func TestAchievementService_GetUserAchievements(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	user := createTestUserForService(t, userRepo, 1001)
	ach1 := createTestAchievement(t, achievementRepo, "user_ach1", "User Achievement 1", models.CategoryProgress)
	ach2 := createTestAchievement(t, achievementRepo, "user_ach2", "User Achievement 2", models.CategoryCompletion)

	if err := achievementRepo.AssignToUser(user.ID, ach1.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user.ID, ach2.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}

	userAchievements, err := service.GetUserAchievements(user.ID)
	if err != nil {
		t.Fatalf("GetUserAchievements failed: %v", err)
	}

	if len(userAchievements) != 2 {
		t.Errorf("Expected 2 user achievements, got %d", len(userAchievements))
	}
}

func TestAchievementService_GetUserAchievementsByCategory(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	user := createTestUserForService(t, userRepo, 1002)
	progressAch := createTestAchievement(t, achievementRepo, "progress_ach", "Progress Achievement", models.CategoryProgress)
	completionAch := createTestAchievement(t, achievementRepo, "completion_ach", "Completion Achievement", models.CategoryCompletion)

	if err := achievementRepo.AssignToUser(user.ID, progressAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user.ID, completionAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}

	progressAchievements, err := service.GetUserAchievementsByCategory(user.ID, models.CategoryProgress)
	if err != nil {
		t.Fatalf("GetUserAchievementsByCategory failed: %v", err)
	}

	if len(progressAchievements) != 1 {
		t.Errorf("Expected 1 progress achievement, got %d", len(progressAchievements))
	}

	completionAchievements, err := service.GetUserAchievementsByCategory(user.ID, models.CategoryCompletion)
	if err != nil {
		t.Fatalf("GetUserAchievementsByCategory failed: %v", err)
	}

	if len(completionAchievements) != 1 {
		t.Errorf("Expected 1 completion achievement, got %d", len(completionAchievements))
	}
}

func TestAchievementService_GetUserAchievementCount(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	user := createTestUserForService(t, userRepo, 1003)
	ach1 := createTestAchievement(t, achievementRepo, "count_ach1", "Count Achievement 1", models.CategoryProgress)
	ach2 := createTestAchievement(t, achievementRepo, "count_ach2", "Count Achievement 2", models.CategoryProgress)
	ach3 := createTestAchievement(t, achievementRepo, "count_ach3", "Count Achievement 3", models.CategoryProgress)

	if err := achievementRepo.AssignToUser(user.ID, ach1.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user.ID, ach2.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user.ID, ach3.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}

	count, err := service.GetUserAchievementCount(user.ID)
	if err != nil {
		t.Fatalf("GetUserAchievementCount failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func TestAchievementService_GetUserAchievementSummary(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	user := createTestUserForService(t, userRepo, 1004)
	progressAch1 := createTestAchievement(t, achievementRepo, "summary_progress1", "Progress 1", models.CategoryProgress)
	progressAch2 := createTestAchievement(t, achievementRepo, "summary_progress2", "Progress 2", models.CategoryProgress)
	completionAch := createTestAchievement(t, achievementRepo, "summary_completion", "Completion", models.CategoryCompletion)

	if err := achievementRepo.AssignToUser(user.ID, progressAch1.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user.ID, progressAch2.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user.ID, completionAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}

	summary, err := service.GetUserAchievementSummary(user.ID)
	if err != nil {
		t.Fatalf("GetUserAchievementSummary failed: %v", err)
	}

	if summary.TotalCount != 3 {
		t.Errorf("Expected total count 3, got %d", summary.TotalCount)
	}

	if len(summary.AchievementsByCategory[models.CategoryProgress]) != 2 {
		t.Errorf("Expected 2 progress achievements, got %d", len(summary.AchievementsByCategory[models.CategoryProgress]))
	}

	if len(summary.AchievementsByCategory[models.CategoryCompletion]) != 1 {
		t.Errorf("Expected 1 completion achievement, got %d", len(summary.AchievementsByCategory[models.CategoryCompletion]))
	}
}

func TestAchievementService_GetAchievementStatistics(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	// Get initial statistics (default achievements exist)
	initialStats, err := service.GetAchievementStatistics()
	if err != nil {
		t.Fatalf("GetAchievementStatistics failed: %v", err)
	}
	initialTotalAchievements := initialStats.TotalAchievements
	initialProgressCount := initialStats.AchievementsByCategory[models.CategoryProgress]

	user1 := createTestUserForService(t, userRepo, 2001)
	user2 := createTestUserForService(t, userRepo, 2002)
	user3 := createTestUserForService(t, userRepo, 2003)

	progressAch := createTestAchievement(t, achievementRepo, "stats_progress", "Stats Progress", models.CategoryProgress)
	completionAch := createTestAchievement(t, achievementRepo, "stats_completion", "Stats Completion", models.CategoryCompletion)
	createTestAchievement(t, achievementRepo, "stats_special", "Stats Special", models.CategorySpecial)

	if err := achievementRepo.AssignToUser(user1.ID, progressAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user2.ID, progressAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user3.ID, progressAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user1.ID, completionAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user2.ID, completionAch.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}

	stats, err := service.GetAchievementStatistics()
	if err != nil {
		t.Fatalf("GetAchievementStatistics failed: %v", err)
	}

	if stats.TotalAchievements != initialTotalAchievements+3 {
		t.Errorf("Expected %d total achievements, got %d", initialTotalAchievements+3, stats.TotalAchievements)
	}

	if stats.TotalUsers != 3 {
		t.Errorf("Expected 3 total users, got %d", stats.TotalUsers)
	}

	if stats.TotalUserAchievements != 5 {
		t.Errorf("Expected 5 total user achievements, got %d", stats.TotalUserAchievements)
	}

	if stats.AchievementsByCategory[models.CategoryProgress] != initialProgressCount+1 {
		t.Errorf("Expected %d progress category achievements, got %d", initialProgressCount+1, stats.AchievementsByCategory[models.CategoryProgress])
	}

	// Popular achievements should include all achievements with user assignments
	if len(stats.PopularAchievements) < 2 {
		t.Errorf("Expected at least 2 popular achievements with assignments, got %d", len(stats.PopularAchievements))
	}

	if stats.PopularAchievements[0].UserCount != 3 {
		t.Errorf("Expected most popular achievement to have 3 users, got %d", stats.PopularAchievements[0].UserCount)
	}

	if stats.PopularAchievements[0].Achievement.Key != "stats_progress" {
		t.Errorf("Expected most popular achievement to be 'stats_progress', got '%s'", stats.PopularAchievements[0].Achievement.Key)
	}

	expectedPercentage := 100.0
	if stats.PopularAchievements[0].Percentage != expectedPercentage {
		t.Errorf("Expected percentage %.2f, got %.2f", expectedPercentage, stats.PopularAchievements[0].Percentage)
	}
}

func TestAchievementService_GetUsersWithMostAchievements(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	user1 := createTestUserForService(t, userRepo, 3001)
	user2 := createTestUserForService(t, userRepo, 3002)
	user3 := createTestUserForService(t, userRepo, 3003)

	ach1 := createTestAchievement(t, achievementRepo, "ranking_ach1", "Ranking 1", models.CategoryProgress)
	ach2 := createTestAchievement(t, achievementRepo, "ranking_ach2", "Ranking 2", models.CategoryProgress)
	ach3 := createTestAchievement(t, achievementRepo, "ranking_ach3", "Ranking 3", models.CategoryProgress)

	if err := achievementRepo.AssignToUser(user1.ID, ach1.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user1.ID, ach2.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user1.ID, ach3.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user2.ID, ach1.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user2.ID, ach2.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user3.ID, ach1.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}

	rankings, err := service.GetUsersWithMostAchievements(10)
	if err != nil {
		t.Fatalf("GetUsersWithMostAchievements failed: %v", err)
	}

	if len(rankings) != 3 {
		t.Errorf("Expected 3 rankings, got %d", len(rankings))
	}

	if rankings[0].User.ID != user1.ID {
		t.Errorf("Expected user1 to be first, got user %d", rankings[0].User.ID)
	}
	if rankings[0].AchievementCount != 3 {
		t.Errorf("Expected user1 to have 3 achievements, got %d", rankings[0].AchievementCount)
	}

	if rankings[1].User.ID != user2.ID {
		t.Errorf("Expected user2 to be second, got user %d", rankings[1].User.ID)
	}
	if rankings[1].AchievementCount != 2 {
		t.Errorf("Expected user2 to have 2 achievements, got %d", rankings[1].AchievementCount)
	}

	if rankings[2].User.ID != user3.ID {
		t.Errorf("Expected user3 to be third, got user %d", rankings[2].User.ID)
	}
	if rankings[2].AchievementCount != 1 {
		t.Errorf("Expected user3 to have 1 achievement, got %d", rankings[2].AchievementCount)
	}
}

func TestAchievementService_GetUsersWithMostAchievements_WithLimit(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	for i := 1; i <= 5; i++ {
		user := createTestUserForService(t, userRepo, int64(4000+i))
		ach := createTestAchievement(t, achievementRepo, "limit_ach_"+string(rune('a'+i-1)), "Limit Achievement", models.CategoryProgress)
		if err := achievementRepo.AssignToUser(user.ID, ach.ID, time.Now(), false); err != nil {
			t.Fatal(err)
		}
	}

	rankings, err := service.GetUsersWithMostAchievements(3)
	if err != nil {
		t.Fatalf("GetUsersWithMostAchievements failed: %v", err)
	}

	if len(rankings) != 3 {
		t.Errorf("Expected 3 rankings with limit, got %d", len(rankings))
	}
}

func TestAchievementService_HasUserAchievement(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	user := createTestUserForService(t, userRepo, 5001)
	ach := createTestAchievement(t, achievementRepo, "has_ach_test", "Has Achievement Test", models.CategoryProgress)

	hasAchievement, err := service.HasUserAchievement(user.ID, "has_ach_test")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if hasAchievement {
		t.Error("User should not have achievement before assignment")
	}

	if err := achievementRepo.AssignToUser(user.ID, ach.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}

	hasAchievement, err = service.HasUserAchievement(user.ID, "has_ach_test")
	if err != nil {
		t.Fatalf("HasUserAchievement failed: %v", err)
	}
	if !hasAchievement {
		t.Error("User should have achievement after assignment")
	}
}

func TestAchievementService_GetAchievementHolders(t *testing.T) {
	queue, cleanup := setupAchievementServiceTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	userRepo := db.NewUserRepository(queue)
	service := NewAchievementService(achievementRepo, userRepo)

	user1 := createTestUserForService(t, userRepo, 6001)
	user2 := createTestUserForService(t, userRepo, 6002)
	ach := createTestAchievement(t, achievementRepo, "holders_test", "Holders Test", models.CategoryProgress)

	if err := achievementRepo.AssignToUser(user1.ID, ach.ID, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := achievementRepo.AssignToUser(user2.ID, ach.ID, time.Now().Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}

	holders, err := service.GetAchievementHolders("holders_test")
	if err != nil {
		t.Fatalf("GetAchievementHolders failed: %v", err)
	}

	if len(holders) != 2 {
		t.Errorf("Expected 2 holders, got %d", len(holders))
	}

	if holders[0] != user1.ID {
		t.Errorf("Expected first holder to be user1 (%d), got %d", user1.ID, holders[0])
	}
}
