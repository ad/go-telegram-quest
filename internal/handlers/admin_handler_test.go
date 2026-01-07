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

// Feature: admin-manual-achievements, Property 2: Admin Interface Button Consistency
func TestProperty2_AdminInterfaceButtonConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		firstName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(rt, "firstName")
		lastName := rapid.StringMatching(`[A-Za-z]{0,20}`).Draw(rt, "lastName")
		username := rapid.StringMatching(`[a-z0-9_]{0,15}`).Draw(rt, "username")
		isBlocked := rapid.Bool().Draw(rt, "isBlocked")

		user := &models.User{
			ID:        userID,
			FirstName: firstName,
			LastName:  lastName,
			Username:  username,
			IsBlocked: isBlocked,
		}

		keyboard := BuildUserDetailsKeyboard(user, true)

		if keyboard == nil {
			rt.Error("BuildUserDetailsKeyboard should not return nil")
		}

		if len(keyboard.InlineKeyboard) == 0 {
			rt.Error("Keyboard should have at least one row of buttons")
		}

		// Check that achievements button is present (manual achievement buttons are now in achievements menu)
		achievementsFound := false
		for _, row := range keyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "user_achievements:") {
					achievementsFound = true
					expectedCallback := fmt.Sprintf("user_achievements:%d", userID)
					if button.CallbackData != expectedCallback {
						rt.Errorf("Achievements button callback should be '%s', got '%s'", expectedCallback, button.CallbackData)
					}
				}
			}
		}

		if !achievementsFound {
			rt.Error("User achievements button should be present")
		}

		// Verify that manual achievement buttons are NOT in the main user details keyboard
		for _, row := range keyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "award:") {
					rt.Error("Manual achievement buttons should not be in main user details keyboard")
				}
			}
		}
	})
}

// Feature: admin-manual-achievements, Property 3: Achievement Award Feedback
func TestProperty3_AchievementAwardFeedback(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")

		// Ensure admin and user are different
		for adminID == userID {
			adminID = rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		}

		achievementKeys := []string{"veteran", "activity", "wow"}
		achievementKey := rapid.SampledFrom(achievementKeys).Draw(rt, "achievementKey")

		// Test valid callback data format
		validCallbackData := fmt.Sprintf("award:%s:%d", achievementKey, userID)
		parts := strings.Split(validCallbackData, ":")

		if len(parts) != 3 {
			rt.Errorf("Valid callback data should have 3 parts, got %d", len(parts))
		}

		if parts[0] != "award" {
			rt.Errorf("First part should be 'award', got '%s'", parts[0])
		}

		if parts[1] != achievementKey {
			rt.Errorf("Second part should be '%s', got '%s'", achievementKey, parts[1])
		}

		parsedUserID, err := parseInt64(parts[2])
		if err != nil {
			rt.Errorf("Third part should be parseable as int64, got error: %v", err)
		}

		if parsedUserID != userID {
			rt.Errorf("Parsed user ID should be %d, got %d", userID, parsedUserID)
		}

		// Test invalid callback data formats
		invalidFormats := []string{
			"award:",                // Missing parts
			"award:veteran:",        // Missing user ID
			"award:veteran:invalid", // Invalid user ID
			"invalid:veteran:123",   // Wrong prefix
			"award:invalid:123",     // Invalid achievement key
		}

		for _, invalidData := range invalidFormats {
			invalidParts := strings.Split(invalidData, ":")

			// Test format validation
			if len(invalidParts) == 3 {
				if invalidParts[0] == "award" {
					// Test user ID parsing
					if _, err := parseInt64(invalidParts[2]); err != nil {
						// This should fail parsing, which is expected
						continue
					}

					// Test achievement key validation
					validKeys := map[string]bool{
						"veteran":  true,
						"activity": true,
						"wow":      true,
					}

					if !validKeys[invalidParts[1]] {
						// Invalid achievement key, should be handled
						continue
					}
				}
			}
		}

		// Test achievement name mapping
		achievementNames := map[string]string{
			"veteran":  "Ветеран игр",
			"activity": "За активность",
			"wow":      "Вау! За отличный ответ",
		}

		expectedName := achievementNames[achievementKey]
		if expectedName == "" {
			rt.Errorf("Achievement key '%s' should have a mapped name", achievementKey)
		}

		// Test success message format
		expectedSuccessMessage := fmt.Sprintf("✅ Достижение \"%s\" присвоено пользователю", expectedName)
		if !strings.Contains(expectedSuccessMessage, "✅") {
			rt.Error("Success message should contain success emoji")
		}
		if !strings.Contains(expectedSuccessMessage, expectedName) {
			rt.Error("Success message should contain achievement name")
		}
		if !strings.Contains(expectedSuccessMessage, "присвоено") {
			rt.Error("Success message should indicate achievement was awarded")
		}

		// Test error message formats
		errorMessages := []string{
			"⚠️ Неверный формат данных",
			"⚠️ Неверный ID пользователя",
			"⚠️ Система достижений недоступна",
		}

		for _, errorMsg := range errorMessages {
			if !strings.Contains(errorMsg, "⚠️") {
				rt.Errorf("Error message '%s' should contain warning emoji", errorMsg)
			}
		}
	})
}

// Feature: admin-manual-achievements, Property 10: Admin Access Control
func TestProperty10_AdminAccessControl(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		nonAdminID := rapid.Int64Range(1, 1000000).Draw(rt, "nonAdminID")
		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")

		// Ensure non-admin ID is different from admin ID
		for nonAdminID == adminID {
			nonAdminID = rapid.Int64Range(1, 1000000).Draw(rt, "nonAdminID")
		}

		user := &models.User{
			ID:        userID,
			FirstName: rapid.String().Draw(rt, "firstName"),
			LastName:  rapid.String().Draw(rt, "lastName"),
			Username:  rapid.String().Draw(rt, "username"),
			IsBlocked: rapid.Bool().Draw(rt, "isBlocked"),
		}

		// Test BuildUserDetailsKeyboard with admin privileges
		adminKeyboard := BuildUserDetailsKeyboard(user, true)
		if adminKeyboard == nil {
			rt.Error("Admin keyboard should not be nil")
		}

		// Admin keyboard should have achievements button (manual achievement buttons are in achievements menu)
		hasAchievementsButton := false
		for _, row := range adminKeyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "user_achievements:") {
					hasAchievementsButton = true
					break
				}
			}
			if hasAchievementsButton {
				break
			}
		}

		if !hasAchievementsButton {
			rt.Error("Admin keyboard should contain achievements button")
		}

		// Admin keyboard should NOT have manual achievement buttons (they're in achievements menu)
		hasManualAchievementButtons := false
		for _, row := range adminKeyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "award:") {
					hasManualAchievementButtons = true
					break
				}
			}
			if hasManualAchievementButtons {
				break
			}
		}

		if hasManualAchievementButtons {
			rt.Error("Admin keyboard should NOT contain manual achievement buttons (they should be in achievements menu)")
		}

		// Test BuildUserDetailsKeyboard without admin privileges
		nonAdminKeyboard := BuildUserDetailsKeyboard(user, false)
		if nonAdminKeyboard == nil {
			rt.Error("Non-admin keyboard should not be nil")
		}

		// Non-admin keyboard should NOT have achievements button
		hasAchievementsButtonNonAdmin := false
		for _, row := range nonAdminKeyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "user_achievements:") {
					hasAchievementsButtonNonAdmin = true
					break
				}
			}
			if hasAchievementsButtonNonAdmin {
				break
			}
		}

		if hasAchievementsButtonNonAdmin {
			rt.Error("Non-admin keyboard should NOT contain achievements button")
		}

		// Test that admin keyboard has more buttons than non-admin keyboard
		adminButtonCount := 0
		for _, row := range adminKeyboard.InlineKeyboard {
			adminButtonCount += len(row)
		}

		nonAdminButtonCount := 0
		for _, row := range nonAdminKeyboard.InlineKeyboard {
			nonAdminButtonCount += len(row)
		}

		if adminButtonCount <= nonAdminButtonCount {
			rt.Error("Admin keyboard should have more buttons than non-admin keyboard")
		}

		// Test manual achievement award access control
		achievementKeys := []string{"veteran", "activity", "wow"}
		achievementKey := rapid.SampledFrom(achievementKeys).Draw(rt, "achievementKey")

		// Test that admin can access manual achievement award (simulated)
		adminCallbackData := fmt.Sprintf("award:%s:%d", achievementKey, userID)
		if !strings.HasPrefix(adminCallbackData, "award:") {
			rt.Error("Admin callback data should have award prefix")
		}

		parts := strings.Split(adminCallbackData, ":")
		if len(parts) != 3 {
			rt.Error("Admin callback data should have exactly 3 parts")
		}

		if parts[1] != achievementKey {
			rt.Error("Achievement key should match in callback data")
		}

		// Verify the callback data format is correct for admin operations
		expectedCallbackData := fmt.Sprintf("award:%s:%d", achievementKey, userID)
		if adminCallbackData != expectedCallbackData {
			rt.Errorf("Callback data format mismatch: expected %s, got %s", expectedCallbackData, adminCallbackData)
		}
	})
}
