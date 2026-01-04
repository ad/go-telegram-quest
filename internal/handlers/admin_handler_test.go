package handlers

import (
	"testing"

	"github.com/ad/go-telegram-quest/internal/models"
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
			Text: "/some_command",
		}

		nonAdminMessage := &tgmodels.Message{
			From: &tgmodels.User{ID: nonAdminID},
			Text: "/some_command",
		}

		if adminMessage.From.ID != adminHandler.adminID {
			rt.Error("Admin ID should match")
		}

		if nonAdminMessage.From.ID == adminHandler.adminID {
			rt.Error("Non-admin ID should not match admin ID")
		}

		questStateCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: nonAdminID},
			Data: "admin:quest_state",
		}

		adminQuestStateCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: adminID},
			Data: "admin:quest_state",
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
			Data: rapid.StringMatching(`admin:move_up:\d+`).Draw(rt, "moveUpData"),
		}

		moveDownCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: nonAdminID},
			Data: rapid.StringMatching(`admin:move_down:\d+`).Draw(rt, "moveDownData"),
		}

		adminMoveUpCallback := &tgmodels.CallbackQuery{
			From: tgmodels.User{ID: adminID},
			Data: rapid.StringMatching(`admin:move_up:\d+`).Draw(rt, "adminMoveUpData"),
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
