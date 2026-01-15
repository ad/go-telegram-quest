package models

type AnswerType string

const (
	AnswerTypeText  AnswerType = "text"
	AnswerTypeImage AnswerType = "image"
)

type ProgressStatus string

const (
	StatusPending       ProgressStatus = "pending"
	StatusWaitingReview ProgressStatus = "waiting_review"
	StatusApproved      ProgressStatus = "approved"
	StatusRejected      ProgressStatus = "rejected"
	StatusSkipped       ProgressStatus = "skipped"
)
