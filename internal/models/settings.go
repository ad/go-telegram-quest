package models

type Settings struct {
	WelcomeMessage       string
	FinalMessage         string
	CorrectAnswerMessage string
	WrongAnswerMessage   string
	RequiredGroupChatID  int64
	GroupChatInviteLink  string
}
