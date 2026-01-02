package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/ad/go-telegram-quest/internal/services"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type BotHandler struct {
	bot               *bot.Bot
	adminID           int64
	errorManager      *services.ErrorManager
	stateResolver     *services.StateResolver
	answerChecker     *services.AnswerChecker
	msgManager        *services.MessageManager
	statsService      *services.StatisticsService
	userRepo          *db.UserRepository
	stepRepo          *db.StepRepository
	progressRepo      *db.ProgressRepository
	answerRepo        *db.AnswerRepository
	settingsRepo      *db.SettingsRepository
	chatStateRepo     *db.ChatStateRepository
	adminMessagesRepo *db.AdminMessagesRepository
	adminHandler      *AdminHandler
}

func NewBotHandler(
	b *bot.Bot,
	adminID int64,
	errorManager *services.ErrorManager,
	stateResolver *services.StateResolver,
	answerChecker *services.AnswerChecker,
	msgManager *services.MessageManager,
	statsService *services.StatisticsService,
	userRepo *db.UserRepository,
	stepRepo *db.StepRepository,
	progressRepo *db.ProgressRepository,
	answerRepo *db.AnswerRepository,
	settingsRepo *db.SettingsRepository,
	chatStateRepo *db.ChatStateRepository,
	adminMessagesRepo *db.AdminMessagesRepository,
	adminStateRepo *db.AdminStateRepository,
) *BotHandler {
	adminHandler := NewAdminHandler(b, adminID, stepRepo, answerRepo, settingsRepo, adminStateRepo)

	return &BotHandler{
		bot:               b,
		adminID:           adminID,
		errorManager:      errorManager,
		stateResolver:     stateResolver,
		answerChecker:     answerChecker,
		msgManager:        msgManager,
		statsService:      statsService,
		userRepo:          userRepo,
		stepRepo:          stepRepo,
		progressRepo:      progressRepo,
		answerRepo:        answerRepo,
		settingsRepo:      settingsRepo,
		chatStateRepo:     chatStateRepo,
		adminMessagesRepo: adminMessagesRepo,
		adminHandler:      adminHandler,
	}
}

func (h *BotHandler) HandleUpdate(ctx context.Context, b *bot.Bot, update *tgmodels.Update) {
	defer h.recoverPanic(ctx, update)

	if update.Message != nil {
		h.handleMessage(ctx, update.Message)
	} else if update.CallbackQuery != nil {
		h.handleCallback(ctx, update.CallbackQuery)
	}
}

func (h *BotHandler) recoverPanic(ctx context.Context, update *tgmodels.Update) {
	if r := recover(); r != nil {
		h.errorManager.NotifyAdmin(ctx, r, update)
	}
}

func (h *BotHandler) handleMessage(ctx context.Context, msg *tgmodels.Message) {
	if msg.From == nil {
		return
	}

	userID := msg.From.ID

	if msg.Text == "/start" {
		h.handleStart(ctx, msg)
		return
	}

	if userID == h.adminID {
		if h.adminHandler.HandleCommand(ctx, msg) {
			return
		}
	}

	if len(msg.Photo) > 0 {
		h.handleImageAnswer(ctx, msg)
		return
	}

	if msg.MediaGroupID != "" {
		return
	}

	if msg.Text != "" {
		h.handleTextAnswer(ctx, msg)
	}
}

func (h *BotHandler) handleCallback(ctx context.Context, callback *tgmodels.CallbackQuery) {
	if callback.From.ID != h.adminID {
		return
	}

	if h.adminHandler.HandleCallback(ctx, callback) {
		return
	}

	if strings.HasPrefix(callback.Data, "approve:") || strings.HasPrefix(callback.Data, "reject:") {
		h.handleAdminDecision(ctx, callback)
	}
}

func (h *BotHandler) handleStart(ctx context.Context, msg *tgmodels.Message) {
	user := &models.User{
		ID:        msg.From.ID,
		FirstName: msg.From.FirstName,
		LastName:  msg.From.LastName,
		Username:  msg.From.Username,
	}

	if err := h.userRepo.CreateOrUpdate(user); err != nil {
		h.sendError(ctx, msg.Chat.ID, "Ошибка при регистрации")
		return
	}

	state, err := h.stateResolver.ResolveState(user.ID)
	if err != nil {
		h.sendError(ctx, msg.Chat.ID, "Ошибка при определении состояния")
		return
	}

	if state.IsCompleted {
		settings, _ := h.settingsRepo.GetAll()
		finalMsg := "Поздравляем! Вы прошли квест!"
		if settings != nil && settings.FinalMessage != "" {
			finalMsg = settings.FinalMessage
		}
		h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   finalMsg,
		})
		return
	}

	if state.Status == models.StatusPending || state.Status == "" {
		settings, _ := h.settingsRepo.GetAll()
		welcomeMsg := "Добро пожаловать в квест!"
		if settings != nil && settings.WelcomeMessage != "" {
			welcomeMsg = settings.WelcomeMessage
		}
		h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   welcomeMsg,
		})
	}

	h.sendStep(ctx, user.ID, state.CurrentStep)
}

func (h *BotHandler) sendStep(ctx context.Context, userID int64, step *models.Step) {
	if step == nil {
		return
	}

	progress, _ := h.progressRepo.GetByUserAndStep(userID, step.ID)
	if progress == nil {
		h.progressRepo.Create(&models.UserProgress{
			UserID: userID,
			StepID: step.ID,
			Status: models.StatusPending,
		})
	}

	answerHint := ""
	if step.AnswerType == models.AnswerTypeText {
		answerHint = "\n\n📝 Ответьте текстом"
	} else if step.AnswerType == models.AnswerTypeImage {
		answerHint = "\n\n📷 Отправьте фото"
	}

	stepWithHint := &models.Step{
		ID:           step.ID,
		StepOrder:    step.StepOrder,
		Text:         step.Text + answerHint,
		AnswerType:   step.AnswerType,
		HasAutoCheck: step.HasAutoCheck,
		IsActive:     step.IsActive,
		IsDeleted:    step.IsDeleted,
		Images:       step.Images,
		Answers:      step.Answers,
	}

	h.msgManager.SendTask(ctx, userID, stepWithHint)
}

func (h *BotHandler) handleTextAnswer(ctx context.Context, msg *tgmodels.Message) {
	userID := msg.From.ID

	state, err := h.stateResolver.ResolveState(userID)
	if err != nil || state.IsCompleted || state.CurrentStep == nil {
		return
	}

	step := state.CurrentStep

	if step.AnswerType == models.AnswerTypeImage {
		h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)
		h.msgManager.SendReaction(ctx, userID, "📷 Для этого задания нужно отправить фото")
		return
	}

	h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)

	h.answerRepo.CreateTextAnswer(userID, step.ID, msg.Text)

	if step.HasAutoCheck && len(step.Answers) > 0 {
		result, err := h.answerChecker.CheckTextAnswer(step.ID, msg.Text)
		if err != nil {
			h.sendError(ctx, msg.Chat.ID, "Ошибка при проверке ответа")
			return
		}

		if result.IsCorrect {
			h.handleCorrectAnswer(ctx, userID, step, result.Percentage)
		} else {
			settings, _ := h.settingsRepo.GetAll()
			wrongMsg := "❌ Неверно, попробуйте ещё раз"
			if settings != nil && settings.WrongAnswerMessage != "" {
				wrongMsg = settings.WrongAnswerMessage
			}
			h.msgManager.SendReaction(ctx, userID, wrongMsg)
		}
	} else {
		h.progressRepo.Update(&models.UserProgress{
			UserID: userID,
			StepID: step.ID,
			Status: models.StatusWaitingReview,
		})
		h.sendToAdminForReview(ctx, userID, step, msg.Text, nil)
		h.msgManager.SendReaction(ctx, userID, "⏳ Ваш ответ отправлен на проверку")
	}
}

func (h *BotHandler) handleCorrectAnswer(ctx context.Context, userID int64, step *models.Step, percentage int) {
	h.progressRepo.Update(&models.UserProgress{
		UserID: userID,
		StepID: step.ID,
		Status: models.StatusApproved,
	})

	settings, _ := h.settingsRepo.GetAll()
	correctMsg := "✅ Правильно!"
	if settings != nil && settings.CorrectAnswerMessage != "" {
		correctMsg = settings.CorrectAnswerMessage
	}

	if percentage > 0 {
		correctMsg = fmt.Sprintf("%s\n\n📊 До этого шага дошли %d%% участников", correctMsg, percentage)
	}

	h.msgManager.SendReaction(ctx, userID, correctMsg)
	h.updateStatistics(ctx)

	h.moveToNextStep(ctx, userID, step.StepOrder)
}

func (h *BotHandler) moveToNextStep(ctx context.Context, userID int64, currentOrder int) {
	nextStep, err := h.stepRepo.GetNextActive(currentOrder)
	if err != nil || nextStep == nil {
		settings, _ := h.settingsRepo.GetAll()
		finalMsg := "🎉 Поздравляем! Вы прошли квест!"
		if settings != nil && settings.FinalMessage != "" {
			finalMsg = settings.FinalMessage
		}

		h.msgManager.DeletePreviousMessages(ctx, userID)
		h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   finalMsg,
		})

		h.notifyAdminQuestCompleted(ctx, userID)
		return
	}

	h.sendStep(ctx, userID, nextStep)
}

func (h *BotHandler) notifyAdminQuestCompleted(ctx context.Context, userID int64) {
	user, _ := h.userRepo.GetByID(userID)
	displayName := fmt.Sprintf("[%d]", userID)
	if user != nil {
		displayName = user.DisplayName()
	}

	h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
		ChatID: h.adminID,
		Text:   fmt.Sprintf("🏆 Пользователь %s завершил квест!", displayName),
	})
}

func (h *BotHandler) sendError(ctx context.Context, chatID int64, text string) {
	h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "⚠️ " + text,
	})
}

func (h *BotHandler) handleImageAnswer(ctx context.Context, msg *tgmodels.Message) {
	userID := msg.From.ID

	state, err := h.stateResolver.ResolveState(userID)
	if err != nil || state.IsCompleted || state.CurrentStep == nil {
		return
	}

	step := state.CurrentStep

	if step.AnswerType == models.AnswerTypeText {
		h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)
		h.msgManager.SendReaction(ctx, userID, "📝 Для этого задания нужно отправить текст")
		return
	}

	h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)

	var fileID string
	if len(msg.Photo) > 0 {
		fileID = msg.Photo[len(msg.Photo)-1].FileID
	}

	h.answerRepo.CreateImageAnswer(userID, step.ID, []string{fileID})

	h.progressRepo.Update(&models.UserProgress{
		UserID: userID,
		StepID: step.ID,
		Status: models.StatusWaitingReview,
	})

	h.sendToAdminForReview(ctx, userID, step, "", []string{fileID})
	h.msgManager.SendReaction(ctx, userID, "⏳ Ваше фото отправлено на проверку")
}

func (h *BotHandler) handleAdminDecision(ctx context.Context, callback *tgmodels.CallbackQuery) {
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 3 {
		return
	}

	action := parts[0]
	userID, _ := parseInt64(parts[1])
	stepID, _ := parseInt64(parts[2])

	if userID == 0 || stepID == 0 {
		return
	}

	progress, err := h.progressRepo.GetByUserAndStep(userID, stepID)
	if err != nil || progress == nil {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	user, _ := h.userRepo.GetByID(userID)
	displayName := fmt.Sprintf("[%d]", userID)
	if user != nil {
		displayName = user.DisplayName()
	}

	if action == "approve" {
		progress.Status = models.StatusApproved
		if err := h.progressRepo.Update(progress); err != nil {
			return
		}

		h.editCallbackMessage(ctx, callback, fmt.Sprintf("✅ Одобрено\n👤 %s\n📋 Шаг %d", displayName, step.StepOrder))

		percentage, _ := h.answerChecker.CheckTextAnswer(stepID, "")
		h.handleCorrectAnswer(ctx, userID, step, percentage.Percentage)
	} else if action == "reject" {
		progress.Status = models.StatusRejected
		if err := h.progressRepo.Update(progress); err != nil {
			return
		}

		h.editCallbackMessage(ctx, callback, fmt.Sprintf("❌ Отклонено\n👤 %s\n📋 Шаг %d", displayName, step.StepOrder))

		settings, _ := h.settingsRepo.GetAll()
		wrongMsg := "❌ Неверно, попробуйте ещё раз"
		if settings != nil && settings.WrongAnswerMessage != "" {
			wrongMsg = settings.WrongAnswerMessage
		}
		h.msgManager.SendReaction(ctx, userID, wrongMsg)
	}

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
	})
}

func (h *BotHandler) editCallbackMessage(ctx context.Context, callback *tgmodels.CallbackQuery, newText string) {
	msg := callback.Message.Message
	if msg == nil {
		return
	}

	if len(msg.Photo) > 0 {
		_, err := h.bot.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
			ChatID:      msg.Chat.ID,
			MessageID:   msg.ID,
			Caption:     newText,
			ReplyMarkup: nil,
		})
		if isMessageNotFoundError(err) {
			h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID:  msg.Chat.ID,
				Photo:   &tgmodels.InputFileString{Data: msg.Photo[len(msg.Photo)-1].FileID},
				Caption: newText,
			})
		}
	} else {
		_, err := h.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      msg.Chat.ID,
			MessageID:   msg.ID,
			Text:        newText,
			ReplyMarkup: nil,
		})
		if isMessageNotFoundError(err) {
			h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   newText,
			})
		}
	}
}

func isMessageNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "message to edit not found") ||
		strings.Contains(errStr, "message is not modified") ||
		strings.Contains(errStr, "MESSAGE_ID_INVALID")
}

func parseInt64(s string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func (h *BotHandler) sendToAdminForReview(ctx context.Context, userID int64, step *models.Step, textAnswer string, imageFileIDs []string) {
	user, _ := h.userRepo.GetByID(userID)
	displayName := fmt.Sprintf("[%d]", userID)
	if user != nil {
		displayName = user.DisplayName()
	}

	caption := fmt.Sprintf("📋 Ответ на шаг %d\n👤 %s", step.StepOrder, displayName)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{
				{Text: "✅ Правильно", CallbackData: fmt.Sprintf("approve:%d:%d", userID, step.ID)},
				{Text: "❌ Ошибка", CallbackData: fmt.Sprintf("reject:%d:%d", userID, step.ID)},
			},
		},
	}

	if textAnswer != "" {
		h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID:      h.adminID,
			Text:        caption + "\n\n💬 " + textAnswer,
			ReplyMarkup: keyboard,
		})
	} else if len(imageFileIDs) > 0 {
		h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:      h.adminID,
			Photo:       &tgmodels.InputFileString{Data: imageFileIDs[0]},
			Caption:     caption,
			ReplyMarkup: keyboard,
		})
	}
}

func (h *BotHandler) updateStatistics(ctx context.Context) {
	stats, err := h.statsService.CalculateStats()
	if err != nil {
		return
	}

	var sb strings.Builder
	sb.WriteString("📊 Статистика квеста\n\n")

	sb.WriteString("📋 Прогресс по шагам:\n")
	for _, s := range stats.StepStats {
		sb.WriteString(fmt.Sprintf("  Шаг %d: %d чел.\n", s.StepOrder, s.Count))
	}

	if len(stats.Leaders) > 0 {
		sb.WriteString("\n🏆 Лидеры:\n")
		maxLeaders := 5
		if len(stats.Leaders) < maxLeaders {
			maxLeaders = len(stats.Leaders)
		}
		for i := 0; i < maxLeaders; i++ {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, stats.Leaders[i].DisplayName()))
		}
	}

	text := sb.String()

	adminMsg, err := h.adminMessagesRepo.Get("statistics")
	if err == nil && adminMsg != nil {
		_, editErr := h.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    adminMsg.ChatID,
			MessageID: adminMsg.MessageID,
			Text:      text,
		})
		if isMessageNotFoundError(editErr) {
			msg, sendErr := h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
				ChatID: adminMsg.ChatID,
				Text:   text,
			})
			if sendErr == nil && msg != nil {
				h.adminMessagesRepo.Set("statistics", adminMsg.ChatID, msg.ID)
			}
		}
	} else {
		msg, err := h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID: h.adminID,
			Text:   text,
		})
		if err == nil && msg != nil {
			h.adminMessagesRepo.Set("statistics", h.adminID, msg.ID)
		}
	}
}
