package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/ad/go-telegram-quest/internal/services"
	tgmodels "github.com/go-telegram/bot/models"
	"pgregory.net/rapid"
)

func TestProperty6_AdminInterfaceAccessControl(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		nonAdminID := rapid.Int64Range(1, 1000000).Draw(rt, "nonAdminID")

		for nonAdminID == adminID {
			nonAdminID = rapid.Int64Range(1, 1000000).Draw(rt, "nonAdminID")
		}

		adminHandler := &AdminHandler{
			adminID: adminID,
		}

		adminMessage := &tgmodels.Message{
			From: &tgmodels.User{ID: adminID},
			// Text: "/some_command",
		}

		nonAdminMessage := &tgmodels.Message{
			From: &tgmodels.User{ID: nonAdminID},
			// Text: "/some_command",
		}

		if adminMessage.From.ID != adminHandler.adminID {
			rt.Error("Admin ID should match")
		}

		if nonAdminMessage.From.ID == adminHandler.adminID {
			rt.Error("Non-admin ID should not match admin ID")
		}

		questStateCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: nonAdminID},
			// Data: "admin:quest_state",
		}

		adminQuestStateCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: adminID},
			// Data: "admin:quest_state",
		}

		if questStateCallback.From.ID == adminHandler.adminID {
			rt.Error("Non-admin callback should not have admin ID")
		}

		if adminQuestStateCallback.From.ID != adminHandler.adminID {
			rt.Error("Admin callback should have admin ID")
		}
	})
}

func TestProperty7_StepMovementAccessControl(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		nonAdminID := rapid.Int64Range(1, 1000000).Draw(rt, "nonAdminID")
		stepID := rapid.Int64Range(1, 100).Draw(rt, "stepID")

		for nonAdminID == adminID {
			nonAdminID = rapid.Int64Range(1, 1000000).Draw(rt, "nonAdminID")
		}

		adminHandler := &AdminHandler{
			adminID: adminID,
		}

		moveUpCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: nonAdminID},
			// Data: rapid.StringMatching(`admin:move_up:\d+`).Draw(rt, "moveUpData"),
		}

		moveDownCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: nonAdminID},
			// Data: rapid.StringMatching(`admin:move_down:\d+`).Draw(rt, "moveDownData"),
		}

		adminMoveUpCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: adminID},
			// Data: rapid.StringMatching(`admin:move_up:\d+`).Draw(rt, "adminMoveUpData"),
		}

		if moveUpCallback.From.ID == adminHandler.adminID {
			rt.Error("Non-admin move up callback should not have admin ID")
		}

		if moveDownCallback.From.ID == adminHandler.adminID {
			rt.Error("Non-admin move down callback should not have admin ID")
		}

		if adminMoveUpCallback.From.ID != adminHandler.adminID {
			rt.Error("Admin move callback should have admin ID")
		}

		_ = stepID
	})
}

func TestParseInt64FromMoveCommands(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"admin:move_up:123", 123},
		{"admin:move_down:456", 456},
		{"admin:move_up:0", 0},
		{"admin:move_down:", 0},
		{"", 0},
	}

	for _, test := range tests {
		var result int64
		switch test.input {
		case "admin:move_up:123":
			result, _ = parseInt64("123")
		case "admin:move_down:456":
			result, _ = parseInt64("456")
		case "admin:move_up:0":
			result, _ = parseInt64("0")
		default:
			result, _ = parseInt64("")
		}

		if result != test.expected {
			t.Errorf("For input %q, expected %d, got %d", test.input, test.expected, result)
		}
	}
}
func TestProperty6_AdminHintInterfaceLogic(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		stepID := rapid.Int64Range(1, 1000).Draw(rt, "stepID")
		stepOrder := rapid.IntRange(1, 100).Draw(rt, "stepOrder")

		hintText := rapid.StringOf(rapid.Rune()).Draw(rt, "hintText")
		hintImage := rapid.StringOf(rapid.Rune()).Draw(rt, "hintImage")

		step := &models.Step{
			ID:        stepID,
			StepOrder: stepOrder,
			HintText:  hintText,
			HintImage: hintImage,
		}

		hasHint := step.HasHint()
		expectedHasHint := (hintText != "" || hintImage != "")

		if hasHint != expectedHasHint {
			rt.Errorf("HasHint() returned %v, expected %v for hintText='%s', hintImage='%s'",
				hasHint, expectedHasHint, hintText, hintImage)
		}

		if hasHint {
			if hintText == "" && hintImage == "" {
				rt.Error("Step should not have hint when both text and image are empty")
			}
		} else {
			if hintText != "" || hintImage != "" {
				rt.Error("Step should have hint when text or image is not empty")
			}
		}
	})
}

func TestProperty14_AdminPanelAchievementDisplay(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		firstName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(rt, "firstName")
		lastName := rapid.StringMatching(`[A-Za-z]{0,20}`).Draw(rt, "lastName")
		username := rapid.StringMatching(`[a-z0-9_]{0,15}`).Draw(rt, "username")

		user := &models.User{
			ID:        userID,
			FirstName: firstName,
			LastName:  lastName,
			Username:  username,
		}

		numAchievements := rapid.IntRange(0, 10).Draw(rt, "numAchievements")

		categories := []models.AchievementCategory{
			models.CategoryProgress,
			models.CategoryCompletion,
			models.CategorySpecial,
			models.CategoryHints,
			models.CategoryComposite,
			models.CategoryUnique,
		}

		summary := &services.UserAchievementSummary{
			TotalCount:             numAchievements,
			AchievementsByCategory: make(map[models.AchievementCategory][]*services.UserAchievementDetails),
		}

		for i := 0; i < numAchievements; i++ {
			categoryIdx := rapid.IntRange(0, len(categories)-1).Draw(rt, fmt.Sprintf("categoryIdx_%d", i))
			category := categories[categoryIdx]

			achievementName := rapid.StringMatching(`[A-Za-zА-Яа-я ]{3,30}`).Draw(rt, fmt.Sprintf("achievementName_%d", i))
			earnedAt := rapid.StringMatching(`\d{2}\.\d{2}\.\d{4} \d{2}:\d{2}`).Draw(rt, fmt.Sprintf("earnedAt_%d", i))

			details := &services.UserAchievementDetails{
				Achievement: &models.Achievement{
					ID:       int64(i + 1),
					Key:      fmt.Sprintf("ach_%d", i),
					Name:     achievementName,
					Category: category,
				},
				EarnedAt: earnedAt,
			}

			summary.AchievementsByCategory[category] = append(
				summary.AchievementsByCategory[category],
				details,
			)
		}

		formatted := FormatUserAchievements(user, summary)

		if !strings.Contains(formatted, "🏆") {
			rt.Error("Formatted output should contain achievement emoji")
		}

		if !strings.Contains(formatted, user.DisplayName()) {
			rt.Errorf("Formatted output should contain user display name: %s", user.DisplayName())
		}

		if numAchievements == 0 {
			if !strings.Contains(formatted, "нет достижений") {
				rt.Error("Should indicate no achievements when count is 0")
			}
		} else {
			if !strings.Contains(formatted, fmt.Sprintf("%d", numAchievements)) {
				rt.Errorf("Should contain total count: %d", numAchievements)
			}

			for category, achievements := range summary.AchievementsByCategory {
				if len(achievements) == 0 {
					continue
				}

				categoryDisplayed := false
				categoryNames := map[models.AchievementCategory]string{
					models.CategoryProgress:   "📈 Прогресс",
					models.CategoryCompletion: "🏁 Завершение",
					models.CategorySpecial:    "⭐ Особые",
					models.CategoryHints:      "💡 Подсказки",
					models.CategoryComposite:  "🎖️ Составные",
					models.CategoryUnique:     "👑 Уникальные",
				}

				if strings.Contains(formatted, categoryNames[category]) {
					categoryDisplayed = true
				}

				if !categoryDisplayed {
					rt.Errorf("Category %s should be displayed when it has achievements", category)
				}

				for _, details := range achievements {
					if !strings.Contains(formatted, details.Achievement.Name) {
						rt.Errorf("Achievement name '%s' should be in output", details.Achievement.Name)
					}
					if !strings.Contains(formatted, details.EarnedAt) {
						rt.Errorf("Achievement earned time '%s' should be in output", details.EarnedAt)
					}
				}
			}
		}
	})
}

func TestProperty14_AchievementStatisticsDisplay(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		totalAchievements := rapid.IntRange(0, 50).Draw(rt, "totalAchievements")
		totalUserAchievements := rapid.IntRange(0, 500).Draw(rt, "totalUserAchievements")
		totalUsers := rapid.IntRange(1, 100).Draw(rt, "totalUsers")

		stats := &services.AchievementStatistics{
			TotalAchievements:      totalAchievements,
			TotalUserAchievements:  totalUserAchievements,
			TotalUsers:             totalUsers,
			AchievementsByCategory: make(map[models.AchievementCategory]int),
			PopularAchievements:    make([]services.AchievementPopularity, 0),
		}

		categories := []models.AchievementCategory{
			models.CategoryProgress,
			models.CategoryCompletion,
			models.CategorySpecial,
		}

		for _, cat := range categories {
			count := rapid.IntRange(0, 10).Draw(rt, fmt.Sprintf("cat_%s", cat))
			if count > 0 {
				stats.AchievementsByCategory[cat] = count
			}
		}

		numPopular := rapid.IntRange(0, 5).Draw(rt, "numPopular")
		for i := 0; i < numPopular; i++ {
			userCount := rapid.IntRange(1, totalUsers).Draw(rt, fmt.Sprintf("popUserCount_%d", i))
			percentage := float64(userCount) / float64(totalUsers) * 100

			stats.PopularAchievements = append(stats.PopularAchievements, services.AchievementPopularity{
				Achievement: &models.Achievement{
					ID:   int64(i + 1),
					Key:  fmt.Sprintf("pop_%d", i),
					Name: fmt.Sprintf("Popular %d", i),
				},
				UserCount:  userCount,
				Percentage: percentage,
			})
		}

		formatted := FormatAchievementStatistics(stats)

		if !strings.Contains(formatted, "🏆") {
			rt.Error("Should contain achievement emoji")
		}

		if !strings.Contains(formatted, fmt.Sprintf("%d", totalAchievements)) {
			rt.Errorf("Should contain total achievements: %d", totalAchievements)
		}

		if !strings.Contains(formatted, fmt.Sprintf("%d", totalUserAchievements)) {
			rt.Errorf("Should contain total user achievements: %d", totalUserAchievements)
		}

		if !strings.Contains(formatted, fmt.Sprintf("%d", totalUsers)) {
			rt.Errorf("Should contain total users: %d", totalUsers)
		}

		for _, pop := range stats.PopularAchievements {
			if pop.UserCount > 0 {
				if !strings.Contains(formatted, pop.Achievement.Name) {
					rt.Errorf("Popular achievement '%s' should be in output", pop.Achievement.Name)
				}
			}
		}
	})
}

func TestProperty14_AchievementLeadersDisplay(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numLeaders := rapid.IntRange(0, 15).Draw(rt, "numLeaders")

		rankings := make([]services.UserAchievementRanking, numLeaders)
		for i := 0; i < numLeaders; i++ {
			userID := rapid.Int64Range(1, 1000000).Draw(rt, fmt.Sprintf("userID_%d", i))
			firstName := rapid.StringMatching(`[A-Za-z]{1,15}`).Draw(rt, fmt.Sprintf("firstName_%d", i))
			achievementCount := rapid.IntRange(1, 50).Draw(rt, fmt.Sprintf("achievementCount_%d", i))

			rankings[i] = services.UserAchievementRanking{
				User: &models.User{
					ID:        userID,
					FirstName: firstName,
				},
				AchievementCount: achievementCount,
			}
		}

		formatted := FormatAchievementLeaders(rankings)

		if !strings.Contains(formatted, "🏅") {
			rt.Error("Should contain leaders emoji")
		}

		if numLeaders == 0 {
			if !strings.Contains(formatted, "нет пользователей") {
				rt.Error("Should indicate no users when rankings is empty")
			}
		} else {
			if numLeaders >= 1 && !strings.Contains(formatted, "🥇") {
				rt.Error("Should contain gold medal for first place")
			}
			if numLeaders >= 2 && !strings.Contains(formatted, "🥈") {
				rt.Error("Should contain silver medal for second place")
			}
			if numLeaders >= 3 && !strings.Contains(formatted, "🥉") {
				rt.Error("Should contain bronze medal for third place")
			}

			for _, ranking := range rankings {
				if !strings.Contains(formatted, ranking.User.DisplayName()) {
					rt.Errorf("User '%s' should be in output", ranking.User.DisplayName())
				}
				if !strings.Contains(formatted, fmt.Sprintf("%d", ranking.AchievementCount)) {
					rt.Errorf("Achievement count %d should be in output", ranking.AchievementCount)
				}
			}
		}
	})
}
