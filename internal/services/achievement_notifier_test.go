package services

import (
	"database/sql"
	"html"
	"strings"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func setupNotifierTestDB(t testing.TB) (*db.DBQueue, func()) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
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

func createNotifierTestAchievement(t testing.TB, repo *db.AchievementRepository, key, name, description string, category models.AchievementCategory) *models.Achievement {
	achievement := &models.Achievement{
		Key:         key,
		Name:        name,
		Description: description,
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

func TestProperty13_AchievementNotificationDelivery(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupNotifierTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)

		notifier := &AchievementNotifier{
			achievementRepo: achievementRepo,
		}

		categories := []models.AchievementCategory{
			models.CategoryProgress,
			models.CategoryCompletion,
			models.CategorySpecial,
			models.CategoryHints,
			models.CategoryComposite,
			models.CategoryUnique,
		}

		numAchievements := rapid.IntRange(1, 10).Draw(rt, "numAchievements")
		var achievements []*models.Achievement

		for i := 0; i < numAchievements; i++ {
			key := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "key")
			name := rapid.StringMatching(`[А-Яа-яA-Za-z ]{5,20}`).Draw(rt, "name")
			description := rapid.StringMatching(`[А-Яа-яA-Za-z ]{10,50}`).Draw(rt, "description")
			categoryIdx := rapid.IntRange(0, len(categories)-1).Draw(rt, "categoryIdx")
			category := categories[categoryIdx]

			achievement := createNotifierTestAchievement(t, achievementRepo, key, name, description, category)
			achievements = append(achievements, achievement)
		}

		for _, achievement := range achievements {
			notification := notifier.FormatNotification(achievement)

			if !strings.Contains(notification, "🎉") {
				rt.Errorf("Notification should contain congratulatory emoji 🎉")
			}

			if !strings.Contains(notification, "Вы получили достижение:") {
				rt.Errorf("Notification should contain congratulatory text")
			}

			if !strings.Contains(notification, achievement.Name) {
				rt.Errorf("Notification should contain achievement name '%s'", achievement.Name)
			}

			if !strings.Contains(notification, achievement.Description) {
				rt.Errorf("Notification should contain achievement description '%s'", achievement.Description)
			}

			emoji := notifier.GetAchievementEmoji(achievement)
			if emoji == "" {
				rt.Errorf("Achievement should have an emoji")
			}

			if !strings.Contains(notification, emoji) {
				rt.Errorf("Notification should contain achievement emoji '%s'", emoji)
			}
		}

		var achievementKeys []string
		for _, a := range achievements {
			achievementKeys = append(achievementKeys, a.Key)
		}

		notifications, err := notifier.PrepareNotifications(achievementKeys)
		if err != nil {
			rt.Fatalf("PrepareNotifications failed: %v", err)
		}

		if len(notifications) != len(achievements) {
			rt.Errorf("Expected %d notifications, got %d", len(achievements), len(notifications))
		}

		for i, notification := range notifications {
			if notification.AchievementKey != achievementKeys[i] {
				rt.Errorf("Notification %d should have key '%s', got '%s'", i, achievementKeys[i], notification.AchievementKey)
			}

			if notification.Achievement == nil {
				rt.Errorf("Notification %d should have achievement object", i)
			}

			if notification.Message == "" {
				rt.Errorf("Notification %d should have non-empty message", i)
			}

			if !strings.Contains(notification.Message, notification.Achievement.Name) {
				rt.Errorf("Notification message should contain achievement name")
			}
		}
	})
}

func TestProperty9_ContentPreservation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, cleanup := setupNotifierTestDB(t)
		defer cleanup()

		achievementRepo := db.NewAchievementRepository(queue)
		notifier := &AchievementNotifier{
			achievementRepo: achievementRepo,
		}

		categories := []models.AchievementCategory{
			models.CategoryProgress,
			models.CategoryCompletion,
			models.CategorySpecial,
			models.CategoryHints,
			models.CategoryComposite,
			models.CategoryUnique,
		}

		key := rapid.StringMatching(`[a-z_]{5,15}`).Draw(rt, "key")
		name := rapid.StringMatching(`[А-Яа-яA-Za-z0-9 &<>"']{5,30}`).Draw(rt, "name")
		description := rapid.StringMatching(`[А-Яа-яA-Za-z0-9 &<>"'.,!?]{10,100}`).Draw(rt, "description")
		categoryIdx := rapid.IntRange(0, len(categories)-1).Draw(rt, "categoryIdx")
		category := categories[categoryIdx]

		achievement := createNotifierTestAchievement(t, achievementRepo, key, name, description, category)
		notification := notifier.FormatNotification(achievement)

		escapedName := html.EscapeString(achievement.Name)
		escapedDescription := html.EscapeString(achievement.Description)

		if !strings.Contains(notification, escapedName) {
			rt.Errorf("Notification should contain properly escaped achievement name. Expected: %s, Got notification: %s", escapedName, notification)
		}

		if !strings.Contains(notification, escapedDescription) {
			rt.Errorf("Notification should contain properly escaped achievement description. Expected: %s, Got notification: %s", escapedDescription, notification)
		}

		if !strings.Contains(notification, "🎉") {
			rt.Errorf("Notification should preserve congratulatory emoji")
		}

		if !strings.Contains(notification, "Вы получили достижение:") {
			rt.Errorf("Notification should preserve congratulatory text")
		}

		emoji := notifier.GetAchievementEmoji(achievement)
		if !strings.Contains(notification, emoji) {
			rt.Errorf("Notification should preserve achievement emoji")
		}

		if !strings.Contains(notification, "<b>") || !strings.Contains(notification, "</b>") {
			rt.Errorf("Notification should preserve HTML bold formatting")
		}

		if !strings.Contains(notification, "<code>") || !strings.Contains(notification, "</code>") {
			rt.Errorf("Notification should preserve HTML pre formatting")
		}

		if !strings.Contains(notification, "<i>") || !strings.Contains(notification, "</i>") {
			rt.Errorf("Notification should preserve HTML italic formatting")
		}
	})
}

func TestAchievementNotifier_GetAchievementEmoji(t *testing.T) {
	queue, cleanup := setupNotifierTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	notifier := &AchievementNotifier{
		achievementRepo: achievementRepo,
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"pioneer", "🔥"},
		{"second_place", "🌟"},
		{"third_place", "💫"},
		{"beginner_5", "🌱"},
		{"winner", "🏆"},
		{"lightning", "⚡"},
		{"rocket", "🚀"},
		{"photographer", "📸"},
		{"bullseye", "🎯"},
		{"legend", "👑"},
	}

	for _, tt := range tests {
		achievement := &models.Achievement{
			Key:      tt.key,
			Category: models.CategoryProgress,
		}
		emoji := notifier.GetAchievementEmoji(achievement)
		if emoji != tt.expected {
			t.Errorf("GetAchievementEmoji(%s) = %s, want %s", tt.key, emoji, tt.expected)
		}
	}
}

func TestAchievementNotifier_GetAchievementEmoji_CategoryFallback(t *testing.T) {
	queue, cleanup := setupNotifierTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	notifier := &AchievementNotifier{
		achievementRepo: achievementRepo,
	}

	tests := []struct {
		category models.AchievementCategory
		expected string
	}{
		{models.CategoryProgress, "📈"},
		{models.CategoryCompletion, "🏆"},
		{models.CategorySpecial, "⭐"},
		{models.CategoryHints, "💡"},
		{models.CategoryComposite, "👑"},
		{models.CategoryUnique, "🎖️"},
	}

	for _, tt := range tests {
		achievement := &models.Achievement{
			Key:      "unknown_key_xyz",
			Category: tt.category,
		}
		emoji := notifier.GetAchievementEmoji(achievement)
		if emoji != tt.expected {
			t.Errorf("GetAchievementEmoji for category %s = %s, want %s", tt.category, emoji, tt.expected)
		}
	}
}

func TestAchievementNotifier_FormatNotification(t *testing.T) {
	queue, cleanup := setupNotifierTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	notifier := &AchievementNotifier{
		achievementRepo: achievementRepo,
	}

	achievement := &models.Achievement{
		Key:         "test_achievement",
		Name:        "Тестовое Достижение",
		Description: "Описание тестового достижения",
		Category:    models.CategoryProgress,
	}

	notification := notifier.FormatNotification(achievement)

	if !strings.Contains(notification, "🎉") {
		t.Error("Notification should contain celebration emoji")
	}

	if !strings.Contains(notification, "Вы получили достижение:") {
		t.Error("Notification should contain congratulatory text")
	}

	if !strings.Contains(notification, achievement.Name) {
		t.Errorf("Notification should contain achievement name: %s", achievement.Name)
	}

	if !strings.Contains(notification, achievement.Description) {
		t.Errorf("Notification should contain achievement description: %s", achievement.Description)
	}
}

func TestAchievementNotifier_PrepareNotifications(t *testing.T) {
	queue, cleanup := setupNotifierTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	notifier := &AchievementNotifier{
		achievementRepo: achievementRepo,
	}

	ach1 := createNotifierTestAchievement(t, achievementRepo, "prep_ach1", "Achievement 1", "Description 1", models.CategoryProgress)
	ach2 := createNotifierTestAchievement(t, achievementRepo, "prep_ach2", "Achievement 2", "Description 2", models.CategoryCompletion)
	ach3 := createNotifierTestAchievement(t, achievementRepo, "prep_ach3", "Achievement 3", "Description 3", models.CategorySpecial)

	keys := []string{ach1.Key, ach2.Key, ach3.Key}
	notifications, err := notifier.PrepareNotifications(keys)
	if err != nil {
		t.Fatalf("PrepareNotifications failed: %v", err)
	}

	if len(notifications) != 3 {
		t.Errorf("Expected 3 notifications, got %d", len(notifications))
	}

	for i, notification := range notifications {
		if notification.AchievementKey != keys[i] {
			t.Errorf("Notification %d key mismatch: expected %s, got %s", i, keys[i], notification.AchievementKey)
		}
		if notification.Achievement == nil {
			t.Errorf("Notification %d should have achievement", i)
		}
		if notification.Message == "" {
			t.Errorf("Notification %d should have message", i)
		}
	}
}

func TestAchievementNotifier_PrepareNotifications_InvalidKey(t *testing.T) {
	queue, cleanup := setupNotifierTestDB(t)
	defer cleanup()

	achievementRepo := db.NewAchievementRepository(queue)
	notifier := &AchievementNotifier{
		achievementRepo: achievementRepo,
	}

	ach1 := createNotifierTestAchievement(t, achievementRepo, "valid_ach", "Valid Achievement", "Description", models.CategoryProgress)

	keys := []string{ach1.Key, "invalid_key", "another_invalid"}
	notifications, err := notifier.PrepareNotifications(keys)
	if err != nil {
		t.Fatalf("PrepareNotifications failed: %v", err)
	}

	if len(notifications) != 1 {
		t.Errorf("Expected 1 notification (only valid key), got %d", len(notifications))
	}

	if notifications[0].AchievementKey != ach1.Key {
		t.Errorf("Expected notification for %s, got %s", ach1.Key, notifications[0].AchievementKey)
	}
}
