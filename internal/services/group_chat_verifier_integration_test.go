package services

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	_ "modernc.org/sqlite"
)

func TestGroupChatVerifierIntegration_CompleteFlow(t *testing.T) {
	dbConn, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer dbConn.Close()

	if err := db.InitSchema(dbConn); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	queue := db.NewDBQueue(dbConn)
	defer queue.Close()

	settingsRepo := db.NewSettingsRepository(queue)

	t.Run("Verification disabled by default", func(t *testing.T) {
		verifier := NewGroupChatVerifier((*bot.Bot)(nil), settingsRepo)

		enabled, err := verifier.IsVerificationEnabled()
		if err != nil {
			t.Fatalf("IsVerificationEnabled failed: %v", err)
		}
		if enabled {
			t.Error("Verification should be disabled by default")
		}
	})

	t.Run("Enable verification flow", func(t *testing.T) {
		groupChatID := int64(-1001234567890)
		inviteLink := "https://t.me/+TestInviteLink"

		if err := settingsRepo.SetRequiredGroupChatID(groupChatID); err != nil {
			t.Fatalf("Failed to set group chat ID: %v", err)
		}
		if err := settingsRepo.SetGroupChatInviteLink(inviteLink); err != nil {
			t.Fatalf("Failed to set invite link: %v", err)
		}

		verifier := NewGroupChatVerifier((*bot.Bot)(nil), settingsRepo)

		enabled, err := verifier.IsVerificationEnabled()
		if err != nil {
			t.Fatalf("IsVerificationEnabled failed: %v", err)
		}
		if !enabled {
			t.Error("Verification should be enabled after configuration")
		}

		retrievedID, err := settingsRepo.GetRequiredGroupChatID()
		if err != nil {
			t.Fatalf("Failed to get group chat ID: %v", err)
		}
		if retrievedID != groupChatID {
			t.Errorf("Expected group chat ID %d, got %d", groupChatID, retrievedID)
		}

		retrievedLink, err := settingsRepo.GetGroupChatInviteLink()
		if err != nil {
			t.Fatalf("Failed to get invite link: %v", err)
		}
		if retrievedLink != inviteLink {
			t.Errorf("Expected invite link %s, got %s", inviteLink, retrievedLink)
		}
	})

	t.Run("Edit group chat ID", func(t *testing.T) {
		newGroupChatID := int64(-1009876543210)

		if err := settingsRepo.SetRequiredGroupChatID(newGroupChatID); err != nil {
			t.Fatalf("Failed to update group chat ID: %v", err)
		}

		retrievedID, err := settingsRepo.GetRequiredGroupChatID()
		if err != nil {
			t.Fatalf("Failed to get updated group chat ID: %v", err)
		}
		if retrievedID != newGroupChatID {
			t.Errorf("Expected updated group chat ID %d, got %d", newGroupChatID, retrievedID)
		}
	})

	t.Run("Edit invite link", func(t *testing.T) {
		newInviteLink := "https://t.me/+NewTestInviteLink"

		if err := settingsRepo.SetGroupChatInviteLink(newInviteLink); err != nil {
			t.Fatalf("Failed to update invite link: %v", err)
		}

		retrievedLink, err := settingsRepo.GetGroupChatInviteLink()
		if err != nil {
			t.Fatalf("Failed to get updated invite link: %v", err)
		}
		if retrievedLink != newInviteLink {
			t.Errorf("Expected updated invite link %s, got %s", newInviteLink, retrievedLink)
		}
	})

	t.Run("Disable verification flow", func(t *testing.T) {
		if err := settingsRepo.SetRequiredGroupChatID(0); err != nil {
			t.Fatalf("Failed to disable verification: %v", err)
		}
		if err := settingsRepo.SetGroupChatInviteLink(""); err != nil {
			t.Fatalf("Failed to clear invite link: %v", err)
		}

		verifier := NewGroupChatVerifier((*bot.Bot)(nil), settingsRepo)

		enabled, err := verifier.IsVerificationEnabled()
		if err != nil {
			t.Fatalf("IsVerificationEnabled failed: %v", err)
		}
		if enabled {
			t.Error("Verification should be disabled after setting ID to 0")
		}
	})

	t.Run("Member status validation", func(t *testing.T) {
		if err := settingsRepo.SetRequiredGroupChatID(-1001234567890); err != nil {
			t.Fatalf("Failed to enable verification: %v", err)
		}

		verifier := NewGroupChatVerifier((*bot.Bot)(nil), settingsRepo)

		testCases := []struct {
			name   string
			status tgmodels.ChatMemberType
		}{
			{"Owner", tgmodels.ChatMemberTypeOwner},
			{"Administrator", tgmodels.ChatMemberTypeAdministrator},
			{"Member", tgmodels.ChatMemberTypeMember},
			{"Left", tgmodels.ChatMemberTypeLeft},
			{"Banned", tgmodels.ChatMemberTypeBanned},
			{"Restricted", tgmodels.ChatMemberTypeRestricted},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				isValid := verifier.isValidMemberStatus(tc.status)
				expectedValid := tc.status == tgmodels.ChatMemberTypeOwner ||
					tc.status == tgmodels.ChatMemberTypeAdministrator ||
					tc.status == tgmodels.ChatMemberTypeMember

				if isValid != expectedValid {
					t.Errorf("Status %s: expected %v, got %v", tc.name, expectedValid, isValid)
				}
			})
		}
	})
}

func TestGroupChatVerifierIntegration_Settings(t *testing.T) {
	dbConn, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer dbConn.Close()

	if err := db.InitSchema(dbConn); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	queue := db.NewDBQueue(dbConn)
	defer queue.Close()

	settingsRepo := db.NewSettingsRepository(queue)

	t.Run("GetAll includes group chat fields", func(t *testing.T) {
		groupChatID := int64(-1001234567890)
		inviteLink := "https://t.me/+TestLink"

		if err := settingsRepo.SetRequiredGroupChatID(groupChatID); err != nil {
			t.Fatalf("Failed to set group chat ID: %v", err)
		}
		if err := settingsRepo.SetGroupChatInviteLink(inviteLink); err != nil {
			t.Fatalf("Failed to set invite link: %v", err)
		}

		settings, err := settingsRepo.GetAll()
		if err != nil {
			t.Fatalf("GetAll failed: %v", err)
		}

		if settings.RequiredGroupChatID != groupChatID {
			t.Errorf("Expected RequiredGroupChatID %d, got %d", groupChatID, settings.RequiredGroupChatID)
		}
		if settings.GroupChatInviteLink != inviteLink {
			t.Errorf("Expected GroupChatInviteLink %s, got %s", inviteLink, settings.GroupChatInviteLink)
		}
	})

	t.Run("Settings persistence", func(t *testing.T) {
		groupChatID := int64(-1009999999999)
		inviteLink := "https://t.me/+PersistenceTest"

		if err := settingsRepo.SetRequiredGroupChatID(groupChatID); err != nil {
			t.Fatalf("Failed to set group chat ID: %v", err)
		}
		if err := settingsRepo.SetGroupChatInviteLink(inviteLink); err != nil {
			t.Fatalf("Failed to set invite link: %v", err)
		}

		newSettingsRepo := db.NewSettingsRepository(queue)

		retrievedID, err := newSettingsRepo.GetRequiredGroupChatID()
		if err != nil {
			t.Fatalf("Failed to get group chat ID: %v", err)
		}
		if retrievedID != groupChatID {
			t.Errorf("Expected persisted group chat ID %d, got %d", groupChatID, retrievedID)
		}

		retrievedLink, err := newSettingsRepo.GetGroupChatInviteLink()
		if err != nil {
			t.Fatalf("Failed to get invite link: %v", err)
		}
		if retrievedLink != inviteLink {
			t.Errorf("Expected persisted invite link %s, got %s", inviteLink, retrievedLink)
		}
	})
}

func TestGroupChatVerifierIntegration_ValidationScenarios(t *testing.T) {
	dbConn, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer dbConn.Close()

	if err := db.InitSchema(dbConn); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	queue := db.NewDBQueue(dbConn)
	defer queue.Close()

	settingsRepo := db.NewSettingsRepository(queue)

	t.Run("Valid negative group chat ID", func(t *testing.T) {
		validID := int64(-1001234567890)
		if err := settingsRepo.SetRequiredGroupChatID(validID); err != nil {
			t.Fatalf("Failed to set valid negative group chat ID: %v", err)
		}

		retrievedID, err := settingsRepo.GetRequiredGroupChatID()
		if err != nil {
			t.Fatalf("Failed to get group chat ID: %v", err)
		}
		if retrievedID != validID {
			t.Errorf("Expected %d, got %d", validID, retrievedID)
		}
	})

	t.Run("Valid invite link format", func(t *testing.T) {
		validLink := "https://t.me/+AbCdEfGhIjKlMnOp"
		if err := settingsRepo.SetGroupChatInviteLink(validLink); err != nil {
			t.Fatalf("Failed to set valid invite link: %v", err)
		}

		retrievedLink, err := settingsRepo.GetGroupChatInviteLink()
		if err != nil {
			t.Fatalf("Failed to get invite link: %v", err)
		}
		if retrievedLink != validLink {
			t.Errorf("Expected %s, got %s", validLink, retrievedLink)
		}
	})

	t.Run("Empty invite link", func(t *testing.T) {
		if err := settingsRepo.SetGroupChatInviteLink(""); err != nil {
			t.Fatalf("Failed to set empty invite link: %v", err)
		}

		retrievedLink, err := settingsRepo.GetGroupChatInviteLink()
		if err != nil {
			t.Fatalf("Failed to get invite link: %v", err)
		}
		if retrievedLink != "" {
			t.Errorf("Expected empty string, got %s", retrievedLink)
		}
	})

	t.Run("Zero group chat ID disables verification", func(t *testing.T) {
		if err := settingsRepo.SetRequiredGroupChatID(0); err != nil {
			t.Fatalf("Failed to set zero group chat ID: %v", err)
		}

		verifier := NewGroupChatVerifier((*bot.Bot)(nil), settingsRepo)

		enabled, err := verifier.IsVerificationEnabled()
		if err != nil {
			t.Fatalf("IsVerificationEnabled failed: %v", err)
		}
		if enabled {
			t.Error("Verification should be disabled when group chat ID is 0")
		}
	})
}
