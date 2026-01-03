package services

type QuestStateMiddleware struct {
	stateManager *QuestStateManager
	adminID      int64
}

func NewQuestStateMiddleware(stateManager *QuestStateManager, adminID int64) *QuestStateMiddleware {
	return &QuestStateMiddleware{
		stateManager: stateManager,
		adminID:      adminID,
	}
}

func (m *QuestStateMiddleware) ShouldProcessMessage(userID int64) (bool, string) {
	isAdmin := userID == m.adminID

	if m.stateManager.IsUserAllowed(userID, isAdmin) {
		return true, ""
	}

	currentState, err := m.stateManager.GetCurrentState()
	if err != nil {
		return true, ""
	}

	notification := m.stateManager.GetStateMessage(currentState)
	return false, notification
}

func (m *QuestStateMiddleware) GetStateNotification() string {
	currentState, err := m.stateManager.GetCurrentState()
	if err != nil {
		return ""
	}

	return m.stateManager.GetStateMessage(currentState)
}
