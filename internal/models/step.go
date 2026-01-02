package models

import "time"

type Step struct {
	ID           int64
	StepOrder    int
	Text         string
	AnswerType   AnswerType
	HasAutoCheck bool
	IsActive     bool
	IsDeleted    bool
	Images       []StepImage
	Answers      []string
	CreatedAt    time.Time
}

type StepImage struct {
	ID       int64
	StepID   int64
	FileID   string
	Position int
}
