package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/fsm"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/ad/go-telegram-quest/internal/services"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type AdminHandler struct {
	bot                 *bot.Bot
	adminID             int64
	stepRepo            *db.StepRepository
	answerRepo          *db.AnswerRepository
	settingsRepo        *db.SettingsRepository
	adminStateRepo      *db.AdminStateRepository
	userManager         *services.UserManager
	userRepo            *db.UserRepository
	questStateManager   *services.QuestStateManager
	achievementService  *services.AchievementService
	achievementEngine   *services.AchievementEngine
	achievementNotifier *services.AchievementNotifier
	statsService        *services.StatisticsService
	errorManager        *services.ErrorManager
	dbPath              string
}

func NewAdminHandler(
	b *bot.Bot,
	adminID int64,
	stepRepo *db.StepRepository,
	answerRepo *db.AnswerRepository,
	settingsRepo *db.SettingsRepository,
	adminStateRepo *db.AdminStateRepository,
	userManager *services.UserManager,
	userRepo *db.UserRepository,
	questStateManager *services.QuestStateManager,
	achievementService *services.AchievementService,
	achievementEngine *services.AchievementEngine,
	achievementNotifier *services.AchievementNotifier,
	statsService *services.StatisticsService,
	errorManager *services.ErrorManager,
	dbPath string,
) *AdminHandler {
	return &AdminHandler{
		bot:                 b,
		adminID:             adminID,
		stepRepo:            stepRepo,
		answerRepo:          answerRepo,
		settingsRepo:        settingsRepo,
		adminStateRepo:      adminStateRepo,
		userManager:         userManager,
		userRepo:            userRepo,
		questStateManager:   questStateManager,
		achievementService:  achievementService,
		achievementEngine:   achievementEngine,
		achievementNotifier: achievementNotifier,
		statsService:        statsService,
		errorManager:        errorManager,
		dbPath:              dbPath,
	}
}

func (h *AdminHandler) HandleCommand(ctx context.Context, msg *tgmodels.Message) bool {
	if msg.From.ID != h.adminID {
		return false
	}

	switch msg.Text {
	case "/admin":
		h.showAdminMenu(ctx, msg.Chat.ID, 0)
		return true
	case "/cancel":
		h.cancelOperation(ctx, msg.Chat.ID)
		return true
	}

	state, err := h.adminStateRepo.Get(h.adminID)
	if err != nil || state == nil {
		return false
	}

	return h.handleStateInput(ctx, msg, state)
}

func (h *AdminHandler) HandleCallback(ctx context.Context, callback *tgmodels.CallbackQuery) bool {
	if callback.From.ID != h.adminID {
		return false
	}

	msg := callback.Message.Message
	if msg == nil {
		return false
	}

	h.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
	})

	chatID := msg.Chat.ID
	messageID := msg.ID
	data := callback.Data

	switch {
	case data == "admin:menu":
		h.showAdminMenu(ctx, chatID, messageID)
	case data == "admin:add_step":
		h.startAddStep(ctx, chatID, messageID)
	case data == "admin:list_steps":
		h.showStepsList(ctx, chatID, messageID)
	case data == "admin:users":
		h.showUserList(ctx, chatID, messageID, 1)
	case data == "admin:settings":
		h.showSettingsMenu(ctx, chatID, messageID)
	case data == "admin:group_restriction":
		h.showGroupRestrictionMenu(ctx, chatID, messageID)
	case data == "admin:enable_group_restriction":
		h.startEnableGroupRestriction(ctx, chatID, messageID)
	case data == "admin:disable_group_restriction":
		h.startDisableGroupRestriction(ctx, chatID, messageID)
	case data == "admin:edit_group_id":
		h.startEditGroupID(ctx, chatID, messageID)
	case data == "admin:edit_group_link":
		h.startEditGroupLink(ctx, chatID, messageID)
	case data == "admin:quest_state":
		h.showQuestStateMenu(ctx, chatID, messageID)
	case data == "admin:export_steps":
		h.exportSteps(ctx, chatID, messageID)
	case data == "admin:backup":
		h.createBackup(ctx, chatID, messageID)
	case strings.HasPrefix(data, "admin:quest_state:"):
		h.handleQuestStateChange(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:move_up:"):
		h.moveStepUp(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:move_down:"):
		h.moveStepDown(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:edit_step:"):
		h.startEditStep(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:edit_text:"):
		h.startEditStepText(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:delete_step:"):
		h.deleteStep(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:toggle_step:"):
		h.toggleStep(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:toggle_asterisk:"):
		h.toggleAsterisk(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:answers:"):
		h.showAnswersMenu(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:add_answer:"):
		h.startAddAnswer(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:del_answer:"):
		h.startDeleteAnswer(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:images:"):
		h.showImagesMenu(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:hint:"):
		h.showHintMenu(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:hint_add:"):
		h.startAddHint(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:hint_edit_text:"):
		h.startEditHintText(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:hint_edit_image:"):
		h.startEditHintImage(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:hint_delete:"):
		h.deleteHint(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:add_image:"):
		h.startAddImage(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:replace_image:"):
		h.startReplaceImage(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:delete_image:"):
		h.startDeleteImage(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:add_correct_img:"):
		h.startAddCorrectImage(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:replace_correct_img:"):
		h.startReplaceCorrectImage(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:delete_correct_img:"):
		h.startDeleteCorrectImage(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:edit_setting:"):
		h.startEditSetting(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "userlist:"):
		h.handleUserListNavigation(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "user:"):
		h.showUserDetails(ctx, chatID, messageID, data)
	case data == "admin:userlist":
		h.showUserList(ctx, chatID, messageID, 1)
	case strings.HasPrefix(data, "block:"):
		h.handleBlockFromDetails(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "unblock:"):
		h.handleUnblockFromDetails(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "reset:"):
		h.handleResetFromDetails(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "reset_achievements:"):
		h.handleResetAchievementsFromDetails(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "user_achievements:"):
		h.showUserAchievements(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "award:"):
		h.handleManualAchievementAward(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:send_message:"):
		h.startSendMessage(ctx, chatID, messageID, data)
	case data == "admin:achievement_stats":
		h.showAchievementStatistics(ctx, chatID, messageID)
	case strings.HasPrefix(data, "admin:achievement_leaders"):
		h.showAchievementLeaders(ctx, chatID, messageID)
	case data == "admin:statistics":
		h.showStatistics(ctx, chatID, messageID)
	case data == "admin:step_type:text":
		h.setStepType(ctx, chatID, messageID, models.AnswerTypeText)
	case data == "admin:step_type:image":
		h.setStepType(ctx, chatID, messageID, models.AnswerTypeImage)
	case data == "admin:skip_images":
		h.skipImages(ctx, chatID, messageID)
	case data == "admin:done_images":
		h.doneImages(ctx, chatID, messageID)
	case data == "admin:skip_answers":
		h.skipAnswers(ctx, chatID, messageID)
	case data == "admin:done_answers":
		h.doneAnswers(ctx, chatID, messageID)
	case data == "admin:skip_hint_image":
		h.skipHintImage(ctx, chatID, messageID)
	default:
		return false
	}

	return true
}

func (h *AdminHandler) editOrSend(ctx context.Context, chatID int64, messageID int, text string, keyboard *tgmodels.InlineKeyboardMarkup) {
	if messageID > 0 {
		params := &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: messageID,
			Text:      text,
			ParseMode: tgmodels.ParseModeHTML,
		}
		if keyboard != nil {
			params.ReplyMarkup = keyboard
		}
		_, err := h.bot.EditMessageText(ctx, params)
		if err != nil {
			log.Printf("[ADMIN] EditMessageText error: %v", err)
			h.sendMessage(ctx, chatID, text, keyboard)
		}
	} else {
		h.sendMessage(ctx, chatID, text, keyboard)
	}
}

func (h *AdminHandler) sendMessage(ctx context.Context, chatID int64, text string, keyboard *tgmodels.InlineKeyboardMarkup) {
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: tgmodels.ParseModeHTML,
	}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}
	_, err := h.bot.SendMessage(ctx, params)
	if err != nil {
		log.Printf("[ADMIN] SendMessage error: %v", err)
		if h.errorManager != nil {
			h.errorManager.NotifyAdminWithCurl(
				ctx,
				chatID,
				params,
				fmt.Errorf("%s", "Ошибка telegram"),
			)
		}
	}
}

func (h *AdminHandler) showAdminMenu(ctx context.Context, chatID int64, messageID int) {
	h.adminStateRepo.Clear(h.adminID)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "➕ Добавить шаг", CallbackData: "admin:add_step"}},
			{{Text: "📋 Список шагов", CallbackData: "admin:list_steps"}},
			{{Text: "📤 Экспорт шагов", CallbackData: "admin:export_steps"}},
			{{Text: "👥 Участники", CallbackData: "admin:users"}},
			{{Text: "🏆 Достижения", CallbackData: "admin:achievement_stats"}},
			{{Text: "💾 Бэкап", CallbackData: "admin:backup"}},
			{{Text: "📊 Статистика", CallbackData: "admin:statistics"}},
			{{Text: "⚙️ Настройки", CallbackData: "admin:settings"}},
		},
	}

	h.editOrSend(ctx, chatID, messageID, "🔧 Админ-панель", keyboard)
}

func (h *AdminHandler) cancelOperation(ctx context.Context, chatID int64) {
	h.adminStateRepo.Clear(h.adminID)
	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "❌ Операция отменена",
	})
	h.showAdminMenu(ctx, chatID, 0)
}

func (h *AdminHandler) startAddStep(ctx context.Context, chatID int64, messageID int) {
	state := &models.AdminState{
		UserID:       h.adminID,
		CurrentState: fsm.StateAdminAddStepText,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "📝 Введите текст нового шага:\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) showStepsList(ctx context.Context, chatID int64, messageID int) {
	steps, err := h.stepRepo.GetAll()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении шагов", nil)
		return
	}

	if len(steps) == 0 {
		keyboard := &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
				{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
			},
		}
		h.editOrSend(ctx, chatID, messageID, "📋 Шагов пока нет", keyboard)
		return
	}

	var buttons [][]tgmodels.InlineKeyboardButton
	for _, step := range steps {
		status := ""
		if !step.IsActive {
			status = "⏸️"
		}

		stepText := step.Text
		if step.IsAsterisk {
			stepText = "* " + stepText
		}

		if len([]rune(stepText)) > 30 {
			stepText = string([]rune(stepText)[:30]) + "..."
		}
		text := fmt.Sprintf("%s %s", status, stepText)

		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: text, CallbackData: fmt.Sprintf("admin:edit_step:%d", step.ID)},
		})
	}
	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: "admin:menu"},
	})

	h.editOrSend(ctx, chatID, messageID, "📋 Выберите шаг для редактирования:", &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}

func (h *AdminHandler) startEditStep(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:edit_step:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Шаг не найден", nil)
		return
	}

	hasProgress, _ := h.stepRepo.HasCompletedProgress(stepID)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 Шаг %d\n\n", step.StepOrder))
	sb.WriteString(fmt.Sprintf("📝 Текст: %s\n\n", truncateText(step.Text, 100)))
	sb.WriteString(fmt.Sprintf("📷 Изображений: %d\n", len(step.Images)))
	sb.WriteString(fmt.Sprintf("💬 Тип ответа: %s\n", step.AnswerType))
	sb.WriteString(fmt.Sprintf("✅ Вариантов ответа: %d\n", len(step.Answers)))

	hasHint := step.HasHint()
	if hasHint {
		sb.WriteString("💡 Подсказка: есть\n")
	}

	status := "Активен"
	if !step.IsActive {
		status = "Отключён"
	}
	sb.WriteString(fmt.Sprintf("📊 Статус: %s\n", status))

	if hasProgress {
		sb.WriteString("\n⚠️ Шаг уже пройден некоторыми пользователями")
	}

	var buttons [][]tgmodels.InlineKeyboardButton

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "✏️ Изменить текст", CallbackData: fmt.Sprintf("admin:edit_text:%d", stepID)},
	})

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "📝 Варианты ответов", CallbackData: fmt.Sprintf("admin:answers:%d", stepID)},
	})

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "📷 Изображения", CallbackData: fmt.Sprintf("admin:images:%d", stepID)},
	})

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "💡 Подсказка", CallbackData: fmt.Sprintf("admin:hint:%d", stepID)},
	})

	if step.CorrectAnswerImage == "" {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "➕ Картинка ответа", CallbackData: fmt.Sprintf("admin:add_correct_img:%d", stepID)},
		})
	} else {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🔄 Заменить картинку ответа", CallbackData: fmt.Sprintf("admin:replace_correct_img:%d", stepID)},
		})
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🗑 Удалить картинку ответа", CallbackData: fmt.Sprintf("admin:delete_correct_img:%d", stepID)},
		})
	}

	toggleText := "⏸️ Отключить"
	if !step.IsActive {
		toggleText = "▶️ Включить"
	}
	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: toggleText, CallbackData: fmt.Sprintf("admin:toggle_step:%d", stepID)},
	})

	asteriskText := "⭐ Звёздочка"
	if step.IsAsterisk {
		asteriskText = "Убрать ⭐"
	}
	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: asteriskText, CallbackData: fmt.Sprintf("admin:toggle_asterisk:%d", stepID)},
	})

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "🗑️ Удалить", CallbackData: fmt.Sprintf("admin:delete_step:%d", stepID)},
	})

	// if !hasProgress {
	var moveButtons []tgmodels.InlineKeyboardButton

	if canMoveUp, _ := h.stepRepo.CanMoveUp(stepID); canMoveUp {
		moveButtons = append(moveButtons, tgmodels.InlineKeyboardButton{
			Text: "⬆️ Вверх", CallbackData: fmt.Sprintf("admin:move_up:%d", stepID),
		})
	}

	if canMoveDown, _ := h.stepRepo.CanMoveDown(stepID); canMoveDown {
		moveButtons = append(moveButtons, tgmodels.InlineKeyboardButton{
			Text: "⬇️ Вниз", CallbackData: fmt.Sprintf("admin:move_down:%d", stepID),
		})
	}

	if len(moveButtons) > 0 {
		buttons = append(buttons, moveButtons)
	}
	// }

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: "admin:list_steps"},
	})

	h.editOrSend(ctx, chatID, messageID, html.EscapeString(sb.String()), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}

func (h *AdminHandler) deleteStep(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:delete_step:"))
	if stepID == 0 {
		return
	}

	if err := h.stepRepo.SoftDelete(stepID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при удалении шага", nil)
		return
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Шаг удалён", nil)
	h.showStepsList(ctx, chatID, 0)
}

func (h *AdminHandler) toggleStep(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:toggle_step:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	newActive := !step.IsActive
	if err := h.stepRepo.SetActive(stepID, newActive); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при изменении статуса", nil)
		return
	}

	h.startEditStep(ctx, chatID, messageID, fmt.Sprintf("admin:edit_step:%d", stepID))
}

func (h *AdminHandler) toggleAsterisk(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:toggle_asterisk:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	newAsterisk := !step.IsAsterisk
	if err := h.stepRepo.SetAsterisk(stepID, newAsterisk); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при изменении статуса звёздочки", nil)
		return
	}

	h.startEditStep(ctx, chatID, messageID, fmt.Sprintf("admin:edit_step:%d", stepID))
}

func (h *AdminHandler) showAnswersMenu(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:answers:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📝 Варианты ответов для шага %d:\n\n", step.StepOrder))

	if len(step.Answers) == 0 {
		sb.WriteString("Вариантов пока нет")
	} else {
		for i, ans := range step.Answers {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, html.EscapeString(ans)))
		}
	}

	buttons := [][]tgmodels.InlineKeyboardButton{
		{{Text: "➕ Добавить вариант", CallbackData: fmt.Sprintf("admin:add_answer:%d", stepID)}},
	}

	if len(step.Answers) > 0 {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🗑️ Удалить вариант", CallbackData: fmt.Sprintf("admin:del_answer:%d", stepID)},
		})
	}

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: fmt.Sprintf("admin:edit_step:%d", stepID)},
	})

	h.editOrSend(ctx, chatID, messageID, sb.String(), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}

func (h *AdminHandler) startAddAnswer(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:add_answer:"))
	if stepID == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminAddAnswer,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "📝 Введите новый вариант ответа:\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) startDeleteAnswer(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:del_answer:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil || len(step.Answers) == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminDeleteAnswer,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	var sb strings.Builder
	sb.WriteString("🗑️ Введите номер варианта для удаления:\n\n")
	for i, ans := range step.Answers {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, ans))
	}
	sb.WriteString("\n/cancel - отмена")

	h.editOrSend(ctx, chatID, messageID, sb.String(), nil)
}

func (h *AdminHandler) showSettingsMenu(ctx context.Context, chatID int64, messageID int) {
	settings, err := h.settingsRepo.GetAll()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении настроек", nil)
		return
	}

	var sb strings.Builder
	sb.WriteString("⚙️ Настройки бота\n\n")
	sb.WriteString(fmt.Sprintf("👋 Приветствие: %s\n\n", truncateText(settings.WelcomeMessage, 50)))
	sb.WriteString(fmt.Sprintf("🏁 Финальное сообщение: %s\n\n", truncateText(settings.FinalMessage, 50)))
	sb.WriteString(fmt.Sprintf("✅ Правильный ответ: %s\n\n", truncateText(settings.CorrectAnswerMessage, 50)))
	sb.WriteString(fmt.Sprintf("❌ Неправильный ответ: %s", truncateText(settings.WrongAnswerMessage, 50)))

	buttons := [][]tgmodels.InlineKeyboardButton{
		{{Text: "🎮 Состояние квеста", CallbackData: "admin:quest_state"}},
		{{Text: "🔐 Ограничение участия", CallbackData: "admin:group_restriction"}},
		{{Text: "👋 Приветствие", CallbackData: "admin:edit_setting:welcome_message"}},
		{{Text: "🏁 Финальное", CallbackData: "admin:edit_setting:final_message"}},
		{{Text: "✅ Правильный ответ", CallbackData: "admin:edit_setting:correct_answer_message"}},
		{{Text: "❌ Неправильный ответ", CallbackData: "admin:edit_setting:wrong_answer_message"}},
		{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
	}

	h.editOrSend(ctx, chatID, messageID, html.EscapeString(sb.String()), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}

func (h *AdminHandler) showGroupRestrictionMenu(ctx context.Context, chatID int64, messageID int) {
	groupChatID, err := h.settingsRepo.GetRequiredGroupChatID()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении настроек", nil)
		return
	}

	inviteLink, err := h.settingsRepo.GetGroupChatInviteLink()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении настроек", nil)
		return
	}

	var sb strings.Builder
	sb.WriteString("🔐 Ограничение участия\n\n")

	var buttons [][]tgmodels.InlineKeyboardButton

	if groupChatID == 0 {
		sb.WriteString("❌ Ограничение участия отключено")
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "✅ Включить ограничение", CallbackData: "admin:enable_group_restriction"},
		})
	} else {
		sb.WriteString("✅ Ограничение участия включено\n\n")
		sb.WriteString(fmt.Sprintf("🔐 ID группы: %d\n", groupChatID))
		sb.WriteString(fmt.Sprintf("🔗 Ссылка: %s", truncateText(inviteLink, 50)))

		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "✏️ Изменить ID группы", CallbackData: "admin:edit_group_id"},
		})
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "✏️ Изменить ссылку", CallbackData: "admin:edit_group_link"},
		})
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "❌ Выключить ограничение", CallbackData: "admin:disable_group_restriction"},
		})
	}

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: "admin:settings"},
	})

	h.editOrSend(ctx, chatID, messageID, html.EscapeString(sb.String()), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}

func (h *AdminHandler) startEnableGroupRestriction(ctx context.Context, chatID int64, messageID int) {
	state := &models.AdminState{
		UserID:       h.adminID,
		CurrentState: fsm.StateAdminEnableGroupRestrictionID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "📝 Введите ID группы (например: -1001234567890):\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) startDisableGroupRestriction(ctx context.Context, chatID int64, messageID int) {
	if err := h.settingsRepo.SetRequiredGroupChatID(0); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при отключении ограничения", nil)
		return
	}

	if err := h.settingsRepo.SetGroupChatInviteLink(""); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при отключении ограничения", nil)
		return
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Ограничение участия отключено", nil)
	h.showGroupRestrictionMenu(ctx, chatID, 0)
}

func (h *AdminHandler) startEditGroupID(ctx context.Context, chatID int64, messageID int) {
	groupChatID, err := h.settingsRepo.GetRequiredGroupChatID()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении настроек", nil)
		return
	}

	state := &models.AdminState{
		UserID:       h.adminID,
		CurrentState: fsm.StateAdminEditGroupID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("📝 Введите новый ID группы:\n\nТекущее значение: %d\n\n/cancel - отмена", groupChatID), nil)
}

func (h *AdminHandler) startEditGroupLink(ctx context.Context, chatID int64, messageID int) {
	inviteLink, err := h.settingsRepo.GetGroupChatInviteLink()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении настроек", nil)
		return
	}

	state := &models.AdminState{
		UserID:       h.adminID,
		CurrentState: fsm.StateAdminEditGroupLink,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("📝 Введите новую ссылку на группу:\n\nТекущее значение:\n%s\n\n/cancel - отмена", inviteLink), nil)
}

func (h *AdminHandler) handleEnableGroupRestrictionID(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	var groupChatID int64
	if _, err := fmt.Sscanf(msg.Text, "%d", &groupChatID); err != nil {
		log.Printf("[ADMIN] Failed to parse group chat ID: %v", err)
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Неверный формат ID. Введите число (например: -1001234567890)",
		})
		return true
	}

	if groupChatID >= 0 {
		log.Printf("[ADMIN] Group chat ID must be negative, got: %d", groupChatID)
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ ID группы должен быть отрицательным числом",
		})
		return true
	}

	log.Printf("[ADMIN] Setting NewGroupChatID to: %d", groupChatID)
	state.NewGroupChatID = groupChatID
	state.CurrentState = fsm.StateAdminEnableGroupRestrictionLink
	h.adminStateRepo.Save(state)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "📝 Введите ссылку на группу (например: https://t.me/+AbCdEfGhIjKlMnOp):\n\n/cancel - отмена",
	})
	return true
}

func (h *AdminHandler) handleEnableGroupRestrictionLink(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	inviteLink := strings.TrimSpace(msg.Text)
	if inviteLink == "" {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ссылка не может быть пустой",
		})
		return true
	}

	if !strings.HasPrefix(inviteLink, "https://t.me/") {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ссылка должна начинаться с https://t.me/",
		})
		return true
	}

	log.Printf("[ADMIN] Saving group chat ID: %d", state.NewGroupChatID)
	if err := h.settingsRepo.SetRequiredGroupChatID(state.NewGroupChatID); err != nil {
		log.Printf("[ADMIN] Failed to save group chat ID: %v", err)
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при сохранении ID группы",
		})
		return true
	}

	log.Printf("[ADMIN] Saving invite link: %s", inviteLink)
	if err := h.settingsRepo.SetGroupChatInviteLink(inviteLink); err != nil {
		log.Printf("[ADMIN] Failed to save invite link: %v", err)
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при сохранении ссылки",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	log.Printf("[ADMIN] Group restriction enabled successfully")
	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Ограничение участия включено",
	})
	h.showGroupRestrictionMenu(ctx, msg.Chat.ID, 0)
	return true
}

func (h *AdminHandler) handleEditGroupID(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	var groupChatID int64
	if _, err := fmt.Sscanf(msg.Text, "%d", &groupChatID); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Неверный формат ID. Введите число (например: -1001234567890)",
		})
		return true
	}

	if groupChatID >= 0 {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ ID группы должен быть отрицательным числом",
		})
		return true
	}

	if err := h.settingsRepo.SetRequiredGroupChatID(groupChatID); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при сохранении ID группы",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ ID группы обновлён",
	})
	h.showGroupRestrictionMenu(ctx, msg.Chat.ID, 0)
	return true
}

func (h *AdminHandler) handleEditGroupLink(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	inviteLink := strings.TrimSpace(msg.Text)
	if inviteLink == "" {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ссылка не может быть пустой",
		})
		return true
	}

	if !strings.HasPrefix(inviteLink, "https://t.me/") {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ссылка должна начинаться с https://t.me/",
		})
		return true
	}

	if err := h.settingsRepo.SetGroupChatInviteLink(inviteLink); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при сохранении ссылки",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Ссылка на группу обновлена",
	})
	h.showGroupRestrictionMenu(ctx, msg.Chat.ID, 0)
	return true
}

func (h *AdminHandler) startEditSetting(ctx context.Context, chatID int64, messageID int, data string) {
	settingKey := strings.TrimPrefix(data, "admin:edit_setting:")

	state := &models.AdminState{
		UserID:         h.adminID,
		CurrentState:   fsm.StateAdminEditSettingValue,
		EditingSetting: settingKey,
	}
	h.adminStateRepo.Save(state)

	settingName := map[string]string{
		"welcome_message":        "приветствие",
		"final_message":          "финальное сообщение",
		"correct_answer_message": "сообщение о правильном ответе",
		"wrong_answer_message":   "сообщение о неправильном ответе",
	}[settingKey]

	currentValue, _ := h.settingsRepo.Get(settingKey)

	h.editOrSend(
		ctx,
		chatID,
		messageID,
		fmt.Sprintf(
			"📝 Введите новое %s:\n\nТекущее значение:\n<pre>%s</pre>\n\n/cancel - отмена",
			settingName,
			html.EscapeString(currentValue),
		),
		nil,
	)
}

func (h *AdminHandler) handleStateInput(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	switch state.CurrentState {
	case fsm.StateAdminAddStepText:
		return h.handleAddStepText(ctx, msg, state)
	case fsm.StateAdminAddStepType:
		return false
	case fsm.StateAdminAddStepImages:
		return h.handleAddStepImages(ctx, msg, state)
	case fsm.StateAdminAddStepAnswers:
		return h.handleAddStepAnswers(ctx, msg, state)
	case fsm.StateAdminEditStepText:
		return h.handleEditStepText(ctx, msg, state)
	case fsm.StateAdminAddAnswer:
		return h.handleAddAnswer(ctx, msg, state)
	case fsm.StateAdminDeleteAnswer:
		return h.handleDeleteAnswer(ctx, msg, state)
	case fsm.StateAdminAddImage:
		return h.handleAddImage(ctx, msg, state)
	case fsm.StateAdminReplaceImage:
		return h.handleReplaceImage(ctx, msg, state)
	case fsm.StateAdminDeleteImage:
		return h.handleDeleteImage(ctx, msg, state)
	case fsm.StateAdminAddCorrectImage:
		return h.handleAddCorrectImage(ctx, msg, state)
	case fsm.StateAdminReplaceCorrectImage:
		return h.handleReplaceCorrectImage(ctx, msg, state)
	case fsm.StateAdminEditSettingValue:
		return h.handleEditSettingValue(ctx, msg, state)
	case fsm.StateAdminAddHintText:
		return h.handleAddHintText(ctx, msg, state)
	case fsm.StateAdminAddHintImage:
		return h.handleAddHintImage(ctx, msg, state)
	case fsm.StateAdminEditHintText:
		return h.handleEditHintText(ctx, msg, state)
	case fsm.StateAdminEditHintImage:
		return h.handleEditHintImage(ctx, msg, state)
	case fsm.StateAdminSendMessage:
		return h.handleSendMessage(ctx, msg, state)
	case fsm.StateAdminEnableGroupRestrictionID:
		return h.handleEnableGroupRestrictionID(ctx, msg, state)
	case fsm.StateAdminEnableGroupRestrictionLink:
		return h.handleEnableGroupRestrictionLink(ctx, msg, state)
	case fsm.StateAdminEditGroupID:
		return h.handleEditGroupID(ctx, msg, state)
	case fsm.StateAdminEditGroupLink:
		return h.handleEditGroupLink(ctx, msg, state)
	}
	return false
}

func (h *AdminHandler) startEditStepText(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:edit_text:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminEditStepText,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("📝 Введите новый текст для шага %d:\n\nТекущий текст:\n%s\n\n/cancel - отмена", step.StepOrder, step.Text), nil)
}

func (h *AdminHandler) handleAddStepText(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	state.NewStepText = msg.Text
	state.CurrentState = fsm.StateAdminAddStepType
	h.adminStateRepo.Save(state)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{
				{Text: "📝 Текст", CallbackData: "admin:step_type:text"},
				{Text: "📷 Изображение", CallbackData: "admin:step_type:image"},
			},
		},
	}

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        "📋 Выберите тип ответа для этого шага:",
		ReplyMarkup: keyboard,
	})
	return true
}

func (h *AdminHandler) setStepType(ctx context.Context, chatID int64, messageID int, answerType models.AnswerType) {
	state, _ := h.adminStateRepo.Get(h.adminID)
	if state == nil || state.CurrentState != fsm.StateAdminAddStepType {
		return
	}

	state.NewStepType = answerType
	state.CurrentState = fsm.StateAdminAddStepImages
	h.adminStateRepo.Save(state)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "⏭️ Пропустить", CallbackData: "admin:skip_images"}},
		},
	}

	h.editOrSend(ctx, chatID, messageID, "📷 Отправьте изображения для шага (можно несколько):\n\nИли нажмите «Пропустить»", keyboard)
}

func (h *AdminHandler) handleAddStepImages(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if len(msg.Photo) == 0 {
		return false
	}

	fileID := msg.Photo[len(msg.Photo)-1].FileID
	state.NewStepImages = append(state.NewStepImages, fileID)
	h.adminStateRepo.Save(state)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "✅ Готово", CallbackData: "admin:done_images"}},
		},
	}

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        fmt.Sprintf("📷 Добавлено изображений: %d\n\nОтправьте ещё или нажмите «Готово»", len(state.NewStepImages)),
		ReplyMarkup: keyboard,
	})
	return true
}

func (h *AdminHandler) skipImages(ctx context.Context, chatID int64, messageID int) {
	state, _ := h.adminStateRepo.Get(h.adminID)
	if state == nil || state.CurrentState != fsm.StateAdminAddStepImages {
		return
	}

	h.proceedToAnswers(ctx, chatID, messageID, state)
}

func (h *AdminHandler) doneImages(ctx context.Context, chatID int64, messageID int) {
	state, _ := h.adminStateRepo.Get(h.adminID)
	if state == nil || state.CurrentState != fsm.StateAdminAddStepImages {
		return
	}

	h.proceedToAnswers(ctx, chatID, messageID, state)
}

func (h *AdminHandler) proceedToAnswers(ctx context.Context, chatID int64, messageID int, state *models.AdminState) {
	if state.NewStepType == models.AnswerTypeImage {
		h.createStep(ctx, chatID, messageID, state)
		return
	}

	state.CurrentState = fsm.StateAdminAddStepAnswers
	h.adminStateRepo.Save(state)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "⏭️ Пропустить (ручная проверка)", CallbackData: "admin:skip_answers"}},
		},
	}

	h.editOrSend(ctx, chatID, messageID, "📝 Введите варианты правильных ответов (по одному в сообщении):\n\nИли нажмите «Пропустить» для ручной проверки", keyboard)
}

func (h *AdminHandler) handleAddStepAnswers(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	state.NewStepAnswers = append(state.NewStepAnswers, msg.Text)
	h.adminStateRepo.Save(state)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "✅ Готово", CallbackData: "admin:done_answers"}},
		},
	}

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        fmt.Sprintf("📝 Добавлено вариантов: %d\n\nВведите ещё или нажмите «Готово»", len(state.NewStepAnswers)),
		ReplyMarkup: keyboard,
	})
	return true
}

func (h *AdminHandler) skipAnswers(ctx context.Context, chatID int64, messageID int) {
	state, _ := h.adminStateRepo.Get(h.adminID)
	if state == nil || state.CurrentState != fsm.StateAdminAddStepAnswers {
		return
	}

	h.createStep(ctx, chatID, messageID, state)
}

func (h *AdminHandler) doneAnswers(ctx context.Context, chatID int64, messageID int) {
	state, _ := h.adminStateRepo.Get(h.adminID)
	if state == nil || state.CurrentState != fsm.StateAdminAddStepAnswers {
		return
	}

	h.createStep(ctx, chatID, messageID, state)
}

func (h *AdminHandler) createStep(ctx context.Context, chatID int64, messageID int, state *models.AdminState) {
	maxOrder, _ := h.stepRepo.GetMaxOrder()

	step := &models.Step{
		StepOrder:    maxOrder + 1,
		Text:         state.NewStepText,
		AnswerType:   state.NewStepType,
		HasAutoCheck: len(state.NewStepAnswers) > 0,
		IsActive:     true,
		IsDeleted:    false,
	}

	stepID, err := h.stepRepo.Create(step)
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при создании шага", nil)
		return
	}

	for i, fileID := range state.NewStepImages {
		h.stepRepo.AddImage(stepID, fileID, i)
	}

	for _, answer := range state.NewStepAnswers {
		h.stepRepo.AddAnswer(stepID, answer)
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("✅ Шаг %d создан!", step.StepOrder),
	})
	h.showAdminMenu(ctx, chatID, 0)
}

func (h *AdminHandler) handleEditStepText(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	if err := h.stepRepo.UpdateText(state.EditingStepID, msg.Text); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при обновлении текста",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Текст шага обновлён",
	})
	h.showStepsList(ctx, msg.Chat.ID, 0)
	return true
}

func (h *AdminHandler) handleAddAnswer(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	if err := h.answerRepo.AddStepAnswer(state.EditingStepID, msg.Text); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при добавлении варианта",
		})
		return true
	}

	step, _ := h.stepRepo.GetByID(state.EditingStepID)
	if step != nil && !step.HasAutoCheck {
		step.HasAutoCheck = true
		h.stepRepo.Update(step)
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Вариант ответа добавлен",
	})
	h.showStepsList(ctx, msg.Chat.ID, 0)
	return true
}

func (h *AdminHandler) handleDeleteAnswer(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	var num int
	if _, err := fmt.Sscanf(msg.Text, "%d", &num); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Введите номер варианта",
		})
		return true
	}

	step, err := h.stepRepo.GetByID(state.EditingStepID)
	if err != nil || step == nil || num < 1 || num > len(step.Answers) {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Неверный номер варианта",
		})
		return true
	}

	answerToDelete := step.Answers[num-1]
	if err := h.answerRepo.DeleteStepAnswer(state.EditingStepID, answerToDelete); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при удалении варианта",
		})
		return true
	}

	if len(step.Answers) == 1 {
		step.HasAutoCheck = false
		h.stepRepo.Update(step)
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Вариант ответа удалён",
	})
	h.showStepsList(ctx, msg.Chat.ID, 0)
	return true
}

func (h *AdminHandler) handleEditSettingValue(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	if err := h.settingsRepo.Set(state.EditingSetting, msg.Text); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при сохранении настройки",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Настройка сохранена",
	})
	h.showSettingsMenu(ctx, msg.Chat.ID, 0)
	return true
}

func (h *AdminHandler) startAddCorrectImage(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:add_correct_img:"))
	if stepID == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminAddCorrectImage,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "📷 Отправьте изображение для правильного ответа:\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) handleAddCorrectImage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if len(msg.Photo) == 0 {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Отправьте изображение",
		})
		return true
	}

	fileID := msg.Photo[len(msg.Photo)-1].FileID
	if err := h.stepRepo.UpdateCorrectAnswerImage(state.EditingStepID, fileID); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при сохранении изображения",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)
	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Изображение сохранено",
	})
	h.startEditStep(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:edit_step:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) startReplaceCorrectImage(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:replace_correct_img:"))
	if stepID == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminReplaceCorrectImage,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "🔄 Отправьте новое изображение для правильного ответа:\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) handleReplaceCorrectImage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if len(msg.Photo) == 0 {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Отправьте изображение",
		})
		return true
	}

	fileID := msg.Photo[len(msg.Photo)-1].FileID
	if err := h.stepRepo.UpdateCorrectAnswerImage(state.EditingStepID, fileID); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при замене изображения",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)
	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Изображение заменено",
	})
	h.startEditStep(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:edit_step:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) startDeleteCorrectImage(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:delete_correct_img:"))
	if stepID == 0 {
		return
	}

	if err := h.stepRepo.UpdateCorrectAnswerImage(stepID, ""); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при удалении изображения", nil)
		return
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Изображение удалено", nil)
	h.startEditStep(ctx, chatID, 0, fmt.Sprintf("admin:edit_step:%d", stepID))
}

func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}

func (h *AdminHandler) showUserList(ctx context.Context, chatID int64, messageID int, page int) {
	result, err := h.userManager.GetUserListPage(page)
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении списка пользователей", nil)
		return
	}

	if len(result.Users) == 0 {
		keyboard := &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
				{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
			},
		}
		h.editOrSend(ctx, chatID, messageID, "👥 Участников пока нет", keyboard)
		return
	}

	// Get quest statistics
	stats, err := h.userManager.GetQuestStatistics()
	if err != nil {
		log.Printf("[ADMIN] Error getting quest statistics: %v", err)
		// Continue without statistics
	}

	var text strings.Builder
	if result.TotalPages > 1 {
		text.WriteString(fmt.Sprintf("👥 Участники (стр. %d/%d)\n\n", result.CurrentPage, result.TotalPages))
	} else {
		text.WriteString("👥 <b>Участники</b>\n\n")
	}

	// Display statistics if available
	if stats != nil {
		text.WriteString("📊 <b>Общая статистика</b>\n")
		text.WriteString(fmt.Sprintf("👤 Всего участников: %d\n", stats.TotalUsers))
		text.WriteString(fmt.Sprintf("✅ Завершили квест: %d\n", stats.CompletedUsers))
		text.WriteString(fmt.Sprintf("🔄 В процессе: %d\n", stats.InProgressUsers))

		if stats.NotStartedUsers > 0 {
			text.WriteString(fmt.Sprintf("⏸️ Не начали: %d\n", stats.NotStartedUsers))
		}

		// Show distribution by steps if there are users in progress
		if len(stats.StepDistribution) > 0 {
			text.WriteString("\n📍 <b>Распределение по шагам</b>\n")

			// Sort step orders for consistent display
			var stepOrders []int
			for stepOrder := range stats.StepDistribution {
				stepOrders = append(stepOrders, stepOrder)
			}

			// Simple bubble sort for small arrays
			for i := 0; i < len(stepOrders); i++ {
				for j := i + 1; j < len(stepOrders); j++ {
					if stepOrders[i] > stepOrders[j] {
						stepOrders[i], stepOrders[j] = stepOrders[j], stepOrders[i]
					}
				}
			}

			for _, stepOrder := range stepOrders {
				count := stats.StepDistribution[stepOrder]
				title := stats.StepTitles[stepOrder]

				// Truncate step text for the list
				displayTitle := title
				if len([]rune(displayTitle)) > 40 {
					displayTitle = string([]rune(displayTitle)[:40]) + "..."
				}
				// Remove newlines to keep it on one line if any
				displayTitle = strings.ReplaceAll(displayTitle, "\n", " ")

				text.WriteString(fmt.Sprintf("   %d. %s: %d чел.\n", stepOrder, html.EscapeString(displayTitle), count))
			}
		}

		text.WriteString("\n")
	}

	keyboard := h.buildUserListKeyboard(result)
	h.editOrSend(ctx, chatID, messageID, text.String(), keyboard)
}

func (h *AdminHandler) buildUserListKeyboard(page *services.UserListPage) *tgmodels.InlineKeyboardMarkup {
	var rows [][]tgmodels.InlineKeyboardButton

	for i := 0; i < len(page.Users); i += 2 {
		row := []tgmodels.InlineKeyboardButton{
			{Text: page.Users[i].DisplayName(), CallbackData: fmt.Sprintf("user:%d", page.Users[i].ID)},
		}
		if i+1 < len(page.Users) {
			row = append(row, tgmodels.InlineKeyboardButton{
				Text:         page.Users[i+1].DisplayName(),
				CallbackData: fmt.Sprintf("user:%d", page.Users[i+1].ID),
			})
		}
		rows = append(rows, row)
	}

	var navRow []tgmodels.InlineKeyboardButton
	if page.HasPrev {
		navRow = append(navRow, tgmodels.InlineKeyboardButton{Text: "◀️", CallbackData: fmt.Sprintf("userlist:%d", page.CurrentPage-1)})
	}
	if page.HasNext {
		navRow = append(navRow, tgmodels.InlineKeyboardButton{Text: "▶️", CallbackData: fmt.Sprintf("userlist:%d", page.CurrentPage+1)})
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	rows = append(rows, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: "admin:menu"},
	})

	return &tgmodels.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (h *AdminHandler) handleUserListNavigation(ctx context.Context, chatID int64, messageID int, data string) {
	page, _ := parseInt64(strings.TrimPrefix(data, "userlist:"))
	if page < 1 {
		page = 1
	}
	h.showUserList(ctx, chatID, messageID, int(page))
}

func (h *AdminHandler) showUserDetails(ctx context.Context, chatID int64, messageID int, data string) {
	userID, _ := parseInt64(strings.TrimPrefix(data, "user:"))
	if userID == 0 {
		return
	}

	details, err := h.userManager.GetUserDetails(userID)
	if err != nil || details == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Пользователь не найден", nil)
		return
	}

	if h.achievementService != nil {
		count, err := h.achievementService.GetUserAchievementCount(userID)
		if err == nil {
			details.AchievementCount = count
		}

		summary, err := h.achievementService.GetUserAchievementSummary(userID)
		if err == nil && summary != nil {
			for _, achievements := range summary.AchievementsByCategory {
				for _, a := range achievements {
					details.Achievements = append(details.Achievements, &services.UserAchievementInfo{
						Name:     a.Achievement.Name,
						Category: a.Achievement.Category,
					})
				}
			}
		}
	}

	text := FormatUserDetails(h, details)

	keyboard := BuildUserDetailsKeyboard(details.User, true)
	h.editOrSend(ctx, chatID, messageID, text, keyboard)
}

func FormatUserDetails(h *AdminHandler, details *services.UserDetails) string {
	var sb strings.Builder
	sb.WriteString("👤 <b>Информация о пользователе</b>\n\n")

	if details.User.FirstName != "" || details.User.LastName != "" {
		name := strings.TrimSpace(details.User.FirstName + " " + details.User.LastName)
		fmt.Fprintf(&sb, "📛 Имя: %s\n", html.EscapeString(name))
	}

	if details.User.Username != "" {
		fmt.Fprintf(&sb, "🔗 Username: @%s\n", html.EscapeString(details.User.Username))
	}

	fmt.Fprintf(&sb, "🆔 ID: %d\n\n", details.User.ID)

	if details.IsCompleted {
		sb.WriteString("📊 Прогресс: ✅ Квест завершён\n")
	} else if details.CurrentStep != nil {
		fmt.Fprintf(&sb, "📊 Прогресс: Шаг %d\n", details.CurrentStep.StepOrder)
		statusText := map[models.ProgressStatus]string{
			models.StatusPending:       "⏳ Ожидает ответа",
			models.StatusWaitingReview: "🔍 На проверке",
			models.StatusApproved:      "✅ Одобрен",
			models.StatusRejected:      "❌ Отклонён",
		}[details.Status]
		if statusText != "" {
			fmt.Fprintf(&sb, "📋 Статус: %s\n", statusText)
		}
	} else {
		sb.WriteString("📊 Прогресс: Не начат\n")
	}

	if details.AchievementCount > 0 {
		fmt.Fprintf(&sb, "\n🏆 <b>Достижений</b> - %d\n", details.AchievementCount)
		for _, a := range details.Achievements {
			fmt.Fprintf(&sb, "  • %s\n", html.EscapeString(a.Name))
		}
	}

	if details.Statistics != nil {
		sb.WriteString("\n")
		sb.WriteString(services.FormatUserStatistics(details.Statistics, details.IsCompleted))
	}

	if h.statsService != nil {
		answeredAsterisk, totalAsterisk, err := h.statsService.GetUserAsteriskStats(details.User.ID)
		if err == nil && totalAsterisk > 0 {
			sb.WriteString(fmt.Sprintf("\n⭐ Вопросы со звёздочкой: %d из %d\n", answeredAsterisk, totalAsterisk))
		}
	}

	sb.WriteString("\n")
	if details.User.IsBlocked {
		sb.WriteString("🚫 Статус: Заблокирован")
	} else {
		sb.WriteString("✅ Статус: Активен")
	}

	return sb.String()
}

func BuildUserDetailsKeyboard(user *models.User, isAdmin bool) *tgmodels.InlineKeyboardMarkup {
	var buttons [][]tgmodels.InlineKeyboardButton

	// Only show admin functions if user has admin privileges
	if isAdmin {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🏆 Достижения", CallbackData: fmt.Sprintf("user_achievements:%d", user.ID)},
		})

		// Message button
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "💬 Написать сообщение", CallbackData: fmt.Sprintf("admin:send_message:%d", user.ID)},
		})

		// Block/unblock button
		var blockBtn tgmodels.InlineKeyboardButton
		if user.IsBlocked {
			blockBtn = tgmodels.InlineKeyboardButton{Text: "✅ Разблокировать", CallbackData: fmt.Sprintf("unblock:%d", user.ID)}
		} else {
			blockBtn = tgmodels.InlineKeyboardButton{Text: "🚫 Заблокировать", CallbackData: fmt.Sprintf("block:%d", user.ID)}
		}
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{blockBtn})

		// Reset button
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🔄 Сбросить прогресс", CallbackData: fmt.Sprintf("reset:%d", user.ID)},
		})

		// Reset achievements button (separate row)
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🏅 Сбросить достижения", CallbackData: fmt.Sprintf("reset_achievements:%d", user.ID)},
		})
	}

	// Back button - always shown
	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: "admin:userlist"},
	})

	return &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}
}

func (h *AdminHandler) handleBlockFromDetails(ctx context.Context, chatID int64, messageID int, data string) {
	userID, _ := parseInt64(strings.TrimPrefix(data, "block:"))
	if userID == 0 {
		return
	}

	if err := h.userRepo.BlockUser(userID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при блокировке пользователя", nil)
		return
	}

	h.showUserDetails(ctx, chatID, messageID, fmt.Sprintf("user:%d", userID))
}

func (h *AdminHandler) handleUnblockFromDetails(ctx context.Context, chatID int64, messageID int, data string) {
	userID, _ := parseInt64(strings.TrimPrefix(data, "unblock:"))
	if userID == 0 {
		return
	}

	if err := h.userRepo.UnblockUser(userID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при разблокировке пользователя", nil)
		return
	}

	h.showUserDetails(ctx, chatID, messageID, fmt.Sprintf("user:%d", userID))
}

func (h *AdminHandler) handleResetFromDetails(ctx context.Context, chatID int64, messageID int, data string) {
	userID, _ := parseInt64(strings.TrimPrefix(data, "reset:"))
	if userID == 0 {
		return
	}

	if err := h.userManager.ResetUserProgress(userID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при сбросе прогресса", nil)
		return
	}

	if h.achievementEngine != nil {
		if _, err := h.achievementEngine.RecalculatePositionAchievements(); err != nil {
			log.Printf("[ADMIN] Error recalculating position achievements: %v", err)
		}
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Прогресс и достижения пользователя сброшены", nil)
	h.showUserDetails(ctx, chatID, 0, fmt.Sprintf("user:%d", userID))
}

func (h *AdminHandler) handleResetAchievementsFromDetails(ctx context.Context, chatID int64, messageID int, data string) {
	userID, _ := parseInt64(strings.TrimPrefix(data, "reset_achievements:"))
	if userID == 0 {
		return
	}

	if h.achievementEngine == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Система достижений недоступна", nil)
		return
	}

	if err := h.achievementEngine.ResetUserAchievements(userID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при сбросе достижений", nil)
		return
	}

	if _, err := h.achievementEngine.RecalculatePositionAchievements(); err != nil {
		log.Printf("[ADMIN] Error recalculating position achievements: %v", err)
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Достижения пользователя сброшены", nil)
	h.showUserDetails(ctx, chatID, 0, fmt.Sprintf("user:%d", userID))
}

func (h *AdminHandler) handleManualAchievementAward(ctx context.Context, chatID int64, messageID int, data string) {
	// Verify caller has admin privileges - additional security check
	// Note: This is already checked in HandleCallback, but we add it here for defense in depth
	if chatID != h.adminID {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Недостаточно прав для выполнения операции", nil)
		return
	}

	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Неверный формат данных", nil)
		return
	}

	achievementKey := parts[1]
	userID, err := parseInt64(parts[2])
	if err != nil || userID == 0 {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Неверный ID пользователя", nil)
		return
	}

	if h.achievementEngine == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Система достижений недоступна", nil)
		return
	}

	if err := h.achievementEngine.AwardManualAchievement(userID, achievementKey, h.adminID); err != nil {
		h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("⚠️ Ошибка при присвоении достижения: %v", err), nil)
		return
	}

	achievementNames := map[string]string{
		"veteran":  "Ветеран игр",
		"activity": "За активность",
		"wow":      "Вау! За отличный ответ",
	}

	achievementName := achievementNames[achievementKey]
	if achievementName == "" {
		achievementName = achievementKey
	}

	h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("✅ Достижение \"%s\" присвоено пользователю", achievementName), nil)

	// Отправляем уведомление пользователю
	h.notifyAchievements(ctx, userID, []string{achievementKey})

	h.showUserAchievements(ctx, chatID, 0, fmt.Sprintf("user_achievements:%d", userID))
}

func (h *AdminHandler) showQuestStateMenu(ctx context.Context, chatID int64, messageID int) {
	currentState, err := h.questStateManager.GetCurrentState()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении состояния квеста", nil)
		return
	}

	stateNames := map[services.QuestState]string{
		services.QuestStateNotStarted: "Не начат",
		services.QuestStateRunning:    "Запущен",
		services.QuestStatePaused:     "На паузе",
		services.QuestStateCompleted:  "Завершён",
	}

	var sb strings.Builder
	sb.WriteString("🎮 Управление состоянием квеста\n\n")
	sb.WriteString(fmt.Sprintf("Текущее состояние: %s\n\n", stateNames[currentState]))
	sb.WriteString("Выберите новое состояние:")

	buttons := [][]tgmodels.InlineKeyboardButton{
		{{Text: "🔄 Не начат", CallbackData: "admin:quest_state:not_started"}},
		{{Text: "▶️ Запустить", CallbackData: "admin:quest_state:running"}},
		{{Text: "⏸️ Пауза", CallbackData: "admin:quest_state:paused"}},
		{{Text: "🏁 Завершить", CallbackData: "admin:quest_state:completed"}},
		{{Text: "⬅️ Назад", CallbackData: "admin:settings"}},
	}

	h.editOrSend(ctx, chatID, messageID, sb.String(), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}

func (h *AdminHandler) handleQuestStateChange(ctx context.Context, chatID int64, messageID int, data string) {
	stateStr := strings.TrimPrefix(data, "admin:quest_state:")
	newState := services.QuestState(stateStr)

	if err := h.questStateManager.SetState(newState); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при изменении состояния квеста", nil)
		return
	}

	stateNames := map[services.QuestState]string{
		services.QuestStateNotStarted: "не начат",
		services.QuestStateRunning:    "запущен",
		services.QuestStatePaused:     "поставлен на паузу",
		services.QuestStateCompleted:  "завершён",
	}

	message := fmt.Sprintf("✅ Квест %s", stateNames[newState])
	h.editOrSend(ctx, chatID, messageID, message, nil)
	h.showQuestStateMenu(ctx, chatID, 0)
}
func (h *AdminHandler) moveStepUp(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:move_up:"))
	if stepID == 0 {
		return
	}

	if err := h.stepRepo.MoveStepUp(stepID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при перемещении шага", nil)
		return
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Шаг перемещён вверх", nil)
	h.showStepsList(ctx, chatID, 0)
}

func (h *AdminHandler) moveStepDown(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:move_down:"))
	if stepID == 0 {
		return
	}

	if err := h.stepRepo.MoveStepDown(stepID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при перемещении шага", nil)
		return
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Шаг перемещён вниз", nil)
	h.showStepsList(ctx, chatID, 0)
}

func (h *AdminHandler) showImagesMenu(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:images:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📷 Изображения для шага %d:\n\n", step.StepOrder))

	if len(step.Images) == 0 {
		sb.WriteString("Изображений пока нет")
	} else {
		for i, img := range step.Images {
			sb.WriteString(fmt.Sprintf("%d. Изображение (ID: %s)\n", i+1, img.FileID[:10]+"..."))
		}
	}

	buttons := [][]tgmodels.InlineKeyboardButton{
		{{Text: "➕ Добавить изображение", CallbackData: fmt.Sprintf("admin:add_image:%d", stepID)}},
	}

	if len(step.Images) > 0 {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🔄 Заменить изображение", CallbackData: fmt.Sprintf("admin:replace_image:%d", stepID)},
		})
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🗑️ Удалить изображение", CallbackData: fmt.Sprintf("admin:delete_image:%d", stepID)},
		})
	}

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: fmt.Sprintf("admin:edit_step:%d", stepID)},
	})

	h.editOrSend(ctx, chatID, messageID, sb.String(), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}

func (h *AdminHandler) startAddImage(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:add_image:"))
	if stepID == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminAddImage,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "📷 Отправьте изображение для добавления:\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) startReplaceImage(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:replace_image:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil || len(step.Images) == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminReplaceImage,
		EditingStepID: stepID,
		ImagePosition: -1,
	}
	h.adminStateRepo.Save(state)

	var sb strings.Builder
	sb.WriteString("🔄 Введите номер изображения для замены:\n\n")
	for i, img := range step.Images {
		sb.WriteString(fmt.Sprintf("%d. Изображение (ID: %s)\n", i+1, img.FileID[:10]+"..."))
	}
	sb.WriteString("\n/cancel - отмена")

	h.editOrSend(ctx, chatID, messageID, sb.String(), nil)
}

func (h *AdminHandler) startDeleteImage(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:delete_image:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil || len(step.Images) == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminDeleteImage,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	var sb strings.Builder
	sb.WriteString("🗑️ Введите номер изображения для удаления:\n\n")
	for i, img := range step.Images {
		sb.WriteString(fmt.Sprintf("%d. Изображение (ID: %s)\n", i+1, img.FileID[:10]+"..."))
	}
	sb.WriteString("\n/cancel - отмена")

	h.editOrSend(ctx, chatID, messageID, sb.String(), nil)
}

func (h *AdminHandler) handleAddImage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if len(msg.Photo) == 0 {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Отправьте изображение",
		})
		return true
	}

	fileID := msg.Photo[len(msg.Photo)-1].FileID
	imageCount, _ := h.stepRepo.GetImageCount(state.EditingStepID)

	if err := h.stepRepo.AddImage(state.EditingStepID, fileID, imageCount); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при добавлении изображения",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Изображение добавлено",
	})
	h.showImagesMenu(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:images:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) handleReplaceImage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if state.ImagePosition < 0 {
		var num int
		if _, err := fmt.Sscanf(msg.Text, "%d", &num); err != nil {
			h.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   "⚠️ Введите номер изображения",
			})
			return true
		}

		step, err := h.stepRepo.GetByID(state.EditingStepID)
		if err != nil || step == nil || num < 1 || num > len(step.Images) {
			h.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   "⚠️ Неверный номер изображения",
			})
			return true
		}

		state.ImagePosition = num - 1
		h.adminStateRepo.Save(state)

		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "📷 Теперь отправьте новое изображение:",
		})
		return true
	}

	if len(msg.Photo) == 0 {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Отправьте изображение",
		})
		return true
	}

	fileID := msg.Photo[len(msg.Photo)-1].FileID

	if err := h.stepRepo.ReplaceImage(state.EditingStepID, state.ImagePosition, fileID); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при замене изображения",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Изображение заменено",
	})
	h.showImagesMenu(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:images:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) handleDeleteImage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	var num int
	if _, err := fmt.Sscanf(msg.Text, "%d", &num); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Введите номер изображения",
		})
		return true
	}

	step, err := h.stepRepo.GetByID(state.EditingStepID)
	if err != nil || step == nil || num < 1 || num > len(step.Images) {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Неверный номер изображения",
		})
		return true
	}

	if err := h.stepRepo.DeleteImage(state.EditingStepID, num-1); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при удалении изображения",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Изображение удалено",
	})
	h.showImagesMenu(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:images:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) showHintMenu(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:hint:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💡 Подсказка для шага %d\n\n", step.StepOrder))

	if step.HasHint() {
		sb.WriteString("✅ Подсказка установлена\n\n")
		if step.HintText != "" {
			hintPreview := step.HintText
			if len([]rune(hintPreview)) > 100 {
				hintPreview = string([]rune(hintPreview)[:100]) + "..."
			}
			sb.WriteString(fmt.Sprintf("📝 Текст: %s\n", hintPreview))
		}
		if step.HintImage != "" {
			sb.WriteString("🖼 Изображение: есть\n")
		}
	} else {
		sb.WriteString("❌ Подсказка не установлена")
	}

	var buttons [][]tgmodels.InlineKeyboardButton

	if step.HasHint() {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "✏️ Редактировать текст", CallbackData: fmt.Sprintf("admin:hint_edit_text:%d", stepID)},
		})
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🖼 Редактировать изображение", CallbackData: fmt.Sprintf("admin:hint_edit_image:%d", stepID)},
		})
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "🗑 Удалить подсказку", CallbackData: fmt.Sprintf("admin:hint_delete:%d", stepID)},
		})
	} else {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "➕ Добавить подсказку", CallbackData: fmt.Sprintf("admin:hint_add:%d", stepID)},
		})
	}

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: fmt.Sprintf("admin:edit_step:%d", stepID)},
	})

	h.editOrSend(ctx, chatID, messageID, sb.String(), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
}
func (h *AdminHandler) startAddHint(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:hint_add:"))
	if stepID == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminAddHintText,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "📝 Введите текст подсказки:\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) handleAddHintText(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	state.NewHintText = msg.Text
	state.CurrentState = fsm.StateAdminAddHintImage
	h.adminStateRepo.Save(state)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "⏭️ Пропустить", CallbackData: "admin:skip_hint_image"}},
		},
	}

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        "🖼 Отправьте изображение для подсказки (опционально):\n\nИли нажмите «Пропустить»",
		ReplyMarkup: keyboard,
	})
	return true
}

func (h *AdminHandler) handleAddHintImage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	var hintImage string
	if len(msg.Photo) > 0 {
		hintImage = msg.Photo[len(msg.Photo)-1].FileID
	}

	if err := h.stepRepo.UpdateHint(state.EditingStepID, state.NewHintText, hintImage); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при сохранении подсказки",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Подсказка добавлена",
	})
	h.showHintMenu(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:hint:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) startEditHintText(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:hint_edit_text:"))
	if stepID == 0 {
		return
	}

	step, err := h.stepRepo.GetByID(stepID)
	if err != nil || step == nil {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminEditHintText,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("📝 Введите новый текст подсказки:\n\nТекущий текст:\n%s\n\n/cancel - отмена", step.HintText), nil)
}

func (h *AdminHandler) handleEditHintText(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if msg.Text == "" {
		return false
	}

	step, err := h.stepRepo.GetByID(state.EditingStepID)
	if err != nil || step == nil {
		return false
	}

	if err := h.stepRepo.UpdateHint(state.EditingStepID, msg.Text, step.HintImage); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при обновлении текста подсказки",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Текст подсказки обновлён",
	})
	h.showHintMenu(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:hint:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) startEditHintImage(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:hint_edit_image:"))
	if stepID == 0 {
		return
	}

	state := &models.AdminState{
		UserID:        h.adminID,
		CurrentState:  fsm.StateAdminEditHintImage,
		EditingStepID: stepID,
	}
	h.adminStateRepo.Save(state)

	h.editOrSend(ctx, chatID, messageID, "🖼 Отправьте новое изображение для подсказки:\n\n/cancel - отмена", nil)
}

func (h *AdminHandler) handleEditHintImage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	if len(msg.Photo) == 0 {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Отправьте изображение",
		})
		return true
	}

	step, err := h.stepRepo.GetByID(state.EditingStepID)
	if err != nil || step == nil {
		return false
	}

	fileID := msg.Photo[len(msg.Photo)-1].FileID
	if err := h.stepRepo.UpdateHint(state.EditingStepID, step.HintText, fileID); err != nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Ошибка при обновлении изображения подсказки",
		})
		return true
	}

	h.adminStateRepo.Clear(h.adminID)

	h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "✅ Изображение подсказки обновлено",
	})
	h.showHintMenu(ctx, msg.Chat.ID, 0, fmt.Sprintf("admin:hint:%d", state.EditingStepID))
	return true
}

func (h *AdminHandler) deleteHint(ctx context.Context, chatID int64, messageID int, data string) {
	stepID, _ := parseInt64(strings.TrimPrefix(data, "admin:hint_delete:"))
	if stepID == 0 {
		return
	}

	if err := h.stepRepo.ClearHint(stepID); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при удалении подсказки", nil)
		return
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Подсказка удалена", nil)
	h.showHintMenu(ctx, chatID, 0, fmt.Sprintf("admin:hint:%d", stepID))
}
func (h *AdminHandler) skipHintImage(ctx context.Context, chatID int64, messageID int) {
	state, _ := h.adminStateRepo.Get(h.adminID)
	if state == nil || state.CurrentState != fsm.StateAdminAddHintImage {
		return
	}

	if err := h.stepRepo.UpdateHint(state.EditingStepID, state.NewHintText, ""); err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при сохранении подсказки", nil)
		return
	}

	h.adminStateRepo.Clear(h.adminID)

	h.editOrSend(ctx, chatID, messageID, "✅ Подсказка добавлена", nil)
	h.showHintMenu(ctx, chatID, 0, fmt.Sprintf("admin:hint:%d", state.EditingStepID))
}

func (h *AdminHandler) exportSteps(ctx context.Context, chatID int64, messageID int) {
	steps, err := h.stepRepo.GetAll()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении заданий", nil)
		return
	}

	if len(steps) == 0 {
		keyboard := &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
				{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
			},
		}
		h.editOrSend(ctx, chatID, messageID, "📋 Заданий пока нет", keyboard)
		return
	}

	activeCount := 0
	for _, step := range steps {
		if step.IsActive {
			activeCount++
		}
	}

	const maxMessageLength = 6000
	var currentMessage strings.Builder

	for i, step := range steps {
		stepText := h.formatStepForExport(step)

		if currentMessage.Len()+len(stepText) > maxMessageLength && currentMessage.Len() > 0 {
			h.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      currentMessage.String(),
				ParseMode: tgmodels.ParseModeHTML,
			})
			currentMessage.Reset()
		}

		currentMessage.WriteString(stepText)

		if i < len(steps)-1 {
			currentMessage.WriteString("\n")
		}
	}

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
		},
	}

	if currentMessage.Len() > 0 {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        currentMessage.String(),
			ReplyMarkup: keyboard,
			ParseMode:   tgmodels.ParseModeHTML,
		})
	} /* else {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        "✅ <b>Экспорт завершен</b>",
			ReplyMarkup: keyboard,
			ParseMode:   tgmodels.ParseModeHTML,
		})
	}*/
}

func (h *AdminHandler) formatStepForExport(step *models.Step) string {
	var stepData strings.Builder
	stepText := step.Text
	if step.IsAsterisk {
		stepText = "⭐ " + stepText
	}
	stepData.WriteString("<b>" + stepText + "</b>\n")

	if step.HasHint() {
		stepData.WriteString("<b>Подсказка:</b> ")
		if step.HintText != "" {
			stepData.WriteString("<i>" + strings.ReplaceAll(step.HintText, "\n", " ") + "</i>\n")
		} else {
			stepData.WriteString("🖼️ <i>изображение</i>\n")
		}
	}

	if len(step.Answers) > 0 {
		stepData.WriteString("<b>Ответы:</b> ")
		stepData.WriteString(strings.Join(step.Answers, ", ") + "\n")
	}

	stepData.WriteString("\n")

	return stepData.String()
}

func (h *AdminHandler) showUserAchievements(ctx context.Context, chatID int64, messageID int, data string) {
	// Verify caller has admin privileges - additional security check
	if chatID != h.adminID {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Недостаточно прав для выполнения операции", nil)
		return
	}

	userID, _ := parseInt64(strings.TrimPrefix(data, "user_achievements:"))
	if userID == 0 {
		return
	}

	if h.achievementService == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Система достижений недоступна", nil)
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Пользователь не найден", nil)
		return
	}

	summary, err := h.achievementService.GetUserAchievementSummary(userID)
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении достижений", nil)
		return
	}

	text := h.FormatUserAchievements(user, summary, userID)

	// Создаём кнопки для ручных достижений, которые ещё не выданы - только для админов
	var buttons [][]tgmodels.InlineKeyboardButton

	manualAchievements := []struct {
		key   string
		name  string
		emoji string
	}{
		{"veteran", "Ветеран", "🛡️"},
		{"activity", "Активность", "🪩"},
		{"wow", "Вау", "💎"},
	}

	for _, achievement := range manualAchievements {
		hasAchievement, err := h.achievementService.HasUserAchievement(userID, achievement.key)
		if err != nil {
			continue // Пропускаем при ошибке
		}

		if !hasAchievement {
			button := tgmodels.InlineKeyboardButton{
				Text:         fmt.Sprintf("%s %s", achievement.emoji, achievement.name),
				CallbackData: fmt.Sprintf("award:%s:%d", achievement.key, userID),
			}
			buttons = append(buttons, []tgmodels.InlineKeyboardButton{button})
		}
	}

	// Добавляем кнопку "Назад"
	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "⬅️ Назад к пользователю", CallbackData: fmt.Sprintf("user:%d", userID)},
	})

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}

	h.editOrSend(ctx, chatID, messageID, text, keyboard)
}

func (h *AdminHandler) FormatUserAchievements(user *models.User, summary *services.UserAchievementSummary, userID int64) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏆 <b>Достижения пользователя</b>\n %s\n\n", html.EscapeString(user.DisplayName())))

	if summary.TotalCount == 0 {
		sb.WriteString("У пользователя пока нет достижений")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("<b>Всего достижений</b>: %d\n\n", summary.TotalCount))

	categoryNames := map[models.AchievementCategory]string{
		models.CategoryProgress:   "📈 Прогресс",
		models.CategoryCompletion: "🏁 Завершение",
		models.CategorySpecial:    "⭐ Особые",
		models.CategoryHints:      "💡 Подсказки",
		models.CategoryComposite:  "🎖️ Составные",
		models.CategoryUnique:     "👑 Уникальные",
	}

	categoryOrder := []models.AchievementCategory{
		models.CategoryUnique,
		models.CategoryComposite,
		models.CategoryCompletion,
		models.CategoryProgress,
		models.CategoryHints,
		models.CategorySpecial,
	}

	for _, category := range categoryOrder {
		achievements, exists := summary.AchievementsByCategory[category]
		if !exists || len(achievements) == 0 {
			continue
		}

		categoryName := categoryNames[category]
		sb.WriteString(fmt.Sprintf("<b>%s</b>\n", html.EscapeString(categoryName)))

		for _, details := range achievements {
			sb.WriteString(fmt.Sprintf("  • %s %s\n", html.EscapeString(details.Achievement.Name), html.EscapeString(details.EarnedAt)))
		}
		sb.WriteString("\n")
	}

	// Добавляем ссылку на стикерпак если он есть
	if h.achievementNotifier != nil {
		stickerPackMessage := h.achievementNotifier.FormatStickerPackMessage(userID)
		if stickerPackMessage != "" {
			sb.WriteString(html.EscapeString(stickerPackMessage))
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// FormatUserAchievements - функция для тестов
func FormatUserAchievements(user *models.User, summary *services.UserAchievementSummary) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏆 Достижения пользователя %s\n\n", html.EscapeString(user.DisplayName())))

	if summary.TotalCount == 0 {
		sb.WriteString("У пользователя пока нет достижений")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Всего достижений: %d\n\n", summary.TotalCount))

	categoryNames := map[models.AchievementCategory]string{
		models.CategoryProgress:   "📈 Прогресс",
		models.CategoryCompletion: "🏁 Завершение",
		models.CategorySpecial:    "⭐ Особые",
		models.CategoryHints:      "💡 Подсказки",
		models.CategoryComposite:  "🎖️ Составные",
		models.CategoryUnique:     "👑 Уникальные",
	}

	categoryOrder := []models.AchievementCategory{
		models.CategoryUnique,
		models.CategoryComposite,
		models.CategoryCompletion,
		models.CategoryProgress,
		models.CategoryHints,
		models.CategorySpecial,
	}

	for _, category := range categoryOrder {
		achievements, exists := summary.AchievementsByCategory[category]
		if !exists || len(achievements) == 0 {
			continue
		}

		categoryName := categoryNames[category]
		sb.WriteString(fmt.Sprintf("%s:\n", categoryName))

		for _, details := range achievements {
			sb.WriteString(fmt.Sprintf("  • %s\n", html.EscapeString(details.Achievement.Name)))
			sb.WriteString(fmt.Sprintf("    %s\n", details.EarnedAt))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (h *AdminHandler) showAchievementStatistics(ctx context.Context, chatID int64, messageID int) {
	if h.achievementService == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Система достижений недоступна", nil)
		return
	}

	stats, err := h.achievementService.GetAchievementStatistics()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении статистики", nil)
		return
	}

	text := h.FormatAchievementStatistics(stats)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "🏅 Лидеры по достижениям", CallbackData: "admin:achievement_leaders"}},
			{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
		},
	}

	h.editOrSend(ctx, chatID, messageID, text, keyboard)
}

func (h *AdminHandler) FormatAchievementStatistics(stats *services.AchievementStatistics) string {
	var sb strings.Builder
	sb.WriteString("🏆 <b>Статистика достижений</b>\n\n")

	sb.WriteString("📊 <b>Общая информация</b>\n")
	sb.WriteString(fmt.Sprintf("• Выдано достижений: %d\n", stats.TotalUserAchievements))
	sb.WriteString(fmt.Sprintf("• Участников: %d\n\n", stats.TotalUsers))

	categoryNames := map[models.AchievementCategory]string{
		models.CategoryProgress:   "📈 Прогресс",
		models.CategoryCompletion: "🏁 Завершение",
		models.CategorySpecial:    "⭐ Особые",
		models.CategoryHints:      "💡 Подсказки",
		models.CategoryComposite:  "🎖️ Составные",
		models.CategoryUnique:     "👑 Уникальные",
	}

	sb.WriteString("📁 <b>По категориям</b>\n")
	for category, count := range stats.AchievementsByCategory {
		name := categoryNames[category]
		sb.WriteString(fmt.Sprintf("• %s: %d\n", html.EscapeString(name), count))
	}
	sb.WriteString("\n")

	if len(stats.PopularAchievements) > 0 {
		sb.WriteString(fmt.Sprintf("🏆 <b>Все достижения</b> (%d)\n", stats.TotalAchievements))

		// Get achievement notifier to access emoji mapping
		var achievementNotifier *services.AchievementNotifier
		if h.achievementNotifier != nil {
			achievementNotifier = h.achievementNotifier
		}

		for _, pop := range stats.PopularAchievements {
			emoji := "🏅" // default emoji
			if achievementNotifier != nil {
				emoji = achievementNotifier.GetAchievementEmoji(pop.Achievement)
			}

			sb.WriteString(
				fmt.Sprintf(
					"%s <b>%s</b> (%d)\n",
					emoji,
					html.EscapeString(pop.Achievement.Name),
					pop.UserCount,
				),
			)
		}
	}

	return sb.String()
}

func (h *AdminHandler) showAchievementLeaders(ctx context.Context, chatID int64, messageID int) {
	if h.achievementService == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Система достижений недоступна", nil)
		return
	}

	rankings, err := h.achievementService.GetUsersWithMostAchievements(15)
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Ошибка при получении рейтинга", nil)
		return
	}

	text := FormatAchievementLeaders(rankings)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "⬅️ Назад к статистике", CallbackData: "admin:achievement_stats"}},
		},
	}

	h.editOrSend(ctx, chatID, messageID, text, keyboard)
}

func FormatAchievementLeaders(rankings []services.UserAchievementRanking) string {
	var sb strings.Builder
	sb.WriteString("🏅 <b>Лидеры по достижениям</b>\n\n")

	if len(rankings) == 0 {
		sb.WriteString("Пока нет пользователей с достижениями")
		return sb.String()
	}

	for i, ranking := range rankings {
		medal := ""
		switch i {
		case 0:
			medal = "🥇 "
		case 1:
			medal = "🥈 "
		case 2:
			medal = "🥉 "
		default:
			medal = fmt.Sprintf("%d. ", i+1)
		}

		sb.WriteString(
			fmt.Sprintf(
				"%s%s: %d\n",
				medal,
				html.EscapeString(ranking.User.DisplayName()),
				ranking.AchievementCount,
			),
		)
	}

	return sb.String()
}
func (h *AdminHandler) createBackup(ctx context.Context, chatID int64, messageID int) {
	h.editOrSend(ctx, chatID, messageID, "💾 <i>Создаю бэкап базы данных...</i>", nil)

	log.Printf("[BACKUP] Starting backup for database: %s", h.dbPath)

	backupData, err := h.generateSQLDump()
	if err != nil {
		log.Printf("[BACKUP] Backup failed: %v", err)
		h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("⚠️ Ошибка при создании бэкапа: %v", err), nil)
		return
	}

	log.Printf("[BACKUP] Backup generated successfully, size: %d bytes", len(backupData))

	filename := fmt.Sprintf("quest_backup_%s.sql", time.Now().Format("2006-01-02_15-04-05"))

	params := &bot.SendDocumentParams{
		ChatID: chatID,
		Document: &tgmodels.InputFileUpload{
			Filename: filename,
			Data:     strings.NewReader(backupData),
		},
		ParseMode: tgmodels.ParseModeHTML,
		Caption:   fmt.Sprintf("💾 <b>Бэкап базы данных</b>\n\n📅 Создан: %s", html.EscapeString(time.Now().Format("02.01.2006 15:04:05"))),
	}

	_, err = h.bot.SendDocument(ctx, params)
	if err != nil {
		log.Printf("[BACKUP] Failed to send document: %v", err)
		h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("⚠️ Ошибка при отправке файла: %v", err), nil)
		return
	}

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
		},
	}

	h.editOrSend(ctx, chatID, messageID, "✅ Бэкап успешно создан и отправлен", keyboard)
}

func (h *AdminHandler) generateSQLDump() (string, error) {
	// Сначала пробуем sqlite3 .dump
	cmd := exec.Command("sqlite3", h.dbPath, ".dump")
	output, err := cmd.CombinedOutput() // Используем CombinedOutput для получения stderr
	if err != nil {
		// Логируем детали ошибки для диагностики
		log.Printf("[BACKUP] sqlite3 command failed: %v, output: %s", err, string(output))

		// Если sqlite3 недоступен, используем Go-реализацию
		return h.generateSQLDumpGo()
	}
	return string(output), nil
}

func (h *AdminHandler) generateSQLDumpGo() (string, error) {
	sqlDB, err := sql.Open("sqlite", h.dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return "", fmt.Errorf("failed to open database: %w", err)
	}
	defer sqlDB.Close()

	var dump strings.Builder
	dump.WriteString("PRAGMA foreign_keys=OFF;\n")
	dump.WriteString("BEGIN TRANSACTION;\n\n")

	// Получаем список всех таблиц
	rows, err := sqlDB.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return "", fmt.Errorf("failed to get tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return "", err
		}
		tables = append(tables, tableName)
	}

	// Дампим каждую таблицу
	for _, table := range tables {
		if err := h.dumpTableGo(sqlDB, &dump, table); err != nil {
			return "", fmt.Errorf("failed to dump table %s: %w", table, err)
		}
	}

	dump.WriteString("COMMIT;\n")
	return dump.String(), nil
}

func (h *AdminHandler) dumpTableGo(db *sql.DB, dump *strings.Builder, tableName string) error {
	// Получаем CREATE TABLE
	rows, err := db.Query("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", tableName)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		var createSQL string
		if err := rows.Scan(&createSQL); err != nil {
			return err
		}
		if createSQL != "" {
			dump.WriteString(createSQL + ";\n")
		}
	}

	// Получаем данные
	dataRows, err := db.Query(fmt.Sprintf("SELECT * FROM %s", tableName))
	if err != nil {
		return err
	}
	defer dataRows.Close()

	columns, err := dataRows.Columns()
	if err != nil {
		return err
	}

	for dataRows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := dataRows.Scan(valuePtrs...); err != nil {
			return err
		}

		dump.WriteString(fmt.Sprintf("INSERT INTO %s VALUES(", tableName))
		for i, val := range values {
			if i > 0 {
				dump.WriteString(",")
			}
			if val == nil {
				dump.WriteString("NULL")
			} else {
				switch v := val.(type) {
				case string:
					dump.WriteString(fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''")))
				case []byte:
					dump.WriteString(fmt.Sprintf("'%s'", strings.ReplaceAll(string(v), "'", "''")))
				default:
					dump.WriteString(fmt.Sprintf("%v", v))
				}
			}
		}
		dump.WriteString(");\n")
	}

	dump.WriteString("\n")
	return nil
}

func (h *AdminHandler) showStatistics(ctx context.Context, chatID int64, messageID int) {
	stats, err := h.statsService.CalculateStats()
	if err != nil {
		h.editOrSend(ctx, chatID, messageID, "❌ Ошибка получения статистики", nil)
		return
	}

	var sb strings.Builder
	sb.WriteString("📊 <b>Статистика квеста</b>\n\n")

	sb.WriteString("📋 <b>Прогресс по шагам</b>\n")
	for _, s := range stats.StepStats {
		sb.WriteString(fmt.Sprintf("%d. %s:  %d чел\n", s.StepOrder, html.EscapeString(truncateText(s.Text, 20)), s.Count))
	}

	asteriskStats, err := h.statsService.GetAsteriskStepsStats()
	if err != nil {
		log.Printf("[ADMIN] Error GetAsteriskStepsStats: %v", err)
	} else if len(asteriskStats) > 0 {
		sb.WriteString("\n⭐ <b>Вопросы со звёздочкой</b>\n")
		totalAsterisk := len(asteriskStats)
		sb.WriteString(fmt.Sprintf("Всего вопросов: %d\n", totalAsterisk))
		for _, as := range asteriskStats {
			sb.WriteString(
				fmt.Sprintf(
					"%d. %s: ответили %d, пропустили %d\n",
					as.StepOrder,
					html.EscapeString(truncateText(as.Text, 20)),
					as.AnsweredCount,
					as.SkippedCount,
				),
			)
		}
	}

	if len(stats.Leaders) > 0 {
		sb.WriteString("\n🏆 <b>Лидеры</b>\n")
		maxLeaders := 10
		if len(stats.Leaders) < maxLeaders {
			maxLeaders = len(stats.Leaders)
		}
		for i := 0; i < maxLeaders; i++ {
			sb.WriteString(
				fmt.Sprintf(
					"  %d. %s\n",
					i+1,
					html.EscapeString(stats.Leaders[i].DisplayName()),
				),
			)
		}
	}

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "⬅️ Назад", CallbackData: "admin:menu"}},
		},
	}

	h.editOrSend(ctx, chatID, messageID, sb.String(), keyboard)
}

func (h *AdminHandler) notifyAchievements(ctx context.Context, userID int64, achievementKeys []string) {
	if h.achievementNotifier == nil || len(achievementKeys) == 0 {
		return
	}

	if err := h.achievementNotifier.NotifyAchievements(ctx, userID, achievementKeys); err != nil {
		log.Printf("[ADMIN] Error notifying achievements: %v", err)
	}
}

func (h *AdminHandler) startSendMessage(ctx context.Context, chatID int64, messageID int, data string) {
	userIDStr := strings.TrimPrefix(data, "admin:send_message:")
	userID, err := parseInt64(userIDStr)
	if err != nil || userID == 0 {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Неверный ID пользователя", nil)
		return
	}

	// Verify that the target user exists
	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		h.editOrSend(ctx, chatID, messageID, "⚠️ Пользователь не найден", nil)
		return
	}

	// Create admin state with target user ID
	state := &models.AdminState{
		UserID:       h.adminID,
		CurrentState: fsm.StateAdminSendMessage,
		TargetUserID: userID,
	}
	h.adminStateRepo.Save(state)

	// Display instructions with /cancel option
	instructions := fmt.Sprintf("💬 Отправка сообщения пользователю %s\n\n📝 Введите текст сообщения:\n\n/cancel - отмена операции", html.EscapeString(user.DisplayName()))
	h.editOrSend(ctx, chatID, messageID, instructions, nil)
}

func (h *AdminHandler) handleSendMessage(ctx context.Context, msg *tgmodels.Message, state *models.AdminState) bool {
	// Check for cancel command
	if msg.Text == "/cancel" {
		h.adminStateRepo.Clear(h.adminID)
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "❌ Отправка сообщения отменена",
		})
		h.showUserDetails(ctx, msg.Chat.ID, 0, fmt.Sprintf("user:%d", state.TargetUserID))
		return true
	}

	// Validate input text (non-empty)
	if strings.TrimSpace(msg.Text) == "" {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "⚠️ Сообщение не может быть пустым. Введите текст сообщения или /cancel для отмены.",
		})
		return true
	}

	// Send message to target user
	h.sendMessageToUser(ctx, msg.Chat.ID, state.TargetUserID, msg.Text)
	return true
}

func (h *AdminHandler) sendMessageToUser(ctx context.Context, adminChatID int64, targetUserID int64, message string) {
	// Get target user information
	user, err := h.userRepo.GetByID(targetUserID)
	if err != nil || user == nil {
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminChatID,
			Text:   "⚠️ Ошибка: пользователь не найден",
		})
		h.adminStateRepo.Clear(h.adminID)
		return
	}

	// Send message to target user
	_, err = h.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: targetUserID,
		Text:   message,
	})

	// Award achievement for receiving message from admin
	if err == nil && h.achievementEngine != nil {
		awarded, achievementErr := h.achievementEngine.OnMessageFromAdmin(targetUserID)
		if achievementErr != nil {
			log.Printf("[ADMIN] Error awarding message from admin achievement: %v", achievementErr)
		} else if len(awarded) > 0 {
			h.notifyAchievements(ctx, targetUserID, awarded)
		}
	}

	// Clear admin state
	h.adminStateRepo.Clear(h.adminID)

	// Show status to administrator
	if err != nil {
		statusMessage := fmt.Sprintf("❌ Ошибка при отправке сообщения пользователю %s:\n%v", html.EscapeString(user.DisplayName()), err)
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminChatID,
			Text:   statusMessage,
		})
	} else {
		statusMessage := fmt.Sprintf("✅ Сообщение успешно отправлено пользователю %s", html.EscapeString(user.DisplayName()))
		h.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    adminChatID,
			Text:      statusMessage,
			ParseMode: tgmodels.ParseModeHTML,
		})
	}

	// Return to user details screen
	h.showUserDetails(ctx, adminChatID, 0, fmt.Sprintf("user:%d", targetUserID))
}
