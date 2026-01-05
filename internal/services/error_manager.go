package services

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type ErrorManager struct {
	bot     *bot.Bot
	adminID int64
}

func NewErrorManager(b *bot.Bot, adminID int64) *ErrorManager {
	return &ErrorManager{
		bot:     b,
		adminID: adminID,
	}
}

func (e *ErrorManager) NotifyAdmin(ctx context.Context, panicValue interface{}, update *models.Update) {
	userInfo := "unknown"
	stepInfo := "unknown"

	if update != nil {
		if update.Message != nil && update.Message.From != nil {
			userInfo = fmt.Sprintf("[%d]", update.Message.From.ID)
			if update.Message.From.FirstName != "" {
				userInfo = update.Message.From.FirstName + " " + userInfo
			}
			if update.Message.From.Username != "" {
				userInfo = userInfo + " @" + update.Message.From.Username
			}
		} else if update.CallbackQuery != nil && update.CallbackQuery.From.ID != 0 {
			userInfo = fmt.Sprintf("[%d]", update.CallbackQuery.From.ID)
			if update.CallbackQuery.From.FirstName != "" {
				userInfo = update.CallbackQuery.From.FirstName + " " + userInfo
			}
			if update.CallbackQuery.From.Username != "" {
				userInfo = userInfo + " @" + update.CallbackQuery.From.Username
			}
		}
	}

	msg := fmt.Sprintf("🚨 Panic in handler\nUser: %s\nStep: %s\nError: %v\n\nStack trace:\n%s",
		userInfo, stepInfo, panicValue, string(debug.Stack()))

	if len(msg) > 4000 {
		msg = msg[:4000] + "\n... (truncated)"
	}

	_, _ = e.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: e.adminID,
		Text:   msg,
	})
}

func (e *ErrorManager) NotifyAdminWithCurl(ctx context.Context, chatID int64, request interface{}, err error) {
	curl := e.buildCurlCommand(chatID, request)

	msg := fmt.Sprintf("❌ Failed to send message\nUser: [%d]\nError: %v\n\nCurl:\n%s",
		chatID, err, curl)

	if len(msg) > 4000 {
		msg = msg[:4000] + "\n... (truncated)"
	}

	_, _ = e.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: e.adminID,
		Text:   msg,
	})
}

func (e *ErrorManager) buildCurlCommand(_ int64, request interface{}) string {
	jsonData, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return fmt.Sprintf("# Failed to serialize request: %v", err)
	}

	return fmt.Sprintf("curl -X POST 'https://api.telegram.org/bot[BOT_TOKEN]/sendMessage' \\\n  -H 'Content-Type: application/json' \\\n  -d '%s'",
		string(jsonData))
}
