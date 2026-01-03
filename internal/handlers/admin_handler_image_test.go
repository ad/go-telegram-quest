package handlers

import (
	"testing"

	"github.com/ad/go-telegram-quest/internal/fsm"
	"github.com/ad/go-telegram-quest/internal/models"
	tgmodels "github.com/go-telegram/bot/models"
)

func TestImageManagementStates(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		expected bool
	}{
		{"Add image state", fsm.StateAdminAddImage, true},
		{"Replace image state", fsm.StateAdminReplaceImage, true},
		{"Delete image state", fsm.StateAdminDeleteImage, true},
		{"Invalid state", "invalid_state", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &models.AdminState{
				CurrentState: tt.state,
			}

			isImageState := state.CurrentState == fsm.StateAdminAddImage ||
				state.CurrentState == fsm.StateAdminReplaceImage ||
				state.CurrentState == fsm.StateAdminDeleteImage

			if isImageState != tt.expected {
				t.Errorf("Expected %v, got %v for state %s", tt.expected, isImageState, tt.state)
			}
		})
	}
}

func TestAdminStateImageFields(t *testing.T) {
	state := &models.AdminState{
		UserID:           123,
		CurrentState:     fsm.StateAdminReplaceImage,
		EditingStepID:    1,
		ImagePosition:    2,
		ReplacingImageID: 456,
	}

	if state.ImagePosition != 2 {
		t.Errorf("Expected ImagePosition to be 2, got %d", state.ImagePosition)
	}

	if state.ReplacingImageID != 456 {
		t.Errorf("Expected ReplacingImageID to be 456, got %d", state.ReplacingImageID)
	}
}

func TestMessagePhotoValidation(t *testing.T) {
	tests := []struct {
		name     string
		msg      *tgmodels.Message
		hasPhoto bool
	}{
		{
			name: "Message with photo",
			msg: &tgmodels.Message{
				Photo: []tgmodels.PhotoSize{
					{FileID: "test_file_id"},
				},
			},
			hasPhoto: true,
		},
		{
			name: "Message without photo",
			msg: &tgmodels.Message{
				Photo: []tgmodels.PhotoSize{},
			},
			hasPhoto: false,
		},
		{
			name: "Message with nil photo",
			msg: &tgmodels.Message{
				Photo: nil,
			},
			hasPhoto: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasPhoto := len(tt.msg.Photo) > 0
			if hasPhoto != tt.hasPhoto {
				t.Errorf("Expected hasPhoto to be %v, got %v", tt.hasPhoto, hasPhoto)
			}
		})
	}
}
