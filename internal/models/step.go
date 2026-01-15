package models

import "time"

type Step struct {
	ID                 int64
	StepOrder          int
	Text               string
	AnswerType         AnswerType
	HasAutoCheck       bool
	IsActive           bool
	IsDeleted          bool
	IsAsterisk         bool
	CorrectAnswerImage string
	Images             []StepImage
	Answers            []string
	HintText           string
	HintImage          string
	CreatedAt          time.Time
}

func (s *Step) HasHint() bool {
	return s.HintText != "" || s.HintImage != ""
}

type StepImage struct {
	ID       int64
	StepID   int64
	FileID   string
	Position int
}
