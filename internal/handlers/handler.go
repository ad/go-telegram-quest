package handlers

import (
	"context"
	"fmt"
	"log"
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
	achievementEngine    *services.AchievementEngine
	achievementNotifier  *services.AchievementNotifier
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
	achievementEngine *services.AchievementEngine,
	achievementNotifier *services.AchievementNotifier,
	achievementService *services.AchievementService,
	dbPath string,
) *BotHandler {
	adminHandler := NewAdminHandler(b, adminID, stepRepo, answerRepo, settingsRepo, adminStateRepo, userManager, userRepo, questStateManager, achievementService, achievementEngine, achievementNotifier, statsService, errorManager, dbPath)
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
		achievementEngine:    achievementEngine,
		achievementNotifier:  achievementNotifier,
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
			Text:   services.EscapeUserContent(notification),
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
		wrongMsg = services.EscapeUserContent(settings.WrongAnswerMessage)
	}
	h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   wrongMsg,
	}, "5046589136895476101") // 💩
}

func (h *BotHandler) handleCallback(ctx context.Context, callback *tgmodels.CallbackQuery) {
	// log.Printf("[HANDLER] handleCallback called with data: %s, from: %d, adminID: %d", callback.Data, callback.From.ID, h.adminID)

	if strings.HasPrefix(callback.Data, "next_step:") {
		h.handleNextStepCallback(ctx, callback)
		return
	}

	if strings.HasPrefix(callback.Data, "hint:") {
		h.handleHintCallback(ctx, callback)
		return
	}

	if strings.HasPrefix(callback.Data, "skip_step:") {
		h.handleSkipStepCallback(ctx, callback)
		return
	}

	if callback.From.ID != h.adminID {
		log.Printf("[HANDLER] callback from non-admin user: %d", callback.From.ID)
		return
	}

	adminHandled := h.adminHandler.HandleCallback(ctx, callback)
	// log.Printf("[HANDLER] adminHandler.HandleCallback returned: %t", adminHandled)
	if adminHandled {
		return
	}

	log.Printf("[HANDLER] processing callback in main handler: %s", callback.Data)
	if strings.HasPrefix(callback.Data, "approve:") || strings.HasPrefix(callback.Data, "reject:") {
		// log.Printf("[HANDLER] calling handleAdminDecision")
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
			Text:   services.EscapeUserContent(notification),
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

		completionStats := h.statsService.FormatCompletionStats(msg.From.ID)
		if completionStats != "" {
			finalMsg = finalMsg + "\n\n" + completionStats
		}

		stickerPackMsg := h.achievementNotifier.FormatStickerPackMessage(msg.From.ID)
		if stickerPackMsg != "" {
			finalMsg = finalMsg + "\n\n" + stickerPackMsg
		}

		h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   services.EscapeUserContent(finalMsg),
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
			Text:   services.EscapeUserContent(welcomeMsg),
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
		Text:         progressText + "\n\n" + services.EscapeUserContent(step.Text) + services.EscapeUserContent(answerHint),
		AnswerType:   step.AnswerType,
		HasAutoCheck: step.HasAutoCheck,
		IsActive:     step.IsActive,
		IsDeleted:    step.IsDeleted,
		IsAsterisk:   step.IsAsterisk,
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
			showHintButton = true
		}
	}

	h.msgManager.SendTaskWithButtons(ctx, userID, stepWithHint, showHintButton, step.IsAsterisk)
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
	if err != nil {
		log.Printf("[HANDLER] Error resolving state for user %d: %v", userID, err)
		return
	}

	h.evaluateSecretAnswer(ctx, userID, msg.Text)

	if state.IsCompleted {
		log.Printf("[HANDLER] User %d completed quest, forwarding message to admin", userID)
		h.evaluateAchievementsOnPostCompletion(ctx, userID)
		h.forwardMessageToAdmin(ctx, msg, nil, "после завершения квеста")
		return
	}

	if state.CurrentStep == nil {
		log.Printf("[HANDLER] User %d has no current step", userID)
		return
	}

	step := state.CurrentStep
	log.Printf("[HANDLER] User %d on step %d (order %d)", userID, step.ID, step.StepOrder)

	chatState, _ := h.chatStateRepo.Get(userID)
	progress, _ := h.progressRepo.GetByUserAndStep(userID, step.ID)

	if chatState != nil && chatState.AwaitingNextStep && (progress == nil || progress.Status == models.StatusPending) {
		log.Printf("[HANDLER] User %d sent text message while awaiting next step, moving to next step", userID)

		if h.achievementEngine != nil {
			awarded, err := h.achievementEngine.OnMessageToAdmin(userID)
			if err != nil {
				log.Printf("[HANDLER] Error awarding message to admin achievement: %v", err)
			} else if len(awarded) > 0 {
				h.notifyAchievements(ctx, userID, awarded)
			}
		}

		prevStep, err := h.stepRepo.GetPreviousActive(step.StepOrder)
		if err == nil && prevStep != nil {
			h.forwardMessageToAdmin(ctx, msg, prevStep, "после правильного ответа")
		} else {
			h.forwardMessageToAdmin(ctx, msg, step, "после правильного ответа")
		}

		h.msgManager.DeletePreviousMessages(ctx, userID)
		h.chatStateRepo.ClearAwaitingNextStep(userID)
		h.sendStep(ctx, userID, step)
		return
	}

	if progress != nil {
		log.Printf("[HANDLER] User %d progress on step %d: status=%s", userID, step.ID, progress.Status)
	}

	if step.AnswerType == models.AnswerTypeImage {
		h.forwardMessageToAdmin(ctx, msg, step, "при отправке текста на вопрос-изображение")

		writerAchievements, err := h.achievementEngine.OnTextOnImageTask(userID)
		if err != nil {
			log.Printf("[HANDLER] Error awarding writer achievement: %v", err)
		} else if len(writerAchievements) > 0 {
			h.notifyAchievements(ctx, userID, writerAchievements)
		}

		h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)
		h.msgManager.SendReaction(ctx, userID, "📷 Для этого задания нужно отправить фото")
		return
	}

	h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)
	h.msgManager.CleanupHintMessage(ctx, userID)

	chatState, err = h.chatStateRepo.Get(userID)
	hintUsed := false
	if err == nil && chatState != nil {
		hintUsed = chatState.CurrentStepHintUsed
	}

	h.answerRepo.CreateTextAnswer(userID, step.ID, msg.Text, hintUsed)

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
			h.handleCorrectAnswer(ctx, userID, step, result.Percentage, msg.Text)
		} else {
			h.msgManager.DeleteUserAnswerAndReaction(ctx, userID)
			settings, _ := h.settingsRepo.GetAll()
			wrongMsg := "❌ Неверно, попробуйте ещё раз"
			if settings != nil && settings.WrongAnswerMessage != "" {
				wrongMsg = services.EscapeUserContent(settings.WrongAnswerMessage)
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
		progress, _ := h.progressRepo.GetByUserAndStep(userID, step.ID)
		if progress == nil {
			h.progressRepo.Create(&models.UserProgress{
				UserID: userID,
				StepID: step.ID,
				Status: models.StatusWaitingReview,
			})
		} else {
			h.progressRepo.Update(&models.UserProgress{
				UserID: userID,
				StepID: step.ID,
				Status: models.StatusWaitingReview,
			})
		}
		h.sendToAdminForReview(ctx, userID, step, msg.Text, nil)
		h.msgManager.SendReaction(ctx, userID, "⏳ *Ваш ответ отправлен на проверку, подождите пока его проверят*\\.")
	}
}

func (h *BotHandler) handleCorrectAnswer(ctx context.Context, userID int64, step *models.Step, percentage int, _ string) {
	// log.Printf("[HANDLER] handleCorrectAnswer started for user %d, step %d", userID, step.ID)

	h.msgManager.DeleteUserAnswerAndReaction(ctx, userID)

	h.progressRepo.Update(&models.UserProgress{
		UserID: userID,
		StepID: step.ID,
		Status: models.StatusApproved,
	})

	nextStep, _ := h.stepRepo.GetNextActive(step.StepOrder, userID)
	isLastStep := nextStep == nil

	// log.Printf("[HANDLER] Evaluating achievements for user %d, isLastStep=%v", userID, isLastStep)

	// Сначала обрабатываем достижения
	if isLastStep {
		h.evaluateAchievementsOnQuestCompleted(ctx, userID)
	} else {
		h.evaluateAchievementsOnCorrectAnswer(ctx, userID, step.ID)
	}

	// log.Printf("[HANDLER] Achievements evaluated, sending correct message to user %d", userID)

	settings, _ := h.settingsRepo.GetAll()
	correctMsg := "✅ Правильно!"
	if settings != nil && settings.CorrectAnswerMessage != "" {
		correctMsg = services.EscapeUserContent(settings.CorrectAnswerMessage)
	}

	if percentage > 0 {
		correctMsg = fmt.Sprintf("%s\n\n📊 До этого шага дошли %d%% участников", correctMsg, percentage)
	}

	correctEffects := []string{
		"5107584321108051014", // 👍
		"5104841245755180586", // 🔥
	}
	effectID := correctEffects[rand.Intn(len(correctEffects))]

	if isLastStep {
		finalMsg := "🎉 Поздравляем! Вы прошли квест!"
		if settings != nil && settings.FinalMessage != "" {
			finalMsg = settings.FinalMessage
		}

		completionStats := h.statsService.FormatCompletionStats(userID)
		if completionStats != "" {
			finalMsg = finalMsg + "\n\n" + completionStats
		}

		stickerPackMsg := h.achievementNotifier.FormatStickerPackMessage(userID)
		if stickerPackMsg != "" {
			finalMsg = finalMsg + "\n\n" + stickerPackMsg
		}

		correctMsg = correctMsg + "\n\n" + finalMsg

		if step.CorrectAnswerImage != "" {
			// log.Printf("[HANDLER] Sending final photo to user %d", userID)
			_, err := h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID:          userID,
				Photo:           &tgmodels.InputFileString{Data: step.CorrectAnswerImage},
				Caption:         bot.EscapeMarkdownUnescaped(correctMsg),
				MessageEffectID: "5046509860389126442", // 🎉
			})
			if err != nil {
				log.Printf("[HANDLER] Failed to send final photo to user %d: %v, sending text message instead", userID, err)
				// Если не удалось отправить фото, отправляем текстовое сообщение
				h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
					ChatID: userID,
					Text:   bot.EscapeMarkdownUnescaped(correctMsg),
				}, "5046509860389126442") // 🎉
			}
		} else {
			h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
				ChatID: userID,
				Text:   bot.EscapeMarkdownUnescaped(correctMsg),
			}, "5046509860389126442") // 🎉
		}

		h.notifyAdminQuestCompleted(ctx, userID)
		return
	}

	nextStepBtn := tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "Следующий вопрос ➡️", CallbackData: fmt.Sprintf("next_step:%d", step.StepOrder)}},
		},
	}

	h.chatStateRepo.SetAwaitingNextStep(userID)

	if step.CorrectAnswerImage != "" {
		msg, err := h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:          userID,
			Photo:           &tgmodels.InputFileString{Data: step.CorrectAnswerImage},
			Caption:         correctMsg,
			ParseMode:       tgmodels.ParseModeMarkdown,
			ReplyMarkup:     nextStepBtn,
			MessageEffectID: effectID,
		})
		if err != nil {
			log.Printf("[HANDLER] Failed to send photo to user %d: %v, sending text message instead", userID, err)
			msg, _ = h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
				ChatID:      userID,
				Text:        correctMsg,
				ReplyMarkup: nextStepBtn,
			}, effectID)
		}
		if msg != nil {
			h.chatStateRepo.UpdateReactionMessageID(userID, msg.ID)
		}
	} else {
		msg, _ := h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
			ChatID:      userID,
			Text:        correctMsg,
			ReplyMarkup: nextStepBtn,
		}, effectID)
		if msg != nil {
			h.chatStateRepo.UpdateReactionMessageID(userID, msg.ID)
		}
	}
}

func (h *BotHandler) moveToNextStep(ctx context.Context, userID int64, currentOrder int) {
	nextStep, err := h.stepRepo.GetNextActive(currentOrder, userID)
	if err != nil || nextStep == nil {
		h.evaluateAchievementsOnQuestCompleted(ctx, userID)

		settings, _ := h.settingsRepo.GetAll()
		finalMsg := "🎉 Поздравляем! Вы прошли квест!"
		if settings != nil && settings.FinalMessage != "" {
			finalMsg = settings.FinalMessage
		}

		completionStats := h.statsService.FormatCompletionStats(userID)
		if completionStats != "" {
			finalMsg = finalMsg + "\n\n" + completionStats
		}

		stickerPackMsg := h.achievementNotifier.FormatStickerPackMessage(userID)
		if stickerPackMsg != "" {
			finalMsg = finalMsg + "\n\n" + stickerPackMsg
		}

		h.msgManager.DeletePreviousMessages(ctx, userID)
		h.msgManager.SendWithRetryAndEffect(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   bot.EscapeMarkdownUnescaped(finalMsg),
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
		Text:   fmt.Sprintf("🏆 Пользователь %s завершил квест\\!", services.EscapeUserContent(displayName)),
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
	if err != nil {
		return
	}

	if state.IsCompleted {
		h.evaluateAchievementsOnPostCompletion(ctx, userID)
		h.forwardMessageToAdmin(ctx, msg, nil, "после завершения квеста")
		return
	}

	if state.CurrentStep == nil {
		return
	}

	step := state.CurrentStep

	chatState, _ := h.chatStateRepo.Get(userID)
	progress, _ := h.progressRepo.GetByUserAndStep(userID, step.ID)

	if chatState != nil && chatState.AwaitingNextStep && (progress == nil || progress.Status == models.StatusPending) {
		log.Printf("[HANDLER] User %d sent image while awaiting next step, moving to next step", userID)

		if h.achievementEngine != nil {
			awarded, err := h.achievementEngine.OnMessageToAdmin(userID)
			if err != nil {
				log.Printf("[HANDLER] Error awarding message to admin achievement: %v", err)
			} else if len(awarded) > 0 {
				h.notifyAchievements(ctx, userID, awarded)
			}
		}

		prevStep, err := h.stepRepo.GetPreviousActive(step.StepOrder)
		if err == nil && prevStep != nil {
			h.forwardMessageToAdmin(ctx, msg, prevStep, "после правильного ответа")
		} else {
			h.forwardMessageToAdmin(ctx, msg, step, "после правильного ответа")
		}

		h.msgManager.DeletePreviousMessages(ctx, userID)
		h.chatStateRepo.ClearAwaitingNextStep(userID)
		h.sendStep(ctx, userID, step)
		return
	}

	isTextTask := step.AnswerType == models.AnswerTypeText
	if isTextTask {
		h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)
		h.msgManager.SendReaction(ctx, userID, "📝 Для этого задания нужно отправить текст")
		h.evaluateAchievementsOnPhotoSubmitted(ctx, userID, true, msg, step)
		return
	}

	h.msgManager.SaveUserAnswerMessageID(userID, msg.ID)
	h.msgManager.CleanupHintMessage(ctx, userID)

	var fileID string
	if len(msg.Photo) > 0 {
		fileID = msg.Photo[len(msg.Photo)-1].FileID
	}

	chatState, err = h.chatStateRepo.Get(userID)
	hintUsed := false
	if err == nil && chatState != nil {
		hintUsed = chatState.CurrentStepHintUsed
	}

	h.answerRepo.CreateImageAnswer(userID, step.ID, []string{fileID}, hintUsed)

	if hintUsed {
		h.chatStateRepo.ResetHintUsed(userID)
	}

	progress, _ = h.progressRepo.GetByUserAndStep(userID, step.ID)
	if progress == nil {
		h.progressRepo.Create(&models.UserProgress{
			UserID: userID,
			StepID: step.ID,
			Status: models.StatusWaitingReview,
		})
	} else {
		h.progressRepo.Update(&models.UserProgress{
			UserID: userID,
			StepID: step.ID,
			Status: models.StatusWaitingReview,
		})
	}

	h.sendToAdminForReview(ctx, userID, step, "", []string{fileID})
	h.msgManager.SendReaction(ctx, userID, "⏳ Ваше фото отправлено на проверку, подождите пока его проверят\\.")

	h.evaluateAchievementsOnPhotoSubmitted(ctx, userID, false, msg, step)
}

func (h *BotHandler) handleAdminDecision(ctx context.Context, callback *tgmodels.CallbackQuery) {
	// log.Printf("[ADMIN_DECISION] starting with data: %s", callback.Data)

	parts := strings.Split(callback.Data, ":")
	// log.Printf("[ADMIN_DECISION] parts: %v", parts)

	if len(parts) != 3 {
		log.Printf("[ADMIN_DECISION] invalid parts length: %d", len(parts))
		return
	}

	action := parts[0]
	userID, _ := parseInt64(parts[1])
	stepID, _ := parseInt64(parts[2])

	// log.Printf("[ADMIN_DECISION] action=%s userID=%d stepID=%d", action, userID, stepID)

	if userID == 0 || stepID == 0 {
		log.Printf("[ADMIN_DECISION] invalid userID or stepID")
		return
	}

	progress, err := h.progressRepo.GetByUserAndStep(userID, stepID)
	if err != nil || progress == nil {
		log.Printf("[ADMIN_DECISION] progress not found: err=%v progress=%v", err, progress)
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		log.Printf("[ADMIN_DECISION] step not found: err=%v step=%v", err, step)
		return
	}

	// log.Printf("[ADMIN_DECISION] found progress and step, proceeding with action: %s", action)

	// user, _ := h.userRepo.GetByID(userID)
	// displayName := fmt.Sprintf("[%d]", userID)
	// if user != nil {
	// 	displayName = user.DisplayName()
	// }

	msg := callback.Message.Message
	if msg == nil {
		return
	}

	switch action {
	case "approve":
		progress.Status = models.StatusApproved
		if err := h.progressRepo.Update(progress); err != nil {
			return
		}

		h.appendToCallbackMessage(ctx, callback, "\n\n✅ Ответ одобрен")

		h.bot.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
		})

		state, _ := h.chatStateRepo.Get(userID)
		if state != nil && state.LastTaskMessageID != 0 {
			h.msgManager.DeleteMessage(ctx, userID, state.LastTaskMessageID)
			h.chatStateRepo.Save(&models.ChatState{
				UserID:                  userID,
				LastTaskMessageID:       0,
				LastUserAnswerMessageID: state.LastUserAnswerMessageID,
				LastReactionMessageID:   state.LastReactionMessageID,
			})
		}

		userAnswer, _ := h.answerRepo.GetUserAnswer(userID, stepID)
		log.Printf("[CALLBACK] userID=%d stepID=%d userAnswer='%s'", userID, stepID, userAnswer)

		percentage, _ := h.answerChecker.CheckTextAnswer(stepID, userAnswer)
		log.Printf("[CALLBACK] percentage=%d", percentage.Percentage)

		h.handleCorrectAnswer(ctx, userID, step, percentage.Percentage, userAnswer)
	case "reject":
		progress.Status = models.StatusRejected
		if err := h.progressRepo.Update(progress); err != nil {
			return
		}

		h.appendToCallbackMessage(ctx, callback, "\n\n❌ Ответ отклонён")

		h.bot.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
		})

		h.msgManager.DeleteUserAnswerAndReaction(ctx, userID)
		settings, _ := h.settingsRepo.GetAll()
		wrongMsg := "❌ Неверно, попробуйте ещё раз"
		if settings != nil && settings.WrongAnswerMessage != "" {
			wrongMsg = services.EscapeUserContent(settings.WrongAnswerMessage)
		}

		wrongEffects := []string{
			"5104858069142078462", // 👎
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
				ParseMode: tgmodels.ParseModeMarkdown,
			})
		} else {
			newText = fmt.Sprintf("🚫 Заблокирован\n👤 %s", displayName)
			h.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: msg.ID,
				Text:      newText,
				ParseMode: tgmodels.ParseModeMarkdown,
			})
		}
	}

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		Text:            "Пользователь заблокирован",
	})
}

// func (h *BotHandler) editCallbackMessage(ctx context.Context, callback *tgmodels.CallbackQuery, newText string) {
// 	msg := callback.Message.Message
// 	if msg == nil {
// 		return
// 	}

// 	if len(msg.Photo) > 0 {
// 		_, err := h.bot.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
// 			ChatID:    msg.Chat.ID,
// 			MessageID: msg.ID,
// 			Caption:   newText,
// 		})
// 		if isMessageNotFoundError(err) {
// 			h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
// 				ChatID:  msg.Chat.ID,
// 				Photo:   &tgmodels.InputFileString{Data: msg.Photo[len(msg.Photo)-1].FileID},
// 				Caption: newText,
// 			})
// 		}
// 	} else {
// 		_, err := h.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
// 			ChatID:    msg.Chat.ID,
// 			MessageID: msg.ID,
// 			Text:      newText,
// 		})
// 		if isMessageNotFoundError(err) {
// 			h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
// 				ChatID: msg.Chat.ID,
// 				Text:   newText,
// 			})
// 		}
// 	}
// }

func (h *BotHandler) appendToCallbackMessage(ctx context.Context, callback *tgmodels.CallbackQuery, appendText string) {
	msg := callback.Message.Message
	if msg == nil {
		return
	}

	if len(msg.Photo) > 0 {
		currentCaption := msg.Caption
		newCaption := currentCaption + appendText

		_, err := h.bot.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Caption:   newCaption,
			ParseMode: tgmodels.ParseModeMarkdown,
		})
		if isMessageNotFoundError(err) {
			h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID:    msg.Chat.ID,
				Photo:     &tgmodels.InputFileString{Data: msg.Photo[len(msg.Photo)-1].FileID},
				Caption:   newCaption,
				ParseMode: tgmodels.ParseModeMarkdown,
			})
		}
	} else {
		currentText := msg.Text
		newText := currentText + appendText

		_, err := h.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: msg.ID,
			Text:      newText,
			ParseMode: tgmodels.ParseModeMarkdown,
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

	caption := fmt.Sprintf("👤 %s\n📋 *Ответ на шаг %d*\n\n📝 _%s_", services.EscapeUserContent(displayName), step.StepOrder, services.EscapeUserContent(step.Text))

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
			Text:        caption + "\n\n💬 `" + services.EscapeUserContent(textAnswer) + "`",
			ReplyMarkup: keyboard,
		})
	} else if len(imageFileIDs) > 0 {
		h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:      h.adminID,
			Photo:       &tgmodels.InputFileString{Data: imageFileIDs[0]},
			Caption:     services.EscapeUserContent(caption),
			ParseMode:   tgmodels.ParseModeMarkdown,
			ReplyMarkup: keyboard,
		})
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

	h.chatStateRepo.ClearAwaitingNextStep(callback.From.ID)

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

	h.evaluateAchievementsOnHintUsed(ctx, userID)
}

func (h *BotHandler) handleSkipStepCallback(ctx context.Context, callback *tgmodels.CallbackQuery) {
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
	if err != nil || step == nil {
		return
	}

	if !step.IsAsterisk {
		h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callback.ID,
			Text:            "Этот шаг нельзя пропустить",
		})
		return
	}

	if err := h.progressRepo.CreateSkipped(userID, stepID); err != nil {
		log.Printf("[HANDLER] Error creating skipped progress: %v", err)
		h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callback.ID,
			Text:            "Ошибка при пропуске шага",
		})
		return
	}

	if callback.Message.Message != nil {
		h.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    callback.Message.Message.Chat.ID,
			MessageID: callback.Message.Message.ID,
		})
	}

	h.moveToNextStep(ctx, userID, step.StepOrder)

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		Text:            "Шаг пропущен",
	})
}

func (h *BotHandler) sendHintMessage(ctx context.Context, userID int64, step *models.Step) (int, error) {
	hintText := strings.TrimSpace(step.HintText)
	if hintText == "" {
		hintText = "Подсказка без текста"
	}

	if step.HintImage != "" {
		msg, err := h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:    userID,
			Photo:     &tgmodels.InputFileString{Data: step.HintImage},
			Caption:   services.EscapeUserContent(hintText),
			ParseMode: tgmodels.ParseModeMarkdown,
		})
		if err != nil {
			log.Printf("[HANDLER] Failed to send hint photo to user %d: %v, sending text instead", userID, err)
			msg, err = h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
				ChatID: userID,
				Text:   services.EscapeUserContent(hintText),
			})
			if err != nil {
				return 0, err
			}
		}

		return msg.ID, nil
	}

	msg, err := h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   services.EscapeUserContent(hintText),
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

func (h *BotHandler) evaluateAchievementsOnCorrectAnswer(ctx context.Context, userID int64, stepID int64) {
	if h.achievementEngine == nil {
		return
	}

	var allAwarded []string

	progressAwarded, err := h.achievementEngine.EvaluateProgressAchievements(userID)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating progress achievements: %v", err)
	} else {
		allAwarded = append(allAwarded, progressAwarded...)
	}

	positionAwarded, err := h.achievementEngine.EvaluatePositionBasedAchievements(userID)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating position achievements: %v", err)
	} else {
		allAwarded = append(allAwarded, positionAwarded...)
	}

	hintAwarded, err := h.achievementEngine.EvaluateHintAchievements(userID)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating hint achievements: %v", err)
	} else {
		allAwarded = append(allAwarded, hintAwarded...)
	}

	asteriskAwarded, err := h.achievementEngine.CheckAsteriskAchievement(userID, stepID)
	if err != nil {
		log.Printf("[HANDLER] Error checking asterisk achievement: %v", err)
	} else {
		allAwarded = append(allAwarded, asteriskAwarded...)
	}

	if len(allAwarded) > 0 {
		compositeAwarded, err := h.achievementEngine.EvaluateCompositeAchievements(userID)
		if err != nil {
			log.Printf("[HANDLER] Error evaluating composite achievements: %v", err)
		} else {
			allAwarded = append(allAwarded, compositeAwarded...)
		}
	}

	h.notifyAchievements(ctx, userID, allAwarded)
}

func (h *BotHandler) evaluateAchievementsOnQuestCompleted(ctx context.Context, userID int64) {
	if h.achievementEngine == nil {
		return
	}

	var allAwarded []string

	completionAwarded, err := h.achievementEngine.EvaluateCompletionAchievements(userID)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating completion achievements: %v", err)
	} else {
		allAwarded = append(allAwarded, completionAwarded...)
	}

	if len(allAwarded) > 0 {
		compositeAwarded, err := h.achievementEngine.EvaluateCompositeAchievements(userID)
		if err != nil {
			log.Printf("[HANDLER] Error evaluating composite achievements: %v", err)
		} else {
			allAwarded = append(allAwarded, compositeAwarded...)
		}
	}

	h.notifyAchievements(ctx, userID, allAwarded)
}

func (h *BotHandler) evaluateAchievementsOnPhotoSubmitted(ctx context.Context, userID int64, isTextTask bool, msg *tgmodels.Message, step *models.Step) {
	if h.achievementEngine == nil {
		return
	}

	awarded, err := h.achievementEngine.OnPhotoSubmitted(userID, isTextTask)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating photo achievements: %v", err)
		return
	}

	h.notifyAchievements(ctx, userID, awarded)

	h.forwardMessageToAdmin(ctx, msg, step, "при отправке изображения на вопрос-текст")
}

func (h *BotHandler) evaluateAchievementsOnPostCompletion(ctx context.Context, userID int64) {
	if h.achievementEngine == nil {
		return
	}

	awarded, err := h.achievementEngine.OnPostCompletionActivity(userID)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating post-completion achievements: %v", err)
		return
	}

	h.notifyAchievements(ctx, userID, awarded)
}

func (h *BotHandler) evaluateAchievementsOnHintUsed(ctx context.Context, userID int64) {
	if h.achievementEngine == nil {
		return
	}

	awarded, err := h.achievementEngine.EvaluateHintAchievements(userID)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating hint achievements: %v", err)
		return
	}

	h.notifyAchievements(ctx, userID, awarded)
}

func (h *BotHandler) notifyAchievements(ctx context.Context, userID int64, achievementKeys []string) {
	if h.achievementNotifier == nil || len(achievementKeys) == 0 {
		return
	}

	if err := h.achievementNotifier.NotifyAchievements(ctx, userID, achievementKeys); err != nil {
		log.Printf("[HANDLER] Error notifying achievements: %v", err)
	}
}

func (h *BotHandler) evaluateSecretAnswer(ctx context.Context, userID int64, answer string) {
	if h.achievementEngine == nil {
		return
	}

	awarded, err := h.achievementEngine.OnAnswerSubmitted(userID, answer)
	if err != nil {
		log.Printf("[HANDLER] Error evaluating secret answer achievements: %v", err)
		return
	}

	h.notifyAchievements(ctx, userID, awarded)
}

func (h *BotHandler) forwardMessageToAdmin(ctx context.Context, msg *tgmodels.Message, step *models.Step, context string) {
	user, _ := h.userRepo.GetByID(msg.From.ID)
	displayName := fmt.Sprintf("[%d]", msg.From.ID)
	if user != nil {
		displayName = user.DisplayName()
	}

	var caption string
	if step != nil {
		stepText := step.Text
		if len(stepText) > 100 {
			stepText = stepText[:100] + "..."
		}
		caption = fmt.Sprintf("💬 Сообщение от %s %s на вопрос \"%s\"", services.EscapeUserContent(displayName), services.EscapeUserContent(context), services.EscapeUserContent(stepText))
	} else {
		caption = fmt.Sprintf("💬 Сообщение от %s %s", services.EscapeUserContent(displayName), services.EscapeUserContent(context))
	}

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "💬 Написать сообщение", CallbackData: fmt.Sprintf("admin:send_message:%d", msg.From.ID)}},
		},
	}

	if len(msg.Photo) > 0 {
		fileID := msg.Photo[len(msg.Photo)-1].FileID
		if msg.Caption != "" {
			caption = caption + "\n\n📝 " + services.EscapeUserContent(msg.Caption)
		}
		h.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:      h.adminID,
			Photo:       &tgmodels.InputFileString{Data: fileID},
			Caption:     caption,
			ParseMode:   tgmodels.ParseModeMarkdown,
			ReplyMarkup: keyboard,
		})
	} else if msg.Text != "" {
		h.msgManager.SendWithRetry(ctx, &bot.SendMessageParams{
			ChatID:      h.adminID,
			Text:        caption + "\n\n💬 " + services.EscapeUserContent(msg.Text),
			ReplyMarkup: keyboard,
		})
	}
}
