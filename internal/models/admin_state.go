package models

type AdminState struct {
	UserID           int64
	CurrentState     string
	EditingStepID    int64
	NewStepText      string
	NewStepType      AnswerType
	NewStepImages    []string
	NewStepAnswers   []string
	EditingSetting   string
	ImagePosition    int
	ReplacingImageID int64
	NewHintText      string
	TargetUserID     int64
	NewGroupChatID   int64
}
