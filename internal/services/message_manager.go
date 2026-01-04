package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type MessageManager struct {
	bot           *bot.Bot
	chatStateRepo *db.ChatStateRepository
	errMgr        *ErrorManager
	maxRetry      int
}

func NewMessageManager(b *bot.Bot, chatStateRepo *db.ChatStateRepository, errMgr *ErrorManager) *MessageManager {
	return &MessageManager{
		bot:           b,
		chatStateRepo: chatStateRepo,
		errMgr:        errMgr,
		maxRetry:      2,
	}
}

func (m *MessageManager) SendWithRetry(ctx context.Context, params *bot.SendMessageParams) (*tgmodels.Message, error) {
	var lastErr error
	for attempt := 0; attempt < m.maxRetry; attempt++ {
		msg, err := m.bot.SendMessage(ctx, params)
		if err == nil {
			return msg, nil
		}
		lastErr = err
	}
	m.errMgr.NotifyAdminWithCurl(ctx, params.ChatID.(int64), params, lastErr)
	return nil, lastErr
}

func (m *MessageManager) SendWithRetryAndEffect(ctx context.Context, params *bot.SendMessageParams, effectID string) (*tgmodels.Message, error) {
	if effectID != "" {
		params.MessageEffectID = effectID
	}
	return m.SendWithRetry(ctx, params)
}

func (m *MessageManager) SendPhotoWithRetry(ctx context.Context, params *bot.SendPhotoParams) (*tgmodels.Message, error) {
	var lastErr error
	for attempt := 0; attempt < m.maxRetry; attempt++ {
		msg, err := m.bot.SendPhoto(ctx, params)
		if err == nil {
			return msg, nil
		}
		lastErr = err
	}
	chatID, _ := params.ChatID.(int64)
	m.errMgr.NotifyAdminWithCurl(ctx, chatID, params, lastErr)
	return nil, lastErr
}

func (m *MessageManager) SendMediaGroupWithRetry(ctx context.Context, params *bot.SendMediaGroupParams) ([]*tgmodels.Message, error) {
	var lastErr error
	for attempt := 0; attempt < m.maxRetry; attempt++ {
		msgs, err := m.bot.SendMediaGroup(ctx, params)
		if err == nil {
			return msgs, nil
		}
		lastErr = err
	}
	chatID, _ := params.ChatID.(int64)
	m.errMgr.NotifyAdminWithCurl(ctx, chatID, params, lastErr)
	return nil, lastErr
}

func (m *MessageManager) SendTask(ctx context.Context, userID int64, step *models.Step) error {
	return m.SendTaskWithHintButton(ctx, userID, step, false)
}

func (m *MessageManager) SendTaskWithHintButton(ctx context.Context, userID int64, step *models.Step, showHintButton bool) error {
	if err := m.DeletePreviousMessages(ctx, userID); err != nil {
		return err
	}

	var keyboard *tgmodels.InlineKeyboardMarkup
	if showHintButton && step.HasHint() {
		keyboard = &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
				{{
					Text:         "💡 Подсказка",
					CallbackData: fmt.Sprintf("hint:%d:%d", userID, step.ID),
				}},
			},
		}
	}

	var taskMsgID int

	if len(step.Images) == 0 {
		msg, err := m.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID:      userID,
			Text:        step.Text,
			ReplyMarkup: keyboard,
		})
		if err != nil {
			return err
		}
		taskMsgID = msg.ID
	} else if len(step.Images) == 1 {
		msg, err := m.SendPhotoWithRetry(ctx, &bot.SendPhotoParams{
			ChatID:      userID,
			Photo:       &tgmodels.InputFileString{Data: step.Images[0].FileID},
			Caption:     step.Text,
			ReplyMarkup: keyboard,
		})
		if err != nil {
			return err
		}
		taskMsgID = msg.ID
	} else {
		media := make([]tgmodels.InputMedia, len(step.Images))
		for i, img := range step.Images {
			photo := &tgmodels.InputMediaPhoto{
				Media: img.FileID,
			}
			if i == 0 {
				photo.Caption = step.Text
			}
			media[i] = photo
		}
		msgs, err := m.SendMediaGroupWithRetry(ctx, &bot.SendMediaGroupParams{
			ChatID: userID,
			Media:  media,
		})
		if err != nil {
			return err
		}
		if len(msgs) > 0 {
			taskMsgID = msgs[0].ID
		}

		// Send hint button as separate message for media groups
		if keyboard != nil {
			m.SendWithRetry(ctx, &bot.SendMessageParams{
				ChatID:      userID,
				Text:        "👆 Ваше задание выше",
				ReplyMarkup: keyboard,
			})
		}
	}

	return m.chatStateRepo.UpdateTaskMessageID(userID, taskMsgID)
}

func (m *MessageManager) SendReaction(ctx context.Context, userID int64, text string) error {
	return m.SendReactionWithEffect(ctx, userID, text, "")
}

func (m *MessageManager) SendReactionWithEffect(ctx context.Context, userID int64, text string, effectID string) error {
	state, _ := m.chatStateRepo.Get(userID)
	if state != nil && state.LastReactionMessageID != 0 {
		_ = m.DeleteMessage(ctx, userID, state.LastReactionMessageID)
	}

	params := &bot.SendMessageParams{
		ChatID: userID,
		Text:   text,
	}
	if effectID != "" {
		params.MessageEffectID = effectID
	}

	msg, err := m.SendWithRetry(ctx, params)
	if err != nil {
		return err
	}

	return m.chatStateRepo.UpdateReactionMessageID(userID, msg.ID)
}

func (m *MessageManager) DeletePreviousMessages(ctx context.Context, userID int64) error {
	state, err := m.chatStateRepo.Get(userID)
	if err != nil {
		return nil
	}

	if state.LastTaskMessageID != 0 {
		_ = m.DeleteMessage(ctx, userID, state.LastTaskMessageID)
	}
	if state.LastUserAnswerMessageID != 0 {
		_ = m.DeleteMessage(ctx, userID, state.LastUserAnswerMessageID)
	}
	if state.LastReactionMessageID != 0 {
		_ = m.DeleteMessage(ctx, userID, state.LastReactionMessageID)
	}

	m.CleanupHintMessage(ctx, userID)

	return m.chatStateRepo.Clear(userID)
}

func (m *MessageManager) CleanupHintMessage(ctx context.Context, userID int64) error {
	state, err := m.chatStateRepo.Get(userID)
	if err != nil || state == nil || state.HintMessageID == 0 {
		return nil
	}

	_ = m.DeleteMessage(ctx, userID, state.HintMessageID)

	return m.chatStateRepo.UpdateHintMessageID(userID, 0)
}

func (m *MessageManager) DeleteUserAnswerAndReaction(ctx context.Context, userID int64) error {
	state, err := m.chatStateRepo.Get(userID)
	if err != nil {
		return nil
	}

	if state.LastUserAnswerMessageID != 0 {
		_ = m.DeleteMessage(ctx, userID, state.LastUserAnswerMessageID)
	}
	if state.LastReactionMessageID != 0 {
		_ = m.DeleteMessage(ctx, userID, state.LastReactionMessageID)
	}

	return m.chatStateRepo.Save(&models.ChatState{
		UserID:                  userID,
		LastTaskMessageID:       state.LastTaskMessageID,
		LastUserAnswerMessageID: 0,
		LastReactionMessageID:   0,
	})
}

func (m *MessageManager) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	_, err := m.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	return err
}

func (m *MessageManager) SaveUserAnswerMessageID(userID int64, messageID int) error {
	return m.chatStateRepo.UpdateAnswerMessageID(userID, messageID)
}

type MessageSender interface {
	Send(ctx context.Context) (*tgmodels.Message, error)
}

var ErrSendFailed = errors.New("failed to send message after retry")
