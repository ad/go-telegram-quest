package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

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
	if params.ParseMode == "" {
		params.ParseMode = tgmodels.ParseModeMarkdown
	}

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
	if params.ParseMode == "" && params.Caption != "" {
		params.ParseMode = tgmodels.ParseModeMarkdown
	}

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
	for i, media := range params.Media {
		if photo, ok := media.(*tgmodels.InputMediaPhoto); ok && photo.Caption != "" && photo.ParseMode == "" {
			photo.ParseMode = tgmodels.ParseModeMarkdown
			params.Media[i] = photo
		}
	}

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
	return m.SendTaskWithButtons(ctx, userID, step, false, false)
}

func (m *MessageManager) SendTaskWithHintButton(ctx context.Context, userID int64, step *models.Step, showHintButton bool) error {
	return m.SendTaskWithButtons(ctx, userID, step, showHintButton, false)
}

func (m *MessageManager) SendTaskWithButtons(ctx context.Context, userID int64, step *models.Step, showHintButton bool, showSkipButton bool) error {
	if err := m.DeletePreviousMessages(ctx, userID); err != nil {
		return err
	}

	// Проверяем что текст не пустой
	stepText := strings.TrimSpace(step.Text)
	if stepText == "" {
		stepText = "Задание без текста"
	}

	var keyboard *tgmodels.InlineKeyboardMarkup
	if showHintButton && step.HasHint() || showSkipButton {
		var buttons [][]tgmodels.InlineKeyboardButton

		if showHintButton && step.HasHint() {
			buttons = append(buttons, []tgmodels.InlineKeyboardButton{
				{
					Text:         "💡 Подсказка",
					CallbackData: fmt.Sprintf("hint:%d:%d", userID, step.ID),
				},
			})
		}

		if showSkipButton {
			buttons = append(buttons, []tgmodels.InlineKeyboardButton{
				{
					Text:         "⏭ Пропустить",
					CallbackData: fmt.Sprintf("skip_step:%d:%d", userID, step.ID),
				},
			})
		}

		if len(buttons) > 0 {
			keyboard = &tgmodels.InlineKeyboardMarkup{
				InlineKeyboard: buttons,
			}
		}
	}

	var taskMsgID int

	if len(step.Images) == 0 {
		params := &bot.SendMessageParams{
			ChatID: userID,
			Text:   bot.EscapeMarkdownUnescaped(stepText),
		}
		if keyboard != nil {
			params.ReplyMarkup = keyboard
		}
		msg, err := m.SendWithRetry(ctx, params)
		if err != nil {
			return err
		}
		taskMsgID = msg.ID
	} else if len(step.Images) == 1 {
		params := &bot.SendPhotoParams{
			ChatID:  userID,
			Photo:   &tgmodels.InputFileString{Data: step.Images[0].FileID},
			Caption: bot.EscapeMarkdownUnescaped(stepText),
		}
		if keyboard != nil {
			params.ReplyMarkup = keyboard
		}
		msg, err := m.SendPhotoWithRetry(ctx, params)
		if err != nil {
			log.Printf("[MESSAGE_MANAGER] Failed to send photo for step %d to user %d: %v, sending text instead", step.ID, userID, err)
			// Если не удалось отправить фото, отправляем текстовое сообщение
			textParams := &bot.SendMessageParams{
				ChatID: userID,
				Text:   bot.EscapeMarkdownUnescaped(stepText),
			}
			if keyboard != nil {
				textParams.ReplyMarkup = keyboard
			}
			msg, err = m.SendWithRetry(ctx, textParams)
			if err != nil {
				return err
			}
		}
		taskMsgID = msg.ID
	} else {
		media := make([]tgmodels.InputMedia, len(step.Images))
		for i, img := range step.Images {
			photo := &tgmodels.InputMediaPhoto{
				Media: img.FileID,
			}
			if i == 0 {
				photo.Caption = bot.EscapeMarkdownUnescaped(stepText)
			}
			media[i] = photo
		}
		msgs, err := m.SendMediaGroupWithRetry(ctx, &bot.SendMediaGroupParams{
			ChatID: userID,
			Media:  media,
		})
		if err != nil {
			log.Printf("[MESSAGE_MANAGER] Failed to send media group for step %d to user %d: %v, sending text instead", step.ID, userID, err)
			// Если не удалось отправить медиа-группу, отправляем текстовое сообщение
			textParams := &bot.SendMessageParams{
				ChatID: userID,
				Text:   bot.EscapeMarkdownUnescaped(stepText),
			}
			if keyboard != nil {
				textParams.ReplyMarkup = keyboard
			}
			msg, err := m.SendWithRetry(ctx, textParams)
			if err != nil {
				return err
			}
			taskMsgID = msg.ID
		} else {
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
