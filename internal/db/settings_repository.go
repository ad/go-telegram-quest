package db

import (
	"database/sql"

	"github.com/ad/go-telegram-quest/internal/models"
)

type SettingsRepository struct {
	queue *DBQueue
}

func NewSettingsRepository(queue *DBQueue) *SettingsRepository {
	return &SettingsRepository{queue: queue}
}

func (r *SettingsRepository) Get(key string) (string, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var value string
		err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
		return value, err
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func (r *SettingsRepository) Set(key, value string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, key, value)
		return nil, err
	})
	return err
}

func (r *SettingsRepository) GetAll() (*models.Settings, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`SELECT key, value FROM settings`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		settings := &models.Settings{}
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				return nil, err
			}
			switch key {
			case "welcome_message":
				settings.WelcomeMessage = value
			case "final_message":
				settings.FinalMessage = value
			case "correct_answer_message":
				settings.CorrectAnswerMessage = value
			case "wrong_answer_message":
				settings.WrongAnswerMessage = value
			}
		}
		return settings, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.Settings), nil
}

func (r *SettingsRepository) SetWelcomeMessage(value string) error {
	return r.Set("welcome_message", value)
}

func (r *SettingsRepository) SetFinalMessage(value string) error {
	return r.Set("final_message", value)
}

func (r *SettingsRepository) SetCorrectAnswerMessage(value string) error {
	return r.Set("correct_answer_message", value)
}

func (r *SettingsRepository) SetWrongAnswerMessage(value string) error {
	return r.Set("wrong_answer_message", value)
}
