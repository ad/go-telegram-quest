package services

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/ad/go-telegram-quest/internal/db"
)

type QuestState string

const (
	QuestStateNotStarted QuestState = "not_started"
	QuestStateRunning    QuestState = "running"
	QuestStatePaused     QuestState = "paused"
	QuestStateCompleted  QuestState = "completed"
)

type QuestStateManager struct {
	settingsRepo  *db.SettingsRepository
	userRepo      *db.UserRepository
	progressRepo  *db.ProgressRepository
	answerRepo    *db.AnswerRepository
	chatStateRepo *db.ChatStateRepository
}

func NewQuestStateManager(settingsRepo *db.SettingsRepository) *QuestStateManager {
	return &QuestStateManager{
		settingsRepo: settingsRepo,
	}
}

func NewQuestStateManagerWithRepos(settingsRepo *db.SettingsRepository, userRepo *db.UserRepository, progressRepo *db.ProgressRepository, answerRepo *db.AnswerRepository, chatStateRepo *db.ChatStateRepository) *QuestStateManager {
	return &QuestStateManager{
		settingsRepo:  settingsRepo,
		userRepo:      userRepo,
		progressRepo:  progressRepo,
		answerRepo:    answerRepo,
		chatStateRepo: chatStateRepo,
	}
}

func (m *QuestStateManager) GetCurrentState() (QuestState, error) {
	value, err := m.settingsRepo.Get("quest_state")
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[QUEST_STATE] No quest_state found in database, defaulting to not_started")
			return QuestStateNotStarted, nil
		}
		// log.Printf("[QUEST_STATE] Database error while getting quest state: %v, falling back to running state", err)
		return QuestStateRunning, nil
	}

	state := QuestState(value)
	if !m.isValidState(state) {
		log.Printf("[QUEST_STATE] Invalid state '%s' found in database, falling back to running state", value)
		return QuestStateRunning, nil
	}

	return state, nil
}

func (m *QuestStateManager) SetState(state QuestState) error {
	if !m.isValidState(state) {
		err := fmt.Errorf("invalid quest state: %s", state)
		// log.Printf("[QUEST_STATE] %v", err)
		return err
	}

	// Verify data integrity before state change if repositories are available
	if m.userRepo != nil && m.progressRepo != nil && m.answerRepo != nil && m.chatStateRepo != nil {
		if err := m.verifyDataIntegrity(); err != nil {
			log.Printf("[QUEST_STATE] Data integrity check failed before state change: %v", err)
			return fmt.Errorf("data integrity check failed: %w", err)
		}
	}

	err := m.settingsRepo.Set("quest_state", string(state))
	if err != nil {
		log.Printf("[QUEST_STATE] Database error while setting quest state to '%s': %v", state, err)
		return fmt.Errorf("failed to set quest state: %w", err)
	}

	// Verify data integrity after state change if repositories are available
	if m.userRepo != nil && m.progressRepo != nil && m.answerRepo != nil && m.chatStateRepo != nil {
		if err := m.verifyDataIntegrity(); err != nil {
			log.Printf("[QUEST_STATE] Data integrity check failed after state change: %v", err)
			// Don't return error here as state change was successful, just log the issue
		}
	}

	// log.Printf("[QUEST_STATE] Quest state changed to: %s", state)
	return nil
}

func (m *QuestStateManager) IsUserAllowed(userID int64, isAdmin bool) bool {
	if isAdmin {
		return true
	}

	state, err := m.GetCurrentState()
	if err != nil {
		log.Printf("[QUEST_STATE] Error getting current state for user %d, allowing access: %v", userID, err)
		return true
	}

	return state == QuestStateRunning
}

func (m *QuestStateManager) StartQuest() error {
	return m.SetState(QuestStateRunning)
}

func (m *QuestStateManager) PauseQuest() error {
	return m.SetState(QuestStatePaused)
}

func (m *QuestStateManager) ResumeQuest() error {
	return m.SetState(QuestStateRunning)
}

func (m *QuestStateManager) CompleteQuest() error {
	return m.SetState(QuestStateCompleted)
}

func (m *QuestStateManager) GetStateMessage(state QuestState) string {
	var key string
	switch state {
	case QuestStateNotStarted:
		key = "quest_not_started_message"
	case QuestStatePaused:
		key = "quest_paused_message"
	case QuestStateCompleted:
		key = "quest_completed_message"
	default:
		log.Printf("[QUEST_STATE] No message key for state: %s", state)
		return ""
	}

	message, err := m.settingsRepo.Get(key)
	if err != nil {
		log.Printf("[QUEST_STATE] Database error while getting message for state '%s': %v, using default message", state, err)
		return m.getDefaultMessage(state)
	}
	return message
}

func (m *QuestStateManager) isValidState(state QuestState) bool {
	switch state {
	case QuestStateNotStarted, QuestStateRunning, QuestStatePaused, QuestStateCompleted:
		return true
	default:
		return false
	}
}

func (m *QuestStateManager) getDefaultMessage(state QuestState) string {
	switch state {
	case QuestStateNotStarted:
		return "Квест ещё не начался. Ожидайте объявления о старте!"
	case QuestStatePaused:
		return "Квест временно приостановлен. Скоро мы продолжим!"
	case QuestStateCompleted:
		return "Квест завершён! Спасибо за участие!"
	default:
		return ""
	}
}

func (m *QuestStateManager) verifyDataIntegrity() error {
	// Get all users to verify their data integrity
	users, err := m.userRepo.GetAll()
	if err != nil {
		return fmt.Errorf("failed to get users for integrity check: %w", err)
	}

	for _, user := range users {
		// Verify user data is accessible
		_, err := m.userRepo.GetByID(user.ID)
		if err != nil {
			return fmt.Errorf("user data integrity check failed for user %d: %w", user.ID, err)
		}

		// Verify user progress is accessible
		_, err = m.progressRepo.GetUserProgress(user.ID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("user progress integrity check failed for user %d: %w", user.ID, err)
		}

		// Verify user answers count is accessible
		_, err = m.answerRepo.CountUserAnswers(user.ID)
		if err != nil {
			return fmt.Errorf("user answers integrity check failed for user %d: %w", user.ID, err)
		}

		// Verify chat state is accessible (it's OK if it doesn't exist)
		_, err = m.chatStateRepo.Get(user.ID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("chat state integrity check failed for user %d: %w", user.ID, err)
		}
	}

	return nil
}

func (m *QuestStateManager) VerifyUserDataPreservation(userID int64) error {
	if m.userRepo == nil || m.progressRepo == nil || m.answerRepo == nil || m.chatStateRepo == nil {
		return fmt.Errorf("repositories not initialized for data preservation checks")
	}

	// Verify user exists and is accessible
	_, err := m.userRepo.GetByID(userID)
	if err != nil {
		return fmt.Errorf("user data not preserved for user %d: %w", userID, err)
	}

	// Verify user progress is preserved
	_, err = m.progressRepo.GetUserProgress(userID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("user progress not preserved for user %d: %w", userID, err)
	}

	// Verify user answers are preserved
	_, err = m.answerRepo.CountUserAnswers(userID)
	if err != nil {
		return fmt.Errorf("user answers not preserved for user %d: %w", userID, err)
	}

	// Verify chat state is preserved (it's OK if it doesn't exist)
	_, err = m.chatStateRepo.Get(userID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("chat state not preserved for user %d: %w", userID, err)
	}

	return nil
}
