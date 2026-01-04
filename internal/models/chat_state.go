package models

type ChatState struct {
	UserID                  int64
	LastTaskMessageID       int
	LastUserAnswerMessageID int
	LastReactionMessageID   int
	HintMessageID           int
	CurrentStepHintUsed     bool
}
