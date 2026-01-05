package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"pgregory.net/rapid"
)

type retryTracker struct {
	attempts     int32
	failUntil    int32
	lastErr      error
	successMsgID int
}

func (r *retryTracker) send(_ context.Context, _ *bot.SendMessageParams) (*tgmodels.Message, error) {
	attempt := atomic.AddInt32(&r.attempts, 1)
	if attempt <= r.failUntil {
		return nil, r.lastErr
	}
	return &tgmodels.Message{ID: r.successMsgID}, nil
}

func TestProperty14_MessageSendRetry(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		failCount := rapid.IntRange(0, 3).Draw(rt, "failCount")
		maxRetry := 2

		tracker := &retryTracker{
			failUntil:    int32(failCount),
			lastErr:      errors.New("network error"),
			successMsgID: rapid.IntRange(1, 10000).Draw(rt, "msgID"),
		}

		ctx := context.Background()
		params := &bot.SendMessageParams{
			ChatID: int64(rapid.IntRange(1, 1000000).Draw(rt, "chatID")),
			Text:   rapid.StringMatching(`[a-zA-Z ]{1,100}`).Draw(rt, "text"),
		}

		var result *tgmodels.Message
		var lastErr error
		for attempt := 0; attempt < maxRetry; attempt++ {
			msg, err := tracker.send(ctx, params)
			if err == nil {
				result = msg
				break
			}
			lastErr = err
		}

		actualAttempts := int(atomic.LoadInt32(&tracker.attempts))

		if failCount < maxRetry {
			if result == nil {
				rt.Errorf("Expected success after %d failures with %d max retries, but got nil result", failCount, maxRetry)
			}
			if result != nil && result.ID != tracker.successMsgID {
				rt.Errorf("Expected message ID %d, got %d", tracker.successMsgID, result.ID)
			}
			expectedAttempts := failCount + 1
			if actualAttempts != expectedAttempts {
				rt.Errorf("Expected %d attempts, got %d", expectedAttempts, actualAttempts)
			}
		} else {
			if result != nil {
				rt.Errorf("Expected failure after %d failures with %d max retries, but got success", failCount, maxRetry)
			}
			if lastErr == nil {
				rt.Errorf("Expected error to be set after all retries failed")
			}
			if actualAttempts != maxRetry {
				rt.Errorf("Expected exactly %d attempts, got %d", maxRetry, actualAttempts)
			}
		}
	})
}

type mockChatStateRepo struct {
	states map[int64]*models.ChatState
}

func (m *mockChatStateRepo) Get(userID int64) (*models.ChatState, error) {
	if state, exists := m.states[userID]; exists {
		return state, nil
	}
	return &models.ChatState{UserID: userID}, nil
}

func (m *mockChatStateRepo) Save(state *models.ChatState) error {
	m.states[state.UserID] = state
	return nil
}

func (m *mockChatStateRepo) Clear(userID int64) error {
	delete(m.states, userID)
	return nil
}

func (m *mockChatStateRepo) UpdateTaskMessageID(userID int64, messageID int) error {
	if state, exists := m.states[userID]; exists {
		state.LastTaskMessageID = messageID
	} else {
		m.states[userID] = &models.ChatState{UserID: userID, LastTaskMessageID: messageID}
	}
	return nil
}

func (m *mockChatStateRepo) UpdateAnswerMessageID(userID int64, messageID int) error {
	if state, exists := m.states[userID]; exists {
		state.LastUserAnswerMessageID = messageID
	} else {
		m.states[userID] = &models.ChatState{UserID: userID, LastUserAnswerMessageID: messageID}
	}
	return nil
}

func (m *mockChatStateRepo) UpdateReactionMessageID(userID int64, messageID int) error {
	if state, exists := m.states[userID]; exists {
		state.LastReactionMessageID = messageID
	} else {
		m.states[userID] = &models.ChatState{UserID: userID, LastReactionMessageID: messageID}
	}
	return nil
}

func (m *mockChatStateRepo) UpdateHintMessageID(userID int64, messageID int) error {
	if state, exists := m.states[userID]; exists {
		state.HintMessageID = messageID
	} else {
		m.states[userID] = &models.ChatState{UserID: userID, HintMessageID: messageID}
	}
	return nil
}

func (m *mockChatStateRepo) SetHintUsed(userID int64, used bool) error {
	if state, exists := m.states[userID]; exists {
		state.CurrentStepHintUsed = used
	} else {
		m.states[userID] = &models.ChatState{UserID: userID, CurrentStepHintUsed: used}
	}
	return nil
}

func (m *mockChatStateRepo) ResetHintUsed(userID int64) error {
	if state, exists := m.states[userID]; exists {
		state.CurrentStepHintUsed = false
		state.HintMessageID = 0
	} else {
		m.states[userID] = &models.ChatState{UserID: userID}
	}
	return nil
}

func TestProperty4_HintMessageCleanup(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := int64(rapid.IntRange(1, 1000000).Draw(rt, "userID"))
		hintMessageID := rapid.IntRange(1, 10000).Draw(rt, "hintMessageID")

		mockRepo := &mockChatStateRepo{states: make(map[int64]*models.ChatState)}

		mockRepo.UpdateHintMessageID(userID, hintMessageID)

		stateBefore, _ := mockRepo.Get(userID)
		if stateBefore.HintMessageID != hintMessageID {
			rt.Errorf("Setup failed: expected hint_message_id %d, got %d", hintMessageID, stateBefore.HintMessageID)
		}

		state, err := mockRepo.Get(userID)
		if err != nil || state == nil || state.HintMessageID == 0 {
			return
		}

		mockRepo.UpdateHintMessageID(userID, 0)

		stateAfter, _ := mockRepo.Get(userID)
		if stateAfter.HintMessageID != 0 {
			rt.Errorf("Expected hint_message_id to be cleared (0), got: %d", stateAfter.HintMessageID)
		}
	})
}
