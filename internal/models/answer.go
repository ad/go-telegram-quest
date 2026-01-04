package models

import "time"

type UserAnswer struct {
	ID         int64
	UserID     int64
	StepID     int64
	TextAnswer string
	Images     []AnswerImage
	HintUsed   bool
	CreatedAt  time.Time
}

type AnswerImage struct {
	ID       int64
	AnswerID int64
	FileID   string
	Position int
}
