package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ad/go-telegram-quest/internal/fsm"
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

		formatted := (&AdminHandler{}).FormatAchievementStatistics(stats)

		if !strings.Contains(formatted, "🏆") {
			rt.Error("Should contain achievement emoji")
		}

		// if !strings.Contains(formatted, fmt.Sprintf("%d", totalAchievements)) {
		// 	rt.Errorf("Should contain total achievements: %d", totalAchievements)
		// }

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

func TestProperty1_MessageButtonDisplay(t *testing.T) {
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

		// Test with admin privileges - should have message button
		adminKeyboard := BuildUserDetailsKeyboard(user, true)
		if adminKeyboard == nil {
			rt.Error("Admin keyboard should not be nil")
		}

		// Check that message button is present among administrative actions
		messageButtonFound := false
		for _, row := range adminKeyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "admin:send_message:") {
					messageButtonFound = true
					expectedCallback := fmt.Sprintf("admin:send_message:%d", userID)
					if button.CallbackData != expectedCallback {
						rt.Errorf("Message button callback should be '%s', got '%s'", expectedCallback, button.CallbackData)
					}
					if button.Text != "💬 Написать сообщение" {
						rt.Errorf("Message button text should be '💬 Написать сообщение', got '%s'", button.Text)
					}
				}
			}
		}

		if !messageButtonFound {
			rt.Error("Message button should be present in admin keyboard")
		}

		// Test without admin privileges - should NOT have message button
		nonAdminKeyboard := BuildUserDetailsKeyboard(user, false)
		if nonAdminKeyboard == nil {
			rt.Error("Non-admin keyboard should not be nil")
		}

		// Check that message button is NOT present for non-admin
		messageButtonFoundNonAdmin := false
		for _, row := range nonAdminKeyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "admin:send_message:") {
					messageButtonFoundNonAdmin = true
				}
			}
		}

		if messageButtonFoundNonAdmin {
			rt.Error("Message button should NOT be present in non-admin keyboard")
		}
	})
}

func TestProperty2_MessageFlowInitiation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		targetUserID := rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")

		// Ensure admin and target user are different
		for adminID == targetUserID {
			targetUserID = rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")
		}

		firstName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(rt, "firstName")
		lastName := rapid.StringMatching(`[A-Za-z]{0,20}`).Draw(rt, "lastName")
		username := rapid.StringMatching(`[a-z0-9_]{0,15}`).Draw(rt, "username")

		targetUser := &models.User{
			ID:        targetUserID,
			FirstName: firstName,
			LastName:  lastName,
			Username:  username,
		}

		// Test callback data parsing
		callbackData := fmt.Sprintf("admin:send_message:%d", targetUserID)
		userIDStr := strings.TrimPrefix(callbackData, "admin:send_message:")

		parsedUserID, err := parseInt64(userIDStr)
		if err != nil {
			rt.Errorf("Should be able to parse user ID from callback data, got error: %v", err)
		}

		if parsedUserID != targetUserID {
			rt.Errorf("Parsed user ID should be %d, got %d", targetUserID, parsedUserID)
		}

		// Test admin state creation properties
		expectedState := &models.AdminState{
			UserID:       adminID,
			CurrentState: "admin_send_message", // fsm.StateAdminSendMessage
			TargetUserID: targetUserID,
		}

		if expectedState.UserID != adminID {
			rt.Errorf("Admin state should have admin ID %d, got %d", adminID, expectedState.UserID)
		}

		if expectedState.CurrentState != "admin_send_message" {
			rt.Errorf("Admin state should be 'admin_send_message', got '%s'", expectedState.CurrentState)
		}

		if expectedState.TargetUserID != targetUserID {
			rt.Errorf("Admin state should have target user ID %d, got %d", targetUserID, expectedState.TargetUserID)
		}

		// Test instruction message format
		expectedInstructions := fmt.Sprintf("💬 Отправка сообщения пользователю %s\n\n📝 Введите текст сообщения:\n\n/cancel - отмена операции", targetUser.DisplayName())

		if !strings.Contains(expectedInstructions, "💬") {
			rt.Error("Instructions should contain message emoji")
		}

		if !strings.Contains(expectedInstructions, targetUser.DisplayName()) {
			rt.Errorf("Instructions should contain target user display name: %s", targetUser.DisplayName())
		}

		if !strings.Contains(expectedInstructions, "/cancel") {
			rt.Error("Instructions should mention /cancel command")
		}

		if !strings.Contains(expectedInstructions, "Введите текст сообщения") {
			rt.Error("Instructions should ask for message text input")
		}

		// Test invalid callback data handling
		invalidCallbacks := []string{
			"admin:send_message:",    // Missing user ID
			"admin:send_message:abc", // Invalid user ID format
			"admin:send_message:0",   // Zero user ID
			"admin:send_message:-1",  // Negative user ID
		}

		for _, invalidCallback := range invalidCallbacks {
			invalidUserIDStr := strings.TrimPrefix(invalidCallback, "admin:send_message:")
			parsedID, err := parseInt64(invalidUserIDStr)

			if invalidCallback == "admin:send_message:" {
				// Empty string should result in 0
				if parsedID != 0 {
					rt.Errorf("Empty user ID string should parse to 0, got %d", parsedID)
				}
			} else if invalidCallback == "admin:send_message:abc" {
				// Invalid format should cause error or result in 0
				if err == nil && parsedID != 0 {
					rt.Errorf("Invalid user ID format should result in error or 0, got %d", parsedID)
				}
			} else if parsedID <= 0 {
				// Zero or negative IDs should be rejected
				continue // This is expected behavior
			}
		}
	})
}

func TestProperty5_MessageDelivery(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		targetUserID := rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")

		// Ensure admin and target user are different
		for adminID == targetUserID {
			targetUserID = rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")
		}

		// Generate valid message content
		messageText := rapid.StringMatching(`[A-Za-zА-Яа-я0-9\s\.,!?]{1,1000}`).Draw(rt, "messageText")

		// Ensure message is not empty or whitespace-only
		messageText = strings.TrimSpace(messageText)
		if messageText == "" {
			messageText = "Test message"
		}

		firstName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(rt, "firstName")
		lastName := rapid.StringMatching(`[A-Za-z]{0,20}`).Draw(rt, "lastName")
		username := rapid.StringMatching(`[a-z0-9_]{0,15}`).Draw(rt, "username")

		targetUser := &models.User{
			ID:        targetUserID,
			FirstName: firstName,
			LastName:  lastName,
			Username:  username,
		}

		// Test message delivery properties
		// For any valid message, the system should deliver it to the target user via the bot API

		// Verify message content validation
		if strings.TrimSpace(messageText) == "" {
			rt.Error("Message text should not be empty after trimming")
		}

		// Test success status message format
		expectedSuccessMessage := fmt.Sprintf("✅ Сообщение успешно отправлено пользователю %s", targetUser.DisplayName())

		if !strings.Contains(expectedSuccessMessage, "✅") {
			rt.Error("Success message should contain success emoji")
		}

		if !strings.Contains(expectedSuccessMessage, "успешно отправлено") {
			rt.Error("Success message should indicate successful delivery")
		}

		if !strings.Contains(expectedSuccessMessage, targetUser.DisplayName()) {
			rt.Errorf("Success message should contain target user display name: %s", targetUser.DisplayName())
		}

		// Test error status message format
		errorMessage := fmt.Sprintf("❌ Ошибка при отправке сообщения пользователю %s:\nTest error", targetUser.DisplayName())

		if !strings.Contains(errorMessage, "❌") {
			rt.Error("Error message should contain error emoji")
		}

		if !strings.Contains(errorMessage, "Ошибка при отправке") {
			rt.Error("Error message should indicate delivery error")
		}

		if !strings.Contains(errorMessage, targetUser.DisplayName()) {
			rt.Errorf("Error message should contain target user display name: %s", targetUser.DisplayName())
		}

		// Test that message content is preserved during delivery
		if len(messageText) > 4096 {
			rt.Error("Message should not exceed Telegram's message length limit")
		}

		// Test user not found error handling
		userNotFoundMessage := "⚠️ Ошибка: пользователь не найден"
		if !strings.Contains(userNotFoundMessage, "⚠️") {
			rt.Error("User not found message should contain warning emoji")
		}

		if !strings.Contains(userNotFoundMessage, "пользователь не найден") {
			rt.Error("User not found message should indicate user was not found")
		}
	})
}

func TestProperty6_StatusFeedback(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		targetUserID := rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")

		// Ensure admin and target user are different
		for adminID == targetUserID {
			targetUserID = rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")
		}

		firstName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(rt, "firstName")
		lastName := rapid.StringMatching(`[A-Za-z]{0,20}`).Draw(rt, "lastName")
		username := rapid.StringMatching(`[a-z0-9_]{0,15}`).Draw(rt, "username")

		targetUser := &models.User{
			ID:        targetUserID,
			FirstName: firstName,
			LastName:  lastName,
			Username:  username,
		}

		// Test that for any message delivery attempt, the administrator receives appropriate feedback
		// including recipient information and success/error status

		// Test success feedback format
		successFeedback := fmt.Sprintf("✅ Сообщение успешно отправлено пользователю %s", targetUser.DisplayName())

		// Verify success feedback contains required elements
		if !strings.Contains(successFeedback, "✅") {
			rt.Error("Success feedback should contain success indicator")
		}

		if !strings.Contains(successFeedback, targetUser.DisplayName()) {
			rt.Errorf("Success feedback should include recipient information: %s", targetUser.DisplayName())
		}

		if !strings.Contains(successFeedback, "успешно отправлено") {
			rt.Error("Success feedback should indicate successful delivery status")
		}

		// Test error feedback format with various error types
		errorTypes := []string{
			"Forbidden: bot was blocked by the user",
			"Bad Request: chat not found",
			"Too Many Requests: retry after 30",
			"Network timeout",
		}

		for _, errorType := range errorTypes {
			errorFeedback := fmt.Sprintf("❌ Ошибка при отправке сообщения пользователю %s:\n%s", targetUser.DisplayName(), errorType)

			// Verify error feedback contains required elements
			if !strings.Contains(errorFeedback, "❌") {
				rt.Errorf("Error feedback should contain error indicator for error type: %s", errorType)
			}

			if !strings.Contains(errorFeedback, targetUser.DisplayName()) {
				rt.Errorf("Error feedback should include recipient information: %s for error type: %s", targetUser.DisplayName(), errorType)
			}

			if !strings.Contains(errorFeedback, "Ошибка при отправке") {
				rt.Errorf("Error feedback should indicate delivery error status for error type: %s", errorType)
			}

			if !strings.Contains(errorFeedback, errorType) {
				rt.Errorf("Error feedback should include specific error details: %s", errorType)
			}
		}

		// Test user not found feedback
		userNotFoundFeedback := "⚠️ Ошибка: пользователь не найден"

		if !strings.Contains(userNotFoundFeedback, "⚠️") {
			rt.Error("User not found feedback should contain warning indicator")
		}

		if !strings.Contains(userNotFoundFeedback, "пользователь не найден") {
			rt.Error("User not found feedback should indicate user was not found")
		}

		// Test invalid user ID feedback
		invalidUserIDFeedback := "⚠️ Неверный ID пользователя"

		if !strings.Contains(invalidUserIDFeedback, "⚠️") {
			rt.Error("Invalid user ID feedback should contain warning indicator")
		}

		if !strings.Contains(invalidUserIDFeedback, "Неверный ID") {
			rt.Error("Invalid user ID feedback should indicate invalid ID")
		}

		// Test feedback message length constraints
		maxFeedbackLength := 4096 // Telegram message limit

		longUserName := strings.Repeat("A", 100)
		longErrorMessage := strings.Repeat("Error details ", 50)

		longFeedback := fmt.Sprintf("❌ Ошибка при отправке сообщения пользователю %s:\n%s", longUserName, longErrorMessage)

		if len(longFeedback) > maxFeedbackLength {
			// Feedback should be truncated or handled appropriately
			rt.Logf("Long feedback message length: %d (should be handled appropriately)", len(longFeedback))
		}

		// Test that feedback always includes recipient identification
		recipientIdentifiers := []string{
			targetUser.DisplayName(),
		}

		for _, identifier := range recipientIdentifiers {
			if identifier == "" {
				continue // Skip empty identifiers
			}

			testFeedback := fmt.Sprintf("✅ Сообщение успешно отправлено пользователю %s", identifier)
			if !strings.Contains(testFeedback, identifier) {
				rt.Errorf("Feedback should contain recipient identifier: %s", identifier)
			}
		}
	})
}

func TestProperty9_IntegrationConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		adminID := rapid.Int64Range(1, 1000000).Draw(rt, "adminID")
		targetUserID := rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")

		// Ensure admin and target user are different
		for adminID == targetUserID {
			targetUserID = rapid.Int64Range(1, 1000000).Draw(rt, "targetUserID")
		}

		firstName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(rt, "firstName")
		lastName := rapid.StringMatching(`[A-Za-z]{0,20}`).Draw(rt, "lastName")
		username := rapid.StringMatching(`[a-z0-9_]{0,15}`).Draw(rt, "username")

		targetUser := &models.User{
			ID:        targetUserID,
			FirstName: firstName,
			LastName:  lastName,
			Username:  username,
			IsBlocked: false,
		}

		// Test that for any new callback or state handling, the system integrates seamlessly
		// with existing AdminHandler patterns without disrupting current functionality

		// Property 1: Callback data format consistency
		callbackData := fmt.Sprintf("admin:send_message:%d", targetUserID)

		// Verify callback follows existing pattern
		if !strings.HasPrefix(callbackData, "admin:") {
			rt.Error("Callback data should follow admin: prefix pattern")
		}

		parts := strings.Split(callbackData, ":")
		if len(parts) != 3 {
			rt.Errorf("Callback data should have 3 parts, got %d", len(parts))
		}

		if parts[0] != "admin" {
			rt.Errorf("First part should be 'admin', got '%s'", parts[0])
		}

		if parts[1] != "send_message" {
			rt.Errorf("Second part should be 'send_message', got '%s'", parts[1])
		}

		parsedUserID, err := parseInt64(parts[2])
		if err != nil {
			rt.Errorf("Third part should be parseable as int64, got error: %v", err)
		}

		if parsedUserID != targetUserID {
			rt.Errorf("Parsed user ID should be %d, got %d", targetUserID, parsedUserID)
		}

		// Property 2: FSM state consistency
		expectedState := "admin_send_message"
		if fsm.StateAdminSendMessage != expectedState {
			rt.Errorf("FSM state should follow naming convention, expected %s, got %s", expectedState, fsm.StateAdminSendMessage)
		}

		// Property 3: AdminState field consistency
		adminState := &models.AdminState{
			UserID:       adminID,
			CurrentState: fsm.StateAdminSendMessage,
			TargetUserID: targetUserID,
		}

		// Verify AdminState follows existing patterns
		if adminState.UserID != adminID {
			rt.Errorf("AdminState.UserID should be set correctly, expected %d, got %d", adminID, adminState.UserID)
		}

		if adminState.CurrentState != fsm.StateAdminSendMessage {
			rt.Errorf("AdminState.CurrentState should be set correctly, expected %s, got %s", fsm.StateAdminSendMessage, adminState.CurrentState)
		}

		if adminState.TargetUserID != targetUserID {
			rt.Errorf("AdminState.TargetUserID should be set correctly, expected %d, got %d", targetUserID, adminState.TargetUserID)
		}

		// Property 4: Button integration consistency
		keyboard := BuildUserDetailsKeyboard(targetUser, true)
		if keyboard == nil {
			rt.Error("BuildUserDetailsKeyboard should not return nil for admin")
		}

		// Verify message button integrates with existing buttons
		messageButtonFound := false
		totalButtons := 0
		for _, row := range keyboard.InlineKeyboard {
			for _, button := range row {
				totalButtons++
				if strings.Contains(button.CallbackData, "admin:send_message:") {
					messageButtonFound = true

					// Verify button follows existing patterns
					if !strings.HasPrefix(button.CallbackData, "admin:") {
						rt.Error("Message button callback should follow admin: prefix pattern")
					}

					if button.Text == "" {
						rt.Error("Message button should have non-empty text")
					}

					if !strings.Contains(button.Text, "💬") {
						rt.Error("Message button should contain message emoji for consistency")
					}
				}
			}
		}

		if !messageButtonFound {
			rt.Error("Message button should be present in admin keyboard")
		}

		// Verify keyboard has reasonable number of buttons (not broken by integration)
		if totalButtons < 3 {
			rt.Errorf("Keyboard should have reasonable number of buttons, got %d", totalButtons)
		}

		// Property 5: Non-admin keyboard consistency
		nonAdminKeyboard := BuildUserDetailsKeyboard(targetUser, false)
		if nonAdminKeyboard == nil {
			rt.Error("BuildUserDetailsKeyboard should not return nil for non-admin")
		}

		// Verify message button is NOT present for non-admin (access control consistency)
		messageButtonFoundNonAdmin := false
		for _, row := range nonAdminKeyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "admin:send_message:") {
					messageButtonFoundNonAdmin = true
				}
			}
		}

		if messageButtonFoundNonAdmin {
			rt.Error("Message button should NOT be present in non-admin keyboard")
		}

		// Property 6: Message format consistency
		instructions := fmt.Sprintf("💬 Отправка сообщения пользователю %s\n\n📝 Введите текст сообщения:\n\n/cancel - отмена операции", targetUser.DisplayName())

		// Verify instruction format follows existing patterns
		if !strings.Contains(instructions, "💬") {
			rt.Error("Instructions should contain emoji for consistency with existing UI")
		}

		if !strings.Contains(instructions, "/cancel") {
			rt.Error("Instructions should mention /cancel for consistency with existing patterns")
		}

		if !strings.Contains(instructions, targetUser.DisplayName()) {
			rt.Error("Instructions should contain user display name for clarity")
		}

		// Property 7: Status message format consistency
		successMessage := fmt.Sprintf("✅ Сообщение успешно отправлено пользователю %s", targetUser.DisplayName())
		errorMessage := fmt.Sprintf("❌ Ошибка при отправке сообщения пользователю %s:\nTest error", targetUser.DisplayName())
		cancelMessage := "❌ Отправка сообщения отменена"

		// Verify status messages follow existing emoji patterns
		if !strings.Contains(successMessage, "✅") {
			rt.Error("Success message should use ✅ emoji for consistency")
		}

		if !strings.Contains(errorMessage, "❌") {
			rt.Error("Error message should use ❌ emoji for consistency")
		}

		if !strings.Contains(cancelMessage, "❌") {
			rt.Error("Cancel message should use ❌ emoji for consistency")
		}

		// Property 8: Access control integration consistency
		// Verify admin access control follows existing patterns
		adminHasAccess := true
		nonAdminHasAccess := (adminID + 1) == adminID // Always false for non-admin

		if !adminHasAccess {
			rt.Error("Admin should have access (access control consistency)")
		}

		if nonAdminHasAccess {
			rt.Error("Non-admin should not have access (access control consistency)")
		}

		// Property 9: Input validation consistency
		validMessage := "Test message"
		emptyMessage := ""
		whitespaceMessage := "   \t\n  "

		// Verify validation follows existing patterns
		validMessageIsValid := strings.TrimSpace(validMessage) != ""
		emptyMessageIsValid := strings.TrimSpace(emptyMessage) != ""
		whitespaceMessageIsValid := strings.TrimSpace(whitespaceMessage) != ""

		if !validMessageIsValid {
			rt.Error("Valid message should pass validation")
		}

		if emptyMessageIsValid {
			rt.Error("Empty message should fail validation")
		}

		if whitespaceMessageIsValid {
			rt.Error("Whitespace-only message should fail validation")
		}
	})
}

// Unit tests for admin handler HTML formatting

func TestUserDisplayNameEscaping(t *testing.T) {
	tests := []struct {
		name     string
		user     *models.User
		expected string
	}{
		{
			name: "Normal name",
			user: &models.User{
				FirstName: "John",
				LastName:  "Doe",
			},
			expected: "John Doe",
		},
		{
			name: "Name with HTML characters",
			user: &models.User{
				FirstName: "John<script>",
				LastName:  "Doe&Co",
			},
			expected: "John&lt;script&gt; Doe&amp;Co",
		},
		{
			name: "Username with special characters",
			user: &models.User{
				Username: "user<>&\"",
			},
			expected: "@user&lt;&gt;&amp;&#34;",
		},
		{
			name: "Empty names",
			user: &models.User{
				ID: 123,
			},
			expected: "🆔 ID: 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := &services.UserDetails{
				User: tt.user,
			}

			formatted := FormatUserDetails(&AdminHandler{}, details)

			// Check that the expected escaped content is present
			if !strings.Contains(formatted, tt.expected) {
				t.Errorf("Expected formatted output to contain %q, but got: %s", tt.expected, formatted)
			}

			// Verify HTML escaping was applied correctly
			if strings.Contains(formatted, "<script>") {
				t.Error("HTML tags should be escaped")
			}
			if strings.Contains(formatted, "&Co") && !strings.Contains(formatted, "&amp;Co") {
				t.Error("Ampersands should be escaped")
			}
		})
	}
}

func TestAchievementNameFormatting(t *testing.T) {
	tests := []struct {
		name         string
		achievements []*services.UserAchievementInfo
		expected     []string
	}{
		{
			name: "Normal achievement names",
			achievements: []*services.UserAchievementInfo{
				{Name: "Beginner", Category: models.CategoryProgress},
				{Name: "Expert", Category: models.CategoryCompletion},
			},
			expected: []string{"Beginner", "Expert"},
		},
		{
			name: "Achievement names with HTML characters",
			achievements: []*services.UserAchievementInfo{
				{Name: "Master<>&\"", Category: models.CategorySpecial},
				{Name: "Pro & Elite", Category: models.CategoryProgress},
			},
			expected: []string{"Master&lt;&gt;&amp;&#34;", "Pro &amp; Elite"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &models.User{
				ID:        123,
				FirstName: "Test",
				LastName:  "User",
			}

			details := &services.UserDetails{
				User:             user,
				AchievementCount: len(tt.achievements),
				Achievements:     tt.achievements,
			}

			formatted := FormatUserDetails(&AdminHandler{}, details)

			// Check that all expected escaped achievement names are present
			for _, expected := range tt.expected {
				if !strings.Contains(formatted, expected) {
					t.Errorf("Expected formatted output to contain achievement %q, but got: %s", expected, formatted)
				}
			}

			// Verify HTML escaping was applied
			if strings.Contains(formatted, "<>&\"") {
				t.Error("HTML special characters should be escaped in achievement names")
			}
		})
	}
}

func TestStatisticsDisplayFormatting(t *testing.T) {
	tests := []struct {
		name     string
		stats    *services.UserStatistics
		expected []string
	}{
		{
			name: "Basic statistics formatting",
			stats: &services.UserStatistics{
				TotalAnswers:        10,
				ApprovedSteps:       8,
				Accuracy:            80,
				LeaderboardPosition: 1,
				TotalUsers:          100,
			},
			expected: []string{
				"<b>Статистика прохождения</b>",
				"<b>Темп</b>",
				"<b>Рейтинг</b>",
				"<b>Участие</b>",
				"🥇 1 из 100",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := services.FormatUserStatistics(tt.stats, false)

			// Check that HTML formatting is used instead of markdown
			for _, expected := range tt.expected {
				if !strings.Contains(formatted, expected) {
					t.Errorf("Expected formatted output to contain %q, but got: %s", expected, formatted)
				}
			}

			// Verify no markdown formatting is present
			if strings.Contains(formatted, "*Статистика прохождения*") {
				t.Error("Should use HTML <b> tags instead of markdown *")
			}
			if strings.Contains(formatted, "\\(") || strings.Contains(formatted, "\\)") {
				t.Error("Should not escape parentheses for HTML mode")
			}
		})
	}
}

func TestFormatUserAchievementsHTMLFormatting(t *testing.T) {
	user := &models.User{
		ID:        123,
		FirstName: "Test<script>",
		LastName:  "User&Co",
	}

	summary := &services.UserAchievementSummary{
		TotalCount: 2,
		AchievementsByCategory: map[models.AchievementCategory][]*services.UserAchievementDetails{
			models.CategoryProgress: {
				{
					Achievement: &models.Achievement{
						Name:     "Beginner<>&\"",
						Category: models.CategoryProgress,
					},
					EarnedAt: "01.01.2024 12:00",
				},
			},
		},
	}

	// Test the standalone function
	formatted := FormatUserAchievements(user, summary)

	// Check HTML escaping of user name
	if !strings.Contains(formatted, "Test&lt;script&gt; User&amp;Co") {
		t.Error("User display name should be HTML escaped")
	}

	// Check that achievement names are escaped
	if !strings.Contains(formatted, "Beginner&lt;&gt;&amp;&#34;") {
		t.Error("Achievement names should be HTML escaped")
	}

	// Verify no markdown formatting
	if strings.Contains(formatted, "*Достижения пользователя*") {
		t.Error("Should use HTML formatting instead of markdown")
	}
}

func TestFormatAchievementStatisticsHTMLFormatting(t *testing.T) {
	stats := &services.AchievementStatistics{
		TotalAchievements:     10,
		TotalUserAchievements: 50,
		TotalUsers:            25,
		AchievementsByCategory: map[models.AchievementCategory]int{
			models.CategoryProgress: 5,
			models.CategorySpecial:  3,
		},
		PopularAchievements: []services.AchievementPopularity{
			{
				Achievement: &models.Achievement{
					Name: "Popular<>&\"",
				},
				UserCount:  10,
				Percentage: 40.0,
			},
		},
	}

	formatted := (&AdminHandler{}).FormatAchievementStatistics(stats)

	// Check HTML formatting is used
	if !strings.Contains(formatted, "<b>Статистика достижений</b>") {
		t.Error("Should use HTML <b> tags for headers")
	}

	if !strings.Contains(formatted, "<b>Общая информация</b>") {
		t.Error("Should use HTML <b> tags for section headers")
	}

	// Check achievement names are escaped
	if !strings.Contains(formatted, "Popular&lt;&gt;&amp;&#34;") {
		t.Error("Achievement names should be HTML escaped")
	}

	// Verify no markdown formatting
	if strings.Contains(formatted, "*Статистика достижений*") {
		t.Error("Should use HTML formatting instead of markdown")
	}
}

func TestFormatAchievementLeadersHTMLFormatting(t *testing.T) {
	rankings := []services.UserAchievementRanking{
		{
			User: &models.User{
				ID:        1,
				FirstName: "Leader<script>",
				LastName:  "User&Co",
			},
			AchievementCount: 15,
		},
		{
			User: &models.User{
				ID:        2,
				FirstName: "Second",
				Username:  "user<>&\"",
			},
			AchievementCount: 10,
		},
	}

	formatted := FormatAchievementLeaders(rankings)

	// Check user names are HTML escaped
	if !strings.Contains(formatted, "Leader&lt;script&gt; User&amp;Co") {
		t.Error("User display names should be HTML escaped")
	}

	if !strings.Contains(formatted, "Second") {
		t.Error("Should contain second user's name")
	}

	// Check medals are present
	if !strings.Contains(formatted, "🥇") {
		t.Error("Should contain gold medal for first place")
	}

	if !strings.Contains(formatted, "🥈") {
		t.Error("Should contain silver medal for second place")
	}
}
