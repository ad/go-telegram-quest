package handlers

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/ad/go-telegram-quest/internal/services"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type BotHandler struct {
	bot                  *bot.Bot
	adminID              int64
	errorManager         *services.ErrorManager
	stateResolver        *services.StateResolver
	answerChecker        *services.AnswerChecker
	msgManager           *services.MessageManager
	statsService         *services.StatisticsService
	userRepo             *db.UserRepository
	stepRepo             *db.StepRepository
	progressRepo         *db.ProgressRepository
	answerRepo           *db.AnswerRepository
	settingsRepo         *db.SettingsRepository
	chatStateRepo        *db.ChatStateRepository
	adminMessagesRepo    *db.AdminMessagesRepository
	adminHandler         *AdminHandler
	questStateMiddleware *services.QuestStateMiddleware
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
	userManager *services.UserManager,
	questStateManager *services.QuestStateManager,
) *BotHandler {
	adminHandler := NewAdminHandler(b, adminID, stepRepo, answerRepo, settingsRepo, adminStateRepo, userManager, userRepo, questStateManager)
	questStateMiddleware := services.NewQuestStateMiddleware(questStateManager, adminID)

	return &BotHandler{
		bot:                  b,
		adminID:              adminID,
		errorManager:         errorManager,
		stateResolver:        stateResolver,
		answerChecker:        answerChecker,
		msgManager:           msgManager,
		statsService:         statsService,
		userRepo:             userRepo,
		stepRepo:             stepRepo,
		progressRepo:         progressRepo,
		answerRepo:           answerRepo,
		settingsRepo:         settingsRepo,
		chatStateRepo:        chatStateRepo,
		adminMessagesRepo:    adminMessagesRepo,
		adminHandler:         adminHandler,
		questStateMiddleware: questStateMiddleware,
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

	shouldProcess, notification := h.questStateMiddleware.ShouldProcessMessage(userID)
	if !shouldProcess {
		h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   notification,
		})
		return
	}

	if h.isUserBlocked(userID) {
		h.sendShadowBanResponse(ctx, msg.Chat.ID)
		return
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

func (h *BotHandler) isUserBlocked(userID int64) bool {
	blocked, err := h.userRepo.IsBlocked(userID)
	if err != nil {
		return false
	}
	return blocked
}

func (h *BotHandler) sendShadowBanResponse(ctx context.Context, chatID int64) {
	settings, _ := h.settingsRepo.GetAll()
	wrongMsg := "❌ Неверно, попробуйте ещё раз"
	if settings != nil && settings.WrongAnswerMessage != "" {
		wrongMsg = settings.WrongAnswerMessage
	}
	h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   wrongMsg,
	}, "5046589136895476101") // 💩
}

func (h *BotHandler) handleCallback(ctx context.Context, callback *tgmodels.CallbackQuery) {
	if strings.HasPrefix(callback.Data, "next_step:") {
		h.handleNextStepCallback(ctx, callback)
		return
	}

	if strings.HasPrefix(callback.Data, "hint:") {
		h.handleHintCallback(ctx, callback)
		return
	}

	if callback.From.ID != h.adminID {
		return
	}

	if h.adminHandler.HandleCallback(ctx, callback) {
		return
	}

	if strings.HasPrefix(callback.Data, "approve:") || strings.HasPrefix(callback.Data, "reject:") {
		h.handleAdminDecision(ctx, callback)
	} else if strings.HasPrefix(callback.Data, "block:") {
		h.handleBlockUser(ctx, callback)
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

	shouldProcess, notification := h.questStateMiddleware.ShouldProcessMessage(user.ID)
	if !shouldProcess {
		h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   notification,
		})
		return
	}

	state, err := h.stateResolver.ResolveState(user.ID)
	if err != nil {
		h.sendError(ctx, msg.Chat.ID, fmt.Sprintf("Ошибка при определении состояния: %v", err))
		return
	}

	if state.IsCompleted {
		settings, _ := h.settingsRepo.GetAll()
		finalMsg := "Поздравляем! Вы прошли квест!"
		if settings != nil && settings.FinalMessage != "" {
			finalMsg = settings.FinalMessage
		}
		h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   finalMsg,
		}, "5046509860389126442") // 🎉
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
	switch step.AnswerType {
	case models.AnswerTypeText:
		// answerHint = "\n\n📝 Ответьте текстом или числом"
	case models.AnswerTypeImage:
		answerHint = "\n\n📷 Отправьте фото"
	}

	// Добавляем прогресс-бар
	progressText := h.getProgressText(userID)

	stepWithHint := &models.Step{
		ID:           step.ID,
		StepOrder:    step.StepOrder,
		Text:         progressText + "\n\n" + step.Text + answerHint,
		AnswerType:   step.AnswerType,
		HasAutoCheck: step.HasAutoCheck,
		IsActive:     step.IsActive,
		IsDeleted:    step.IsDeleted,
		Images:       step.Images,
		Answers:      step.Answers,
		HintText:     step.HintText,
		HintImage:    step.HintImage,
	}

	// Check if hint button should be shown
	showHintButton := false
	if step.HasHint() {
		chatState, err := h.chatStateRepo.Get(userID)
		if err == nil && chatState != nil {
			showHintButton = !chatState.CurrentStepHintUsed
		} else {
			// If no chat state exists or error, show hint button by default
			showHintButton = true
		}
	}

	h.msgManager.SendTaskWithHintButton(ctx, userID, stepWithHint, showHintButton)
}

func (h *BotHandler) getProgressText(userID int64) string {
	_, total, percentage, err := h.statsService.GetUserProgress(userID)
	if err != nil || total == 0 {
		return ""
	}

	barLength := int(percentage * 20 / 100)
	if barLength > 20 {
		barLength = 20
	}

	return strings.Repeat("▰", barLength) + strings.Repeat("▱", 20-barLength)
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
	h.msgManager.CleanupHintMessage(ctx, userID)

	// Get current hint usage state
	chatState, err := h.chatStateRepo.Get(userID)
	hintUsed := false
	if err == nil && chatState != nil {
		hintUsed = chatState.CurrentStepHintUsed
	}

	h.answerRepo.CreateTextAnswer(userID, step.ID, msg.Text, hintUsed)

	// Reset hint usage after saving answer
	if hintUsed {
		h.chatStateRepo.ResetHintUsed(userID)
	}

	if step.HasAutoCheck && len(step.Answers) > 0 {
		result, err := h.answerChecker.CheckTextAnswer(step.ID, msg.Text)
		if err != nil {
			h.sendError(ctx, msg.Chat.ID, "Ошибка при проверке ответа")
			return
		}

		if result.IsCorrect {
			h.handleCorrectAnswer(ctx, userID, step, result.Percentage)
		} else {
			h.msgManager.DeleteUserAnswerAndReaction(ctx, userID)
			settings, _ := h.settingsRepo.GetAll()
			wrongMsg := "❌ Неверно, попробуйте ещё раз"
			if settings != nil && settings.WrongAnswerMessage != "" {
				wrongMsg = settings.WrongAnswerMessage
			}

			wrongEffects := []string{
				"5104858069142078462", // 👎
				// "5170149264327704981", // 🤬
				// "5046551865169281494", // 😢
				// "5125503964049048317", // 🤮
				// "4988134357119009237", // 🥱
				// "4927250902185673331", // 🥴
				// "5122846324185629167", // 🤨
				// "5066978240002786236", // 😐
				// "4961092903720977544", // 🖕
				// "4960944078809203417", // 😈
				// "4925068178331010095", // 😡
				// "4913510691920413388", // 😨
				// "5089524022283076814", // 😫
				// "5089594618660520655", // 😵‍💫
				// "5026331292283700185", // 🤑
				// "5071299733016806207", // 🤒
				// "5086991627960976320", // 🤕
				// "5066635132245378011", // 🤥
				// "5091342528616072685", // 🤦‍♂
				// "5120948558526153760", // 🥵
				// "5026486074315113392", // 🥶
			}
			effectID := wrongEffects[rand.Intn(len(wrongEffects))]
			h.msgManager.SendReactionWithEffect(ctx, userID, wrongMsg, effectID)
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
	// Удаляем предыдущие сообщения пользователя и реакции (включая сообщения о неверных ответах)
	h.msgManager.DeleteUserAnswerAndReaction(ctx, userID)

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

	nextStepBtn := tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "Следующий вопрос ➡️", CallbackData: fmt.Sprintf("next_step:%d", step.StepOrder)}},
		},
	}

	correctEffects := []string{
		"5107584321108051014", // 👍
		// "5159385139981059251", // ❤
		"5104841245755180586", // 🔥
		// "5046509860389126442", // 🎉
		// "5170169077011841524", // 🥰
		// "5170166362592510656", // 👏
		// "5048771083361059460", // 😁
		// "5161554034041029689", // 🤩
		// "5066712811023894584", // 🙏
		// "5066947642655769508", // 👌
		// "4962976753686414048", // 💯
		// "5066993302453093673", // 🤣
		// "5123046001510188023", // 🏆
		// "4913625371842183765", // 🙈
		// "4913435779100836551", // 😇
		// "5087137729863484424", // ✅
		// "5067074180982244082", // ✌
		// "5089460564141278042", // ✨
		// "5134366251107222485", // 🎂
		// "5044101728060834560", // 🎆
		// "5046284769743077765", // 🎈
		// "5041819580008236993", // 🎊
		// "4965357582907606094", // 😊
		// "5089343350188802996", // 🥳
		// "4967721189309940952", // 🫶
	}
	effectID := correctEffects[rand.Intn(len(correctEffects))]

	if step.CorrectAnswerImage != "" {
		h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:          userID,
			Photo:           &tgmodels.InputFileString{Data: step.CorrectAnswerImage},
			Caption:         correctMsg,
			ReplyMarkup:     nextStepBtn,
			MessageEffectID: effectID,
		})
	} else {
		h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
			ChatID:      userID,
			Text:        correctMsg,
			ReplyMarkup: nextStepBtn,
		}, effectID)
	}

	h.updateStatistics(ctx)
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
		h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   finalMsg,
		}, "5046509860389126442") // 🎉

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
	h.msgManager.CleanupHintMessage(ctx, userID)

	var fileID string
	if len(msg.Photo) > 0 {
		fileID = msg.Photo[len(msg.Photo)-1].FileID
	}

	// Get current hint usage state
	chatState, err := h.chatStateRepo.Get(userID)
	hintUsed := false
	if err == nil && chatState != nil {
		hintUsed = chatState.CurrentStepHintUsed
	}

	h.answerRepo.CreateImageAnswer(userID, step.ID, []string{fileID}, hintUsed)

	// Reset hint usage after saving answer
	if hintUsed {
		h.chatStateRepo.ResetHintUsed(userID)
	}

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

	// user, _ := h.userRepo.GetByID(userID)
	// displayName := fmt.Sprintf("[%d]", userID)
	// if user != nil {
	// 	displayName = user.DisplayName()
	// }

	switch action {
	case "approve":
		progress.Status = models.StatusApproved
		if err := h.progressRepo.Update(progress); err != nil {
			return
		}

		h.editCallbackMessage(ctx, callback, "✅ Ответ одобрен")

		percentage, _ := h.answerChecker.CheckTextAnswer(stepID, "")
		h.handleCorrectAnswer(ctx, userID, step, percentage.Percentage)
	case "reject":
		progress.Status = models.StatusRejected
		if err := h.progressRepo.Update(progress); err != nil {
			return
		}

		h.editCallbackMessage(ctx, callback, "❌ Ответ отклонён")

		h.msgManager.DeleteUserAnswerAndReaction(ctx, userID)
		settings, _ := h.settingsRepo.GetAll()
		wrongMsg := "❌ Неверно, попробуйте ещё раз"
		if settings != nil && settings.WrongAnswerMessage != "" {
			wrongMsg = settings.WrongAnswerMessage
		}

		wrongEffects := []string{
			"5104858069142078462", // 👎
			// "5170149264327704981", // 🤬
			// "5046551865169281494", // 😢
			// "5125503964049048317", // 🤮
			// "4988134357119009237", // 🥱
			// "4927250902185673331", // 🥴
			// "5122846324185629167", // 🤨
			// "5066978240002786236", // 😐
			// "4961092903720977544", // 🖕
			// "4960944078809203417", // 😈
			// "4925068178331010095", // 😡
			// "4913510691920413388", // 😨
			// "5089524022283076814", // 😫
			// "5089594618660520655", // 😵‍💫
			// "5026331292283700185", // 🤑
			// "5071299733016806207", // 🤒
			// "5086991627960976320", // 🤕
			// "5066635132245378011", // 🤥
			// "5091342528616072685", // 🤦‍♂
			// "5120948558526153760", // 🥵
			// "5026486074315113392", // 🥶
		}
		effectID := wrongEffects[rand.Intn(len(wrongEffects))]
		h.msgManager.SendReactionWithEffect(ctx, userID, wrongMsg, effectID)
	}

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
	})
}

func (h *BotHandler) handleBlockUser(ctx context.Context, callback *tgmodels.CallbackQuery) {
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 2 {
		return
	}

	userID, _ := parseInt64(parts[1])
	if userID == 0 {
		return
	}

	if err := h.userRepo.BlockUser(userID); err != nil {
		h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callback.ID,
			Text:            "Ошибка при блокировке",
		})
		return
	}

	user, _ := h.userRepo.GetByID(userID)
	displayName := fmt.Sprintf("[%d]", userID)
	if user != nil {
		displayName = user.DisplayName()
	}

	msg := callback.Message.Message
	if msg != nil {
		var newText string
		if len(msg.Photo) > 0 {
			newText = fmt.Sprintf("🚫 Заблокирован\n👤 %s", displayName)
			h.bot.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Caption:   newText,
			})
		} else {
			newText = fmt.Sprintf("🚫 Заблокирован\n👤 %s", displayName)
			h.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Text:      newText,
			})
		}
	}

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		Text:            "Пользователь заблокирован",
	})
}

func (h *BotHandler) editCallbackMessage(ctx context.Context, callback *tgmodels.CallbackQuery, newText string) {
	msg := callback.Message.Message
	if msg == nil {
		return
	}

	if len(msg.Photo) > 0 {
		_, err := h.bot.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Caption:   newText,
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
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      newText,
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

func BuildHintKeyboard(userID int64, stepID int64) *tgmodels.InlineKeyboardMarkup {
	return &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{
				Text:         "💡 Подсказка",
				CallbackData: fmt.Sprintf("hint:%d:%d", userID, stepID),
			}},
		},
	}
}

func (h *BotHandler) sendToAdminForReview(ctx context.Context, userID int64, step *models.Step, textAnswer string, imageFileIDs []string) {
	user, _ := h.userRepo.GetByID(userID)
	displayName := fmt.Sprintf("[%d]", userID)
	if user != nil {
		displayName = user.DisplayName()
	}

	caption := fmt.Sprintf("👤 %s\n📋 Ответ на шаг %d\n📝 Задание: %s", displayName, step.StepOrder, step.Text)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{
				{Text: "✅ Правильно", CallbackData: fmt.Sprintf("approve:%d:%d", userID, step.ID)},
				{Text: "❌ Ошибка", CallbackData: fmt.Sprintf("reject:%d:%d", userID, step.ID)},
			},
			{
				{Text: "🚫 Заблокировать", CallbackData: fmt.Sprintf("block:%d", userID)},
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
		}
	}
}

func (h *BotHandler) handleNextStepCallback(ctx context.Context, callback *tgmodels.CallbackQuery) {
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 2 {
		return
	}

	if callback.Message.Message != nil {
		h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    callback.Message.Message.Chat.ID,
			MessageID: callback.Message.Message.ID,
		})
	}

	currentOrder, _ := parseInt64(parts[1])
	h.moveToNextStep(ctx, callback.From.ID, int(currentOrder))

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
	})
}

func (h *BotHandler) handleHintCallback(ctx context.Context, callback *tgmodels.CallbackQuery) {
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 3 {
		return
	}

	userID, _ := parseInt64(parts[1])
	stepID, _ := parseInt64(parts[2])

	if userID == 0 || stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil || !step.HasHint() {
		return
	}

	hintMsgID, err := h.sendHintMessage(ctx, userID, step)
	if err != nil {
		return
	}

	h.chatStateRepo.UpdateHintMessageID(userID, hintMsgID)
	h.chatStateRepo.SetHintUsed(userID, true)

	h.removeHintButton(ctx, userID, callback.Message.Message.ID)

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
	})
}

func (h *BotHandler) sendHintMessage(ctx context.Context, userID int64, step *models.Step) (int, error) {
	if step.HintImage != "" {
		msg, err := h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:  userID,
			Photo:   &tgmodels.InputFileString{Data: step.HintImage},
			Caption: step.HintText,
		})
		if err != nil {
			return 0, err
		}

		return msg.ID, nil
	}

	msg, err := h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   step.HintText,
	})
	if err != nil {
		return 0, err
	}

	return msg.ID, nil
}

func (h *BotHandler) removeHintButton(ctx context.Context, userID int64, messageID int) {
	h.bot.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    userID,
		MessageID: messageID,
	})
}
