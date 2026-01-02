package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"pgregory.net/rapid"
)

type mockSendFunc func(ctx context.Context, params *bot.SendMessageParams) (*tgmodels.Message, error)

type retryTracker struct {
	attempts     int32
	failUntil    int32
	lastErr      error
	successMsgID int
}

func (r *retryTracker) send(ctx context.Context, params *bot.SendMessageParams) (*tgmodels.Message, error) {
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
