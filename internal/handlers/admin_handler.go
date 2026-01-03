package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/fsm"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/ad/go-telegram-quest/internal/services"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type AdminHandler struct {
	bot            *bot.Bot
	adminID        int64
	stepRepo       *db.StepRepository
	answerRepo     *db.AnswerRepository
	settingsRepo   *db.SettingsRepository
	adminStateRepo *db.AdminStateRepository
	userManager    *services.UserManager
	userRepo       *db.UserRepository
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
) *AdminHandler {
	return &AdminHandler{
		bot:            b,
		adminID:        adminID,
		stepRepo:       stepRepo,
		answerRepo:     answerRepo,
		settingsRepo:   settingsRepo,
		adminStateRepo: adminStateRepo,
		userManager:    userManager,
		userRepo:       userRepo,
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
	case strings.HasPrefix(data, "admin:edit_step:"):
		h.startEditStep(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:edit_text:"):
		h.startEditStepText(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:delete_step:"):
		h.deleteStep(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:toggle_step:"):
		h.toggleStep(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:answers:"):
		h.showAnswersMenu(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:add_answer:"):
		h.startAddAnswer(ctx, chatID, messageID, data)
	case strings.HasPrefix(data, "admin:del_answer:"):
		h.startDeleteAnswer(ctx, chatID, messageID, data)
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
		ChatID: chatID,
		Text:   text,
	}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}
	_, err := h.bot.SendMessage(ctx, params)
	if err != nil {
		log.Printf("[ADMIN] SendMessage error: %v", err)
	}
}

func (h *AdminHandler) showAdminMenu(ctx context.Context, chatID int64, messageID int) {
	h.adminStateRepo.Clear(h.adminID)

	keyboard := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{{Text: "➕ Добавить шаг", CallbackData: "admin:add_step"}},
			{{Text: "📋 Список шагов", CallbackData: "admin:list_steps"}},
			{{Text: "👥 Участники", CallbackData: "admin:users"}},
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
				{{Text: "« Назад", CallbackData: "admin:menu"}},
			},
		}
		h.editOrSend(ctx, chatID, messageID, "📋 Шагов пока нет", keyboard)
		return
	}

	var buttons [][]tgmodels.InlineKeyboardButton
	for _, step := range steps {
		status := "✅"
		if !step.IsActive {
			status = "⏸️"
		}
		text := fmt.Sprintf("%s Шаг %d", status, step.StepOrder)
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: text, CallbackData: fmt.Sprintf("admin:edit_step:%d", step.ID)},
		})
	}
	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "« Назад", CallbackData: "admin:menu"},
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

	status := "Активен"
	if !step.IsActive {
		status = "Отключён"
	}
	sb.WriteString(fmt.Sprintf("📊 Статус: %s\n", status))

	if hasProgress {
		sb.WriteString("\n⚠️ Шаг уже пройден пользователями")
	}

	var buttons [][]tgmodels.InlineKeyboardButton

	if !hasProgress {
		buttons = append(buttons, []tgmodels.InlineKeyboardButton{
			{Text: "✏️ Изменить текст", CallbackData: fmt.Sprintf("admin:edit_text:%d", stepID)},
		})
	}

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "📝 Варианты ответов", CallbackData: fmt.Sprintf("admin:answers:%d", stepID)},
	})

	toggleText := "⏸️ Отключить"
	if !step.IsActive {
		toggleText = "▶️ Включить"
	}
	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: toggleText, CallbackData: fmt.Sprintf("admin:toggle_step:%d", stepID)},
	})

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "🗑️ Удалить", CallbackData: fmt.Sprintf("admin:delete_step:%d", stepID)},
	})

	buttons = append(buttons, []tgmodels.InlineKeyboardButton{
		{Text: "« Назад", CallbackData: "admin:list_steps"},
	})

	h.editOrSend(ctx, chatID, messageID, sb.String(), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
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
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, ans))
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
		{Text: "« Назад", CallbackData: fmt.Sprintf("admin:edit_step:%d", stepID)},
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
		{{Text: "👋 Приветствие", CallbackData: "admin:edit_setting:welcome_message"}},
		{{Text: "🏁 Финальное", CallbackData: "admin:edit_setting:final_message"}},
		{{Text: "✅ Правильный ответ", CallbackData: "admin:edit_setting:correct_answer_message"}},
		{{Text: "❌ Неправильный ответ", CallbackData: "admin:edit_setting:wrong_answer_message"}},
		{{Text: "« Назад", CallbackData: "admin:menu"}},
	}

	h.editOrSend(ctx, chatID, messageID, sb.String(), &tgmodels.InlineKeyboardMarkup{InlineKeyboard: buttons})
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

	h.editOrSend(ctx, chatID, messageID, fmt.Sprintf("📝 Введите новое %s:\n\nТекущее значение:\n%s\n\n/cancel - отмена", settingName, currentValue), nil)
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
	case fsm.StateAdminEditSettingValue:
		return h.handleEditSettingValue(ctx, msg, state)
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

	keyboard := h.buildUserListKeyboard(result)
	text := fmt.Sprintf("👥 Участники (стр. %d/%d)", result.CurrentPage, result.TotalPages)
	h.editOrSend(ctx, chatID, messageID, text, keyboard)
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

	text := FormatUserDetails(details)
	keyboard := BuildUserDetailsKeyboard(details.User)
	h.editOrSend(ctx, chatID, messageID, text, keyboard)
}

func FormatUserDetails(details *services.UserDetails) string {
	var sb strings.Builder
	sb.WriteString("👤 Информация о пользователе\n\n")

	if details.User.FirstName != "" || details.User.LastName != "" {
		name := strings.TrimSpace(details.User.FirstName + " " + details.User.LastName)
		fmt.Fprintf(&sb, "📛 Имя: %s\n", name)
	}

	if details.User.Username != "" {
		fmt.Fprintf(&sb, "🔗 Username: @%s\n", details.User.Username)
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
		fmt.Fprintf(&sb, "📋 Статус: %s\n", statusText)
	} else {
		sb.WriteString("📊 Прогресс: Не начат\n")
	}

	sb.WriteString("\n")
	if details.User.IsBlocked {
		sb.WriteString("🚫 Статус: Заблокирован")
	} else {
		sb.WriteString("✅ Статус: Активен")
	}

	return sb.String()
}

func BuildUserDetailsKeyboard(user *models.User) *tgmodels.InlineKeyboardMarkup {
	var blockBtn tgmodels.InlineKeyboardButton
	if user.IsBlocked {
		blockBtn = tgmodels.InlineKeyboardButton{Text: "✅ Разблокировать", CallbackData: fmt.Sprintf("unblock:%d", user.ID)}
	} else {
		blockBtn = tgmodels.InlineKeyboardButton{Text: "🚫 Заблокировать", CallbackData: fmt.Sprintf("block:%d", user.ID)}
	}

	return &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
			{blockBtn},
			{{Text: "⬅️ Назад", CallbackData: "admin:userlist"}},
		},
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
