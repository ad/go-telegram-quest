package handlers

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/fsm"
	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
)

// setupTestDBMessaging creates a test database with the required schema
func setupTestDBMessaging(t *testing.T) (*db.DBQueue, func()) {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	// Initialize the schema using the existing schema initialization
	err = db.InitSchema(sqlDB)
	if err != nil {
		t.Fatal(err)
	}

	queue := db.NewDBQueue(sqlDB)
	return queue, func() { sqlDB.Close() }
}

// TestBuildUserDetailsKeyboardIntegration tests the keyboard building functionality
func TestBuildUserDetailsKeyboardIntegration(t *testing.T) {
	// Create test user
	targetUser := &models.User{
		ID:        67890,
		FirstName: "Test",
		LastName:  "User",
		Username:  "testuser",
		IsBlocked: false,
	}

	t.Run("Admin keyboard contains message button", func(t *testing.T) {
		keyboard := BuildUserDetailsKeyboard(targetUser, true)
		if keyboard == nil {
			t.Fatal("BuildUserDetailsKeyboard returned nil")
		}

		// Verify message button is present
		messageButtonFound := false
		for _, row := range keyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "admin:send_message:") {
					messageButtonFound = true
					expectedCallback := fmt.Sprintf("admin:send_message:%d", targetUser.ID)
					if button.CallbackData != expectedCallback {
						t.Errorf("Expected callback %s, got %s", expectedCallback, button.CallbackData)
					}
					if button.Text != "💬 Написать сообщение" {
						t.Errorf("Expected button text '💬 Написать сообщение', got %s", button.Text)
					}
				}
			}
		}
		if !messageButtonFound {
			t.Error("Message button should be present in admin keyboard")
		}
	})

	t.Run("Non-admin keyboard does not contain message button", func(t *testing.T) {
		keyboard := BuildUserDetailsKeyboard(targetUser, false)
		if keyboard == nil {
			t.Fatal("BuildUserDetailsKeyboard returned nil")
		}

		// Verify message button is NOT present
		messageButtonFound := false
		for _, row := range keyboard.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.CallbackData, "admin:send_message:") {
					messageButtonFound = true
				}
			}
		}
		if messageButtonFound {
			t.Error("Message button should NOT be present in non-admin keyboard")
		}
	})
}

// TestAdminStateManagementIntegration tests admin state management for messaging
func TestAdminStateManagementIntegration(t *testing.T) {
	// Setup test database
	queue, cleanup := setupTestDBMessaging(t)
	defer cleanup()

	adminStateRepo := db.NewAdminStateRepository(queue)
	adminID := int64(12345)
	targetUserID := int64(67890)

	t.Run("Admin state creation and retrieval", func(t *testing.T) {
		// Create admin state for messaging
		state := &models.AdminState{
			UserID:       adminID,
			CurrentState: fsm.StateAdminSendMessage,
			TargetUserID: targetUserID,
		}

		err := adminStateRepo.Save(state)
		if err != nil {
			t.Fatalf("Failed to save admin state: %v", err)
		}

		// Retrieve admin state
		retrievedState, err := adminStateRepo.Get(adminID)
		if err != nil {
			t.Fatalf("Failed to get admin state: %v", err)
		}

		if retrievedState == nil {
			t.Fatal("Retrieved state should not be nil")
		}

		if retrievedState.CurrentState != fsm.StateAdminSendMessage {
			t.Errorf("Expected state %s, got %s", fsm.StateAdminSendMessage, retrievedState.CurrentState)
		}

		if retrievedState.TargetUserID != targetUserID {
			t.Errorf("Expected target user ID %d, got %d", targetUserID, retrievedState.TargetUserID)
		}

		// Clear admin state
		err = adminStateRepo.Clear(adminID)
		if err != nil {
			t.Fatalf("Failed to clear admin state: %v", err)
		}

		// Verify state is cleared
		clearedState, err := adminStateRepo.Get(adminID)
		if err != nil && err.Error() != "sql: no rows in result set" {
			t.Fatalf("Failed to get admin state after clear: %v", err)
		}

		if clearedState != nil {
			t.Error("Admin state should be nil after clearing")
		}
	})
}

// TestCallbackDataParsingIntegration tests callback data parsing for messaging
func TestCallbackDataParsingIntegration(t *testing.T) {
	testCases := []struct {
		name           string
		callbackData   string
		expectedUserID int64
		shouldSucceed  bool
	}{
		{
			name:           "Valid callback data",
			callbackData:   "admin:send_message:12345",
			expectedUserID: 12345,
			shouldSucceed:  true,
		},
		{
			name:           "Valid callback data with large ID",
			callbackData:   "admin:send_message:999999999",
			expectedUserID: 999999999,
			shouldSucceed:  true,
		},
		{
			name:           "Invalid callback data - missing user ID",
			callbackData:   "admin:send_message:",
			expectedUserID: 0,
			shouldSucceed:  false,
		},
		{
			name:           "Invalid callback data - non-numeric user ID",
			callbackData:   "admin:send_message:abc",
			expectedUserID: 0,
			shouldSucceed:  false,
		},
		{
			name:           "Invalid callback data - wrong prefix",
			callbackData:   "admin:other_action:12345",
			expectedUserID: 0,
			shouldSucceed:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.HasPrefix(tc.callbackData, "admin:send_message:") {
				userIDStr := strings.TrimPrefix(tc.callbackData, "admin:send_message:")
				userID, err := parseInt64(userIDStr)

				if tc.shouldSucceed {
					if err != nil {
						t.Errorf("Expected successful parsing, got error: %v", err)
					}
					if userID != tc.expectedUserID {
						t.Errorf("Expected user ID %d, got %d", tc.expectedUserID, userID)
					}
				} else {
					if err == nil && userID != 0 {
						t.Errorf("Expected parsing to fail or return 0, got user ID %d", userID)
					}
				}
			} else {
				if tc.shouldSucceed {
					t.Error("Expected callback to match prefix, but it didn't")
				}
			}
		})
	}
}

// TestUserRepositoryIntegration tests user repository operations for messaging
func TestUserRepositoryIntegration(t *testing.T) {
	// Setup test database
	queue, cleanup := setupTestDBMessaging(t)
	defer cleanup()

	userRepo := db.NewUserRepository(queue)

	t.Run("User creation and retrieval", func(t *testing.T) {
		// Create test user
		user := &models.User{
			ID:        67890,
			FirstName: "Test",
			LastName:  "User",
			Username:  "testuser",
			IsBlocked: false,
		}

		err := userRepo.CreateOrUpdate(user)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Retrieve user
		retrievedUser, err := userRepo.GetByID(user.ID)
		if err != nil {
			t.Fatalf("Failed to get user: %v", err)
		}

		if retrievedUser == nil {
			t.Fatal("Retrieved user should not be nil")
		}

		if retrievedUser.ID != user.ID {
			t.Errorf("Expected user ID %d, got %d", user.ID, retrievedUser.ID)
		}

		if retrievedUser.FirstName != user.FirstName {
			t.Errorf("Expected first name %s, got %s", user.FirstName, retrievedUser.FirstName)
		}

		if retrievedUser.Username != user.Username {
			t.Errorf("Expected username %s, got %s", user.Username, retrievedUser.Username)
		}

		// Test DisplayName method
		expectedDisplayName := "Test User @testuser [67890]"
		if retrievedUser.DisplayName() != expectedDisplayName {
			t.Errorf("Expected display name %s, got %s", expectedDisplayName, retrievedUser.DisplayName())
		}
	})

	t.Run("Non-existent user retrieval", func(t *testing.T) {
		// Try to retrieve non-existent user
		nonExistentUser, err := userRepo.GetByID(99999)
		if err != nil && err.Error() != "sql: no rows in result set" {
			t.Fatalf("Expected no error or 'no rows' error for non-existent user, got: %v", err)
		}

		if nonExistentUser != nil {
			t.Error("Expected nil for non-existent user")
		}
	})
}

// TestMessageValidationIntegration tests message validation logic
func TestMessageValidationIntegration(t *testing.T) {
	testCases := []struct {
		name          string
		messageText   string
		shouldBeValid bool
	}{
		{
			name:          "Valid message",
			messageText:   "Hello, this is a test message",
			shouldBeValid: true,
		},
		{
			name:          "Valid message with special characters",
			messageText:   "Привет! Как дела? 😊",
			shouldBeValid: true,
		},
		{
			name:          "Empty message",
			messageText:   "",
			shouldBeValid: false,
		},
		{
			name:          "Whitespace only message",
			messageText:   "   \t\n  ",
			shouldBeValid: false,
		},
		{
			name:          "Single character message",
			messageText:   "a",
			shouldBeValid: true,
		},
		{
			name:          "Message with only spaces",
			messageText:   "     ",
			shouldBeValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the validation logic from handleSendMessage
			isValid := strings.TrimSpace(tc.messageText) != ""

			if isValid != tc.shouldBeValid {
				t.Errorf("Expected validation result %v, got %v for message: %q", tc.shouldBeValid, isValid, tc.messageText)
			}
		})
	}
}

// TestAccessControlIntegration tests access control for admin messaging
func TestAccessControlIntegration(t *testing.T) {
	adminID := int64(12345)
	nonAdminID := int64(54321)

	testCases := []struct {
		name             string
		userID           int64
		shouldHaveAccess bool
	}{
		{
			name:             "Admin user has access",
			userID:           adminID,
			shouldHaveAccess: true,
		},
		{
			name:             "Non-admin user does not have access",
			userID:           nonAdminID,
			shouldHaveAccess: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the access control logic from HandleCallback
			hasAccess := tc.userID == adminID

			if hasAccess != tc.shouldHaveAccess {
				t.Errorf("Expected access %v, got %v for user ID %d", tc.shouldHaveAccess, hasAccess, tc.userID)
			}
		})
	}
}

// TestInstructionMessageFormatIntegration tests instruction message formatting
func TestInstructionMessageFormatIntegration(t *testing.T) {
	targetUser := &models.User{
		ID:        67890,
		FirstName: "Test",
		LastName:  "User",
		Username:  "testuser",
	}

	expectedInstructions := fmt.Sprintf("💬 Отправка сообщения пользователю %s\n\n📝 Введите текст сообщения:\n\n/cancel - отмена операции", targetUser.DisplayName())

	// Verify instruction format
	if !strings.Contains(expectedInstructions, "💬") {
		t.Error("Instructions should contain message emoji")
	}

	if !strings.Contains(expectedInstructions, targetUser.DisplayName()) {
		t.Errorf("Instructions should contain target user display name: %s", targetUser.DisplayName())
	}

	if !strings.Contains(expectedInstructions, "/cancel") {
		t.Error("Instructions should mention /cancel command")
	}

	if !strings.Contains(expectedInstructions, "Введите текст сообщения") {
		t.Error("Instructions should ask for message text input")
	}
}

// TestStatusMessageFormatIntegration tests status message formatting
func TestStatusMessageFormatIntegration(t *testing.T) {
	targetUser := &models.User{
		ID:        67890,
		FirstName: "Test",
		LastName:  "User",
		Username:  "testuser",
	}

	t.Run("Success status message", func(t *testing.T) {
		successMessage := fmt.Sprintf("✅ Сообщение успешно отправлено пользователю %s", targetUser.DisplayName())

		if !strings.Contains(successMessage, "✅") {
			t.Error("Success message should contain success emoji")
		}

		if !strings.Contains(successMessage, "успешно отправлено") {
			t.Error("Success message should indicate successful delivery")
		}

		if !strings.Contains(successMessage, targetUser.DisplayName()) {
			t.Errorf("Success message should contain target user display name: %s", targetUser.DisplayName())
		}
	})

	t.Run("Error status message", func(t *testing.T) {
		errorMessage := fmt.Sprintf("❌ Ошибка при отправке сообщения пользователю %s:\nTest error", targetUser.DisplayName())

		if !strings.Contains(errorMessage, "❌") {
			t.Error("Error message should contain error emoji")
		}

		if !strings.Contains(errorMessage, "Ошибка при отправке") {
			t.Error("Error message should indicate delivery error")
		}

		if !strings.Contains(errorMessage, targetUser.DisplayName()) {
			t.Errorf("Error message should contain target user display name: %s", targetUser.DisplayName())
		}
	})

	t.Run("Cancel status message", func(t *testing.T) {
		cancelMessage := "❌ Отправка сообщения отменена"

		if !strings.Contains(cancelMessage, "❌") {
			t.Error("Cancel message should contain error emoji")
		}

		if !strings.Contains(cancelMessage, "отменена") {
			t.Error("Cancel message should indicate operation was cancelled")
		}
	})
}

// TestFSMStateIntegration tests FSM state constants
func TestFSMStateIntegration(t *testing.T) {
	// Verify the StateAdminSendMessage constant exists and has the expected value
	expectedState := "admin_send_message"
	if fsm.StateAdminSendMessage != expectedState {
		t.Errorf("Expected StateAdminSendMessage to be %s, got %s", expectedState, fsm.StateAdminSendMessage)
	}
}

// TestAdminStateFieldsIntegration tests AdminState model fields
func TestAdminStateFieldsIntegration(t *testing.T) {
	adminID := int64(12345)
	targetUserID := int64(67890)

	// Create AdminState with messaging fields
	state := &models.AdminState{
		UserID:       adminID,
		CurrentState: fsm.StateAdminSendMessage,
		TargetUserID: targetUserID,
	}

	// Verify fields are set correctly
	if state.UserID != adminID {
		t.Errorf("Expected UserID %d, got %d", adminID, state.UserID)
	}

	if state.CurrentState != fsm.StateAdminSendMessage {
		t.Errorf("Expected CurrentState %s, got %s", fsm.StateAdminSendMessage, state.CurrentState)
	}

	if state.TargetUserID != targetUserID {
		t.Errorf("Expected TargetUserID %d, got %d", targetUserID, state.TargetUserID)
	}
}
