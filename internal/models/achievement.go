package models

import (
	"encoding/json"
	"time"
)

type AchievementCategory string

const (
	CategoryProgress   AchievementCategory = "progress"
	CategoryCompletion AchievementCategory = "completion"
	CategorySpecial    AchievementCategory = "special"
	CategoryHints      AchievementCategory = "hints"
	CategoryComposite  AchievementCategory = "composite"
	CategoryUnique     AchievementCategory = "unique"
)

type AchievementType string

const (
	TypeProgressBased AchievementType = "progress_based"
	TypeTimeBased     AchievementType = "time_based"
	TypeActionBased   AchievementType = "action_based"
	TypeComposite     AchievementType = "composite"
	TypeUnique        AchievementType = "unique"
	TypeManual        AchievementType = "manual"
)

type AchievementConditions struct {
	CorrectAnswers        *int     `json:"correct_answers,omitempty"`
	CompletionTimeMinutes *int     `json:"completion_time_minutes,omitempty"`
	NoErrors              *bool    `json:"no_errors,omitempty"`
	NoHints               *bool    `json:"no_hints,omitempty"`
	HintCount             *int     `json:"hint_count,omitempty"`
	SpecificAnswer        *string  `json:"specific_answer,omitempty"`
	PhotoSubmitted        *bool    `json:"photo_submitted,omitempty"`
	ConsecutiveCorrect    *int     `json:"consecutive_correct,omitempty"`
	Position              *int     `json:"position,omitempty"`
	RequiredAchievements  []string `json:"required_achievements,omitempty"`
	HintOnFirstTask       *bool    `json:"hint_on_first_task,omitempty"`
	AllHintsUsed          *bool    `json:"all_hints_used,omitempty"`
	PhotoOnTextTask       *bool    `json:"photo_on_text_task,omitempty"`
	InactiveHours         *int     `json:"inactive_hours,omitempty"`
	PostCompletion        *bool    `json:"post_completion,omitempty"`
	CompletionPosition    *int     `json:"completion_position,omitempty"`
	ProgressReset         *bool    `json:"progress_reset,omitempty"`
	TextOnImageTask       *bool    `json:"text_on_image_task,omitempty"`
	ManualAward           *bool    `json:"manual_award,omitempty"`
}

func (c *AchievementConditions) ToJSON() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ParseAchievementConditions(data string) (*AchievementConditions, error) {
	var conditions AchievementConditions
	if err := json.Unmarshal([]byte(data), &conditions); err != nil {
		return nil, err
	}
	return &conditions, nil
}

type Achievement struct {
	ID          int64
	Key         string
	Name        string
	Description string
	Category    AchievementCategory
	Type        AchievementType
	IsUnique    bool
	Conditions  AchievementConditions
	CreatedAt   time.Time
	IsActive    bool
}

type UserAchievement struct {
	ID            int64
	UserID        int64
	AchievementID int64
	EarnedAt      time.Time
	IsRetroactive bool
}
