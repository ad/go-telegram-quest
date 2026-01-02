package models

import "time"

type UserProgress struct {
	UserID      int64
	StepID      int64
	Status      ProgressStatus
	CompletedAt *time.Time
}
