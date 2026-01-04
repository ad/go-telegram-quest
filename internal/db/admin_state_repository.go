package db

import (
	"database/sql"
	"encoding/json"

	"github.com/ad/go-telegram-quest/internal/models"
)

type AdminStateRepository struct {
	queue *DBQueue
}

func NewAdminStateRepository(queue *DBQueue) *AdminStateRepository {
	return &AdminStateRepository{queue: queue}
}

func (r *AdminStateRepository) Save(state *models.AdminState) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		imagesJSON, _ := json.Marshal(state.NewStepImages)
		answersJSON, _ := json.Marshal(state.NewStepAnswers)

		_, err := db.Exec(`
			INSERT INTO admin_state (user_id, current_state, editing_step_id, new_step_text, new_step_type, new_step_images, new_step_answers, editing_setting, new_hint_text)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id) DO UPDATE SET
				current_state = excluded.current_state,
				editing_step_id = excluded.editing_step_id,
				new_step_text = excluded.new_step_text,
				new_step_type = excluded.new_step_type,
				new_step_images = excluded.new_step_images,
				new_step_answers = excluded.new_step_answers,
				editing_setting = excluded.editing_setting,
				new_hint_text = excluded.new_hint_text
		`, state.UserID, state.CurrentState, state.EditingStepID, state.NewStepText, state.NewStepType, string(imagesJSON), string(answersJSON), state.EditingSetting, state.NewHintText)
		return nil, err
	})
	return err
}

func (r *AdminStateRepository) Get(userID int64) (*models.AdminState, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT user_id, current_state, editing_step_id, new_step_text, new_step_type, new_step_images, new_step_answers, editing_setting, COALESCE(new_hint_text, '')
			FROM admin_state WHERE user_id = ?
		`, userID)

		var state models.AdminState
		var imagesJSON, answersJSON string
		err := row.Scan(&state.UserID, &state.CurrentState, &state.EditingStepID, &state.NewStepText, &state.NewStepType, &imagesJSON, &answersJSON, &state.EditingSetting, &state.NewHintText)
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(imagesJSON), &state.NewStepImages)
		json.Unmarshal([]byte(answersJSON), &state.NewStepAnswers)

		return &state, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.AdminState), nil
}

func (r *AdminStateRepository) Clear(userID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`DELETE FROM admin_state WHERE user_id = ?`, userID)
		return nil, err
	})
	return err
}
